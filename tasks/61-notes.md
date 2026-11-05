# Task 61 notes: advanced palette and the raw args node

## Plan (files, in order)

1. `web/src/api.ts`: `StepTypeInfo.unsafe?: boolean` (the server sends it since task 58; the TS type never got it).
2. `web/src/flowCanvasModel.ts`: `unsafeSteps(graph, palette)`, the pure helper task 62 uses to list
   the steps that pass unmodelled arguments.
3. `web/src/FlowCanvas.tsx`: the step node takes `unsafe` and `note` from the palette and the step; the
   raw_args node gets a warning ring, an inline warning line, and its `note` on the face. Server field
   errors already land on the node (checked: `mapNodeErrors`), kept and tested.
4. `web/src/FlowInspector.tsx`: inline warning for an unsafe step, above its parameters.
5. Tests: `FlowCanvas.test.tsx` (palette iteration, warning text, note visible),
   `FlowCanvasView.test.tsx` (warning styling and error on node, direct render), `flowCanvasModel.test.ts`
   (`unsafeSteps`).
6. `web/e2e/flow.spec.ts`: seeded library, add a raw_args node in the UI, connect it, fill args, save,
   reload, warning shows.

## Interpretations (the task text is loose in three places)

- "Draggable": the palette is click (or keyboard) to add, which the spec allows (drag is never the only
  path). No drag-and-drop is built. The test asserts every palette type can be added.
- "Palette driven from StepTypes()": already true (`FlowCanvasPage` passes the fetched list through). No
  change to `FlowPalette.tsx`, which is outside this task's scope.
- "The flow page lists every library flow containing a raw_args step when the library detail page asks":
  this task adds the pure helper (`unsafeSteps`) that the library detail listing uses. The library detail
  page is not in scope, so the wiring is task 62's.

## Changes made

- `web/src/api.ts`: `StepTypeInfo.unsafe?: boolean`.
- `web/src/flowCanvasModel.ts`: `unsafeSteps(graph, palette)`.
- `web/src/FlowCanvas.tsx`: a raw_args node gets `ring-warning`, `data-unsafe="true"`, the line
  "Passes arguments the app does not model." (`canvas-node-warning-<id>`), and its note on the face
  (`canvas-node-note-<id>`). Its `note` chip is dropped so the note shows once. Server field errors were
  already mapped to nodes (`mapNodeErrors`); a test now covers the raw_args case.
- `web/src/FlowInspector.tsx`: the same warning above the parameters of a selected unsafe step.
- Tests: `FlowCanvas.test.tsx` (every palette type can be added, using a fixture palette with `scale`,
  `if_resolution_above` and the server's `raw_args`; node warning and note; inspector warning; server error on
  the node), `flowCanvasModel.test.ts` (`unsafeSteps`), `web/e2e/flow.spec.ts` (add, connect, fill, save,
  reload, warning again).

## Checks

- `npm run build` (tsc and vite): pass.
- `make test`: pass. Web 64 files, 470 tests (was 464).
- `make lint`: exit 0 (go vet, golangci-lint 0 issues, SPDX, eslint with 0 warnings, prettier).
- `npx playwright test` (all projects, port 8791, after `make build`): 112 passed, 7 skipped. Skips: 6 are the
  pre-existing `file table responsive` cases (2 per project), 1 is the new raw_args test on phone (editing is
  desktop only, the page is view only below md). The raw_args test passes on tablet and desktop.

## Found and fixed during the run

- The first Playwright run failed: the node lost its unsafe marker after reload. Cause: the Go binary embeds
  `internal/webui/dist`, and `go build` alone does not refresh it. `make build` copies the web bundle. Not a
  code bug. Use `make build` before the e2e run.

## Open questions

- None blocking. Interpretations are listed above under "Interpretations".

## Summary

1. Built: `unsafe` on the TS step type, the raw_args node with warning, note and server error, the inspector
   warning, `unsafeSteps` for task 62, and tests for each acceptance item.
2. Skipped: a drag-and-drop palette (the palette has always been click or keyboard add; the spec allows that).
   The library detail listing of raw_args flows (wiring belongs to task 62; the helper is ready).
3. Next task (62) should know: `unsafeSteps(graph, palette)` in `flowCanvasModel.ts` lists the unsafe steps.
   The flow page does not render a library-level summary.
4. The e2e test needs `make build` first (embedded dist).
5. Branch `task/61-advanced-palette`, STATUS left `in_progress`, no PR opened (the supervisor opens it).
