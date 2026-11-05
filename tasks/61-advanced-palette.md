# Task 61: Advanced palette and the raw args node

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec section 7 (Advanced). `web/src/FlowCanvas.tsx`, `web/src/FlowInspector.tsx`,
`web/src/flowCanvasModel.ts`, `web/src/FlowCanvasPage.tsx`, and the `StepTypeInfo.unsafe` marker
added in task 58.

## Scope

`web/src/FlowCanvas.tsx`, `web/src/FlowInspector.tsx`, `web/src/flowCanvasModel.ts`,
`web/src/FlowCanvasPage.tsx`, their test files, `web/e2e/flow.spec.ts`.

## Do

1. Drive the node palette from `StepTypes()` so every step type is draggable, including the
   condition steps and `scale`.
2. Render the `raw_args` node distinctly: warning colour from the existing tokens, an inline
   warning that it passes arguments the app does not model, and its `note` field visible on the
   node itself so an audit does not require opening the inspector.
3. Show server `FieldError`s on the offending node.
4. The flow page lists every library flow containing a `raw_args` step when the library detail
   page asks, which is the hook task 62 uses.

## Out of scope

No tier switching. No changes to how the graph is stored.

## Acceptance

- [x] A test iterates the fetched palette and asserts every type can be added to the canvas.
- [x] The `raw_args` node is visually distinct and its warning text is asserted.
- [x] Playwright test adds one `raw_args` node, saves, reloads and shows the warning again.
- [x] `npm run build` and `npx playwright test` green.

## Notes

Write the plan and any changed assertion in `tasks/61-notes.md`.
