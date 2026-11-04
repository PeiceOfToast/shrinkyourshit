# Task 39: Control primitives

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

new `Badge.tsx`, `Checkbox.tsx`, `Tabs.tsx`, `SegmentedControl.tsx`, `CopyButton.tsx`, `CodeBlock.tsx` with tests; restyle `Switch.tsx`, `StatusChip.tsx` (on Badge, hiding zero, adding the "Shrunk" derivation via a new optional `bytesSaved` prop), `StatCard.tsx` (`sub`, `tone`, Icon names), `skeleton.tsx`, `ErrorBox.tsx` (`onRetry`), `dialog.tsx` (size plus Header/Body/Footer), `ConfirmDialog.tsx`, `toast.tsx`, `DataList.tsx` (`cardsBelow`, `rowHref`, `empty`, `align`, `cardPlacement`).

## Acceptance

- [x] The Checkbox input's bounding box is 44x44 (asserted through its classes `h-11 w-11`) and the visible box is 20 px.
- [x] Tabs arrow-key navigation works.
- [x] SegmentedControl keeps `aria-pressed`.
- [x] Switch has no "On"/"Off" text node and keeps `role=switch` and `aria-checked`.
- [x] StatusChip renders null for 0.
- [x] DataList `cardsBelow="md"` emits `md:hidden` and `hidden md:block`.
- [x] ConfirmDialog order is Cancel then confirm.
- [x] Toast sits at the bottom under `sm`.
- [x] Test updates: `StatCard.test.tsx:34,49` (to the tokens); any test asserting a zero StatusChip is noted, and fixed in the page task that changes it.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/39-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/39-control-primitives` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/39-notes.md`.
