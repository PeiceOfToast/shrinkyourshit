# Task 55 notes: encoder arguments for scale, rate and preset

## Plan (written before implementation)

Files, in order:

1. `internal/flow/target.go`: exported `ScaleApplies(srcHeight, *ScaleTarget) bool`, the one
   rule for "does this scale change the file". Used by the encoders and by the satisfied rule.
2. `internal/flow/evaluate.go`: the satisfied rule gains the scale clause. A file already in
   the target codec whose height the scale would change needs work. (54 notes ask for this;
   without it a 4K HEVC file with HEVC + scale 1080 is never scaled.) Explicit `copy` is
   unchanged: copy never re-encodes, so scale is a no-op there (54 notes).
3. `internal/encoders/encoders.go`: `Spec` gains `Scale`, `Rate`, `Preset`, filled by
   `NewSpec` from `Target.Video` (the runner already calls `NewSpec` with the resolved
   `eff.Target`, so nothing else is wired).
4. `internal/encoders/build.go`: scale is applied in `buildCommand`, the one place every
   encoder's video args pass through. It merges into an existing `-vf` (QSV/VAAPI hwupload)
   so there is still one filter chain. `videoShouldCopy` returns false when the scale would
   apply, so a re-encode is never replaced by a remux that drops the scale.
5. `internal/encoders/rate.go` (new): shared helpers for rate mode args, caps, CRF string,
   preset maps per encoder.
6. Per encoder `BuildArgs`: cpu.go, nvenc.go, qsv.go (qsv and its vaapi variant), vaapi.go,
   videotoolbox.go. Quality mode stays byte-identical.
7. `internal/encoders/encoders.go` `Registry`: videotoolbox is not offered in crf mode (it has
   no CRF concept), so the fallback chain goes to a backend that can honour the rate mode.
8. Tests: exact vectors per encoder (scale, crf, bitrate with cap, preset, one QSV case), golden
   for a source that fits the scale target, a copy-with-scale no-op, and an `Evaluate` test.

## Argument decisions (owner calls are listed in the summary)

- Scale: `scale=-2:<H>`. Applies when source height > H, or source height < H and
  `allow_upscale`. Equal height or unknown height: no filter. `only_if_larger` does not change
  the vector: upscaling is governed by `allow_upscale` alone.
- Quality mode: unchanged vectors (libx265 `-crf` q, libx264 `-crf` q, libsvtav1 `-crf` q,
  NVENC `-cq` q, QSV/VAAPI `-global_quality` q, `-qp` for h264 vaapi, VideoToolbox `-q:v`).
- CRF mode, software: `-crf <crf>` for libx265, libx264 and libsvtav1 (AV1 is included because
  it has the same CRF option; without it AV1 would silently ignore the rate mode).
- CRF mode, NVENC: `-cq round(crf)`, the constant quality knob on the same 0..51 scale.
- CRF mode, QSV and VAAPI: `-global_quality round(crf)` (`-qp` for h264 vaapi), clamped to
  1..51, the ICQ range the quality table uses.
- CRF mode, VideoToolbox: not supported; `Registry` skips it in this mode.
- Bitrate mode, all encoders: `-b:v <n>k`. With a cap: `-maxrate <m>k -bufsize <2m>k`. The cap
  also applies in crf mode.
- Preset, software x265/x264: the name passes through (`medium` is the default). SVT-AV1 takes
  a number: veryfast 10, faster 9, fast 7, medium 6, slow 5, slower 4, veryslow 2.
- Preset, NVENC: veryfast p2, faster p3, fast p4, medium p5 (the default), slow p6, slower p7,
  veryslow p7.
- Preset, QSV: the name passes through as `-preset`. VAAPI and VideoToolbox have no preset
  flag and skip it.
- QSV look-ahead is only emitted in quality and crf modes. It is a look-ahead rate mode and
  does not combine with `-b:v`.

## Owner calls and deviations (read these)

- **Satisfied rule changed (beyond the task file's scope list).** The 54 notes asked for scale in the
  satisfied rule. Done in `internal/flow/evaluate.go` (a 4K HEVC file with an HEVC + 1080 target now
  needs work) and in `videoShouldCopy` (`build.go`). Both use the new `flow.ScaleApplies`.
- **Spec fields.** `Scale`, `Rate` and `Preset` are copied from `Target.Video` by `NewSpec`. The runner
  already calls `NewSpec` with the resolved `eff.Target`, so the runner code is unchanged; the runner
  test `TestRunnerPassesScaleAndRateToEncode` proves the flow reaches the arguments.
- **AV1 CRF on libsvtav1** is included, not only hevc and h264. Without it AV1 would silently ignore crf.
- **CRF on QSV, VAAPI and NVENC** maps the CRF value onto the encoder's constant-quality knob
  (global_quality or cq). This is an approximation, not a same-scale match; a tuning call for the owner.
- **VideoToolbox in crf mode** is skipped by the registry, so the chain falls back. It has no CRF concept.
- **`only_if_larger`** does not change the argument vector; upscaling is governed by `allow_upscale` only.
- **Hand-written graph edge:** if a transcode step is skipped by an `if_codec_not` guard, the codec falls
  back to copy and an unguarded scale step after it is a no-op (the 54 "copy-with-scale is a no-op" rule).
  TargetToFlow guards the scale too, so generated flows are not affected.

## Summary

1. Built: scale (`scale=-2:H`, merged into any existing `-vf`), crf, bitrate with cap and preset
   arguments for cpu (libx265, libx264, libsvtav1), nvenc, qsv (and its vaapi variant), vaapi and
   videotoolbox. Registry skips videotoolbox for crf. Satisfied rule and the copy decision now honour scale.
2. Skipped: per step hardware selection (57). No new step types. No change to the Target JSON shape.
3. Tests: `rate_test.go` (exact vectors, golden for a fitting source on every backend, registry),
   `flow/scale_test.go`, a runner test, and a darwin-only VideoToolbox test. Existing tests unchanged.
4. Verified: `go test ./...` exit 0 (run directly, `make` absent), `go vet`, gofmt on touched files,
   SPDX check, `npm run build`, `npx playwright test` (104 passed, 6 skipped). golangci-lint is not installed.
   The VideoToolbox test was not executed (no macOS here).
5. Next task (56) should know: `if_resolution_above` will reuse `flow.ScaleApplies`-style height reads
   from `Info.Video[0].Height`; `runner_test.go` was already not gofmt-clean at HEAD and was left as is.
