# Task 24 notes: health check and move, as first-class steps

## Plan (files, in order)

1. `internal/flow/graph.go` — `StepHealthCheck`, `StepMove` constants; `healthCheckParams`,
   `moveParams`; `Validate` cases (defaults are legal zeros, out-of-range rejected;
   both steps shaping = at most one each; destination required + absolute after
   trimming trailing slashes).
2. `internal/flow/plan.go` — exported `HealthCheckSpec`; `Plan.HealthCheck/MoveTo/MoveKeepTree`,
   `Effective.HealthCheck/MoveTo/MoveKeepTree`; `Compile`/`ResolvePlan` carry them;
   `Describe` lines; `StepTypes` entries (`Verify the result` + move) with param schemas.
   `ParamField.Min/Max` become `*float64` (JSON unchanged for whole numbers) so the
   tolerance bound 0..0.5 reaches the editor.
3. `internal/server/dto.go` — `JobSpec` gains `move_to`, `move_keep_tree`, `move_rel_dir`.
4. `internal/server/libraryflow.go` — `EffectiveMove(lib)`, save-time check in `UpdateFlow`:
   a move destination must not be the library root or inside it (moved files inside the
   library would be re-scanned forever).
5. `internal/server/resultpolicy.go` — `MoveDirective{Destination, KeepTree, RelDir}` +
   `RelDirUnder(root, path)`; `ApplyResultPolicy` gains the move parameter: destination
   = moveTo (+ rel dir when keep_tree), mkdir 0o755, never clobber, rename with
   copy-fallback, verify size, then remove the original (replace policies only).
6. `internal/api/worker.go` / `internal/api/transfer.go` — spec builder compiles the plan
   (destination mapped to the node view like the source; rel dir computed server-side);
   transfer verify-and-replace passes the move directive.
7. `internal/worker/healthcheck.go` (new) + `internal/worker/runner.go` — resolve
   `eff.HealthCheck` from the flow; after the output verifies and before anything is
   reported: stream check (video/audio the source had must survive), duration check
   (tolerance from the step), then `ffmpeg -v error -xerror -nostdin -i <out> -f null -`
   (with `-ss`/`-t` for the sample window from the middle). A failed check fails the job
   before the result policy; the spec's move fields go into `ApplyResultPolicy`.
8. Tests: `internal/flow/graph_test.go`, `internal/flow/plan_test.go` (new),
   `internal/server/resultpolicy_test.go` (move cases; call sites gain the zero
   directive), `internal/worker/runner_test.go` (unit: short duration, missing stream,
   pass path), `internal/worker/integration_test.go` (real fixture: corrupt output fails
   the decode check, healthy output passes, full-stack move), `internal/api/flow_test.go`
   (step-types + PUT round trip + destination-inside-library rejection).
9. Web: schema-driven palette/editors pick the steps up; `web/src/StepStack.test.tsx`
   covers rendering + editing both steps; mobile rules untouched.
10. Docs: ARCHITECTURE.md step-type list, API.md task-05 spec fields, STATUS.md.

## Decisions made inside the task's seams (recorded, none block)

- **Guards on `health_check`/`move`**: the task routes the move through the compiled plan
  (guards ignored, it always runs) and the health check through the per-file resolution
  (a guard that fails skips the check for that file). Both are legal graphs; guarding
  either is not useful and is not special-cased.
- **`health_check` is shaping** (at most one per graph): it always runs last, so a second
  one could only ever conflict; `Plan.HealthCheck` is a single pointer.
- **Stream check excludes subtitles**: the subtitle policy can legitimately remove them
  (`remove` mode); video and audio cannot be dropped by any policy, so their absence in
  the output means a broken encode. Literal "any stream type" would make
  health_check + remove-subtitles always fail.
- **Truncated MKV decodes cleanly** (verified empirically: ffprobe still reports the full
  duration and `ffmpeg -xerror -f null -` exits 0 with only a warning), so the runner
  integration test corrupts the output body (same size) to make the decode check fail
  deterministically; the duration rule catches real truncation when the probe reflects it.
  Even corruption is build-dependent: the local ffmpeg exits 183 on the corrupted body
  while CI's apt build decoded it fine, so the integration test scripts the decode leg's
  failure (like the existing corrupt-output test) and proves the real decode pass in the
  healthy-path test instead.
- **`JobSpec.move_rel_dir`** (not in the task's two fields): the keep-tree relative path is
  computed server-side at spec build time (`rel(file, libraryRoot)`), view-agnostic, so the
  worker never needs the library root and the destination is mapped to the node view like
  the source. `Plan.MoveTo` in the spec is the node-side mapping.
- **Move + rename template**: unchanged semantics — the template names the keep-both
  output, replace keeps the original name; the destination only names the directory.
- **Move + keep_both**: the produced file moves to the destination, the original stays
  (keep-both semantics); extracts follow to the destination.
- **File rows after a move**: the completed file row takes the output path (outside the
  library), so the next scan marks the library row `missing`. That is honest — the file
  left the library on purpose — and matches what a replace with an extension change
  already does to its old row.

## Summary (written at the end)

Built: `health_check` (probe+duration+decode verification, always last) and `move`
(destination + keep_tree, applied inside the result policy, never clobbering) as
validated flow steps with palette entries, plain-language lines, spec plumbing for both
access modes, and tests at every layer (flow, result policy, runner unit, runner
integration, API round trip, web editor). `ParamField.Min/Max` became `*float64` (JSON
unchanged) so the tolerance bound reaches the editor, and `ApplyResultPolicy` takes a
trailing `MoveDirective`.
Skipped (out of scope): repairing failed checks, per-step retries, moving between
machines, any DECISIONS.md change.
For the next task: `JobSpec` gained `move_to` / `move_keep_tree` / `move_rel_dir`; a
`health_check` decode failure message carries the ffmpeg tail excerpt; a truncated MKV
still decodes cleanly per ffmpeg, so "broken output" tests corrupt the body instead.
