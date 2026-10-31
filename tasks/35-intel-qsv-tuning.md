# Task 35: Getting the real capability out of Intel QSV

## Goal
The QSV path today produces correct files with a fixed argument set. Intel hardware exposes more than that: constant-quality ICQ mode, look-ahead with external bitrate control, an async depth the encoder can use for throughput, and a low-power mode that trades some quality for speed and lower power draw. All four are optional in the driver, so the backend has to probe for them instead of assuming. This task turns them on where they exist, leaves the argument vector byte-identical where they do not, and writes down what the marketing name "Deep Link" covers that a single ffmpeg process cannot reach.

Nothing here may change the output of a machine that supports none of the new flags, and nothing may change it on a machine that supports them until the user asks for it.

## Read first
`internal/encoders/README.md`, `internal/encoders/encoders.go` (the `Encoder` interface), `internal/encoders/qsv.go`, `internal/encoders/vaapi.go` (task 30's device probe and mutex guard, the pattern to copy), `internal/encoders/detect.go`, `internal/encoders/quality.go`, `internal/encoders/build.go`, `internal/encoders/build_test.go`, `internal/encoders/parity_test.go`, `internal/encoders/integration_test.go`, `internal/worker/runner.go` (`registryFor`), `internal/server/settings.go`, `STACK.md`.

## Do

### 1. Probe the optional features, once, per process
In `internal/encoders/qsv.go`, extend the existing capability detection to test each optional feature by running the app's standard one-second `testsrc` self-test with the candidate flag set and checking the exit code, exactly the way the VAAPI backend in task 30 tries candidate devices. The features:

| Feature | Flag under test | Meaning |
|---------|-----------------|---------|
| look-ahead | `-look_ahead 1` with `extbrc` | better quality at the same bitrate, slower |
| extbrc | `-extbrc 1` | external bitrate control, required by look-ahead |
| async depth | `-async_depth 4` | throughput versus latency |
| low power | `-low_power 1` | faster, lower quality, works on Ice Lake and newer |

Cache the result per backend instance behind a mutex, the way task 30's VAAPI backend records its working device, and expose it through the encoder so the UI can show what this machine actually supports. A probe that cannot run (no device, no ffmpeg) must leave every feature off rather than guess.

### 2. Quality mapping
`internal/encoders/quality.go`: the QSV quality table gains ICQ. Map the app's quality value to `-global_quality` for the codecs that accept it (`hevc_qsv`, `av1_qsv`), keeping the existing mapping as the fallback for `h264_qsv` if ICQ is not available there. When look-ahead and extbrc both probe true and the user has not turned the option off, add `-look_ahead 1 -extbrc 1`. State the mapping in a comment with the quality value range and what changes at each end.

### 3. Settings
`internal/server/settings.go` gains two per-node QSV options, both defaulting to today's behaviour:
- `qsv_extra_quality` (bool, default false): turn on look-ahead and extbrc when the machine supports both.
- `qsv_low_power` (bool, default false): pass `-low_power 1` when supported. The settings help text says it is faster and slightly worse, and that it does nothing on older hardware.

`-async_depth` stays internal: pick a value from the quality value (higher quality, lower depth) and document the rule in a comment instead of exposing a knob nobody can reason about. When a feature is requested but the probe says unsupported, log one warn line naming the feature and the machine, then carry on without it: a setting must never turn into a failed job.

### 4. Editor and UI surface
The node detail page shows the probed QSV features for that node (`Look-ahead`, `Low power`, `Async depth`) so a user can see why a setting had no effect, in plain language: `This machine cannot do the faster low-power mode.` No codec names on the easy screen, per `AGENTS.md` rule 14.

### 5. Docs
`docs/intel-qsv.md`, linked from `internal/encoders/README.md` and the README:
- ICQ versus bitrate targeting in plain words, and why constant quality is the default here.
- What look-ahead actually buys and what it costs (speed), and when low-power mode is the wrong choice (archival, high quality targets).
- An honest scope statement on Intel Deep Link: Hyper Encode splits one job across an iGPU and a discrete Arc GPU, which needs two ffmpeg processes and a driver-side scheduler, so **it is not implemented and is not planned**; the bitrate-boost and auto-bitrate features live behind Intel's own driver API and are likewise out of reach from ffmpeg. What this task implements is the QSV feature set ffmpeg does expose. Say that in those words so nobody expects the marketing feature list.

### 6. Tests
- `internal/encoders/build_test.go`: a table of probe-on, probe-off combinations asserting the exact argument vector, including the case where nothing is supported.
- `internal/encoders/parity_test.go`: with both settings off, or with every probe false, the argument vector is byte-identical to `main` before this change. This is the regression that matters.
- A requested-but-unsupported feature produces one warn and no error, proven by a test with a fake ffmpeg that rejects the flag.
- `internal/encoders/integration_test.go`: a real QSV encode with look-ahead on, guarded the way task 30 guarded VAAPI. The build box has no Intel device, so this test must skip with a message naming what is missing; that is expected and is not a failure.

## Out of scope
Hyper Encode or any cross-GPU job splitting, Intel Deep Link's bitrate boost and auto bitrate, AV1 tuning beyond the ICQ mapping, NVIDIA or VAAPI changes, per-library encoder settings, hardware benchmark scoring, `DECISIONS.md` changes.

## Acceptance
- [ ] Capability probing records look-ahead, extbrc, async depth and low power per machine, cached per process, and a machine where the probe cannot run reports all four off. Proven by tests with a fake ffmpeg.
- [ ] With the new settings off, the QSV argument vector is byte-identical to the current behaviour, proven by the parity test.
- [ ] With `qsv_extra_quality` on and both features supported, `-look_ahead 1 -extbrc 1` appear in the right position, proven by a build test.
- [ ] With `qsv_low_power` on and support present, `-low_power 1` appears; with support absent the job still runs and one warn line names the feature.
- [ ] The quality mapping handles both the ICQ and the fallback path with table tests at the quality range ends.
- [ ] The node detail page shows the probed features in plain language, with a `web` test.
- [ ] `docs/intel-qsv.md` exists, states the Deep Link non-goals explicitly, and is linked from the README.
- [ ] `make test`, `make test-integration` and `make lint` are green.
