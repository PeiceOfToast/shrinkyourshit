# Task 56: Condition steps for resolution, audio channels and HDR

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec section 5.2 (the condition rows), and `internal/flow/plan.go` `Condition.pass` plus the
existing `if_bitrate_above` implementation as the pattern to copy.

## Scope

`internal/flow/graph.go`, `internal/flow/plan.go`, `internal/flow/plan_test.go`,
`internal/flow/graph_test.go`, `web/src/api.ts` (types only).

## Do

1. Add `if_resolution_above` (`height`, number, 480 to 4320), `if_audio_channels_above`
   (`channels`, number, 1 to 8) and `if_hdr` (`is_hdr`, toggle) as condition step types, with
   typed param structs, validation and `StepTypes()` entries marked `Condition: true`.
2. Teach `Condition.pass` to evaluate them from `probe.MediaInfo`, reading the fields the probe
   already reports. If a field is genuinely absent from `MediaInfo`, add it to the probe and say
   so in the task notes rather than guessing a value.
3. `Describe` renders each as a plain language clause, consistent with the existing ones.
4. Cover the tree case Moderate needs: a condition followed by an indented child action must
   compile and evaluate correctly, including the case where the condition is false and the child
   is skipped.

## Out of scope

No UI. No changes to how conditions are stored.

## Acceptance

- [ ] Each condition has a passing and a failing fixture test.
- [ ] A condition with a child action compiles to the same plan shape the existing
      `if_bitrate_above` produces, proven by a structural test.
- [ ] `StepTypes()` exposes all three with help text a non expert can act on.
- [ ] `make test` green.

## Notes

Write the plan and a 5 line summary in `tasks/56-notes.md`.
