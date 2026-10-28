# Task 04 notes (encoders)

## Plan

Files in `internal/encoders`:

1. `encoders.go` — `Encoder` interface, `Capability`/`EncoderInfo` (node
   capabilities shape: `{encoders:[{name, codecs:[]}]}`), `Spec`, the three
   registered backends and `Registry(target, caps)`.
2. `quality.go` — the ARCHITECTURE.md step table as one array indexed by step.
3. `detect.go` — `detectEnv` (parsed `-encoders`/`-hwaccels`, device checks),
   `detector` with process-lifetime cache keyed by ffmpeg path, exported
   `DetectAll`, the 1-second `testsrc` self-test helper.
4. `nvenc.go`, `qsv.go`, `cpu.go` — backends. `qsv` keeps a per-codec variant
   (qsv encoder vs VAAPI fallback on Linux) discovered during detection;
   `BuildArgs` uses it.
5. `build.go` — shared command assembly: maps (`0:v:0`, `0:a:<i>`, `0:s:<i>`),
   `-c:v copy` or per-encoder video args, per-stream audio copy or compact
   transcode (aac 160k stereo / opus 128k stereo), per-stream subtitle copy
   with mov_text→srt (to mkv) and srt→mov_text (to mp4), `-movflags +faststart`
   for mp4, `-progress pipe:1 -nostats`, plus `ExtractCommands` for
   `extract_removed` (`.<lang>.srt` beside the output, runner executes them).
6. `fallback.go` — `RunWithFallback(ctx, list, spec, runFn)`; GPU failure
   records a `FallbackReason` and moves on, cpu failure is final; `runFn`
   injectable.
7. Tests: golden `BuildArgs` per encoder × step + copy/compact/subs/mp4/extract,
   detection unit tests with a fake ffmpeg script (no GPU / NVENC / QSV / both,
   cache, self-test failure, registry order), fallback unit test, and the
   CPU-only integration test (3 s testsrc h264→hevc step 3, ffprobe verified).
8. `README.md` — how to add an encoder.

Test-seam design: the detector takes `driDir`/`nvidiaGlob`/binary path so unit
tests point it at a temp dir with fake device files and a fake ffmpeg script;
`Registry` is a pure function of `Target` + detected `Capability`.

## Decisions taken inside task scope (not owner decisions)

- Quality numbers for columns ARCHITECTURE.md does not define are derived and
  documented in `quality.go`: NVENC h264/av1 reuse the NVENC CQ column, QSV
  h264/av1 reuse the QSV global_quality column, VAAPI uses global_quality
  (hevc/av1) or qp (h264). Presets: x264/x265 medium, SVT-AV1 preset 6, NVENC
  p5. Self-tests use fast settings (ultrafast / p1), not the table.
- `Registry(target, caps)` is pure; `DetectAll` caches per ffmpeg path for the
  process lifetime. Task 05 calls `DetectAll` then `Registry`.
- `RunWithFallback` treats the encoder named `cpu` as the final fallback and
  any GPU failure as retryable (interface has no extra method for this).
- Removed-subtitle extraction writes beside `spec.Output` (scratch at job
  time), honoring the never-touch-the-library rule.
- Extract commands get `-v error` (no progress parsing needed); only encodes
  carry `-progress pipe:1 -nostats`.
- mp4 output with srt/ass text subs converts them to mov_text (mirror of the
  mkv rule) so mp4 targets are not guaranteed-broken; bitmap subs in mp4 stay
  a known limitation (copy is emitted; verification in task 05 will catch it).
- Duplicate removed-subtitle languages get an occurrence suffix
  (`movie.eng.srt`, `movie.eng.2.srt`) instead of overwriting.

## Out-of-scope ideas noted, not built

- Mapping attachments (`-map 0:t?`) to keep ASS fonts.
- 10-bit/HDR-aware pix_fmt choices per backend.
- AMD AMF / VideoToolbox encoders (DECISIONS 14: later).

## Summary

- Built: Encoder interface + nvenc/qsv/cpu, quality table, detection with
  cached Capability, Registry ordering, command builder (copy/compact audio,
  subtitle copy+conversion, faststart, progress flags, extract commands),
  RunWithFallback with injectable runFn, golden/detection/fallback unit tests,
  CPU-only integration test, README.
- Skipped: AMD/Apple encoders, attachments mapping, HDR pix_fmt logic
  (documented as limitations/ideas).
- Next task should know: call `encoders.DetectAll(ctx, ffmpegPath)` once
  (cached), then `encoders.Registry(target, caps)`; run
  `encoders.ExtractCommands(spec)` after the main encode; `RunWithFallback`
  takes the registry slice and an injectable run function; `Result` fields are
  `EncoderUsed` and `Fallbacks []FallbackReason{Encoder, Error}`.
