# Task 34: Files that are not media

## Goal
`DECISIONS.md` item 4 says the core must stay generic so other file types can be added later, and `AGENTS.md` rule 9 forbids video-only assumptions in core types. Today a library is assumed to hold media: the scanner probes everything, the planner always builds a media target, and a folder of photos produces a queue full of failures. This task makes a library declare what it holds, teaches the scanner and planner about files that are not video or audio, and lets the steps that make sense (rename, move, plugins, scripts, size rules) work on them while the media-only steps skip cleanly instead of failing.

This is a plumbing and honesty task, not a feature expansion. Non-media libraries must work end to end with no encoder involved.

## Read first
`internal/probe/ffprobe.go`, `internal/probe/media.go`, `internal/probe/fingerprint.go`, `internal/server/scanner.go`, `internal/server/library.go`, `internal/server/libraryflow.go`, `internal/flow/evaluate.go`, `internal/flow/target.go`, `internal/flow/graph.go` (`Validate`, `isShaping`), `internal/flow/plan.go` (`Compile`, `ResolvePlan`, `Target`), `internal/worker/runner.go` (probe, then steps), `internal/store/migrations/` (the latest number), `internal/api/handlers.go`, `web/src/LibrariesPage.tsx`, `web/src/LibraryDetailPage.tsx`, `ARCHITECTURE.md`, `API.md`.

## Do

### 1. A file kind, decided by extension
New `internal/probe/kind.go`:

```go
// Kind is what a file is, decided by extension, not by probing. The store
// and the queue carry a Kind; only internal/probe, internal/encoders and
// internal/flow know what a Kind means for encoding.
type Kind string

const (
    KindVideo    Kind = "video"
    KindAudio    Kind = "audio"
    KindSubtitle Kind = "subtitle"
    KindImage    Kind = "image"
    KindOther    Kind = "other"
)
```

`KindForPath(p string) Kind` with an exported extension table and `KindValues()`. Defaults: the video, audio and subtitle extensions the app already handles, plus `jpg jpeg png gif webp avif heic heif tiff bmp` as image and everything else as `other`. Unknown extensions are `other`, never an error.

### 2. A library declares its kinds
- Migration `NNNN_library_kinds.sql`: `libraries` gains `file_kinds TEXT NOT NULL DEFAULT '["video"]'` holding a JSON array.
- `internal/server/library.go` and the store layer carry `FileKinds []probe.Kind`. Validation: at least one value, every value a valid `Kind`, no duplicates.
- `internal/api/handlers.go`: library create and update accept `file_kinds`, and `GET` returns it. Follow the existing error shape for an invalid kind, naming the field and the bad value.
- Scanner: a file whose kind is not in the library's list is skipped **before** probing, counted separately and logged at debug, not queued and not counted as a failure. A file whose kind is in the list but whose probe fails for a media kind stays a failure exactly as today; a probe failure for a non-media kind is not a failure, because nothing should probe it.

### 3. No media, no media target
- `internal/server/libraryflow.go` and `internal/flow/evaluate.go`: for a non-media kind, skip probing entirely, use the fingerprint path (`internal/probe/fingerprint.go`) for identity and size, and resolve an `Effective` plan with **no** `Target` fields set: no codec, no quality, no audio or subtitle policy.
- `internal/flow/plan.go`: `ResolvePlan` must not error on a file with no streams when the kind is not media. It must set a reason on the plan the same way a satisfied file is reported today, for example `not media: nothing to encode`, so the UI shows a skip with a reason rather than a failure.
- `target.go` and the guards: `if_codec_not` and `if_bitrate_above` evaluate false on a non-media file rather than erroring, because there is no codec and no bitrate.

### 4. Which steps run
In `internal/flow/graph.go` `Validate`, and enforced at dispatch in `internal/worker/runner.go`, split the steps by kind:
- Steps that work on any kind: `rename`, `move`, `skip_if_smaller`, `plugin` (task 32), and a wasm script step (task 33).
- Steps that need media: `probe`, `transcode_video`, `remux_container`, `convert_audio`, `subtitles`, `health_check`, `if_codec_not`, `if_bitrate_above`. On a non-media file each of these is **skipped with a reason in the job log**, never a failure, and never silently: the job log line names the step and the file kind.
- A graph containing only media steps attached to a library of `image` files is legal to save but must produce skips with reasons, so the user sees why nothing happens. Add the plain-language hint in the UI: a library of photos with a video flow says so on the library page.

### 5. UI
- `web/src/LibrariesPage.tsx` / `LibraryDetailPage.tsx`: a file-kind picker in library settings, plain language first (`What should this library look for?` with `Video`, `Audio`, `Photos`, `Subtitles`, `Anything else`, multi-select), 44 px controls, `DataList` cards below `sm`.
- The library list shows the kinds, and a library detail page shows the skipped count from the last scan with the reason, so a photos library that is doing nothing looks deliberate.
- Settings copy avoids codec and FFmpeg words, per `AGENTS.md` rule 14.

### 6. Docs
- `ARCHITECTURE.md`: the `Kind` enum in the data shapes section, the library `file_kinds` field, and a short paragraph on the rule that media assumptions live in `probe`, `encoders` and `flow` only.
- `API.md`: `file_kinds` on the library endpoints with its default and validation.
- `README.md`: one line that a library can hold non-video files.
- `DECISIONS.md` is not touched. Item 4 already permits this; the notes file should quote it.

## Out of scope
Encoding images or converting them to anything, archives and ROM sets, per-file rule trees for non-media kinds, an extension editor in the UI (the table is code for v1), subtitle-only libraries changing how subtitle handling works inside media files, thumbnails, deduplication across kinds, `DECISIONS.md` changes.

## Acceptance
- [ ] A library with `file_kinds: ["image"]` scans a folder of images, queues nothing it cannot handle, and reports a distinct skipped count with a reason; a folder of videos added to it is skipped by the same rule. Proven by `scanner_test.go` with ffmpeg-free fixtures.
- [ ] An invalid kind, an empty list and a duplicate are each rejected by the API with an error naming the field and the value. Proven by an API test.
- [ ] A non-media job completes: `rename` plus `move` on an image produces the renamed, moved file and a `done` job with no encoder invoked. Proven end to end in a test that asserts no ffmpeg process is started.
- [ ] A media-only flow attached to an image library produces skips with a reason naming the step and the kind, and the job is not marked failed. Proven by a worker test.
- [ ] A video library still behaves exactly as today: the existing scanner, queue and worker tests pass unchanged, and a default library still reports `["video"]`.
- [ ] The library pages show and edit the kinds, and show the skipped count with its reason. Proven by `web` tests.
- [ ] `ARCHITECTURE.md`, `API.md` and `README.md` are updated in the same commit as the code.
- [ ] `make test`, `make test-integration` and `make lint` are green.
