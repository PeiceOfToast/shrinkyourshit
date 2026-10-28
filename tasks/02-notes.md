# Task 02 notes — probe, store, Target, Evaluate

## Plan (files, in order)

1. **`internal/probe`**
   - `media.go` — `MediaInfo`, `VideoStream`, `AudioStream`, `SubtitleStream`; container normalization (`matroska,webm` + filename ext → `mkv`/`webm`; `mov,mp4,...` → `mp4`); HDR from `color_transfer` (`smpte2084`, `arib-std-b67`).
   - `ffprobe.go` — `Parse` (raw ffprobe JSON → MediaInfo, unit-testable without ffmpeg) and `Probe` (runs `ffprobe -v quiet -print_format json -show_format -show_streams <path>` via `os/exec`).
   - `fingerprint.go` — `Fingerprint(path)` = `size:mtime_unix:sha256(first 1MiB + last 1MiB)`; files ≤ 1MiB hash whole file (sample would be duplicated otherwise).
   - tests: unit (canned ffprobe JSON, fingerprint on temp files) + `integration` (lavfi fixtures, `Probe`, HDR flag).
2. **`internal/flow`**
   - `target.go` — `Target`/`VideoTarget`/`AudioTarget`/`SubtitleTarget` exactly matching ARCHITECTURE.md schema; `Validate`; `Default()` (hevc/3/keep_all/keep_all/mkv per B1 and the schema example).
   - `evaluate.go` — `Evaluate(probe.MediaInfo, Target) Decision{Needs, Reasons}` implementing the satisfied rule; `SelectAudio`, `SelectSubtitles` helpers (indexes to keep) for task 04 reuse.
   - tests: hand-built MediaInfo for every rule branch (unit); no ffmpeg needed.
3. **`internal/store`**
   - `store.go` — media-free types (`Library`, `File`, `Job`, `Node`, `PairingToken`), `Store` interface (full CRUD for all six tables), enum validation, `ErrNotFound`, `NewID` (UUID v4 via crypto/rand, no new dependency).
   - `migrations/0001_init.sql` + `migrations.go` (go:embed, `schema_migrations` table, applied in order, idempotent).
   - `sqlite.go` — modernc.org/sqlite implementation; single open connection to avoid lock contention (v1 home-lab scale).
   - tests: temp SQLite file; migrate twice; CRUD round-trips for every table; bad enum values rejected.
4. Docs/status: tick acceptance, update STATUS.md, summary.

## Decisions / justifications

- **No new dependencies**: UUID v4 generated with `crypto/rand` (STACK lists no uuid lib); testify already present.
- **Attached pictures excluded from `Video`**: MKV cover art shows up as a video stream (`mjpeg`/`png`, `disposition.attached_pic=1`); keeping it would make Evaluate demand re-encoding of a static image. Documented in the stream-mapping code.
- **webm vs mkv**: ffprobe reports `format_name=matroska,webm` for both, so the extension from `format.filename` disambiguates; a `.webm` file must NOT satisfy a `mkv` target (it needs remuxing), so normalization matters. `.mov` normalizes to `mp4` (same QuickTime family, ffprobe cannot tell them apart either) — acceptable for v1, noted for post-MVP.
- **`File.SatisfiedReason`** holds the joined Evaluate reasons (why the file is in its current evaluation state), joined with `; ` — task 03 shows it in the UI.
- **`Job.Speed`** is the ffmpeg x-factor as float64; `Percent` float64 0..100; `ETASeconds` int64. `FallbackReasons` stored as JSON array text.
- **Settings** are opaque key/value with constants for the known keys (ARCHITECTURE.md); no enum validation there because the values are free-form (`login_enabled` etc. gain meaning in task 06).
- **Store keeps `Target` and `Probe` as opaque JSON strings** (rule 9: no media types in store); flow/probe do the parsing.
- **Language matching** is case-insensitive and `und`/empty never matches a preferred language (falls back to "keep all" per the rule).

## Environment notes

- ffmpeg 7.0.2 static build present (johnvansickle) — includes libx264/libx265; integration fixtures generate with lavfi `testsrc` + `sine`.
- go 1.27.3 local, go.mod pinned 1.23; golangci-lint v2.15.0.

## Summary (filled at the end)

- **Built:** `internal/probe` (MediaInfo + stream types, ffprobe JSON parsing, `Probe` subprocess wrapper, 1 MiB-head/tail fingerprint); `internal/flow` (`Target` matching the ARCHITECTURE.md schema exactly, `Validate`, `ParseTarget`, `Evaluate` satisfied rule with UI-ready reasons, `SelectAudio`/`SelectSubtitles` helpers for task 04); `internal/store` (media-free types, full-CRUD `Store` interface, SQLite impl via modernc.org/sqlite, embedded `0001_init.sql` migration + `schema_migrations`, enum validation on write, UUID v4 `NewID`, `ErrNotFound`); `internal/testutil` (integration-only lavfi fixture generator, skips clearly without ffmpeg).
- **Skipped (out of scope):** scanning, watching, queueing, Claim, encoding, HTTP endpoints, easy-mode compiler.
- **Tests:** unit tests pass without ffmpeg (verified with ffmpeg hidden from PATH); integration tests (tag `integration`) generate h264+2-audio+srt mkv, hevc+aac+srt mkv, h264+aac mp4 and an HDR10 mkv with lavfi and assert every acceptance case; store tests cover idempotent migrations, CRUD round-trips for all six tables, bad enum rejection.
- **Decisions for the owner to know:** modernc.org/sqlite pinned to **v1.34.5** because v1.61 requires Go 1.26 while CI/STACK pin Go 1.23 (upgrade when the toolchain moves); cover-art (attached_pic) video streams are excluded from `MediaInfo.Video`; `.webm` normalizes to `webm` (must not satisfy a mkv target), `.mov` to `mp4`; timestamps stored RFC3339Nano.
- **Next task should know:** `Store` is the full CRUD surface but queue operations (`Claim`, heartbeat sweeper, priority/oldest ordering) belong to task 03 and can be added to the interface + impl; `internal/testutil.GenerateFixtures` (integration tag) provides shared ffmpeg fixtures; `flow.SelectAudio/SelectSubtitles` return indexes into `MediaInfo.Audio/Subtitle` (in stream order) for building ffmpeg mappings; `probe.Parse` is exported so parsers can be tested without ffprobe.
