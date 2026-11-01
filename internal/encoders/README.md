# internal/encoders

Encoder detection, the quality step table, the ffmpeg command builder and
GPU-to-CPU fallback. Nothing here reimplements ffmpeg; everything runs it as
a subprocess (STACK.md).

## What lives here

| File | Contents |
|------|----------|
| `encoders.go` | The `Encoder` interface, `Capability`/`EncoderInfo`, `Spec`, backend registration, `Registry` |
| `detect.go` | Detection: `-encoders`/`-hwaccels` parsing, device checks, the 1-second `testsrc` self-test, process-lifetime cache |
| `nvenc.go` | NVIDIA NVENC (hevc_nvenc, av1_nvenc, h264_nvenc) |
| `qsv.go` | Intel QSV (hevc_qsv, av1_qsv, h264_qsv), with VAAPI equivalents as a Linux fallback; the optional feature probe and the user's QSV opt-ins (task 35, [docs/intel-qsv.md](../../docs/intel-qsv.md)) |
| `vaapi.go` | The VAAPI backend (hevc_vaapi, av1_vaapi, h264_vaapi): AMD and other VAAPI hardware (task 30) |
| `videotoolbox.go` | Apple VideoToolbox (h264_videotoolbox, hevc_videotoolbox); `videotoolbox_other.go` is the non-macOS stub |
| `cpu.go` | Software encoders (libx265, libsvtav1, libx264) |
| `quality.go` | The quality step table from ARCHITECTURE.md, as one data structure |
| `build.go` | The shared command builder and the `extract_removed` commands |
| `fallback.go` | `RunWithFallback`, `Result`, `FallbackReason` |

## How a job uses this package (task 05)

```go
caps, err := encoders.DetectAll(ctx, ffmpegPath) // cached for the process lifetime
// ... per job:
spec := encoders.NewSpec(input, output, info, target) // or fill Spec yourself
list := encoders.Registry(target, caps)
res, err := encoders.RunWithFallback(ctx, list, spec, runFn)
if err == nil {
    for _, ec := range encoders.ExtractCommands(spec) {
        runFn(ctx, ec.Args) // subtitles.extract_removed side commands
    }
}
```

`runFn` executes one ffmpeg command and returns its error; the runner owns
process handling, `-progress pipe:1` parsing and cancellation.

## The command that gets built

One ffmpeg invocation per encode, covering video, audio and subtitles at
once:

```
-hide_banner -nostdin -progress pipe:1 -nostats -y
[-vaapi_device <node>]              (qsv backend's vaapi variant only)
-i <input>
-map 0:V:0  -map 0:a:<i>...  -map 0:s:<i>...   (selected streams only)
<video args>                        (-c:v copy or the encoder's quality args)
<audio args per stream>             (copy, or compact: aac 160k / opus 128k stereo)
<subtitle args per stream>          (copy; mov_text→srt into mkv, text→mov_text into mp4)
[-movflags +faststart]              (mp4 only)
<output>
```

Details worth knowing:

- Video mapping uses `0:V:0` (uppercase) so cover-art streams are excluded,
  matching `probe.MediaInfo.Video`. Audio and subtitle indexes are positions
  within their stream type, exactly the `Spec.KeepAudio`/`KeepSubs` values
  (computed by `flow.SelectAudio`/`flow.SelectSubtitles`).
- `-c:v copy` is used for an explicit `video.codec=copy` target **and** when
  the file's video already is the target codec with
  `reencode_if_same_codec` off (the satisfied rule, DECISIONS.md B2): those
  jobs only remux tracks and change the container. `-y` is included so a
  failed GPU attempt cannot leave a partial scratch output that blocks the
  next fallback attempt.
- `video.codec=copy` never re-encodes: `-c:v copy`, identical from every
  encoder, so the registry for a copy target is just `cpu`.
- Per-stream specifiers (`-c:a:0`, `-c:s:1`) refer to output stream
  positions in map order.
- Compact audio (`audio.mode=compact`) transcodes the kept track unless it
  already is the compact codec, then it is copied as-is.
- Removed text subtitles are extracted to `<output-base>.<lang>.srt`
  (beside the output, which is in scratch at job time — the library folder
  is never written before verification). Repeated languages get an
  occurrence suffix (`movie.eng.srt`, `movie.eng.2.srt`). Bitmap subtitles
  (PGS and friends) cannot be converted and get no extract command.
- Subtitle extraction runs with `-v error` and no progress flags; only real
  encodes carry `-progress pipe:1 -nostats`.

## Detection

### Precedence: nvenc, then qsv, then vaapi, then cpu

The backends register in this order, and detection, the registry and the
fallback all follow it (asserted by `TestEncoderPrecedence`):

| Backend | Needs at runtime | Codecs (subject to the self-test) |
|---------|------------------|-----------------------------------|
| `nvenc` | an NVIDIA GPU (`/dev/nvidia*`) and its driver; ffmpeg built with the `*_nvenc` encoders | hevc, av1, h264 |
| `qsv` | a `/dev/dri` render node the user can open (usually the `render` or `video` group) plus Intel's runtime (libmfx/oneVPL, or its VAAPI variant) | hevc, av1, h264 |
| `vaapi` | a `/dev/dri/renderD128-130` node the user can open (`render`/`video` group) and a Mesa/kernel driver with VAAPI encode entrypoints; AV1 needs a driver recent enough | hevc, av1, h264 |
| `videotoolbox` | macOS only (see below); hevc additionally needs macOS 10.13 or newer and 6th-generation Intel or Apple Silicon hardware | h264, hevc |
| `cpu` | nothing — the final fallback, always present | hevc, av1, h264 |

Intel Arc and AMD cards both surface as `vaapi` or `qsv` depending on the
driver stack (see the device-claiming rules in the vaapi section below).

`DetectAll` gathers, once per ffmpeg path:

1. `ffmpeg -hide_banner -encoders` — which ffmpeg encoders this build has.
2. `ffmpeg -hide_banner -hwaccels` — used to gate the VAAPI fallback.
3. Device presence: `/dev/nvidia*` for NVENC, `/dev/dri` (render node) for
   QSV/VAAPI. Skipped on Windows, where the self-test is the only gate.
4. A one-second `testsrc` self-test per candidate codec (fast settings —
   the self-test proves the encoder session works, it does not measure
   quality). The self-test is the final word: a listed encoder can still
   fail on the actual hardware or driver.

Results are cached for the process lifetime, keyed by ffmpeg path; a
canceled or timed-out run is not cached. The reported capability is the
encoders part of the node capabilities shape (`{encoders: [{name, codecs}]}`);
each backend lists only the codecs that passed detection, so a build without
libsvtav1 or a GPU without AV1 support is reported honestly.

### The qsv backend's VAAPI variant

On Linux, when a codec's `*_qsv` self-test fails but the VAAPI stack is
present, the backend retries with the `*_vaapi` encoder (`-vaapi_device`
plus `format=nv12,hwupload`). The chosen variant is stored on the backend
and used by `BuildArgs` — so `BuildArgs` for the "qsv" backend may emit
`-c:v hevc_vaapi`. This is per DECISIONS.md 14 (Intel QSV/VAAPI as one
backend from the user's point of view).

### The qsv backend's optional features (task 35)

Detection also probes the optional QSV encoder features per codec —
look-ahead with extbrc, async depth and low power — by running the same
one-second self-test with the candidate flag set and reading the exit
code. Results are cached per backend instance (the same pattern as the
variant/device maps), reported on the qsv entry's `qsv_features` in the
node capabilities, and consulted by `BuildArgs`: a flag is emitted only
when the user opted in (`qsv_extra_quality`, `qsv_low_power` — carried to
the worker on the heartbeat) **and** the probe accepted it. With the
settings off the argument vector is byte-identical to the pre-task-35
behaviour, and a requested-but-unsupported feature produces one warn and
carries on. ffmpeg's `-look_ahead` option exists only on `h264_qsv`, so
look-ahead can only probe true for the h264 target. The plain-language
version of all of this is [docs/intel-qsv.md](../../docs/intel-qsv.md).

## The vaapi backend (AMD and friends, task 30)

`vaapi` is the standalone VAAPI backend: `-vaapi_device <node>`, the
`format=nv12,hwupload` filter chain and the `*_vaapi` encoders. Quality
maps onto the QSV column of the quality table: `-global_quality` for hevc
and av1, `-qp` for h264 (h264_vaapi has no global_quality mode). Detection
requires `ffmpeg -hwaccels` to list `vaapi`, then self-tests each codec
against each candidate render node — `/dev/dri/renderD128`, `renderD129`,
`renderD130`, in that order — until one works; the working device is
recorded per codec, exactly like the qsv variant map, and used by
`BuildArgs`.

**Device claiming (no double counting).** The backends run detection in the
precedence order below, and the vaapi backend skips any render node the qsv
backend already recorded as one of its vaapi-variant devices, so one GPU is
never reported twice. A machine can still legitimately report both `qsv`
and `vaapi`: when qsv works through its own `*_qsv` encoders it claims no
device node, and on a machine with several render nodes the vaapi backend
takes the ones qsv left. Intel Arc and AMD cards both surface as `vaapi` or
`qsv` depending on the driver stack — on an AMD-only machine the qsv
backend's VAAPI variant typically claims the card first, so it reports as
`qsv` while actually driving the VAAPI encoders underneath.

**AMD AMF is a documented non-goal.** On Linux, AMF needs AMD's proprietary
kernel stack (amdgpu-pro) and is unusable inside a container; VAAPI through
Mesa is the AMD hardware path and is what this package ships. Do not file
AMF against this package.

## Fallback

`RunWithFallback` tries the registry in order: a GPU encoder failure is
recorded as a `FallbackReason` and the next encoder is tried; the `cpu`
encoder is the final fallback and its failure ends the job. Registry order
is the precedence order: detected GPU encoders that support the target
codec (detection order: nvenc, qsv, vaapi, then videotoolbox on macOS),
then cpu — always cpu, even when detection found nothing.

## How to add an encoder

The package ships nvenc, qsv, vaapi and cpu, plus videotoolbox on macOS.
Adding another one (the remaining DECISIONS.md 14 items are covered; AMF is
a documented non-goal, see above):

1. **Implement the interface** in a new file `sample.go`:

   ```go
   type sampleEncoder struct{}

   func (e *sampleEncoder) Name() string { return "sample" }
   func (e *sampleEncoder) Codecs() []string { return []string{flow.CodecHEVC, flow.CodecH264} }
   func (e *sampleEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
       // Parse the environment (or accept it, see below), check hardware
       // presence, self-test per codec, return your EncoderInfo.
   }
   func (e *sampleEncoder) SelfTest(ctx context.Context) error { ... }
   func (e *sampleEncoder) BuildArgs(spec Spec) []string {
       // Video args from the quality table, then buildCommand(spec, pre, video).
       // buildCommand handles copy, audio, subtitles, container and maps.
   }
   ```

2. **Wire it in for batch detection**: add a `case *sampleEncoder:` line to
   `detector.detectEncoder` in `detect.go` with a `detectWithEnv` method (so
   it shares the parsed `-encoders` list instead of re-parsing), then add
   `Sample` to the `allEncoders` slice in `encoders.go` **before cpu**.
3. **Add a quality column**: add a field to `qualityValues` in
   `quality.go`, fill the column for all five steps, and use it in your
   `BuildArgs`. Columns ARCHITECTURE.md does not define are derived in the
   comment above the table — document yours the same way.
4. **Test**: golden `BuildArgs` cases (every step, at least one per codec),
   a detection case with the fake ffmpeg script (`fakeFFmpeg` in
   `detect_test.go`, including the "self-test fails" path), and a fallback
   case exercising your encoder failing to the next one.

Rules that apply: never reimplement ffmpeg (call it), never assume video is
the only media type in core types (this package is allowed to be
video-focused), and never touch a user file — this package only writes to
`spec.Output` paths given to it.

## Known limitations (deliberate, post-MVP material)

- Bitmap subtitles are copied everywhere; into mp4 they will fail and the
  job verification (task 05) reports it instead of silently dropping tracks.
- Attachments (ASS fonts) are not re-mapped into the output.
- Pixel format is left to ffmpeg; 10-bit/HDR inputs keep their depth where
  the encoder supports it. HDR-aware quality decisions are post-MVP.
- The vaapi paths (the vaapi backend and the qsv backend's fallback variant)
  force `format=nv12` (8-bit).
