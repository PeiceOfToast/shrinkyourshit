# Task 57 notes: per step hardware selection

Branch: `task/57-per-step-hardware` (from main at 97c1fd6).

## Plan

1. `internal/flow/plan.go`: `Effective` gains `Hardware` and `HardwareStep`. `ApplyStep` sets both
   when a transcode step runs, so a skipped transcode leaves them empty.
2. `internal/worker/runner.go`: `registryFor` takes the step's hardware and the step ID.
   `auto` or empty returns today's list unchanged. An explicit encoder keeps only that encoder,
   with no cpu appended. `software` keeps only cpu. A missing encoder returns an error naming the
   step, and the job fails before any encode runs.
3. The existing `encoder chosen:` job log line gains `(step <id> set hardware to <value>)` only
   when the choice is explicit, so auto logs are byte-identical.
4. Tests: `internal/flow/hardware_test.go` (new, carries the choice through `ResolvePlan`);
   three new tests appended to `internal/worker/runner_test.go` using a `fakeHW` backend in the
   `fakeGPU` style. No existing test changed.
5. `ARCHITECTURE.md`: the `hardware` sentence now describes the behaviour instead of saying
   the plan carries it "from task 57". No new Target fields, no new enum values.

## Decisions taken

- The choice lives on `Effective`, not on `Target`, as the spec says. `Target` is the shape stored
  in the spec and the satisfied rule, and it stays unchanged.
- An explicit encoder never falls back to software, even when the encode itself fails at runtime.
  This is the "6 hour job without telling anyone" rule from the task. `software` is the only way to
  get the CPU on purpose.
- `nvenc` is not offered as a per step value. The graph validator (task 54) enumerates
  `auto|qsv|vaapi|videotoolbox|software`, and changing that is out of scope.
- No UI (tasks 61 and 62).

## Evidence for acceptance

Commands run from the repo root with the PATH from the environment notes.

- `make test`: exit 0. Web: 62 files, 444 tests passed. Go: 13 packages `ok`, no FAIL.
- `make lint`: exit 0 (`go vet`, `golangci-lint`, spdx, eslint, prettier all clean).
- `make test-integration`: exit 0, 13 packages `ok` (ffmpeg present).
- `go test ./internal/flow/ ./internal/worker/ -run 'Hardware|Effective|Runner' -v`: all PASS.

## Summary

1. Built: per step `hardware` on transcode is carried into `Effective` and enforced by the runner.
   `auto` keeps the global policy. An explicit encoder runs alone, with no fallback. A missing
   encoder fails the job naming the step. The job log names the step's choice.
2. Skipped: the UI for the field (tasks 61 and 62), `nvenc` as a per step value (not in the
   validator's list), and any change to `Target`.
3. Tests: 2 new flow tests in `hardware_test.go`, 3 new runner tests in `runner_test.go`
   (`TestRunnerHardwareQSVOnNodeWithoutQSVFailsNamingStep`,
   `TestRunnerHardwareSoftwareOnQSVNodeUsesLibx265`,
   `TestRunnerHardwareExplicitQSVNeverFallsBackToSoftware`). Auto is covered by the unchanged
   `TestRunnerPassesScaleAndRateToEncode` and `TestRunnerFallbackReasonsRecorded`.
4. Pre-existing: `gofmt -d internal/worker/runner.go` reports a struct-field alignment in the
   `runner` struct (`runFn` and `plugs` comments). It was there before this task and was left alone.
5. Next task (58) should know: `Effective.Hardware` and `Effective.HardwareStep` now exist, and
   `registryFor(target, hardware, step)` returns an error. The `raw_args` work does not need to touch
   either. `flowModel.ts` still needs the UI for `hardware` (task 61) and must not assume a
   per step encoder reaches `Target`.
