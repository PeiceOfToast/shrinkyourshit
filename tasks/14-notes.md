# Task 14 notes — flow schema, easy preview, step stack

## Plan (files, in order)

1. `internal/flow/graph.go` — Graph/Step/Edge types, step-type constants, typed
   param structs, `Validate(Graph) []FieldError` (unique ids, edge endpoints,
   acyclic single chain, exactly one entry, entry is `probe`, reachable nodes,
   per-type params, at most one step per shaping type).
2. `internal/flow/plan.go` — `Compile(Graph) (Plan, error)` (ordered steps with
   resolved guard conditions), `ResolvePlan(plan, probe.MediaInfo)` → effective
   Target + skip_if_smaller + rename, `Plan.Target()` (static, condition-free
   target for mismatch checks), `Describe` (plain-language lines),
   `StepTypes()` palette with parameter schemas.
3. `internal/flow/targetflow.go` — `TargetToFlow(Target) Graph` (the compiler
   that keeps easy mode and target-only saves speaking the flow format).
4. Migration `0007_library_flow.sql` (flow_json, flow_source columns) +
   store.Library fields + crud plumbing.
5. Server: LibraryParams carries flow; `build` regenerates the flow from the
   compiled target; `UpdateFlow` (PUT /flow) validates and saves + reevaluates;
   startup backfill for legacy rows; `EffectiveTarget(lib, info)` used by the
   scanner and re-evaluation; mismatch logging (flow vs target, warn);
   transfer outputExt/policy from the flow.
6. API: `GET/PUT /api/v1/libraries/{id}/flow`, `POST /api/v1/flows/validate`,
   `GET /api/v1/flows/step-types`, `POST /api/v1/flows/preview` (added per
   AGENTS rule 4; keeps easy-mode compilation server-side so the "what this
   does" panel is live without duplicating the compiler in TS). JobSpec gains
   `flow` + `flow_source`.
7. Worker: runner compiles `spec.Flow` and resolves it against the probed
   source; the resolved target/policy/rename drive the encode. Legacy specs
   without a flow fall back to spec.Target (defensive only; the server always
   sends a flow now).
8. Parity test `internal/encoders/parity_test.go` (unit, no ffmpeg): for the
   easy-mode table, flow path args == target path args for cpu + nvenc + qsv.
9. Web: flowModel.ts, FlowPreviewPanel (easy "what this does"), RulesPage with
   tier selector + StepStackEditor, api.ts additions, routes/links, tests.
10. Docs: API.md (task 14 endpoints + JobSpec flow fields), ARCHITECTURE.md
    (graph schema, step types, rename/skip semantics).

## Design decisions (recorded here because they fill gaps in the docs)

- Conditions live in `if_*` steps (params), not in `Edge.When` — `When` stays a
  reserved string field for the task-15 canvas; compile ignores it.
- A step gated only by `if_codec_not` maps to `reencode_if_same_codec=false`
  (pass implies a different codec); unguarded or gated by anything else maps to
  `true` (the step must really run). This is what makes the parity gate exact.
- `skip_if_smaller` ⇒ the job's effective result policy is replace_if_smaller
  (flow wins over the library column). `rename {template}` names the produced
  output for keep-both results (tokens {name}, {ext}); with replace policies it
  cannot rename over the original, so it is logged and ignored.
- Legacy libraries: startup backfill compiles + persists flows; the spec
  builder also compiles lazily so a job claimed mid-upgrade still works
  (TestLegacyTargetStillRuns).

## Summary

(5-line summary at the end of the run.)
