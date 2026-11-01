# Task 32 notes: a plugin as a flow step

## Plan (files, in order)

1. `internal/plugin/manifest.go` — `TypeStep`, `TypeValues()`, exported `ValidName`/`ValidEnvName` (the flow reuses them instead of a second regex).
2. `internal/store/plugins.go` — `PluginTypeStep` in the store's value list (write validation).
3. `internal/plugin/runner.go` — `RunStep`: phase `step`, stdin JSON additions (`file_path`, `source_path`, `step_index`, `settings`), `SYS_PHASE=step` + `SYS_STEP_INDEX`, final `{"file": ...}` result parsing, scratch-only + exists validation. `JobContext` gains the three fields (omitempty so hooks are unchanged).
4. `internal/flow/graph.go` — `StepPlugin`, `pluginParams`, `validateStepParams` (name required + manifest name rule; settings keys env-name shaped; position rule). Position check in `Validate` after the chain walk (needs the chain; the parameter-path errors stay per node).
5. `internal/flow/plan.go` — plugin rides the plan as a chain step (already does via the default branch); `ParseParams` case; `ApplyStep` + `FreshEffective` split out of `ResolvePlan` so the worker can walk step by step and re-probe between plugin steps; `describeStep` line; `StepTypes()` entry with a `kind: "plugin"` param (options filled by the API handler).
6. `internal/server/dto.go` — `JobSpec.Plugins []PluginStep` + the `PluginStep` struct.
7. `internal/server/plugins.go` — `CheckGraph` (save-time 422 check: installed, enabled, type step) wired into `LibraryManager.UpdateFlow` via a setter; `ResolveSteps` (dispatch-time resolution + merge of per-step settings over stored values). New `ErrPluginStep` in `libraryflow.go` → API 422 `plugin_unavailable`.
8. `internal/api/flows.go` — step-types fills the plugin param options with enabled step plugins; `putLibraryFlow` maps `ErrPluginStep` → 422.
9. `internal/api/worker.go` — `workerJobSpec` resolves plugin steps; on failure fails the job server-side (Queue.Fail) and answers 422 so the worker drops it.
10. `internal/worker/` — Config gains `DataDir` + `NodeName`; the runner's run becomes a step walk (`flow.ApplyStep` per plan step, conditions against the current probe), plugin steps run via `plugin.Runner.RunStep` on the node's `<data dir>/plugins/<name>/`, re-probe after each, `continue_on_error` warn-and-continue, source-plugin log lines through the task 27 sink. `cmd/shrinkyourshit/main.go` wires the two new config fields.
11. Web: `api.ts` (PluginInfo + kind), `flowModel.ts` (defaults/chips), `flowParams.tsx` (plugin select + manifest-derived settings editor + continue-on-error switch), `FlowPalette.tsx` (entry only when an enabled step plugin exists, else explainer + link), `StepStackEditor.tsx` (render the step, keep task 22 rules).
12. Docs: `docs/plugins.md` (Step plugins section), `ARCHITECTURE.md` (type list, flow schema, plugins section), `API.md` (422, JobSpec.Plugins), `docs/openapi.yaml` (422 response on PUT flow).

## Interpretation notes (owner can override)

- **Plugin steps run pre-encode.** The v1 encode is one combined ffmpeg command built from the whole resolved Target, so it cannot split around a mid-chain plugin. A plugin step therefore runs at its chain position *before* the encode, on the working copy in scratch, and every guard after it evaluates against the re-probed result. This satisfies "a later guard sees what it did" and the skip_if_smaller acceptance (a plugin that shrinks the working copy changes the output size, so the replace decision changes). Position relative to shaping steps is still the user's decision for guard visibility and multi-plugin order; a plugin that must act on the finished output stays a `postprocess`/`notify` plugin in v1. Documented in docs/plugins.md.
- **Position-error path** is `nodes[i].type` (not a params field): the problem is the step's position, not a parameter value; the UI attaches both shapes to the row the same way.
- **Required setting with no value on a step run** fails the step (skipped ≠ ran) — on the critical path a skipped step must not silently continue; `continue_on_error` still applies. Not-installed-on-node failures fail the job even with `continue_on_error` (an installation error, not a plugin verdict).
- **step_index** is the plugin's index in the compiled chain (`plan.Steps`, probe included), matching what the worker walks.
- **Env fallback for step plugins** is the environment of the process running the plugin (the node for remote runs) after settings — the task says "server environment"; on the built-in node those are the same process.

## Gates

(to be filled at the end)
