# Task 42 notes: Dashboard

## Plan (as written before implementing)
1. `DashboardPage.tsx`: PageHeader "Dashboard"; EncodingStatus `panel` with `waiting` from stats; Warnings as Banners; four StatCards with the 5.3 labels and subs; `grid lg:grid-cols-12` with Working now (`lg:col-span-8`) and Nodes (`lg:col-span-4`); Recent activity as a full-width Card (`padding="none"`, 10 rows) showing `file_name`. Empty dashboard: PageHeader plus EmptyState with the "Add library" button.
2. `DashboardPage.test.tsx`: wrap in ToastProvider (EncodingStatus needs it); update the assertions the spec changes (listed below); add one test per EncodingStatus state, stat subs, file_name rows, and the 8/4 split.
3. `App.test.tsx`: the empty-dashboard link name changes to "Add library" (same spec change).

## Changed assertions (file:line, old to new, why)
- `web/src/DashboardPage.test.tsx`, empty-state test: title "shows a one-line empty state..." becomes "shows the EmptyState...". `toHaveTextContent('No libraries yet.')` becomes `toHaveTextContent('Add your first library')`, and the link name "Add one" becomes "Add library". Why: spec 5.3 empty dashboard copy.
- `web/src/DashboardPage.test.tsx`, stats test: title and comment now say 2 columns below `lg`, 4 from `lg`; `toContain('sm:grid-cols-4')` becomes `toContain('lg:grid-cols-4')`. Why: spec 5.3 stat grid is `grid-cols-2 gap-4 lg:grid-cols-4`.
- `web/src/DashboardPage.test.tsx`, history test: `toHaveTextContent('shrunk')` becomes `'Shrunk'`, and `'failed'` becomes `'Failed'`. Why: the row caption uses the spec 5.0 status words ("Shrunk" and "Failed").
- `web/src/DashboardPage.test.tsx`, history test: `node-activity-node-1` `'ready'` becomes `'Running'`, and `node-activity-node-2` `'waiting for approval · working on'` becomes `'Waiting for approval'`. Why: the Nodes card now shows the spec 5.0 lifecycle badge and no job ids.
- `web/src/App.test.tsx`, empty dashboard test: link name "Add one" becomes "Add library". Same spec change as above.
No assertion was deleted. Every existing `data-testid` survives.

## Deviations and choices
1. **Working now keeps its own row, not `NodeJobLine`.** Spec 5.3 says to use `NodeJobLine`. It cannot be used without renaming test ids and changing an asserted speed string: it renders `2.5x` (ASCII x) where the test expects `2.5×`, its progress bar has no `job-progress-{id}` testid, and it adds a Log button. The row shows `file_name`, the percent, a `bg-info` bar and speed/ETA. Owner: say if you want `NodeJobLine` here and the testid rename.
2. **Node progress bar kept under the Nodes row.** Spec 5.3 lists name, badge and check-in only. The bar stays (`node-progress-{id}`) because the existing test asserts it and it is the only live signal for a node that is working.
3. **Added test ids:** `dashboard-working` and `dashboard-nodes` on the two Cards, so the 8/4 split is testable. The history empty state keeps `history-empty`.
4. **Empty dashboard keeps the h1.** It renders PageHeader "Dashboard" above the EmptyState; `empty-dashboard` stays on the container, and `dashboard` is still absent there.
5. **Recent activity history is capped at 10 rows** (`HISTORY_ROWS`, spec 5.3). The 50-job query is unchanged.
6. **Not done, not in the acceptance list:** spec 3.7 wants page-level query errors to render an ErrorBox with "Try again". The dashboard still shows nothing for a failed query, as before. Left for a later task.
7. **Plain language and tokens.** The page uses only design tokens (no slate, emerald or red palette classes), so it is not a lint blocker for task 53.

## Verification
- `web`: `npx tsc --noEmit` clean. `npx eslint . --max-warnings 0` clean. `npx prettier --check .` clean. `npm run build` clean.
- `npx vitest run`: 57 files, 364 tests passed (baseline 357; +7 new dashboard tests).
- `make e2e` (Playwright, needs `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`, the libatk fix from task 41): 61 passed, 3 skipped (the suite's own per-viewport skips), exit 0.
- `DECISIONS.md` unchanged against origin/main. No Go or API change.

## Summary
1. **Built:** Dashboard on the 5.3 spec. PageHeader; EncodingStatus panel (all four sentences); Warnings as Banners; four StatCards with labels and subs; Working now and Nodes split 8/4 at `lg`; Recent activity with file names and size changes; EmptyState for the empty dashboard.
2. **Skipped:** `NodeJobLine` (deviation 1), the page-level ErrorBox with retry (deviation 6).
3. **Next tasks should know:** the Dashboard no longer has a `job {id}` anywhere. Test ids were kept, so `job-progress-{id}` and `node-progress-{id}` still work. The `StatCard` grid is now `lg:grid-cols-4`, which task 44 and later pages may copy.
4. **Owner decisions:** deviation 1 (own job row versus `NodeJobLine`) and deviation 6 (ErrorBox on dashboard queries).
5. **Known gaps:** none in the acceptance list. Playwright needed the local Chromium libs on LD_LIBRARY_PATH.
