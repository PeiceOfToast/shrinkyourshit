# Task 14: Flow schema, easy mode preview and step stack editor

## Goal
One canonical flow graph that easy mode, the step stack and the future canvas all speak, plus a visible step stack editor. `DECISIONS.md` item 6 already requires all three tiers on one engine; this task makes the storage and API match that decision and ships tier 2.

## Read first
UI review at `docs/ui-review-2026-10-29.md` section 3. ARCHITECTURE.md (Target schema, flow schema). DECISIONS.md items 6 and B3. Depends on tasks 02, 04, 10.

## Do
- `internal/flow`: add the graph types.
  - `Graph{Schema int, Nodes []Step, Edges []Edge}`, `Step{ID, Type, Params json.RawMessage}`, `Edge{From, To, When}`.
  - `Validate(Graph) []FieldError` with node references, acyclicity, one entry point, and per type parameter validation.
  - `Compile(Graph) (Plan, error)` producing the ordered action plan the worker already knows how to run.
  - Step types for v1: `probe`, `if_codec_not`, `if_bitrate_above`, `transcode_video`, `remux_container`, `convert_audio`, `subtitles`, `rename`, `skip_if_smaller`.
- Store: libraries get `flow_json` alongside the existing `target` column. Easy mode keeps writing `target`, and a compiler generates the equivalent flow so both are always present. No dual source of truth: `flow` wins when present, and a mismatch is logged.
- Parity test: for a table of easy mode inputs, `Compile(EasyToFlow(target))` produces the same command arguments the current command builder produces. This is the gate that proves the flow engine is not a regression.
- API (document in API.md):
  - `GET /api/v1/libraries/{id}/flow`
  - `PUT /api/v1/libraries/{id}/flow` with `source` of `easy`, `steps` or `advanced`
  - `POST /api/v1/flows/validate` returning field errors without saving
  - `GET /api/v1/flows/step-types` returning the palette with parameter schemas, so the UI never hardcodes the step list twice
- Easy mode: an expandable "what this does" panel that renders the generated flow in plain language, one line per step. This is the teaching surface for the step stack.
- Step stack editor in the UI at `/libraries/:id/rules`:
  - Ordered rows, drag to reorder, add and remove, each row showing its condition in muted text and its parameters as chips.
  - Save goes through validate first and shows field errors on the row that failed.
  - Tier selector at the top: Easy, Steps, Advanced. Switching from Steps to Advanced is allowed and converts; going backwards warns that manual steps will be lost.
- Worker: consume `Compile` output. Keep the old Target path behind the parity test until it is proven on the integration suite, then delete the duplicate path in the same task.

## Out of scope
The canvas editor (task 15), plugin steps, conditional branches beyond a single linear chain with conditions.

## Acceptance
- [ ] Flow round trip: write a flow via the API, read it back byte identical, and the library reports `source` correctly. (`TestFlowRoundTrip`)
- [ ] `POST /api/v1/flows/validate` returns 200 for a valid graph and 400 with a field error for a cycle, a dangling edge, an unknown step type, and a missing required parameter. (`TestFlowValidate`)
- [ ] Parity: for every row of the easy mode table, the compiled plan matches the current command builder output exactly. (`TestEasyFlowParity`)
- [ ] An existing library created before this task still encodes correctly with no manual migration. (`TestLegacyTargetStillRuns`)
- [ ] The step stack editor reorders, adds, removes and saves, and a validation failure renders on the offending row. (`StepStack.test.tsx`)
- [ ] The easy mode "what this does" panel lists one plain language line per generated step. (`EasyLibraryForm.test.tsx`)
- [ ] API.md and ARCHITECTURE.md updated with the graph schema, the step type list and the new endpoints.
