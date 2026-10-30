# Task 20 notes: mobile touch targets and shared controls

## Plan

Files, in order:

1. `web/e2e/mobile.spec.ts` — the audit gate, written first, recorded failing.
2. `web/src/uiControls.ts` + `web/src/uiControls.test.ts` — shared control classes.
3. `web/src/DataList.tsx` + `web/src/DataList.test.tsx` — table/cards primitive.
4. `web/src/NodesPage.tsx` (+ test) — buttons onto shared classes.
5. `web/src/LibrariesPage.tsx`, `web/src/LibraryDetailPage.tsx` (+ tests) — DataList.
6. `web/src/EncodeSettingsPage.tsx`, `web/src/AppSettingsPage.tsx`, `web/src/AboutCard.tsx` (+ tests) — inputs/buttons.
7. Run `make test`, `make lint`, `npm run build`, Playwright audit green.
8. Tick acceptance, update STATUS.md, PR.

## Documented deviations from the task text (all forced by "do not change any data-testid that exists today" and "tasks 11, 12, 18 and 19 tests pass unchanged")

- `DataList` gets optional `cardCell`, `cardNoLabel` and `hideInCards` per column and
  separate `rowTestId` (table) / `cardRowTestId` (card) hooks: the existing page tests
  assert different test ids on the table tree (`library-name-x`, `file-status-x`) than on
  the card tree (`library-name-x-card`, `file-card-status-x`), and a single shared cell
  function cannot render both.
- The `/libraries` table keeps one "Status" column with the four chips instead of the
  task's "Satisfied, Waiting" split: the `library-chip-*` test ids and task 19 tests
  depend on it.
- The `/libraries/:id` file table keeps the "Why", "Ratio" and "Encoder" columns (task
  names only Name/Before/After/Saved/Status/actions) because `file-reason-*`,
  `file-ratio-*` and `file-encoder-*` test ids are asserted by task 19 tests. A "Saved"
  column is added per the task.
- The audit spec waits for each page's data-driven content instead of
  `waitForLoadState('networkidle')`: the app holds a permanent SSE connection, so
  networkidle never fires.
- Dashboard warning links got `min-h-11` (was `min-h-[36px]`): the audit gate covers `/`,
  and a fresh seed (built-in node stopped) renders a warning card whose link would fail
  it. One-class change only; full dashboard restyle stays in task 21.
- AboutCard's update-check checkbox becomes a 44px `appearance-none` checkbox: the audit
  measures `main input` heights and a 16px native checkbox fails it.

## Audit run before any UI change (recorded as required)

(filled in after first run)

## Summary

(filled in at the end)
