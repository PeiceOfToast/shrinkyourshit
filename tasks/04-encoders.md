# Task 04: Encoder detection, quality table, command builder, GPU to CPU fallback

## Goal
Detect available hardware, build correct FFmpeg commands from a Target plus a probe, and fall back from GPU to CPU automatically.

## Read first
ARCHITECTURE.md (hardware and encoders, Target schema, quality step table), DECISIONS.md (7, 14). Depends on task 02 (uses `probe.MediaInfo`, `flow.Target`, selection helpers).

## Do
- `internal/encoders`:
  - `Encoder` interface: `Name() string`, `Codecs() []string`, `Detect(ctx, ffmpegPath) (Capability, error)`, `SelfTest(ctx) error`, `BuildArgs(spec Spec) []string`.
  - `Spec{Input, Output string; Info probe.MediaInfo; Target flow.Target; KeepAudio, KeepSubs []int}`.
  - Implementations: `nvenc` (hevc_nvenc, av1_nvenc, h264_nvenc), `qsv` (hevc_qsv, av1_qsv, h264_qsv; on Linux also try VAAPI equivalents when QSV init fails), `cpu` (libx265, libsvtav1, libx264).
  - `quality.go`: the step table from ARCHITECTURE.md as one data structure.
  - Detection: parse `ffmpeg -hide_banner -encoders` and `-hwaccels`; check `/dev/dri` and NVIDIA device presence; then a 1-second `testsrc` encode as self-test. Results cached for the process lifetime and reported as `Capability{Encoders: [{Name, Codecs}]}` in the node capabilities shape from ARCHITECTURE.md.
  - `Registry`: ordered list for a Target: GPU encoders whose `Codecs()` include `target.video.codec`, then cpu.
  - Command builder: maps selected streams (`-map 0:v:0`, audio and subtitle indexes from the spec), video codec and quality from the table, audio copy or `compact` transcode, subtitle copy (convert `mov_text` to `srt` when going to mkv), `-movflags +faststart` for mp4, `-progress pipe:1 -nostats`, output to `spec.Output`. For `video.codec=copy` use `-c:v copy`. When `subtitles.extract_removed` is true, emit an additional ffmpeg command per removed text subtitle writing `.<lang>.srt` (the runner in task 05 executes it).
  - `RunWithFallback(ctx, registry, spec, runFn) Result{EncoderUsed string; Fallbacks []FallbackReason}`: tries encoders in order; a non-zero exit from a GPU encoder moves to the next; CPU failure is final. `runFn` is injectable for tests.
- `internal/encoders/README.md`: how to add an encoder (implement the interface, register it, add a column to the quality table).

## Out of scope
Scratch dirs, copying, verification, job lifecycle (task 05). AMD and Apple encoders.

## Acceptance
- [x] Golden-file unit tests for `BuildArgs` for each encoder and each quality step, plus `copy`, `compact` audio, subtitle removal, mp4 output, and `extract_removed`.
- [x] Detection unit tests with a fake ffmpeg script covering: no GPU, NVENC only, QSV only, both; `Registry` order is correct in each case.
- [x] Fallback unit test: fake nvenc fails, cpu succeeds, `Result` reports `EncoderUsed="cpu"` and one fallback reason naming nvenc and the exit error.
- [x] Integration test on a CPU-only machine: encode a 3-second `testsrc` fixture from h264 to hevc with step 3 and ffprobe confirms hevc video, correct track count.
- [x] Capability JSON matches the node capabilities shape in ARCHITECTURE.md.
