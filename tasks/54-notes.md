# Task 54 notes: Target extension, scale step, richer transcode_video

Branch: `task/54-target-scale-and-rate`. Spec: `docs/design/encode-settings-tiers-2026-11-04.md` sections 2, 3, 5.1, 5.2.

## Plan (written before the first edit)

1. Capture v1.9 behaviour as a golden: a v1.9-shaped graph (`testdata/v19_graph.json`) and the
   Target, Describe, ResolvePlan and easy-default outputs from the unchanged code.
2. `target.go`: `ScaleTarget`, `RateTarget`, `Preset` on `VideoTarget` (omitempty), shared
   `rateRuleError`, Target validation for the new fields.
3. `graph.go`: `scale` step, `transcode_video` extended (rate, crf, bitrate, max, preset,
   hardware), validation, `scale` in the shaping and needs-media sets.
4. `plan.go`: `applyTranscode` keeps a scale placed before the encode; `scale` sets Scale;
   `Plan.Target` and `ApplyStep` share the helpers; `Describe`; `StepTypes()`.
5. `targetflow.go`: a Target carrying scale, rate or preset must survive
   `TargetToFlow` (see deviation 1).
6. Tests in the three named files, plus the golden file; `api.ts` types; ARCHITECTURE.

## Scope additions (outside the listed files)

1. **`internal/flow/targetflow.go` changed.** Not in the scope list. Without it a Target with
   scale, rate or preset (now valid via `PUT`) would be silently dropped when compiled to a
   graph, which breaks the "nothing is silently dropped" rule and the `Compile(TargetToFlow(t))`
   parity invariant. A Target without the new fields generates the same graph JSON as before
   (a test pins it).
2. **`internal/flow/golden_v19_test.go` and `internal/flow/testdata/`** are new. The golden
   proves the first acceptance item. The golden was captured from the v1.9 code (main at
   e824b7d) before any edit. It regenerates only with `FLOW_UPDATE_GOLDEN=1`.
3. **`gofmt` on `plan.go`** also reformatted one unrelated struct (`StepTypeInfo`) alignment.
   Formatting only. `targetflow.go` was already not gofmt clean at HEAD, left as found.

## Decisions I made (owner can redirect)

- **Scale height accepts `1080` and `"1080"`.** The spec asks for a select, and the web form
  sends select values as strings, so a plain int would fail to decode from the UI. A custom
  unmarshal on the step's height type accepts both. Anything else is rejected.
- **The seven cross-field rules** (each has a failing and a passing test in
  `TestValidateTranscodeCrossFieldRules`): (1) crf mode needs a crf; (2) bitrate mode needs
  `bitrate_kbps`; (3) crf set outside crf mode; (4) bitrate set outside bitrate mode; (5)
  `max_kbps` in quality mode; (6) `max_kbps` below `bitrate_kbps` in bitrate mode; (7) unknown
  `rate_mode`. The crf range 0 to 51 is a separate single field check.
- **`quality` stays required (1 to 5) in every rate mode.** It is the existing scalar and
  `Target.Video.Quality` is always validated, so it cannot be "not set". crf and bitrate modes
  ignore it at encode time. Task 55 should confirm this when it builds arguments.
- **`rate_mode` quality (or empty) resolves to `Rate: nil`.** Keeps existing graphs and Targets
  byte-identical. Explicit `Rate{Mode: "quality"}` in a Target is valid and means the same.
- **`only_if_larger` and `allow_upscale` are kept as spec'd.** Their combination is slightly
  redundant. `only_if_larger=false` with `allow_upscale=false` reads as "scale to the height,
  never enlarge", which is the third `describeScale` wording. Worth a glance from the owner.
- **Scale placed after the encode in `TargetToFlow`, with the encode guard repeated before it.**
  Inserting it between the guard and the transcode would move the guard onto the scale step and
  leave the encode unguarded. A skipped encode therefore never scales.
- **`hardware` is validated and described, not carried.** Task 57 carries it into the plan.
  Describe omits `auto`.

## Known gaps for later tasks (not regressions: nothing reads these fields yet)

- **Encoders ignore scale, rate and preset until task 55.** A flow with a scale step now runs
  as an unscaled encode. The wave is unreleased, so no user sees this, but do not ship between
  54 and 55.
- **`Evaluate` (the satisfied rule) does not look at scale.** A file already HEVC and taller than
  the requested height is currently "satisfied" and never scaled. Task 55 or a follow up must
  add a scale check to the satisfied rule, or it will skip files it should scale.
- **A scale on a copy video** (encode skipped by its guard) resolves to `codec copy` with a
  `Scale` set. This is correct for the flow (the skipped encode does not scale) but the encoder
  must treat "copy with scale" as a no-op, not an error.

## Verification

- `go test ./...`: all packages `ok` (`internal/api` and `internal/server` take about 5 minutes
  each on this VM).
- `go vet ./...`: clean. `golangci-lint` is not installed on this VM, so it was not run.
- `./scripts/check-spdx.sh`: clean.
- `npm run test` (web): 62 files, 444 tests pass. `npm run build`, `npm run lint`,
  `npm run format:check`: pass. `tsc --noEmit`: pass.
- `npx playwright test`: 104 passed, 6 skipped, 0 failed (the skips are the ffmpeg-dependent
  cases; no ffmpeg on this VM). The first run failed before any test ran: this VM's headless
  Chromium could not load `libatk-1.0.so.0`. The rerun set `LD_LIBRARY_PATH` to the user-local
  copy at `~/.local/pwlibs` (no system change). The first attempt also hit port 8712, which
  belongs to another running `shrinkyourshit` process (pid 1480109), so the run used
  `SHRINK_E2E_PORT=8795` and that process was left alone.
- No existing assertion was changed.

## Summary

1. Built: `Target.video` gains optional `scale`, `rate` and `preset`; the `scale` step; the
   `transcode_video` rate, crf, bitrate, max, preset and hardware fields with seven cross-field
   rules; palette entries; plain-language Describe; TargetToFlow carries the new fields.
2. Skipped: encoder arguments (55), satisfied rule over scale (not listed; see gaps), hardware
   carried into the plan (57), the three-tier UI (60 onward).
3. Owner calls to glance at: scale height accepts a numeric string; `only_if_larger` and
   `allow_upscale` combination; the seven rules as enumerated above.
4. Task 55 must: map rate and preset to arguments, treat copy-with-scale as a no-op, confirm
   quality is ignored in crf and bitrate modes, and add scale to the satisfied rule.
5. Not done here: the CI run and PR (supervisor opens it).
