# Task 39 notes: control primitives

## Plan (as written before starting, and what happened)
1. Tokens-only classes. Tint recipe lives once in `Badge.tsx` as literal class strings. Done.
2. New primitives with tests: `Badge.tsx`, `Checkbox.tsx`, `Tabs.tsx`, `SegmentedControl.tsx`, `CopyButton.tsx`, `CodeBlock.tsx`. Done.
3. Restyle `Switch.tsx`, `StatusChip.tsx`, `StatCard.tsx`, `skeleton.tsx`, `ErrorBox.tsx`, `dialog.tsx`, `ConfirmDialog.tsx`, `toast.tsx`, `DataList.tsx`. Done.
4. `DashboardPage.tsx`: four StatCard calls move from raw SVG paths to Icon names (the prop type changed). Done.
5. Checks: vitest, lint, format, build, `make test`, `make lint`, `make e2e`. All green (see Verification).

Environment: `node` 20 and `go` are not on the default PATH here. They are at `/home/mb/.local/node/bin` and `/home/mb/.local/go/bin`, and `make` at `/home/mb/.local/bin`. Playwright's headless Chromium needed system libraries that are not installed and cannot be installed without sudo (libatk, libgbm, libcups, and others). They were installed into a user prefix with micromamba at `/home/mb/.local/chromium-deps`, and `LD_LIBRARY_PATH` points there. Nothing in the repo changed for this.

## Deviations and decisions
1. **Zero-chip test updates are done in this PR, not in task 43/44.** Spec 5.4 and 5.7 assign them to the page tasks, and the task file says "fixed in the page task that changes it". But `StatusChip` now renders nothing for 0, so the two page tests that asserted `0 Done` or `0 Failed` would fail in this PR, and CI has to be green. I changed those assertions to "no chip rendered" with the same intent (see the list below). The pages themselves are not changed.
2. **`StatCard` takes an `Icon` name, not a raw path.** The spec says "Icon names". The `icon` prop type is `IconName`. Dashboard mapping: Space saved → `encode`, Files → `library`, Nothing to do → `check`, Waiting → `clock`. The owner may want different glyphs; there is no trend or stats icon in the set.
3. **`StatCard` value is `font-semibold`, not `font-bold`.** Spec 2.3 says "stat value … semibold". Its tests select `p.font-bold`, so the selectors moved to `p.font-semibold`.
4. **`DataList` gets a column flag `actions?: boolean`.** It is not in the spec's column API. It says the row overlay must not cover action buttons, and it needs to know which column holds them. Action columns are last, right-aligned and `w-px`, and go in the card footer. Cells in them are lifted with `relative z-10` only when `rowHref` is set. Existing pages set no flag, so their layout does not change.
5. **Cards use `div.contents` wrappers** for `dt`/`dd` pairs and footer items, so the `dl` grid and the footer flex work as specified without changing the DOM contract beyond that.
6. **`CodeBlock` puts the copy button on its own row above the text**, not absolutely over the top-right corner. Spec 4.1 says "top-right". Overlaying it would hide the first line of long commands, and the spacing scale has no step that makes room for it. The button is still at the top right.
7. **`Switch` track.** The 44 px hit area is the button (`h-11`). The visible track is `h-6 w-11` with a 20 px knob. Without `visibleLabel` the button is 44 px wide, so there is no wider touch target.
8. **`dialog.tsx` default panel changed to `p-0`, as the spec says.** Only ConfirmDialog uses the default className. Every other ModalDialog passes its own className, so their look does not change in this task.
9. **`ConfirmDialog` buttons use `order-first` on mobile**, with `sm:order-none` to restore DOM order from `sm` up, so Cancel stays first in the DOM and the confirm sits on top on a phone.
10. **Toast.** The dismiss button is `btnIcon` (44 px). The left border is 3 px in the tone colour. The region is at the bottom under `sm` and at the top from `sm` up.
11. **`ErrorBox` text split.** The plain title and reason come first. The technical line reads `HTTP 500 · internal` (spec 3.7). `error-status` and `error-code` keep their test ids. `error-raw` is unchanged.
12. **`Tabs` selected indicator uses an `after:` pseudo-element** (a 2 px accent underline), not the spec's `shadow-[inset_0_-2px_0] shadow-accent`. I did not test the arbitrary-shadow form; the pseudo-element is the approach I know renders the accent colour.
13. **`Checkbox` label.** The input is named by the label text only (`aria-labelledby`). The help text is `aria-describedby`, so it does not join the accessible name.
14. **Not done in this task (later tasks):** pages do not use the new primitives yet. `StatusChip` in pages still gets no `bytesSaved`, so "Shrunk" does not appear in the UI until the page tasks pass it. `SegmentedControl`, `Tabs`, `Checkbox`, `CopyButton` and `CodeBlock` have no page users yet.

## Changed assertions (existing tests)
| file:line | old | new | why |
|---|---|---|---|
| `StatCard.test.tsx` (first test, `icon` prop) | `icon="M4 6h16"` | `icon="library"` | spec 4.1: `StatCard` takes an Icon name (deviation 2) |
| `StatCard.test.tsx` (first test, path check) | `getAttribute('d')` toBe `'M4 6h16'` | toBe `ICONS.library` | same intent, path drawn by the named icon |
| `StatCard.test.tsx` (first test, value check) | `querySelector('p.font-bold')` | `p.font-semibold` | spec 2.3, stat value is semibold (deviation 3) |
| `StatCard.test.tsx:34` | `toContain('text-slate-100')` | `toContain('text-fg')` | spec 2.2 token (task 39 test update listed in the task file) |
| `StatCard.test.tsx` (tone good test, `icon` prop) | `icon="M4 17l6-6"` | `icon="encode"` | spec 4.1 |
| `StatCard.test.tsx` (tone good test, value selector) | `querySelector('p.font-bold')` | `p.font-semibold` | spec 2.3 |
| `StatCard.test.tsx` (tone good test) | `toContain('text-emerald-400')` | `toContain('text-accent-text')` | spec 4.1 and task file: the tone is the accent text token |
| `StatCard.test.tsx` (tone good test) | `not.toContain('text-slate-100')` | `not.toContain('text-fg')` | same intent |
| `DashboardPage.test.tsx:275` | `querySelector('p.font-bold')` | `p.font-semibold` | spec 2.3 (deviation 3) |
| `LibrariesPage.test.tsx:116-121` | `toHaveTextContent('0 Done')`, `'0 Failed'` | `queryByTestId(...)` is null for both | spec 2.1 "hide zero" and 5.4 (deviation 1) |
| `LibraryDetailPage.test.tsx:213-214` | `toHaveTextContent('0 Failed')` | `queryByTestId('library-summary-failed')` is null | spec 2.1 and 5.7 (deviation 1) |

No existing assertion was deleted without a replacement. New tests cover each removed intent.

## Tests added
- `Badge.test.tsx` (3), `Checkbox.test.tsx` (2), `Tabs.test.tsx` (2), `SegmentedControl.test.tsx` (2), `CopyButton.test.tsx` (2), `CodeBlock.test.tsx` (2), `Switch.test.tsx` (2), `StatusChip.test.tsx` (3), `ErrorBox.test.tsx` (2), `dialog.test.tsx` (2), `toast.test.tsx` (2).
- Added to `StatCard.test.tsx` (`sub`, warning and danger tones), `DataList.test.tsx` (5: `cardsBelow` md and default, `rowHref` link with `z-10` actions, `empty`, `align` and badge placement) and `ConfirmDialog.test.tsx` (footer order).

## Verification (run on this branch)
- `npx vitest run`: 54 files, 325 tests passed (baseline 43 files, 293).
- `npm run lint` (`--max-warnings 0`), `npm run format:check`: pass.
- `npm run build` (`tsc --noEmit` and `vite build`): pass. Built CSS contains the classes the components rely on (`peer-checked:[&>svg]`, `after:inset-0`, `z-toast`, `sm:bottom-auto`).
- `make test`: exit 0 (Go packages and web 325 tests).
- `make lint`: exit 0 (go vet, golangci-lint, SPDX check, eslint, prettier).
- `make e2e`: exit 0. 61 passed, 3 skipped (the suite's per-viewport skips). The counts match the task 38 notes.
- `DECISIONS.md` and `web/src/api.ts`: unchanged. No Go changes.

## Summary
1. **Built:** `Badge`, `Checkbox`, `Tabs`, `SegmentedControl`, `CopyButton` and `CodeBlock`, each with tests. Restyled `Switch`, `StatusChip` (on Badge, null at 0, "Shrunk" via `bytesSaved`), `StatCard` (`sub`, `tone`, Icon names), `skeleton`, `ErrorBox` (`onRetry`), `dialog` (`size`, `DialogHeader`/`Body`/`Footer`), `ConfirmDialog`, `toast` (bottom under sm) and `DataList` (`cardsBelow`, `rowHref`, `empty`, `align`, `cardPlacement`, `actions`).
2. **Skipped:** no page changes beyond the Dashboard's four icon names. Pages still use their old classes and will move in tasks 42 to 51.
3. **Next tasks should know:** `StatusChip` needs `bytesSaved` passed where "Shrunk" should show (tasks 43 and 44). `DataList` needs `actions: true` on action columns and `rowHref` for the Libraries list. `StatCard`'s `icon` is an `IconName`, so pass names, not paths. `ConfirmDialog` footer is now Cancel then confirm.
4. **Owner decisions:** the stat icon choices (deviation 2). Whether the `CodeBlock` copy button sits above the text (deviation 6). The Tabs indicator is an underline, not an inset shadow (deviation 12).
5. **Known gaps:** `SegmentedControl` is meant for the Rules tiers (task 45). `Tabs` and `Checkbox` are not used by any page yet, so their keyboard and touch behaviour is covered by unit tests only, not by the e2e suite.
