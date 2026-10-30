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
- AboutCard's update-check checkbox becomes a 44px checkbox: the audit measures
  `main input` heights and a 16px native checkbox fails it.
- `DataList` takes an optional `cardTestid` (defaults to `${testid}-cards`) and the two
  list pages pass their existing `libraries-cards` / `file-cards` names. The task also
  wants a `{testid}-cards` name, but the alternative was renaming test ids that task 12
  and 19 assert.
- `DataList`'s table wrapper carries `relative` as well as `overflow-x-auto`: the action
  column's `sr-only` header is absolutely positioned, and without a positioned ancestor
  it widened the page by its offset at 768 px (caught by the layout spec).
- The `/libraries/:id` cards hide the After/Ratio columns and fold the size comparison
  into one "Size:" row; show "Saved" only when there is a saving. The table keeps every
  existing column and test id.
- Filter selects and the search field on the detail page got `min-h-11` in place rather
  than the full `input` string: `input` is `w-full`, which breaks the inline filter row.
  The settings pages do use `input` on every text/number field and select.

## Audit run before any UI change (recorded as required)

Ran `make e2e` on the pre-change build (with `LD_LIBRARY_PATH` pointing at the local
Playwright system libs in this sandbox). The audit failed at `/` for every width with:

```
/ at 320px: 1 control(s) under 44px tall: [{"tag":"a","label":"Go to Nodes","height":36}]
```

The other four pages had already been brought to 44 px by tasks 12, 18 and 19; the gate
still failed, so it was a real red baseline. After the change the same command reports
`61 passed, 3 skipped` (the skips are the phone-only/desktop-only branches of the
existing layout spec).

## Gate results

- `make e2e`: 61 passed, 3 skipped; mobile audit green at 320, 390, 414, 768 on `/`,
  `/libraries`, `/nodes`, `/encode`, `/app` and the seeded `/libraries/:id`.
- `make test`: Go suite + 174 web tests pass.
- `make lint`: `go vet`, `golangci-lint` (0 issues), SPDX check, ESLint, Prettier all pass.
- `cd web && npm run build`: `tsc --noEmit` and Vite build exit 0.

## Summary

Built the task 20 mobile foundation: `mobile.spec.ts` audit (4 widths x 5 pages + seeded
detail), `uiControls.ts`, generic `DataList` (cards below `sm`, wrapped table above), and
migrated Nodes, Libraries, Library detail, Encode settings, App settings and About.
Skipped the dashboard stat-card / rail work (tasks 21 and 22) and any API change.
Next task should know: `DataList` has card-specific hooks (`cardCell`, `cardLabel`,
`hideInCard`, `cardTestid`) so test ids survive; use them rather than renaming.
Playwright in this sandbox needs `LD_LIBRARY_PATH=/tmp/opencode/pwlibs/root/usr/lib/x86_64-linux-gnu`;
CI's `playwright install --with-deps` handles that normally.
