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

## Discovery

`internal/discovery`. A server advertises itself two ways, both best effort:
mDNS service `_shrinkyourshit._tcp` with TXT records `version`, `name` and
`id` (the built-in node id, so a server can hide itself from its own wizard
list), and a UDP broadcast responder on port 8766 that answers the probe
magic `SYS-DISCOVER` with a JSON `{id, name, url, version}` reply — the URL
is the answer, the rest feeds the wizard's server list. A worker (or a
fresh role-both install) probes broadcast addresses plus loopback and
browses mDNS; both transports can be disabled independently
(`SHRINK_DISCOVERY_MDNS`, `SHRINK_DISCOVERY_BROADCAST`) which is how CI
tests each path in isolation.

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

**Library:** `id, name, path, target (JSON), easy (JSON, empty unless the library was saved from the easy screen), flow (JSON), flow_source (easy | steps | advanced), watch bool, schedule (cron string or empty), result_policy, created_at, updated_at, last_scan_at` (last_scan_at: when a scan of the library last finished without error, empty when never scanned; task 19)

**File:** `id, library_id, path (absolute), size, mtime, fingerprint, probe (JSON), status, satisfied_reason, last_job_id, bytes_saved, created_at, updated_at`
- `status`: `new | satisfied | queued | processing | done | failed | missing`
- `fingerprint`: `size:mtime_unix:sha256(first 1MiB + last 1MiB)` as a string. Re-probe only when it changes.

**Job:** `id, file_id, library_id, node_id, status, priority, attempt, encoder_used, fallback_reasons (JSON), percent, speed, eta_seconds, bytes_before, bytes_after, bytes_so_far (the live output size, task 18), log (text, capped at 64KiB tail), error, access_mode, claimed_at, started_at, finished_at, heartbeat_at, created_at`
- `status`: `queued | claimed | running | done | failed | cancelled`
- `access_mode`: how the node actually reached the source — `shared` or `transfer`. Empty until the node reports it (progress and complete, task 09).

**Node:** `id, name, key_hash, key_once, os, arch, capabilities (JSON), access_mode, path_mappings (JSON), throughput_limit_mbps, desired_state, concurrency, state_error, status, enabled, last_heartbeat_at, created_at`. The list API folds each node's finished-job totals (`files_done`, positive-savings `bytes_saved`) into the response (task 18); they are not stored on the node.
- `status`: `pending | approved | revoked`
- `desired_state`: `stopped | running | draining` — the operator intent (task 11). New nodes are created `stopped`; a claim needs `status=approved`, `enabled=true` and `desired_state=running`.
- `concurrency`: 1 to 16, the node's cap on parallel jobs (heartbeat reports it).
- `state_error`: the reason a lifecycle transition failed to apply; `effective_state` is `error` while set, otherwise it mirrors `desired_state`. Cleared by the next successful transition.
- `access_mode`: `auto | shared | transfer`
- `throughput_limit_mbps`: cap on this node's transfer speed (MB/s, 0 = unlimited; task 09)
- `capabilities`: `{encoders: [{name, codecs: []}], cpu_cores, ffmpeg_version}`
- `key_once`: the raw key between admin approval and the node's first retrieval; wiped on retrieval, revoke and delete (only the hash remains afterwards).

**PairingToken:** `id, token_hash, expires_at, used_at`

**Session:** `token_hash (PK), created_at, expires_at`. Admin login sessions (task 06): random 32-byte token, only the SHA-256 hex hash is stored, 30-day expiry checked on lookup. Stored in the `sessions` table (migration 0002).

**Settings:** key/value table. Keys: `login_enabled, admin_username, admin_password_hash, worker_concurrency, default_result_policy, scratch_dir, setup_complete, preferred_languages, default_video_codec, paused_nodes` (task 11: the pause-all snapshot of each node's prior `desired_state`, deleted on resume-all), `update_check, update_channel, backup_before_update` (task 16: the self-update switch and feed — `stable` or `prerelease`, default `stable` — and the pre-update database backup; nothing applies an update by default).

**Webhook:** `id, name, url, secret, events (comma separated: job.done | job.failed | library.scanned), enabled, created_at, updated_at` (task 25, migration 0011). The secret is generated server side at create time (32 random bytes, hex) and returned exactly once; after that every response carries `secret_set` instead.

**Webhook delivery:** `id, webhook_id, event, status, error, created_at` (task 25). One row per delivered event per hook; the dispatcher prunes each hook to the newest 50 after every insert. `status` is the remote's HTTP status — 0 when the request never got an answer — and `error` holds the head of a non-2xx body or the transport failure text.

**WebhookEvent (the delivery body):** every field carries an explicit JSON tag; the nested values are the same DTOs the API publishes.
```json
{
  "event": "job.done | job.failed | library.scanned",
  "job":    { "…JobDTO": "" },
  "file":   { "…FileDTO": "" },
  "library":{ "…LibraryDTO": "" },
  "scan":   { "seen": 0, "probed": 0, "reused": 0, "skipped": 0, "queued": 0, "satisfied": 0, "missing": 0, "errors": 0 },
  "at":     "2026-10-31T12:00:00Z"
}
```
`job` and `file` are set on the job events, `scan` only on `library.scanned`, `library` whenever it is known. Deliveries sign the exact bytes sent with `X-SYS-Signature: sha256=<hex HMAC-SHA256 under the hook's secret>`; Discord and Slack hosts receive the wrapped one-line summary instead of the raw JSON.

**Integration:** `id, kind, name, base_url, api_key, enabled, refresh_on_done, created_at, updated_at` (task 23)
- `kind`: `jellyfin | emby | plex | sonarr | radarr`
- `api_key` is stored verbatim for the outbound call and never leaves the server through the API: responses carry `api_key_set: true|false` instead.
- `enabled` gates every call; `refresh_on_done` subscribes the integration to job completions (the manual test endpoint works either way).

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

- **shared:** node can read the library path (possibly at a different mount point; path mappings stored on the node). Verified read-only: the node asks `POST /worker/jobs/{id}/access-check`, which returns `{path (node-side), size, sample_offset, sample_sha256}` — a 1 MiB sample at a random offset of the job's source. The node hashes the same bytes through its mapping and compares. Match → shared for that library; the node caches the outcome per library for 10 minutes.
- **transfer:** server streams the source to the node's scratch dir over the worker API (range requests, resumable), node encodes, node uploads the result in 8 MiB chunks with `Content-Range`; the final chunk carries `X-Checksum-SHA256`, which the server verifies before applying the result policy itself (verify-and-replace reuses the shared result-policy code). The server keeps uploads under `<scratch>/uploads/job-<id>/`.
- **auto (default):** the worker runs the access check per job (cached per library for 10 minutes): shared if it passes, otherwise transfer. The node reports the mode it chose in the progress payload and the server stores it on the job.

A node's `throughput_limit_mbps` (0 = unlimited) caps both transfer directions; the worker paces its reads and writes.

In every mode the node copies to local scratch, encodes there, verifies, then writes back. Never encode in place on a network share.

## Job lifecycle

```
queued -> claimed (atomic, one worker) -> running -> done | failed | cancelled
```
- Heartbeat timeout (default 120s) returns a `claimed`/`running` job to `queued` and increments `attempt`. Max 3 attempts, then `failed`.
- Result policies: `replace_if_smaller` (default), `replace`, `keep_both` (writes `<name>.shrunk.<ext>` beside the original).
- Replacement is atomic: write to a temp name in the destination directory, fsync, rename over the original. Original is never deleted before the verified output exists in place.

## Worker lifecycle (v1.1)

Operator intent lives in `nodes.desired_state`, separate from the hard `enabled` switch that gates node auth.

```
stopped --start--> running --stop(drain)--> draining --idle--> stopped
                      \--stop(now)-------------------------------^
                      \--error--> error --start--> running
```

- `desired_state` is `stopped`, `running` or `draining`; `concurrency` (1 to 16) caps parallel jobs.
- The instruction reaches the worker on the heartbeat it already sends. No new port, no push channel, no long poll. The worker reports its own state back on the same heartbeat: a `stopped` report while the node is `draining` completes the drain.
- `stopped` makes no claim calls, only heartbeats, so a stopped node costs one small request per interval. A stop-now also abandons in-flight encodes (no report): the sweeper returns those jobs to `queued` after the heartbeat timeout, original files untouched.
- `draining` claims nothing new and finishes what it holds, then reports `stopped`.
- `enabled=false` remains the hard off switch enforced by node auth; `desired_state` is the day to day control. Both must be satisfied for a claim to succeed.
- A fresh install and a completed setup wizard leave the built in node `stopped`. Encoding never starts because a wizard was finished.
- Stopping never deletes a job. A claimed job whose worker stops returns to `queued` after the heartbeat timeout, unchanged on disk.

## Flow schema (v1, task 14)

The canonical rule object is a graph. `Target` remains as the compiled easy
mode shape and is always accompanied by the generated flow.

```
Graph{ schema, nodes: [Step], edges: [Edge] }
Step{ id, type, params }
Edge{ from, to, when }
```

- `schema` is 1. `Edge.When` is reserved for the task-15 canvas; v1
  conditions are `if_*` guard steps, so compile ignores a non-empty `when`.
- Step types for v1: `probe`, `if_codec_not`, `if_bitrate_above`,
  `transcode_video`, `remux_container`, `convert_audio`, `subtitles`,
  `rename`, `skip_if_smaller`, `health_check`, `move` (the last two are
  task 24). A graph is a single linear chain: at most one
  edge in and one out per node, exactly one entry step of type `probe`, no
  loops, no unreachable steps, and a guard always has a following step.
- Parameters per type (validated with unknown-field rejection):
  - `if_codec_not {codec: hevc|av1|h264}` — runs the next step only when
    the video is not that codec.
  - `if_bitrate_above {mbps: >0}` — runs the next step only above this
    video bitrate.
  - `transcode_video {codec: hevc|av1|h264, quality: 1..5}` — `copy` is
    expressed by omitting the step, so it is a validation error here.
  - `remux_container {container: mkv|mp4}` — exactly one per graph; it
    decides the output file type, which also rejects an empty graph.
  - `convert_audio {mode, languages, compact_codec}` — the audio policy
    (same modes as `Target.audio`).
  - `subtitles {mode, languages, extract_removed}` — the subtitle policy.
  - `rename {template}` — names the produced output for keep-both results;
    `{name}` is the original base name, `{ext}` the extension, no path
    separators, `{name}` required.
  - `health_check {mode: sample|full, minutes: 1..120,
    duration_tolerance: 0..0.5}` — zero fields take the defaults (`sample`,
    2 minutes, 0.02). It never changes the Target; it may sit anywhere in
    the chain because it always runs last.
  - `move {destination: absolute path, keep_tree: bool}` — after a
    successful result the file goes under `destination` (never the library
    root or inside it, checked when the flow is saved); `keep_tree` rebuilds
    the file's path relative to the library root under it. Exactly one per
    graph, trailing slashes trimmed.
  - `skip_if_smaller` and `probe` take no parameters.
- At most one step of each shaping type (transcode, container, audio,
  subtitles, rename, skip, health check, move); a graph always describes
  one policy.
- `flow.Validate(Graph) []FieldError` checks all of the above; field errors
  name the path so the UI can attach them to a row.
- `flow.Compile(Graph) (Plan, error)` flattens the chain into ordered plan
  steps, each carrying the conditions gathered from the guards before it.
- `flow.ResolvePlan(plan, probe)` evaluates the conditions per file and
  yields the effective `Target` (the shape the satisfied rule and the
  command builder already speak) plus the flow-only outcomes:
  - a guarded transcode that passes becomes the effective video with
    `reencode_if_same_codec=false` when the only guards are
    `if_codec_not` (the guard already excludes the same codec — this is
    what makes `Compile(TargetToFlow(t))` byte-identical to `t`, the
    parity gate); guarded by anything else, or unguarded, it runs with
    `reencode_if_same_codec=true` (a bitrate-gated re-encode must really
    run even on same-codec files);
  - a skipped transcode leaves the video at copy (never re-encode);
  - a `skip_if_smaller` step overrides the library's result policy with
    `replace_if_smaller` for the job (flow wins);
  - a `rename` step names the keep-both output; with replace policies it
    cannot rename over the original, so it is logged and ignored;
  - a `health_check` step (task 24) is carried as `Effective.HealthCheck`
    and never touches the Target: after the output is produced the worker
    fails the job when a stream type the encode pipeline must keep (video,
    audio) is missing, when the output duration sits under
    `source * (1 - duration_tolerance)`, or when a decode pass
    (`ffmpeg -v error -xerror -nostdin -i <out> -f null -`, narrowed to the
    sample window) exits non-zero. A failed check ends the job before the
    result policy runs; the original is untouched;
  - a `move` step is carried as `Effective.MoveTo`/`MoveKeepTree` and by
    the job spec (`move_to`, `move_keep_tree`, `move_rel_dir`); the result
    policy places the output under the destination (never clobbering,
    rename with a verified copy fallback) and removes the original only
    after the destination is verified at its expected size — replace
    policies remove it, keep-both keeps it.
- Storage: libraries carry `flow_json` plus `flow_source` of `easy`,
  `steps` or `advanced`. Easy mode and target-only saves keep writing
  `target` and regenerate the equivalent flow
  (`flow.TargetToFlow`); a target-only save with an unchanged Target
  keeps a hand-built flow, so name/watch edits never destroy manual
  steps. When both `target` and `flow` exist, `flow` wins and a mismatch
  is logged at warn; `PUT /libraries/{id}/flow` re-syncs the stored Target
  from the compiled flow so the compat column never lies. Legacy rows are
  backfilled at startup and lazily compiled on read — no manual migration.
- The step palette ships from `GET /api/v1/flows/step-types` so the UI
  holds no second copy of the step list. `POST /api/v1/flows/preview`
  renders the plain-language lines (one per compiled step) that the easy
  screen's "what this does" panel shows; the UI never duplicates the
  compiler.

## UI routes (v1.1)

| Route | Page | Notes |
|-------|------|-------|
| `/` | Dash | read only: stats, live jobs, node health, activity, warnings |
| `/libraries` | Library | list, add, detail, files |
| `/libraries/:id/edit` | Library | target and easy mode |
| `/libraries/:id/rules` | Library | tier selector, step stack (task 14) |
| `/libraries/:id/flow` | Library | canvas (task 15) |
| `/nodes` | Nodes | lifecycle controls, pairing, concurrency |
| `/encode` | Encode settings | codec, quality, container, audio, subtitles, result policy |
| `/app` | App settings | server, auth, paths, logs, about |

Shell rules: under `md` the nav is a drawer behind a top bar, at `md` and up it is the left rail. The rail shows labels at `lg` and up and icons only below that; the toggle writes an explicit override to `localStorage` under `sys.rail` (`expanded`/`collapsed`; anything else means auto). Stat tiles use `clamp()` type and scale with the viewport (`web/src/StatCard.tsx`). The page background is painted on `html, body` so no region outside the shell renders white. Every page uses one container, `mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8`. `settings` redirects to `app`.

Layout rules: every interactive control is at least 44px tall (`web/src/uiControls.ts`).
Tables above `sm` render as cards below it via `web/src/DataList.tsx`. Stat tiles use
`clamp()` type and scale with the viewport (`web/src/StatCard.tsx`). The rail auto
collapses below `lg` and remembers an explicit override in `localStorage` under `sys.rail`.
The flow canvas (`/libraries/:id/flow`) is view only below `md`: no palette, inspector
or edit controls, just `flow-mobile-notice` plus the canvas with pan/pinch zoom; editing
state is `requested && wide`, so a resize mid-edit degrades to viewing with the unsaved
guard armed, and the step-types `params` is normalized to `[]` client-side (`web/src/api.ts`).
`web/e2e/mobile.spec.ts` fails CI on any horizontal overflow or sub 44px control at
320, 390, 414 and 768 px, including the library detail and flow routes.

## Update path

Three install shapes, three update paths, and the app always says which one applies.

- **Container.** The image is published to `ghcr.io/peiceoftoast/shrinkyourshit` on every `v*` tag, tagged with the version and `latest`. Updating is `docker compose pull` then `docker compose up -d`. The app never tries to update itself from inside a container; it detects `/.dockerenv` or `SHRINK_IN_CONTAINER=1` and shows the commands instead.
- **Binary.** `shrink update` asks GitHub for the latest release, picks the asset for `GOOS`/`GOARCH`, verifies the sha256 from the release `checksums.txt`, writes the new binary beside the old one, keeps `<binary>.prev`, and renames over it. Restart is a systemd `Restart=always` bounce or an exec. `shrink update --rollback` restores the previous binary.
- **Source.** `git pull` plus the normal build, documented and never automated.

Rules that apply to all three:

- No update applies itself by default. Checking is on, applying is always a human click or an explicit command.
- The SQLite database is backed up to `<data-dir>/backups/pre-update-<version>.db` before any update, last three kept.
- A download that fails checksum verification aborts with the running binary untouched.
- The version string is injected at build time (`main.version`, `main.commit`, `main.date`); a build without it reports `v0.0.0` and cannot self update safely, so releases always inject it.
- Schema migrations run on start and only move forward. A failed migration leaves the backup as the recovery point.

## Security

- Node pairing needs an admin approval click or a one-time token (expires in 24h, single use).
- Admin password optional (off allowed), set in first-run setup. Argon2id.
- Worker API authenticated with per-node keys; keys stored hashed on the server. Keys are 32 random bytes, base64url; only the SHA-256 is stored (high-entropy key, so the fast hash is enough — task 08).
- Unpaired or revoked nodes cannot claim jobs or read files.
- TLS: optional manual cert and key paths; reverse proxy is the expected setup.

## Plugins (post-MVP)

- Bundled set shipped with the binary.
- User-added git repos, cloned and cached locally. Works offline after first fetch.
- Declarative YAML/JSON with FFmpeg templates. Script escape hatch is a later, sandboxed runtime.

## Integrations

REST API (OpenAPI document shipped with the binary, task 25) and outbound webhooks. Task 23 adds the
configured integrations (`jellyfin`, `emby`, `plex`, `sonarr`, `radarr`): after a job ends
`done` with real savings, the refresher calls each enabled integration with `refresh_on_done`
through one pinned request per kind, coalesced to one in-flight call per integration id with
a 30 second floor between calls. Task 25 adds the webhooks: a server-generated secret, an
event dispatcher (`job.done`, `job.failed`, `library.scanned`) fed by the same outbound hook
the refresher filters on, retries with a recorded delivery history, and the OpenAPI document
(`docs/openapi.yaml`) whose served copy and the route table are tested to agree. Sonarr/
Radarr/Plex/Jellyfin/Emby refresh sits on this API; per-library filters stay post-MVP.

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
