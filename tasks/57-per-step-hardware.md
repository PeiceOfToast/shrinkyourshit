# Task 57: Per step hardware selection with the global policy as default

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec section 5.2 (`hardware` param) and section 4 item 4. `internal/encoders/detect.go`,
`internal/worker/runner.go` (how the encoder is chosen today), `internal/flow/plan.go`
(`Effective`), and the existing QSV policy in the encode settings.

## Scope

`internal/flow/plan.go`, `internal/flow/graph.go` (param declaration only if 54 missed it),
`internal/worker/runner.go`, `internal/worker/runner_test.go`, `internal/encoders/detect.go`,
`internal/encoders/detect_test.go`, `internal/server/handlers.go` if the setting is read there.

## Do

1. Carry `hardware` in `Effective` and resolve it in the runner: `auto` means the global policy,
   which is today's behaviour and must stay the default for every existing graph.
2. An explicit choice that the node cannot provide fails that job with an actionable error naming
   the step and the missing encoder. It must never silently fall back to software, because that
   turns a 10 minute job into a 6 hour job without telling anyone.
3. Record the chosen encoder in the job log line that already reports encoders, so a per step
   choice is auditable after the fact.
4. Tests use the existing `fakeGPU` pattern in `runner_test.go`.

## Out of scope

No UI for the field (task 61 and 62).

## Acceptance

- [x] A graph with `hardware: auto` behaves identically to today, proven by an existing test that
      does not change. Evidence: `TestRunnerPassesScaleAndRateToEncode` and `TestRunnerFallbackReasonsRecorded`
      are unchanged (`git diff` on `runner_test.go` only adds lines) and pass in `go test ./internal/worker/`.
- [x] `hardware: qsv` on a node with no QSV fails the job with an error naming the step. Evidence:
      `TestRunnerHardwareQSVOnNodeWithoutQSVFailsNamingStep` passes (error contains `step encode` and `qsv`, zero encodes ran).
- [x] `hardware: software` on a node that has QSV uses libx265 and the log says so. Evidence:
      `TestRunnerHardwareSoftwareOnQSVNodeUsesLibx265` passes (args contain `libx265`, log contains
      `encoder chosen: cpu (step encode set hardware to software)`).
- [x] `make test` green. Evidence: `make test` exit 0; 62 web files and 444 tests passed; 13 Go packages `ok`.

## Notes

Write the plan and a 5 line summary in `tasks/57-notes.md`.
