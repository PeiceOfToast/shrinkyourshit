# Task 02: Probe, store, Target, Evaluate (skip-if-satisfied)

## Goal
Probe media files with ffprobe, store results in SQLite, and decide whether a file already meets a Target so it is never reprocessed.

## Read first
ARCHITECTURE.md (data model, Target schema, satisfied rule), STACK.md (database, migrations, tests), DECISIONS.md (B2, B5).

## Do
- `internal/probe`: run `ffprobe -v quiet -print_format json -show_format -show_streams`, parse into typed structs: `MediaInfo{Container, DurationSeconds, SizeBytes, Video []VideoStream, Audio []AudioStream, Subtitle []SubtitleStream}`. Video: codec, width, height, bitrate, fps, pixel format, HDR flag (from color transfer `smpte2084` or `arib-std-b67`). Audio: codec, channels, language, bitrate, title, default flag. Subtitle: codec, language, forced, default, text-based bool.
- Fingerprint function per ARCHITECTURE.md: `size:mtime:sha256(first 1MiB + last 1MiB)`.
- `internal/store`: `Store` interface covering libraries, files, jobs, nodes, pairing tokens, settings (full CRUD needed by later tasks; implement what you can test now, stub nothing). SQLite implementation with embedded SQL migrations and a `schema_migrations` table. Enums stored as text, validated in Go.
- `internal/flow`: `Target` type matching the JSON schema exactly, with validation. `Evaluate(info probe.MediaInfo, t Target) Decision{Needs bool, Reasons []string}` implementing the satisfied rule, including audio and subtitle track selection logic as helper functions that task 04 will reuse (`SelectAudio(info, t) []int`, `SelectSubtitles(info, t) []int` returning stream indexes to keep).
- Keep `store` free of media-specific types: `File.Probe` is stored as opaque JSON.

## Out of scope
Scanning folders, watching, queueing, encoding, HTTP.

## Acceptance
- [ ] Integration tests generate fixtures with ffmpeg (h264+aac+2 audio langs+1 srt in mkv; hevc+aac in mkv; h264 in mp4) and check: h264 vs hevc Target needs work with reason "video is h264, target is hevc"; hevc vs hevc Target is satisfied; hevc with `subtitles.mode=remove` and a subtitle track needs work; hevc with `audio.mode=keep_best` and 2 tracks needs work; same file with 1 track is satisfied; mp4 vs `container=mkv` needs work.
- [ ] Unit tests (no ffmpeg) cover `Evaluate` and the selection helpers with hand-built `MediaInfo` values, including `keep_preferred` with no matching language keeping all.
- [ ] Fingerprint is stable across runs and changes when the file content changes.
- [ ] Store tests run against a temp SQLite file: migrations are idempotent (apply twice), CRUD round-trips for every table, enum validation rejects bad values.
- [ ] `Target` JSON round-trips and rejects unknown enum values.
