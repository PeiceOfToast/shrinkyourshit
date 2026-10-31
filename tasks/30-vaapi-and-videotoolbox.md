# Task 30: VAAPI and VideoToolbox encoders

## Goal
AMD and other VAAPI hardware gets real hardware encoding, and the macOS binaries get VideoToolbox. The encoder registry stops being Intel and NVIDIA only.

## Read first
`internal/encoders/README.md` (the documented way to add a backend), `internal/encoders/encoders.go` (the `Encoder` interface), `internal/encoders/qsv.go` (the existing QSV path, which already falls back to VAAPI variants), `internal/encoders/nvenc.go`, `internal/encoders/detect.go` (how devices are found and how `-encoders` output is parsed), `internal/encoders/fallback.go`, `internal/encoders/quality.go`, `internal/worker/runner.go` (`registryFor`), `internal/flow/target.go` (the codec names).

## Do

### 1. `internal/encoders/vaapi.go` — a real VAAPI backend
A backend named `vaapi`, codecs `hevc`, `av1`, `h264` (honest detection decides which actually work):
- `Detect`: require `ffmpeg -hwaccels` to list `vaapi`, then find a usable device by trying `/dev/dri/renderD128`, `renderD129`, `renderD130` in order, and run the one-second testsrc self-test per codec against each candidate device until one works. Record the working device in the backend so `BuildArgs` can use it, exactly the way the QSV variant map does it today, and guard it with a mutex.
- `BuildArgs`: the standard VAAPI shape — `-vaapi_device <node>`, `-vf format=nv12,hwupload`, `-c:v <codec>_vaapi` for h264 and hevc, `-c:v av1_vaapi` where the driver supports it, plus the quality mapping from `quality.go` mapped to `-qp` for VAAPI. Keep the same audio, subtitle, container and metadata flags the other backends use; copy the surrounding argument order rather than inventing it.
- Skip VAAPI devices that belong to a QSV-capable Intel GPU **only if QSV already claimed them** — do not silently double count. State the precedence in `internal/encoders/README.md`: nvenc, then qsv, then vaapi, then cpu, and explain that a machine can legitimately report both qsv and vaapi.

### 2. `internal/encoders/videotoolbox.go`
- Build-tagged: the implementation lives behind `//go:build darwin`, with a `//go:build !darwin` sibling that returns no capability and errors on detect, so the registry compiles everywhere and the capability list is honest per platform.
- `Name()` is `videotoolbox`, codecs `h264` and `hevc` (AV1 is not available there).
- `BuildArgs` uses `-c:v h264_videotoolbox` / `hevc_videotoolbox` with `-q:v` mapped from the quality scale, and `-allow_sw 0` so a software fallback cannot masquerade as hardware. Apple has no AV1 hardware encoder; do not pretend otherwise.

### 3. Registry, docs, and honest capability reporting
- Register both in whatever list `detect.go` iterates, keeping the existing order semantics.
- `internal/encoders/README.md`: the precedence table, what each backend needs at runtime (device node with the right group, macOS version floor for hevc), and the line that Intel Arc and AMD cards both surface as `vaapi` or `qsv` depending on the driver stack.
- `web/src/EncodeSettingsPage.tsx` and the nodes page: if either hardcodes the known encoder names (`nvenc`, `qsv`, `cpu`), add `vaapi` and `videotoolbox` with a plain language label. Search for the names rather than assuming.
- `CHANGELOG.md`: entries under `[0.1.3] - unreleased`.

### 4. Explicitly out of scope, and say so in the PR
AMD AMF: on Linux it needs AMD's proprietary stack and is unusable in a container, so VAAPI is the correct AMD path. Note it in the encoders README as a documented non-goal rather than leaving the question open.

## Out of scope
AMF, Intel Deep Link, multi-GPU scheduling in one job, changing the quality scale, changing the fallback policy.

## Acceptance
- [x] `vaapi` appears in a node's capability list on a machine with a working VAAPI device, and `nvenc` / `qsv` reporting is unchanged on machines that have them. If the build box has no VAAPI device, say so in the PR and prove detection with a fake `ffmpeg` script plus a skipped integration test.
- [x] `GOOS=darwin GOARCH=arm64 go build ./...` and `GOOS=darwin GOARCH=amd64 go build ./...` succeed, and a darwin build registers `videotoolbox` while a linux build does not. Proven by a test that asserts the per-platform registration.
- [x] Encoding with VAAPI produces a file whose duration and streams match the source on real hardware, proven by the existing integration test running against the vaapi backend when a device exists.
- [x] The precedence order is written in `internal/encoders/README.md` and matched by a test.
- [x] `web/` shows the two new names wherever encoder names are listed, with matching tests.
- [x] `make test`, `make test-integration` and `make lint` are green.

Notes on the ticked boxes: the build box has no VAAPI device (`/dev/dri`
exists but holds no `renderD*` node, and the local ffmpeg is a static build
whose `-hwaccels` lists only `vdpau`), so per the carve-outs above detection
is proven with the fake ffmpeg script suite in `vaapi_test.go`, and
`TestEncodeVAAPI` (integration) skips with a message naming exactly what is
missing — it runs for real on any machine with a working render node.
