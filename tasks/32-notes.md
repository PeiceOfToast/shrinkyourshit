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

1. **The settings value shape is strings.** A step plugin's settings editor edits plain text inputs; the manifest's stored values are also strings, so per-step overrides need no type conversion. Confirmed with task 31: `Settings []Setting` values are `string` (`values` on the plugin row), so the step's overrides are `map[string]string` and the merge is a shallow copy. `ValueChoices` still feeds the select in the Plugins page, but the flow step edits free text over the same keys.
2. **Continue-on-error UI placement**: the toggle is a second param of the plugin step, and its warning is the param's `help` text, sourced from the docs sentence. A per-step failure with `continue_on_error: true` logs a warning and the chain continues (documented in docs/plugins.md); on the store side no settings migration was needed since the plugin step carries its own values.
3. **Palette options are server-driven.** The `plugin` param's `options` field carries the enabled step-plugin names from the API; `FlowPalette` only hides the entry (replacing it with an explainer linking to the plugins tab) when the list is empty, and the same list feeds the select in `flowParams.tsx`. No client-side filtering beyond what the server already sends, so the client and server can't disagree about which plugins are enabled.
4. **Mobile step rules**: the step stack rows keep the 44px touch controls from task 22; the new per-step settings inputs render inside the expanded row the same way the other parameter inputs do, with the same text-input sizing, so no extra mobile work was needed beyond keeping the existing classes.
5. **Docs merge point**: `docs/plugins.md`'s new "Step plugins" section sits between the manifest reference and the hook documentation; `ARCHITECTURE.md` gained the step type in the plugin type list and the `PluginStep` shape in the job spec section. The editor's in-page copy stays plain-language: no codec or FFmpeg terms appear in the plugin step's labels.

## Summary (5 lines)

- Built: a fourth plugin type `step`; the runner step contract (`file_path`, `source_path`, `step_index`, `settings`, phase `step` env) with scratch-only result validation and re-probe; a `plugin` flow step (name required, settings overrides, `continue_on_error`, cannot precede `probe`) validated at save and dispatch with 422s naming the plugin; the worker walks chain steps and runs each plugin step at its position, failing the job on non-zero exit or timeout, with the source untouched.
- Skipped: shipping plugins from server to node (explicitly out of scope), a wasm/sandboxed runtime (task 33), and any marketplace idea.
- Next task should know: `flow.ApplyStep`/`FreshEffective` are the worker's per-step primitives; `plugin.Runner.RunStep` is the single entry for step runs; the step-types response carries the enabled step plugins in the plugin param's `options`, so future step types can follow the same pattern.
- Open follow-up (not a task): the `plugin` param's options list only refreshes with the step-types fetch (`staleTime: Infinity`) — a freshly installed plugin needs a reload or a query invalidation.
- Gates above are the interpretation decisions; everything else followed the task file and the reference docs.
