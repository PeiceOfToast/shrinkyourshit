# Task 30 notes: VAAPI and VideoToolbox encoders

## Plan (files, in order)

1. `internal/encoders/vaapi.go` — new `vaapiEncoder`: requires `ffmpeg -hwaccels`
   to list `vaapi`; candidate devices are the renderD128/129/130 entries that
   exist in `/dev/dri`; per-codec self-test per device until one works
   (variant map like qsv, mutex-guarded); skips devices the qsv backend
   recorded as its vaapi-variant devices (no double count). BuildArgs mirrors
   the proven qsv vaapi-variant shape: `-vaapi_device <node>` pre-input,
   `-vf format=nv12,hwupload`, `-global_quality` for hevc/av1 and `-qp` for
   h264 (h264_vaapi has no global_quality mode — same rule qsv.go applies).
2. `internal/encoders/videotoolbox.go` + `videotoolbox_other.go` — build-tag
   split (`darwin` / `!darwin`); name `videotoolbox`, codecs h264+hevc only;
   `-c:v {codec}_videotoolbox -q:v <table> -allow_sw 0`; the `!darwin`
   sibling errors on detect.
3. `internal/encoders/encoders.go` — register VAAPI and VideoToolbox in
   `allEncoders` before cpu; `internal/encoders/detect.go` — add the
   `detectWithEnv` cases.
4. `internal/encoders/quality.go` — add a derived VideoToolbox column
   (1..100 constant-quality scale, higher is better).
5. Tests: `vaapi_test.go` (fake-ffmpeg detection, claimed-device skip,
   BuildArgs goldens), `videotoolbox_test.go` (per-platform registration,
   both build tags), `build_test.go` goldens, `fallback_test.go`,
   `parity_test.go` gains VAAPI, `integration_test.go` gains a VAAPI encode
   that skips without a device, precedence test.
6. `internal/encoders/README.md` — precedence (nvenc, qsv, vaapi, then cpu),
   runtime requirements per backend, Intel Arc/AMD driver-stack line, AMF as
   a documented non-goal. `CHANGELOG.md` under `[0.1.3] - unreleased`.
7. Web: neither `EncodeSettingsPage.tsx` nor the nodes page hardcodes encoder
   names (verified by grep — both render what the API returns), so nothing to
   add there; tests added to prove `vaapi`/`videotoolbox` render on the nodes
   page and in the file history.

## Environment notes

- Build box: `/dev/dri` exists but has **no renderD node** (only `card0`),
  and the local ffmpeg is a static build whose `-hwaccels` lists only
  `vdpau`. So real VAAPI encoding cannot be proven here: detection is proven
  with the fake ffmpeg script, and the real-hardware integration test skips
  with a clear message (to be stated in the PR, per acceptance 1).

## Questions / decisions

- Quality flag: the task text says "mapped to `-qp` for VAAPI"; `h264_vaapi`
  has no global-quality mode (existing qsv.go comment) while hevc/av1 use the
  proven `-global_quality` path. The standalone backend copies the exact
  per-codec shape qsv.go already ships for its vaapi variant instead of
  inventing a new one ("copy the surrounding argument order rather than
  inventing it").
- AMF (acceptance 4): on Linux it needs AMD's proprietary kernel stack and is
  unusable in the container; VAAPI is the AMD path. Recorded as a non-goal in
  the encoders README, noted in the PR.

## Summary (end of run)

- Built: real `vaapi` backend (per-codec device selection, qsv claimed-device skip), build-tagged `videotoolbox` backend, registry/README/changelog updates, web render tests for the new names, precedence + parity tests, and a real-hardware VAAPI integration test.
- Skipped (out of scope): AMF (documented non-goal in the encoders README), Intel Deep Link, multi-GPU scheduling, quality scale and fallback policy changes.
- Flagged for the owner: on an AMD-only machine the qsv backend's vaapi variant claims the card and it reports as `qsv` (the task's own README line covers this); a copied video no longer carries `-vaapi_device` args (qsv changed too, keeps the parity gate byte-identical).
- Real-hardware VAAPI encode could not be run on the build box (no renderD node, no vaapi hwaccel in the local ffmpeg); covered by fake-ffmpeg unit tests plus a self-skipping integration test.
- Next task should know: encoder registry order is now nvenc, qsv, vaapi, videotoolbox (darwin only), cpu; `qualityFor` gained a VideoToolbox column.
