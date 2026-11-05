# Task 60: The Moderate decision tree editor

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec section 7 (Moderate) and the UI contract in `docs/design/ui-redesign-2026-11-04.md` for
tokens, primitives and mobile rules. `web/src/FlowCanvasPage.tsx` for how a graph page loads and
saves, `web/src/FlowInspector.tsx` for how `StepTypes()` drives fields, `web/src/api.ts`.

## Scope

Create: `web/src/FlowTreePage.tsx`, `web/src/FlowTreePage.test.tsx`, `web/src/treeModel.ts`,
`web/src/treeModel.test.ts`, `web/e2e/tree.spec.ts`.
Modify: `web/src/App.tsx` (route `/libraries/:id/tree`), `web/src/api.ts` (helpers only),
`web/src/LibraryDetailPage.tsx` or equivalent to link the tree editor.

## Do

1. Render the graph as ordered rows. A condition row owns its children, indented under it. An
   action row is a leaf. Build the tree from `edges` the way the canvas does, and put that
   transformation in `treeModel.ts` with unit tests, not inline in the component.
2. Add and remove rows, and reorder with both a drag handle and keyboard controls. 44 px targets.
3. Render fields from `StepTypes()` only. No step name, option or label may be hard coded in this
   component, so a new step type appears here without a second change.
4. Show server `FieldError`s on the row that owns the field, and never guess client side.
5. Save writes the whole graph back through the existing flow endpoint, with a toast on success
   and the server error on failure.
6. Works at 390 px: rows stack, the indentation cap is visible, and nothing scrolls sideways.

## Out of scope

No canvas changes (61). No tier switching UI (62).

## Acceptance

- [x] `treeModel` unit tests cover a flat chain, a condition with two children, and a nested
      condition.
- [x] Playwright test loads a library with a known graph, adds a condition and a child, saves,
      reloads and shows the same tree.
- [x] The step palette in this page adds every `StepTypes()` entry, proven by a test that iterates
      the fetched palette rather than a hard coded list.
- [x] `npm run build`, `npx playwright test` and `make lint` green.

## Notes

Write the plan, the list of any changed existing assertion and a 5 line summary in
`tasks/60-notes.md`.
