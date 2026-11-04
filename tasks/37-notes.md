# Task 37 notes: control classes and format helpers

## Plan (files, in order)

1. `web/src/format.ts` (new): move `formatBytes` and `formatEta` out of `easyModel.ts` unchanged, add the spec 4.1 helpers (`formatDuration`, `formatCount`, `formatPercent`, `formatDateTime`, `formatRelative`, `shortHash`, `encoderLabel`).
2. `web/src/easyModel.ts`: import and re-export `formatBytes` and `formatEta` from `format.ts`.
3. `web/src/uiControls.ts`: replace with the 2.10 strings verbatim; `focus` and `base` stay private.
4. `web/src/useDocumentTitle.ts` (new): `documentTitle(parts)` pure function plus the hook.
5. Tests: `format.test.ts`, `useDocumentTitle.test.ts` (new); `uiControls.test.ts` extended, its two existing assertions kept.
6. Verify: `npm run build`, `npm run lint`, `npm run format:check`, `make test`, `make lint`, `npx playwright test`, `scripts/check-spdx.sh`.

## Changed assertions

None. No existing assertion was edited or removed. `easyModel.test.ts` passes unchanged, and the two original `uiControls.test.ts` assertions are kept as they were. Only new assertions were added.

## Deviations and decisions (read these)

1. **`btnIcon` has `h-11 w-11`, not `min-h-11`.** The acceptance item says every control string contains `min-h-11`, but the 2.10 string for `btnIcon` sizes the box with `h-11 w-11` (44 px). I kept the spec string exact, because the task says "exports exactly as in 2.10". The test asserts `h-11 w-11` for `btnIcon` instead. **Owner: if you want the literal `min-h-11` as well, say so and I will add it. It is a one-token change.**
2. **"Every control string" is read as the touch-target controls.** `link`, `card`, `cardPad` and `btnBlock` do not carry `min-h-11` and are not touch targets, so the test checks the button, field and `linkTouch` strings (plus `btnIcon`, see 1).
3. **`formatDuration` is the canonical name and `formatEta` is an alias.** The spec names the helper `formatDuration`, but the task file says `easyModel` re-exports `formatEta`. Both names are exported and point to the same function.
4. **`formatRelative` output has a space before the unit** ("5 s ago"). This follows the spec's examples. The old `timeAgo` in `NodesPage.tsx` still prints "5s ago". I did not switch `NodesPage` because it is out of scope here. Task 48 (nodes) should replace `timeAgo` with `formatRelative`.
5. **`encoderLabel('videotoolbox')` returns "Apple".** The spec lists cpu, qsv, vaapi and nvenc but does not name videotoolbox. "Apple" follows the same plain-language pattern.
6. **`formatCount(n, locale?)`** takes an optional locale. The test passes `'en-US'`, so it does not depend on the machine's locale.
7. **`formatPercent(percent)`** takes a percentage number (not a fraction). It rounds to a whole number, uses U+2212 for negatives, and gives no sign for positives. The spec gives "−10 %" as the example. The ratio helper ("1.9× smaller") is not in the spec's helper list, so I did not add it. It stays in `LibraryDetailPage` for now.
8. **`formatDateTime` returns `''` for missing or invalid input**, and `formatRelative` returns `'never'` (the same as `timeAgo`).
9. **Left for later tasks, outside this task's scope:** `NodesPage.tsx` still has `${btnGhost} !text-red-300 hover:!bg-red-950/40` in 4 places. The spec says to replace them with `btnDangerGhost`. That is a page change, so it belongs to task 48. The `!` ban in the uiControls test covers only the exported strings, not pages.

## Verification

- `npm run test` (via `make test`): 35 files, **262 tests passed**. That is 245 from before plus 17 new (12 in `format.test.ts`, 3 in `useDocumentTitle.test.ts`, 2 added to `uiControls.test.ts`).
- `make test`: passes (all Go packages, plus the web suite).
- `make lint`: passes (`go vet`, `golangci-lint`, eslint with `--max-warnings 0`, prettier check).
- `npm run build`: passes (`tsc --noEmit` and `vite build`). The built CSS contains every Tailwind class that the new 2.10 strings use. I checked `bg-danger-solid-hover`, `bg-danger\/10`, `ring-offset-canvas`, `ring-focus`, `duration-fast`, `ease-standard`, `rounded-control`, `placeholder:text-fg-muted`, `focus-visible:border-focus`, `text-accent-text`, `ring-line`, `rounded-card`, `h-11` and `pr-9`.
- `npx playwright test`: **61 passed, 3 skipped** (exit 0). This matches task 36's result. The 3 skips are the suite's own per-viewport skips. The first run had 6 skips because ffmpeg was not on PATH, so the file-table tests had no seeded row. I reran with ffmpeg on PATH.
- `scripts/check-spdx.sh`: passes.

Environment note: `go`, `node` and `ffmpeg` are not on the default PATH in this sandbox. They are at `~/.local/go/bin`, `~/.local/node/bin` and `~/.local/bin`, and the commands above put them on PATH. Playwright also needs the system libraries in `/tmp/pwlibs`, from task 36.

## Summary

1. Built: `format.ts` (bytes, duration, count, percent, date-time, relative time, short hash, encoder label), the 2.10 control strings in `uiControls.ts`, and `useDocumentTitle`. `formatBytes` and `formatEta` now live in `format.ts` and are re-exported from `easyModel.ts`. Tests: 17 new, all passing.
2. Skipped: no page changes. The `!text-red-300` overrides in `NodesPage.tsx` (4 places) are left for task 48. The `timeAgo` in `NodesPage` is still the old format for the same reason.
3. Next tasks can import `btn*`, `input`, `select`, `card`, `cardPad`, `linkTouch` and the `format.ts` helpers. The 14 existing `uiControls` imports now render the new strings, so those controls change look here. Each page's own restyle comes in tasks 38 to 53.
4. Owner decision needed: `btnIcon` uses `h-11 w-11` as the spec gives it, not `min-h-11` (deviation 1).
5. Owner decision needed: `encoderLabel('videotoolbox')` is "Apple" (deviation 5). The rest of the decisions above are reversible and listed so you can redirect them.
