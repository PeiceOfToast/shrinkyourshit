# Task 32: A plugin as a flow step

## Goal
Task 31 gave plugins three fixed moments: `preflight`, `postprocess` and `notify`. This task makes a plugin something the flow can place: a fourth manifest type `step`, and a `plugin` flow step whose position in the chain is the user's decision. A step plugin runs on the file the job is working on, inside that job's scratch directory, in the middle of the chain, so a later guard sees what it did. It is on the critical path: if it fails, the job fails and the original file is untouched.

## Read first
`tasks/31-plugins-v1.md`, `internal/plugin/manifest.go`, `internal/plugin/runner.go`, `internal/plugin/install.go`, `internal/store/plugins.go`, `internal/store/migrations/0013_plugins.sql`, `internal/flow/graph.go` (step constants, `validateStepParams`, `Validate`), `internal/flow/plan.go` (`Compile`, `ResolvePlan`, `PlanStep`, `StepTypes`, `describeStep`, `ParseParams`), `internal/server/libraryflow.go`, `internal/server/dto.go` (`JobSpec`), `internal/worker/runner.go` (the step execution path), `internal/api/flows.go` (flow save), `web/src/flowParams.tsx`, `web/src/FlowPalette.tsx`, `web/src/StepStackEditor.tsx`, `web/src/flowModel.ts`, `docs/plugins.md`.

Task 31 must be merged before this starts. If it is not, stop and say so.

## Do

### 1. A fourth plugin type
`internal/plugin/manifest.go`: add `TypeStep = "step"` and include it in `TypeValues()`. A `step` plugin never runs at preflight, postprocess or notify; it runs only where a flow places it, and only when it is enabled. Update the type list in `docs/plugins.md` and `ARCHITECTURE.md` in the same commit.

### 2. The step contract
In `internal/plugin/runner.go`, the step run reuses the task 31 runner with the same captured stdout, stderr and timeout handling, with these additions to the JSON written to the plugin's stdin:

- `"phase": "step"`
- `"file_path"`: absolute path of the working file inside the job's scratch directory. This is what the plugin operates on.
- `"source_path"`: the original library file, unchanged and never handed to a plugin as a writable path.
- `"step_index"`: zero-based position of this step in the executed chain.
- `"settings"`: the merged settings for this step (see 3).

Environment additions: `SYS_PHASE=step` and `SYS_STEP_INDEX=<n>`, alongside the existing `SYS_PLUGIN_NAME` and `SYS_DATA_DIR`.

Result protocol: the plugin may modify `file_path` in place. It may instead print one final JSON object on stdout, `{"file": "<absolute path>"}`, to hand the rest of the chain a different file. Rules:
- The path must resolve inside the job's scratch directory. Anything else is a failure naming both paths. No exceptions, no following symlinks out of scratch.
- A named file that does not exist is a failure with the path in the message.
- With no result line, the existing `file_path` continues down the chain.
- After a successful step, **re-probe** the working file with the existing prober and rebuild the job's view of it before the next step runs, so `if_codec_not`, `if_bitrate_above` and `skip_if_smaller` evaluate against what the plugin actually produced. A plugin that leaves an unreadable file fails the step with the probe error.

Failure and timeout rules, which differ from the notify/postprocess hooks on purpose: a non-zero exit **fails the job**, with the plugin name, the exit code and the last 20 lines of stderr in the job's error message. Exceeding `timeout_seconds` kills the process group and fails the job. The result policy never runs on a failed job, so the original stays byte identical. All plugin output continues to land in the job log with `source: "plugin"` (task 27's viewer).

### 3. Flow layer
`internal/flow/graph.go`:

```go
StepPlugin = "plugin"

// plugin: run an installed step plugin against the working file.
// settings overrides the plugin's stored settings for this step only.
pluginParams struct {
    Plugin          string            `json:"plugin"`            // installed plugin name
    Settings        map[string]string `json:"settings"`          // per-step overrides
    ContinueOnError bool              `json:"continue_on_error"` // default false
}
```

`validateStepParams` rules, each with a `FieldError` whose path is the parameter:
- `plugin` is required and must satisfy the task 31 manifest name rule (reuse `plugin`'s name check rather than writing a second regex).
- `settings` keys must be non-empty and must look like manifest setting keys; values may be empty strings.
- A `plugin` step may not be the entry step unless the graph also contains a `probe` step before it in the chain, because the step contract hands the plugin probe output.
- `continue_on_error` stays false unless set. When true, a failing step logs a warning and the chain continues; `docs/plugins.md` says plainly that this exists for logging and sidecar style steps, not for anything that changes the file.

`internal/flow/plan.go`: carry the plugin step **as a chain step** exactly the way `remux_container` is carried today (a `PlanStep` in order), not as a side spec like `health_check`. `ResolvePlan` merges settings with this precedence: per-step `settings` override, then the plugin's stored settings, then the server environment for `env` names. `StepTypes()` gains an entry with `Kind: "plugin"` for the plugin field (so the editor renders a select of installed, enabled step plugins), a settings editor derived from the manifest, and a `describeStep` line in the same voice as its neighbours, for example `Run plugin remove-chapters`.

Server-side checks that the pure graph validator cannot do, in `internal/api/flows.go` on save and again at job dispatch: the named plugin must be installed and enabled and of type `step`, otherwise a 422 whose message names the plugin and says which of the three failed. A flow that names a plugin somebody later disabled must not silently skip the step; the job fails with that message.

### 4. Job spec and worker
`internal/server/dto.go`: `JobSpec` gains

```go
Plugins []PluginStep `json:"plugins,omitempty"` // one entry per plugin step, in chain order

// PluginStep is a resolved step plugin: the manifest facts the node needs,
// the merged settings, and where it sits in the chain.
type PluginStep struct {
    Name       string            `json:"name"`
    Command    []string          `json:"command"`
    EnvNames   []string          `json:"env_names"`
    TimeoutSec int               `json:"timeout_seconds"`
    Settings   map[string]string `json:"settings"`
    StepIndex  int               `json:"step_index"`
    ContinueOnError bool         `json:"continue_on_error,omitempty"`
}
```

`internal/worker/runner.go` executes each entry at its `step_index` point in the chain, using the node's own `<data dir>/plugins/<name>/` directory. If the directory or its entry point is missing on that node, the job fails with a message naming the plugin **and** the node, for example `plugin remove-chapters is not installed on node nas-01`. Do not try to pull the plugin from the server in v1. A single install with role `both` is the common case and already works; the message tells a distributed user what to do.

### 5. Editor surface
- `web/src/FlowPalette.tsx`: a `Run a plugin` entry, shown only when at least one enabled step plugin exists; otherwise the palette entry explains how to install one and links to the plugins tab.
- `web/src/flowParams.tsx`: a select of enabled step plugins, a settings form generated from the selected manifest (with per-step values overriding the stored ones and a `Use installed value` reset), and a `Continue on error` switch with the warning text from `docs/plugins.md`.
- `web/src/StepStackEditor.tsx`: renders the step with its plugin name and keeps the task 22 mobile rules intact.
- `web/src/flowModel.ts`: client defaults matching the server.

### 6. Docs
`docs/plugins.md` gains a `Step plugins` section: the exact JSON and environment a step plugin receives, the in-place and `{"file": ...}` result options, the scratch-only rule, what happens on failure and timeout, the re-probe behaviour, the `continue_on_error` warning, and a worked example. Use a plugin that writes a sidecar `.nfo` style metadata file next to the working copy and one that strips chapters in place, so both result shapes are shown. Say plainly that a plugin runs as the server or node user with the same access to the media folders.

## Out of scope
A wasm or otherwise sandboxed runtime (task 33), a plugin marketplace or index, shipping plugins from server to node, plugin-to-plugin dependencies, plugin output as a flow condition, any change to `DECISIONS.md`.

## Acceptance
- [x] A manifest with `type: step` installs and is listed as a step plugin; a `preflight` plugin is not offered in the flow editor. Proven by tests in `internal/plugin` and `internal/api`.
- [x] `plugin` steps validate: a missing name, an unknown setting key and a plugin step ahead of `probe` each produce a `FieldError` with the parameter path; a valid step round trips through `PUT /api/v1/libraries/{id}/flow`. Proven by `internal/flow/graph_test.go` and an API test.
- [x] Saving a flow that names a disabled or missing plugin returns 422 naming the plugin and the reason, and dispatching that flow fails the job instead of skipping the step. Proven by an API test and a worker test.
- [x] A step plugin that rewrites the file in place is visible to a later guard: a job whose `skip_if_smaller` decision changes because of the plugin takes the new branch. Proven by an integration test.
- [x] A step plugin that exits non-zero fails the job with its name and exit code, and the source file is byte identical afterwards. Proven by a worker test.
- [x] A step plugin that prints `{"file": "/etc/passwd"}` fails with a message naming the path, and a step plugin that sleeps past its timeout is killed and fails the job. Proven by tests.
- [x] A plugin step naming a file that is not on the node fails with a message naming the plugin and the node.
- [x] The palette, step stack and parameter editor handle a plugin step, including the mobile layout rules from task 22. Proven by `web` tests.
- [x] `docs/plugins.md` documents step plugins, and `make test`, `make test-integration` and `make lint` are green.
