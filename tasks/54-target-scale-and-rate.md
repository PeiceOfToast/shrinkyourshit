# Task 54: Target extension, scale step, richer transcode_video

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec sections 3, 4, 5.1 and 5.2. `internal/flow/target.go`, `internal/flow/graph.go`,
`internal/flow/plan.go` (`StepTypes`, `ApplyStep`, `Plan.Target`, `Describe`).

## Scope

`internal/flow/target.go`, `internal/flow/graph.go`, `internal/flow/plan.go`,
`internal/flow/target_test.go`, `internal/flow/graph_test.go`, `internal/flow/plan_test.go`,
`web/src/api.ts` (types only), `ARCHITECTURE.md` (the Target JSON section).

## Do

1. Add `ScaleTarget`, `RateTarget` and `Preset` to `VideoTarget` exactly as spec 5.1 draws them,
   with the defaults documented in the comments. JSON tags are `omitempty` so a stored old graph
   round-trips byte identical.
2. Add the `scale` step type: `scaleHeight`, `scaleOnlyIfLarger` (default true),
   `scaleAllowUpscale` (default false), validated in the same style as the existing typed param
   structs, with a `FieldError` for a height outside 2160/1440/1080/720/480.
3. Extend `transcode_video` params with `rate_mode` (quality/crf/bitrate), `crf` (0 to 51),
   `bitrate_kbps`, `max_kbps`, `preset` (veryfast to veryslow), and `hardware`
   (auto/qsv/vaapi/videotoolbox/software). Every one of them optional, so an existing two field
   graph still validates. Cross field rule: mode crf requires a non zero crf, mode bitrate
   requires a non zero bitrate, and a value set outside its mode is a `FieldError` rather than
   silently ignored.
4. `Effective` carries scale, rate and preset through `ApplyStep`, and `Plan.Target()` fills the
   new `Target` fields so easy, step and canvas flows all report the same thing.
5. `Describe` renders the new fields in plain language, one line per step, matching the tone of
   the existing descriptions.
6. Add both new entries to `StepTypes()` with labels, help text and defaults, because the whole
   UI palette is generated from there.
7. Mirror the new types in `web/src/api.ts` (`Target`, `FlowGraph` step params). Types only, no
   component changes in this task.
8. Update the Target JSON section of `ARCHITECTURE.md`.

## Out of scope

No encoder argument changes (task 55). No new condition steps (56). No UI.

## Acceptance

- [ ] A graph saved by the current release still parses, validates and produces the identical
      `Target` and `Describe` output. Prove it with a golden test built from a v1.9 graph.
- [ ] `scale` rejects an unsupported height with a `FieldError` naming the field path.
- [ ] Each of the seven new `transcode_video` cross field rules has a failing test and passes.
- [ ] `StepTypes()` lists `scale` and the widened `transcode_video` with every option.
- [ ] `go test ./internal/flow/...` passes and `make test` is green.
- [ ] `ARCHITECTURE.md` documents the new Target fields.

## Notes

Write the plan and a 5 line summary in `tasks/54-notes.md`.
