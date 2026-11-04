# Task 46: Flow page

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`FlowCanvasPage.tsx`, `FlowCanvas.tsx`, `FlowInspector.tsx`, `FlowPalette.tsx`, `index.css` (React Flow vars if not done in 36), and tests.

## Acceptance

- [x] Shares the Rules PageHeader and SegmentedControl.
- [x] One "Edit flow".
- [x] Canvas background equals canvas `rgb(2,6,23)` (Playwright: computed `background-color` of `.react-flow`).
- [x] No step-id text on node faces (FlowCanvas test: `canvas-node-probe` does not contain "probe" as a separate element); the inspector shows the id.
- [x] fitView `minZoom` 0.85.
- [x] Minimap only at 10 or more nodes.
- [x] The mobile notice is a Banner.
- [x] `e2e/mobile.spec.ts` flow leg still green.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/46-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/46-flow-page` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/46-notes.md`.
