# Task 58: The validated raw arguments escape hatch

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec section 5.3 in full. `internal/flow/plan.go` (`ApplyStep`, `Describe`), `internal/config`
(settings shape), `internal/server/handlers.go` (how a setting is read and validated).

## Scope

Create: `internal/flow/rawargs.go`, `internal/flow/rawargs_test.go`.
Modify: `internal/flow/graph.go`, `internal/flow/plan.go`, `internal/config/*.go`,
`internal/server/handlers.go`, `internal/server/api.go`, the settings DTO, and the settings tests.

## Do

1. Add the `raw_args` step: `stage` (`input` or `output`), `args` (string), `note` (string).
2. Tokenise with a shell quoting parser only, never a shell. Write the tokeniser by hand or use a
   package already in `go.mod`; do not add a dependency for it without recording the reason in the
   notes.
3. Reject, each with its own test and a `FieldError`: any attempt to set an input or output path
   (`-i`, a bare output positional), `-f` pointing at a device, shell metacharacters (`;`, `|`,
   `&`, backtick, `$(`), newlines, and an unbalanced quote.
4. Add setting `allow_raw_args` (bool, default true). When false, a graph containing the step fails
   validation with a clear message, and the API returns 400 rather than dropping the step.
5. `Describe` prints the raw string verbatim, prefixed so an audit reads it as unsafe.
6. Extend `StepTypes()` with an `unsafe` marker field on `StepTypeInfo` so the UI can style it,
   and set it only on this step.

## Out of scope

No UI (61). Do not implement process isolation or a sandbox: the argument vector never reaches a
shell, and that is the whole guarantee.

## Acceptance

- [ ] Tokeniser tests cover quoted arguments, escapes and an unbalanced quote.
- [ ] Every rejected pattern has a test that produces a `FieldError` with the field path.
- [ ] A well formed `raw_args` step appears in the compiled plan at the right stage and in
      `Describe` verbatim.
- [ ] `allow_raw_args: false` makes a graph containing the step fail with 400, proven by an API
      test.
- [ ] `make test` and `make lint` green.

## Notes

Write the plan, the denylist and the reasons for each entry in `tasks/58-notes.md`.
