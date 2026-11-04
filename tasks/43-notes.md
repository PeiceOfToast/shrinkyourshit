# Task 43 notes: Libraries list

## Plan (as written before implementing)
1. `LibrariesPage.tsx`: `PageHeader` "Libraries" with `btnPrimary` "Add library"; `DataList` with `rowHref` to `/libraries/{id}`; columns in spec order (Library with mono path under the name, Status, Files, Saved, Looking for, Actions); Status shows all non-zero chips in the table, and the single most urgent chip (or "Up to date") on a card; `EmptyState` through the DataList `empty` prop; loading via `PageHeader` plus `SkeletonRows`; error via `ErrorBox` with `onRetry`; palette classes replaced by tokens.
2. `LibrariesPage.test.tsx`: update the assertions the spec changes (listed below); add a layout block covering the header action, non-zero chips only, "Up to date", and the mono path.
3. No `api.ts`, Go or `DECISIONS.md` change.

## Changed assertions (file:line, old to new, why)
`web/src/LibrariesPage.test.tsx`
- :136 and :139, `toHaveAttribute('href', ...)` on `library-name-lib-1` and `-lib-2` became `.closest('a')` then the same href check. Why: the name is now a span inside the whole-row link (DataList `rowHref`), so the link is the ancestor. Same intent: the name leads to the detail page.
- :164, card chip `library-chip-satisfied-lib-1-card` `'2 Nothing to do'` became `library-chip-queued-lib-1-card` `'1 Waiting'`, plus a `queryByTestId(satisfied-card)` null check. Why: spec 5.4, the card shows only the most urgent chip.
- :168, card saved `'2.0 GB saved'` became `'2.0 GB'`, plus `not.toHaveTextContent('saved')` at :172. Why: spec 5.4, the card value has no "saved" suffix.
- :175, card name `toHaveAttribute('href')` became `.closest('a')`. Same reason as :136.
- :342 and :345, the empty-state test: `'Add your first folder'` became `'No libraries yet'` and `'Add library'`. Why: spec 3.7 EmptyState copy and action.

No assertion was deleted. The 44 px test was briefly relaxed for the whole-row link, then restored to the original strict check after the fix below. Every existing `data-testid` survives.

Not changed, already satisfied: the acceptance line "`LibrariesPage.test.tsx:116-121` assert `0 Done` and `0 Failed` chips". In this repo those lines already use `queryByTestId(...)).toBeNull()` (:117 and :118), so no edit was needed there.

## Deviations and choices
0. **Shared primitive changed (`web/src/DataList.tsx`).** The whole-row link (`rowHref`) is now `flex min-h-11 flex-col justify-center`. Without it the link measured 38 px, and the mobile audit (`e2e/mobile.spec.ts`, 44 px rule for every link) failed on `/libraries` at 320 to 768 px in CI run 41783486582. The change only makes the link at least 44 px tall. `rowHref` has no other users. The DataList test (`toHaveClass('after:absolute', 'after:inset-0')`) still passes.
1. **Remove stays in the row** as `btnDangerGhost`. Spec 5.4 moves it to a Danger zone on the Edit page (5.7). The move touches `LibraryEditPage`, which is outside this task's scope, and the spec allows leaving it in the row until task 47. Task 47 should do the move. The ConfirmDialog flow stays on this page and its test ids are unchanged.
2. **Chip set is all seven file statuses** (new, satisfied, queued, processing, done, failed, missing), not the old four. Spec 5.4 says "only the non-zero chips", and 5.0 defines all seven. Zero counts render nothing, so a library with only satisfied and done files looks the same as before.
3. **"Up to date" rule:** shown when `files > 0` and `satisfied + done === files`. An empty library shows no badge and no chip.
4. **Card badge priority:** failed, missing, working (processing), waiting (queued), new, then "Up to date". Spec 5.4 lists four levels and does not name `missing` or `new`. I placed `missing` after `failed` as a warning, and `new` last. The owner can reorder.
5. **"Shrunk" is not used on the chips.** Spec 5.0 says `satisfied` with `bytes_saved > 0` reads "Shrunk". The library `bytes_saved` is a total for the whole library, not per file, so a library with any saved bytes would label every satisfied file "Shrunk", which can be wrong. The chip keeps "Nothing to do". **Owner question:** do you want per-file bytes (needs an API field, so it is out of scope here) or accept the library-level word?
6. **Path column folded into the name cell** as a `font-mono text-caption text-fg-muted truncate` line, per spec 5.4. The table keeps `title={path}` for the full path.
7. **Testids kept, moved where needed:** `library-name-{id}` now sits on the inner span (the row link is the outer `a`). Card status chip ids follow the same `-card` suffix.
8. **Page-level error** now renders under the PageHeader with `ErrorBox onRetry` (spec 3.7). Loading shows the PageHeader plus `SkeletonRows` (3 rows) in a Card, not the old two skeleton cards.
9. **Document `<title>`** via `useDocumentTitle` is not added. The Dashboard (task 42) does not use it either, so this follows the current pattern. Left for the copy sweep (task 52).
10. **Not built:** the `NodeJobLine` and other shared work are not in this page. No new primitives were added.

## Verification
- `web`: `npx vitest run src/LibrariesPage.test.tsx`: 13 passed (9 existing, 4 new).
- `npx vitest run` (full web): 57 files, 368 tests passed (baseline 364; +4 new layout tests).
- `npm run lint` (eslint `--max-warnings 0`) clean. `npm run format:check` clean. `npx tsc --noEmit` clean. `npm run build` clean.
- `npx playwright test` (with `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`): 58 passed, 6 skipped (the suite's own per-viewport skips for the file table), exit 0. This includes the mobile audit.
- CI run 41783486582 on PR #46: `e2e` failed on the mobile 44 px audit (above). Fixed as deviation 0. Local rerun of `e2e/mobile.spec.ts` with a rebuilt binary (`SHRINK_E2E_BIN=/tmp/shrink-e2e/shrinkyourshit`, port 8731): 4 passed. Full `npx playwright test` on the same binary: 58 passed, 6 skipped. My earlier local Playwright run had used a stale embedded UI, so it did not catch this.
- `make` is not installed on this machine, so I ran the underlying commands. `go test ./...` passed (all packages `ok`, exit 0).
- `DECISIONS.md` and `web/src/api.ts` are unchanged against `origin/main`.
- Palette leftovers: none in `LibrariesPage.tsx` (grep for slate, emerald, red, text-xs and text-2xl is empty).

## Summary
1. **Built:** the Libraries list on spec 5.4. PageHeader with "Add library"; whole-row links to detail; mono path under the name; status column with non-zero chips, or one "Up to date" badge; card badge for the most urgent status; saved value without "saved"; EmptyState; error with Try again; tokens only.
2. **Skipped:** moving Remove to the Edit page (deviation 1, task 47); per-file "Shrunk" (deviation 5, owner question); document title (deviation 9).
3. **Next tasks should know:** the row link is a DataList `rowHref` stretched over the row, so the Libraries list has no 44 px link of its own (the 44 px test accounts for this). Task 47 should move Remove to a Danger zone on the Edit page and remove the `btnDangerGhost` row action here. Task 44 (library detail) can copy the PageHeader breadcrumb and the `Up to date` badge rule.
4. **Owner decisions:** deviation 1 (Remove in the row until task 47), deviation 4 (card badge priority for `missing` and `new`), deviation 5 (library-level "Shrunk" versus per-file).
5. **Known gaps:** no shared primitive changes; `go test ./...` was not in this task's scope (no Go files changed) and is reported separately.
