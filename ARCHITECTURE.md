# Architecture (v1)

## Glossary

| Term | Meaning |
|------|---------|
| Server | The process that owns the database, queue, web UI and node registry. One per install. |
| Worker | A process that claims jobs and runs FFmpeg. The server has a built-in one; remote nodes run one each. |
| Node | A registered worker machine, with its capabilities and access mode. |
| Library | A folder (or list of folders) plus a Target and watch/schedule settings. |
| File | One media file inside a library, with its probe result and status. |
| Target | The desired end state for files in a library (codec, quality, audio and subtitle rules, container). |
| Flow | Post-MVP: a graph of conditions and actions. Easy mode and step stack compile to it. In the MVP, a library holds a Target directly (decision B3). |
| Job | One unit of work: apply a Target to one File on one Node. |
| Scratch | Fast local temp space on a worker where encodes actually happen. |

## One binary, three roles

`shrinkyourshit` is a single Go binary. Role is a runtime setting (`SHRINK_ROLE`), not a different build.

- **server:** API, web UI, database, queue, library watchers, node registry.
- **worker:** pulls jobs from a server and runs FFmpeg.
- **both:** default. The built-in worker talks to the server through the same worker API a remote worker uses, so remote nodes need no special code path.

First launch with no data dir: try to discover a server on the LAN (mDNS, UDP broadcast fallback). Found one: offer to join as a worker. None found: run first-run server setup in the browser.

## Components

```
Web UI (React, embedded)
        |
   REST API + SSE events            (internal/api)
        |
Server                              (internal/server)
  library manager, scanner, watcher, queue/scheduler, node registry, auth, webhooks
        |
Store interface -> SQLite           (internal/store)
        |
Workers, local or remote            (internal/worker)
  hardware detect (internal/encoders), job runner, FFmpeg subprocess (internal/probe for ffprobe)
```

## Data model

Status enums are fixed strings; do not add values without updating this file.

**Library:** `id, name, path, target (JSON), watch bool, schedule (cron string or empty), result_policy, created_at, updated_at`

**File:** `id, library_id, path (absolute), size, mtime, fingerprint, probe (JSON), status, satisfied_reason, last_job_id, bytes_saved, created_at, updated_at`
- `status`: `new | satisfied | queued | processing | done | failed | missing`
- `fingerprint`: `size:mtime_unix:sha256(first 1MiB + last 1MiB)` as a string. Re-probe only when it changes.

**Job:** `id, file_id, library_id, node_id, status, priority, attempt, encoder_used, fallback_reasons (JSON), percent, speed, eta_seconds, bytes_before, bytes_after, log (text, capped at 64KiB tail), error, claimed_at, started_at, finished_at, heartbeat_at, created_at`
- `status`: `queued | claimed | running | done | failed | cancelled`

**Node:** `id, name, key_hash, os, arch, capabilities (JSON), access_mode, path_mappings (JSON), status, enabled, last_heartbeat_at, created_at`
- `status`: `pending | approved | revoked`
- `access_mode`: `auto | shared | transfer`
- `capabilities`: `{encoders: [{name, codecs: []}], cpu_cores, ffmpeg_version}`

**PairingToken:** `id, token_hash, expires_at, used_at`

**Session:** `token_hash (PK), created_at, expires_at`. Admin login sessions (task 06): random 32-byte token, only the SHA-256 hex hash is stored, 30-day expiry checked on lookup. Stored in the `sessions` table (migration 0002).

**Settings:** key/value table. Keys: `login_enabled, admin_username, admin_password_hash, worker_concurrency, default_result_policy, scratch_dir, setup_complete, preferred_languages, default_video_codec`.

**Webhook:** `id, url, events (JSON list), enabled` (task post-MVP; schema can wait).

## Target schema

Stored as JSON on the library. All tasks that read or write a Target use this exact shape.

```json
{
  "video": {
    "codec": "hevc",
    "quality": 3,
    "reencode_if_same_codec": false
  },
  "audio": {
    "mode": "keep_all",
    "preferred_languages": ["eng"],
    "compact_codec": "aac"
  },
  "subtitles": {
    "mode": "keep_all",
    "preferred_languages": ["eng"],
    "extract_removed": false
  },
  "container": "mkv"
}
```

- `video.codec`: `hevc | av1 | h264 | copy`. `copy` means never re-encode video (remux and track changes only).
- `video.quality`: integer 1 to 5. 1 = most space saved, 5 = least. Maps to encoder settings in the table below.
- `audio.mode`: `keep_all | keep_preferred | keep_best | compact`
  - `keep_all`: no change.
  - `keep_preferred`: keep tracks whose language is in `preferred_languages`; if none match, keep all.
  - `keep_best`: keep one track: first match in `preferred_languages` by highest channel count, else the first track.
  - `compact`: same selection as `keep_best`, then transcode it to `compact_codec` (`aac` 160k stereo or `opus` 128k stereo) unless it already is that codec.
- `subtitles.mode`: `keep_all | keep_preferred | remove`. `extract_removed` writes removed text subtitles as `.srt` beside the file (engine supports it in task 04; UI exposes it post-MVP).
- `container`: `mkv | mp4`. MVP UI always uses `mkv`.

## Easy-mode mapping

The easy screen has: folder, a 5-step quality slider, a "remove subtitles" checkbox, an audio slider with 4 positions, Start.

| UI control | Target field |
|------------|-------------|
| Quality slider step N | `video.quality = N`, `video.codec = "hevc"` (from settings default), `container = "mkv"` |
| Remove subtitles checked | `subtitles.mode = "remove"`; unchecked: `keep_all` |
| Audio slider 1 "Keep everything" | `audio.mode = "keep_all"` |
| Audio slider 2 "Keep my languages" | `audio.mode = "keep_preferred"` |
| Audio slider 3 "Keep the best track" | `audio.mode = "keep_best"` |
| Audio slider 4 "Keep the best track, make it small" | `audio.mode = "compact"` |

Preferred languages default to `["eng"]`, editable in settings.

### Quality step table (starting points, tune after home-lab testing)

| Step | x265 CRF (preset medium) | NVENC HEVC CQ (preset p5) | QSV HEVC global_quality | SVT-AV1 CRF (preset 6) | x264 CRF |
|------|------|------|------|------|------|
| 1 | 28 | 32 | 30 | 38 | 26 |
| 2 | 26 | 30 | 28 | 35 | 24 |
| 3 | 24 | 28 | 26 | 32 | 22 |
| 4 | 22 | 26 | 24 | 30 | 20 |
| 5 | 20 | 24 | 22 | 27 | 18 |

Lives in `internal/encoders/quality.go` as one table so it is tuned in one place.

## Satisfied rule (never reprocess)

A file is `satisfied` when all of the following hold:

1. Video: `codec == "copy"`, or the file's video codec already equals `video.codec` and `reencode_if_same_codec` is false.
2. Audio: the file's audio tracks already match what `audio.mode` would produce (same set of tracks, and for `compact`, already in `compact_codec`).
3. Subtitles: the file's subtitle tracks already match what `subtitles.mode` would produce.
4. Container matches `container`.

`Evaluate(probe, target)` returns `{needs: bool, reasons: []string}`; reasons are shown in the UI (for example "video is h264, target is hevc").

After a successful job, the output's fingerprint is stored, so the next scan or watch event sees an unchanged fingerprint and does nothing. Changing a library's Target triggers re-evaluation of all its files, which queues only those that now need work.

## Hardware and encoders

Each worker detects on start: NVENC (NVIDIA), QSV and VAAPI (Intel), CPU (libx264, libx265, libsvtav1). The `Encoder` interface per backend builds FFmpeg arguments and runs a one-second self-test. Order: GPU encoders that support the target codec, then CPU. If a GPU encode fails, the job retries on the next encoder and the failure reason is recorded. Adding AMF or VideoToolbox later means adding one implementation.

## File access modes (per node)

- **shared:** node can read the library path (possibly at a different mount point; path mappings stored on the node). Verified read-only: server returns `{path, size, sample_offset, sample_sha256}` for an existing file and the node must produce the same hash.
- **transfer:** server streams the source to the node's scratch dir over the worker API (range requests, resumable), node encodes, node uploads the result with a checksum.
- **auto (default):** shared if the check passes for that library, otherwise transfer.

In every mode the node copies to local scratch, encodes there, verifies, then writes back. Never encode in place on a network share.

## Job lifecycle

```
queued -> claimed (atomic, one worker) -> running -> done | failed | cancelled
```
- Heartbeat timeout (default 120s) returns a `claimed`/`running` job to `queued` and increments `attempt`. Max 3 attempts, then `failed`.
- Result policies: `replace_if_smaller` (default), `replace`, `keep_both` (writes `<name>.shrunk.<ext>` beside the original).
- Replacement is atomic: write to a temp name in the destination directory, fsync, rename over the original. Original is never deleted before the verified output exists in place.

## Security

- Node pairing needs an admin approval click or a one-time token (expires in 24h, single use).
- Admin password optional (off allowed), set in first-run setup. Argon2id.
- Worker API authenticated with per-node keys; keys stored hashed on the server.
- Unpaired or revoked nodes cannot claim jobs or read files.
- TLS: optional manual cert and key paths; reverse proxy is the expected setup.

## Plugins (post-MVP)

- Bundled set shipped with the binary.
- User-added git repos, cloned and cached locally. Works offline after first fetch.
- Declarative YAML/JSON with FFmpeg templates. Script escape hatch is a later, sandboxed runtime.

## Integrations

REST API (OpenAPI document generated post-MVP) and outbound webhooks. Sonarr/Radarr and Plex/Jellyfin/Emby come later and sit on top of the API.

## Repo layout

```
/cmd/shrinkyourshit    entrypoint, subcommands
/internal/api          HTTP handlers, SSE, auth middleware
/internal/server       queue, scheduler, library manager, scanner, watcher, node registry
/internal/worker       claim loop, job runner, scratch management, transfer client
/internal/encoders     Encoder interface, nvenc, qsv, cpu, quality table, fallback
/internal/flow         Target type, Evaluate, easy-mode compiler (flow graph post-MVP)
/internal/probe        ffprobe wrapper and MediaInfo types
/internal/store        Store interface, sqlite implementation, migrations
/internal/discovery    mDNS and broadcast
/internal/config       env, flags, config file
/plugins               bundled plugin definitions (post-MVP)
/web                   React app, built into web/dist and embedded
/docker                Dockerfile, compose.example.yml
/scripts               installer.sh, installer.ps1
/docs                  user docs
/tasks                 agent task specs and STATUS.md
```
