# Task 24: Health check and move, as first-class steps

## Goal
Two gaps against Tdarr: nothing verifies the file it just produced, and nothing can move the result anywhere. Both become real flow steps with validation, palette entries, editor parameters and tests.

## Read first
`internal/flow/graph.go` (step constants, param structs, `Validate`), `internal/flow/plan.go` (`Compile`, `ResolvePlan`, `Target`, `StepTypes`), `internal/server/dto.go` (`JobSpec`), `internal/server/resultpolicy.go` (`ApplyResultPolicy`, `expandRename`), `internal/worker/runner.go` (the run path, roughly lines 53 to 245), `web/src/StepStackEditor.tsx`, `web/src/flowParams.tsx`, `web/src/FlowPalette.tsx`, `web/src/flowModel.ts`.

Confirmed present today and **not** part of this task: `remux_container`, `convert_audio`, `subtitles` with `ExtractRemoved`, and `rename` templates. Do not rebuild them.

## Do

### 1. `health_check` step

`internal/flow/graph.go`:
- `StepHealthCheck = "health_check"`.
- Params:

```go
// health_check: verify the produced file. "sample" decodes minutes from
// the middle of the file, "full" decodes all of it. duration_tolerance is
// the fraction of the source duration the output must still have.
healthCheckParams struct {
    Mode              string  `json:"mode"`   // "sample" | "full"
    Minutes           int     `json:"minutes"`
    DurationTolerance float64 `json:"duration_tolerance"`
}
```
- Defaults applied in `Validate` when a field is zero: `mode` `sample`, `minutes` 2, `duration_tolerance` 0.02. Reject `mode` outside the two values, `minutes` outside 1 to 120, `duration_tolerance` outside 0 to 0.5.

`internal/flow/plan.go`: `Compile` and `ResolvePlan` carry it as `Plan.HealthCheck *HealthCheckSpec` and `Effective.HealthCheck`. It never affects `Target` (it changes nothing about the encode) and it may appear anywhere in the chain, because it always runs last. `StepTypes()` gains an entry with a label `Verify the result`, a description in the same voice as the neighbouring entries, and the parameter list for the editor.

Verification behaviour, in `internal/worker`:
- After the output file exists and before anything is reported complete: probe the output with the existing prober, and fail the job with a clear message when any stream type present in the source is missing, or when `outputDuration < sourceDuration * (1 - tolerance)`.
- Then decode: `ffmpeg -v error -xerror -nostdin -i <output> -f null -`, with `-ss` and `-t` when the mode is `sample`. Any non-zero exit fails the job. `-xerror` is what makes it a real check rather than a probe.
- A failed health check must not apply the result policy: the source file stays untouched and the job ends `failed` with the ffmpeg tail in its error message, exactly like an encode failure today.

### 2. `move` step

`internal/flow/graph.go`:
```go
StepMove = "move"

// move: after a successful result, put the file under destination,
// keeping its path relative to the library root when keep_tree is set.
moveParams struct {
    Destination string `json:"destination"`
    KeepTree    bool   `json:"keep_tree"`
}
```
`Validate`: exactly one `move` step per graph, `destination` must be absolute and must not be the library root or inside the source file's own directory. Trim trailing slashes.

Plumbing: `JobSpec` in `internal/server/dto.go` gains `MoveTo string \`json:"move_to,omitempty"\`` and `MoveKeepTree bool \`json:"move_keep_tree,omitempty"\``, set from the compiled plan where the library flow is turned into a spec (the same place `rename` comes from today, `internal/server/libraryflow.go`). The worker does not move anything itself; it reports the output and the server applies the policy, which is where the move belongs:

`internal/server/resultpolicy.go`, inside `ApplyResultPolicy`, when `moveTo` is set:
- Build the destination as `moveTo` plus, when `keep_tree` is set, the path of the original relative to the library root.
- Create the destination directory with `0o755` if missing.
- **Never clobber**: if the destination exists, fail with a message naming both paths, leave the output where it is and leave the original in place.
- Move with `os.Rename`, fall back to copy-then-`fsync`-then-remove across filesystems, and remove the original only after the destination is verified to exist at the expected size.
- `move` composes with a rename template: the template names the file, the destination names the directory.

### 3. Editor surface
- `web/src/flowPalette.tsx`/`FlowPalette.tsx` and `web/src/flowParams.tsx`: palette entries for both steps and parameter editors (a select for mode, number inputs for minutes and tolerance, a text input plus a `Switch` for destination and keep tree).
- `web/src/StepStackEditor.tsx`: render both steps in the stack with their summaries, and keep the mobile rules from task 22 intact.
- `web/src/flowModel.ts`: client-side defaults matching the server.

### 4. Tests
- `internal/flow/graph_test.go`: defaults fill in, out-of-range values produce `FieldError`s with the right paths, a second `move` step is rejected, a relative destination is rejected.
- `internal/flow/plan_test.go`: both steps compile and resolve; `health_check` does not change `Target`.
- `internal/server/resultpolicy_test.go`: move into a new tree keeps the relative path; an existing destination fails without touching either file; a cross-device move (simulate by failing `os.Rename` once) still produces the file and removes the original.
- `internal/worker/runner_test.go`: a truncated output fails a `health_check` job and the source is untouched. Use a short real fixture guarded the way the existing integration tests are.
- `web/src/StepStack.test.tsx`: both new steps render and edit.

## Out of scope
Repairing a failed health check, per-step retry counts, moving *between* machines, any change to `DECISIONS.md`.

## Acceptance
- [ ] `health_check` and `move` validate, appear in `GET /api/v1/flows/step-types`, and round trip through `PUT /api/v1/libraries/{id}/flow`. Proven by an API test.
- [ ] A job whose output is shorter than the tolerance fails with a message naming the durations, and its source file is byte identical afterwards.
- [ ] `move` places the file at the destination, refuses to overwrite, and never loses the original when the move fails. Proven by `resultpolicy_test.go`.
- [ ] Both steps are editable in the step stack and the palette, and the flow canvas still renders read only below `md`.
- [ ] `make test`, `make test-integration` and `make lint` are green.
