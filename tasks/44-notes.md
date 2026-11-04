# Task 44 notes: Library detail, FileHistory, JobLogDialog

## Plan (as written before implementing)
1. `LibraryDetailPage.tsx`: PageHeader with breadcrumb, title, Watching badge, mono path, and actions Rules, Edit, Scan now (primary, last). Summary Card with four stats, the status chips for non-zero counts, the last scan line, and "Rules · {tier}" with the plain-language steps and "Change rules". Toolbar with labels above controls and Retry only when failed > 0. DataList `cardsBelow="md"`, columns per spec 5.6. Pager only when `total > perPage`.
2. `FileHistory.tsx` and `JobLogDialog.tsx`: DialogHeader, DialogBody and DialogFooter, tokens only, existing test ids kept.
3. `NodeJobLine.tsx`: colours only (tokens); drop the `!` overrides on its Log button.
4. Tests: update only the assertions the spec changes (listed below); add one layout block per acceptance item; add a dialog-structure test to each dialog.
5. No Go change, no `api.ts` change, no `DECISIONS.md` change.

## Changed assertions (file:line, old to new, why)
`web/src/LibraryDetailPage.test.tsx`
- :185, `file-encoder-f2` `'nvenc'` becomes `'NVIDIA'`. Why: spec 5.6 Encoder column uses the plain words from `encoderLabel`.
- :223, `library-summary-saved` `'700 MB saved'` becomes `'700 MB'`. Why: spec 5.6 Saved is the large value under its own "Saved" label, with no word "saved" in the value.
- :349, card label `'Size:'` becomes `'Before and after'`. Why: spec 5.6 has one "Before and after" column; the card label is that column's header.
- :350, card label `'Encoder:'` becomes `'Encoder'`. Why: spec 3.5 says labels have no colons.
- :488, :503, :520, pager `'Page 1 of 3 · 120 files'`, `'Page 2 of 3'`, `'Page 1 of 5'` become `'1–50 of 120'`, `'51–100 of 120'`, `'1–25 of 120'`. Why: spec 5.6 pager reads "{from}–{to} of {total}".

`web/src/FileHistory.test.tsx`, `web/src/JobLogDialog.test.tsx`: no existing assertion changed. Added one new describe block each.

No assertion was deleted. Every existing `data-testid` survives.

## Deviations and choices
1. **History, not Log plus History.** Spec 5.6 lists "Log" and "History" as row actions. The existing "Log" button opens the per-file job history (`file-log-{id}`, test asserts it), and the per-job Log lives inside that dialog. Kept one action, relabelled "History" with the same test id and an aria-label "Job history for {name}". A separate Log would need a job fetch the row does not have. Owner: say if you want a second button.
2. **Cancel stays in the row** as `btnGhost`. The spec's action list omits it, but the cancel flow has tests and it is not a destructive action.
3. **Scan now is in both places.** Spec 5.6 says Scan "moves" from the Libraries row, but spec 5.4 still lists "Scan now" in the Libraries row actions. I added it to the header and left the Libraries row unchanged (task 43 owns that file). Owner question: remove it from the row?
4. **Summary chips never show "Shrunk".** Chips are library-level, and `bytes_saved` is a library total (same problem as task 43 deviation 5). "Shrunk" is shown per file in the table, where `bytes_saved` belongs to the file.
5. **Failed stat vs Retry count.** The summary "Failed" is library-wide (stats). "Retry failed (n)" still counts only the failed rows on the visible page (existing behaviour, unchanged). The retry itself still covers every failed file.
6. **"Last scan:" kept with a colon.** Spec 5.6 writes "Last scan {formatDateTime}". Kept the existing text so the existing assertion stays.
7. **Search has no icon.** The Icon set has no search glyph, and adding one means editing `Icon.tsx`, which is outside this task's scope.
8. **Sort direction** is a `btnIcon` with a ↑ or ↓ glyph and `aria-label` "Sort ascending" or "Sort descending". The `min-h-11` class is added at the call site: the `btnIcon` string in `uiControls.ts` has `h-11 w-11` but not `min-h-11`, although spec 2.10 says every string keeps `min-h-11`. Not changed here (out of scope). Owner: fix the string or the spec text.
9. **Headings.** Breadcrumb plus PageHeader h1, then sr-only h2s ("Summary", "Files") and an h3 for the rules, so no heading level is skipped (spec 6). The sr-only h2 is a choice, not spec text.
10. **Why in cards** keeps its "Why: " prefix. The existing test asserts it, and the spec's no-colon rule is about labels, not values.
11. **Why column.** The table always shows the Why column ("—" for an empty reason, as before). Only the cards hide it when there is no reason (existing `hideInCard`), which matches spec 5.6 "only when non-empty" for the card view.
12. **FileHistory keeps raw encoder ids** (`qsv`, `vaapi`). Its test asserts them verbatim (task 30). Only the file table uses the plain words.
13. **Document `<title>`** not added (same as tasks 42 and 43; left for task 52).
14. **Dialog X plus footer Close.** DialogHeader gets a close X, and the footer keeps its Close button under its existing test id. Two close controls, same action.
15. **Not built:** nothing in the acceptance list is left out. `NodeJobLine` is colour-only, as the scope says.

## Verification
- `web`: `npx tsc --noEmit` clean. `npm run lint` (`eslint . --max-warnings 0`) clean. `npm run format:check` clean. `npm run build` clean.
- `npx vitest run src/LibraryDetailPage.test.tsx src/FileHistory.test.tsx src/JobLogDialog.test.tsx src/NodeJobLine.test.tsx`: 41 passed (32 before, 9 new).
- `npx vitest run` (full web): 57 files, 377 tests passed (baseline 368; +9 new).
- `npx playwright test` (embedded fresh build, `SHRINK_E2E_PORT=8731`, `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`): 58 passed, 6 skipped (the suite's own per-viewport skips), exit 0. Includes the mobile 44 px audit on `/libraries/:id`.
- `go test ./...`: all packages ok (no Go change).
- `make` is installed here; I ran the underlying commands (the Makefile targets wrap them).
- `DECISIONS.md` and `web/src/api.ts` unchanged against `main`.
- Palette leftovers in the four files: none (grep for slate, emerald, red, amber, sky is empty).

## Acceptance (each has a test)
- [x] Header actions Rules, Edit, Scan now (primary): "puts Rules, Edit and Scan now in the header, Scan last and primary".
- [x] Summary Card with four numbers and the rules summary: "summarises the library in four numbers with the rules in plain language".
- [x] Labels above toolbar controls: "puts a label above each toolbar control".
- [x] Retry hidden at 0: "hides Retry when nothing has failed and shows it with the count when something has".
- [x] Pager hidden when `total <= perPage`: "hides the pager when every file fits on one page".
- [x] Status "Shrunk" for satisfied with savings: "reads a satisfied file that was made smaller as Shrunk…".
- [x] Encoder words: same test as above (CPU, Intel) and the changed assertion at :185 (NVIDIA).
- [x] `cardsBelow="md"`: "switches from cards to the table at md, not sm".
- [x] Dialogs use DialogHeader/Body/Footer: "builds the dialog from the shared header, body and footer" in `FileHistory.test.tsx` and `JobLogDialog.test.tsx`.

## Summary
1. **Built:** the library detail page on spec 5.6 (PageHeader with three actions, summary Card, labelled toolbar, Retry and pager only when meaningful, DataList on `md` with before and after, Saved with ratio, plain encoder words, History action); FileHistory and JobLogDialog on DialogHeader/Body/Footer; NodeJobLine and both dialogs on tokens.
2. **Skipped:** a second "Log" button (deviation 1), a search icon (deviation 7), the document title (deviation 13).
3. **Next tasks should know:** `btnIcon` lacks `min-h-11` (deviation 8), so call sites add it. Task 47 should move Remove off the Libraries row as the notes for task 43 say. The `library-summary-*` and `file-*` test ids are unchanged; the pager text is now "1–50 of 120".
4. **Owner decisions:** deviation 1 (History versus Log plus History), deviation 3 (Scan now in the Libraries row too), deviation 4 (no per-file "Shrunk" in the library chips) and deviation 8 (`btnIcon` string versus spec 2.10).
5. **Known gaps:** no Go or API change, so no new library-wide "Shrunk" or per-file bytes; Retry count is page-scoped (deviation 5, existing behaviour).
