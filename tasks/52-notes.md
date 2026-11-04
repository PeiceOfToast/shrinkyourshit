# Task 52 notes: Copy sweep (spec 6 Naming and Empty copy, 3.3 titles, 4.1 useDocumentTitle)

## Plan

1. Re-check acceptance item 1 first: grep `Add folder`, `job {`, `Back to files`, `Back to rules` in `web/src/*.tsx`
   (non-test). Already clean on main, so no code change for that item.
2. Add `useDocumentTitle` to every route page that does not set it yet: Setup, Login, Dashboard, Libraries,
   New library, Library detail, Edit library, Nodes, Encode settings, App settings, Integrations. Rules and Flow
   already set it. Titles follow the page's h1 (spec 6 "Nav labels equal h1s"); Library detail and Edit use the
   library name, as spec 3.3 and 5.6 say.
3. The Flow loading fallback in `App.tsx` is also a route. Its h1 said "Flow", which the spec retires (the page is
   "Rules", 5.8 and 5.9). It now reads "Rules" and sets the same title.
4. Empty copy: the dashboard history empty state titled "Nothing here yet" becomes "No activity yet" (spec 6).
5. Tests: one new file, `web/src/routeTitles.test.tsx`, renders every route through the real `App` with the API
   stubbed at fetch level and asserts `document.title` for all 13 routes. No existing file was edited for tests.

## Changed assertions

None. `git diff main -- '*.test.ts' '*.test.tsx'` lists only the new `web/src/routeTitles.test.tsx`. No assertion was
edited or removed. Every `data-testid` survives (the only text change is the dashboard empty title, and no test
asserts "Nothing here yet" outside `EmptyState.test.tsx`, which uses its own string).

## Verification

Run on the VM with node from `/home/mb/.local/node/bin` (not on the default PATH):

- `npx tsc --noEmit`: exit 0, no output.
- `npm run lint` (`eslint . --max-warnings 0`): clean, no output after the header.
- `npx prettier --check .`: "All matched files use Prettier code style!"
- `npx vitest run` (full web suite): `Test Files  62 passed (62)`, `Tests  444 passed (444)`. Task 51 was 61 files and
  431 tests, so this adds 1 file and 13 tests (the new route title tests).
- `npx vitest run src/routeTitles.test.tsx` before the full run: `Tests 13 passed (13)`.
- `npm run build`: `✓ built in 2.47s`.
- `go test ./...`: every package `ok` (worker 25.3 s). No Go source changed.
- Acceptance grep, run as written: `grep -n "Add folder\|job {\|Back to files\|Back to rules" *.tsx | grep -v "\.test\.tsx"`
  printed nothing (exit 1 from grep, meaning no match).
- `make lint` / `make test`: make is installed on this VM, but the steps were run one at a time above, so
  `golangci-lint` was not re-run (no Go changed).
- `npx playwright test`: NOT RUN. Per the run instructions, Chromium cannot launch on this VM (system libraries missing,
  no passwordless sudo). CI e2e is the gate for this task. The e2e suite does not match on the changed copy:
  `grep` over `web/e2e` for "Nothing here", "'Flow'", "Add a library" and document-title assertions found nothing.

## Owner calls

1. **Titles chosen.** Setup "Setup", Login "Log in", Dashboard "Dashboard", Libraries "Libraries", New library
   "Add a library" (the h1, as spec 5.5 gives), Library detail "Movies · Libraries · ShrinkYourShit", Edit "Edit library ·
   Movies · …", Nodes "Nodes", Encode "Encode settings", App "App settings", Integrations "Integrations". Say if you want
   different words.
2. **Empty copy left as is (not in the spec's named list).** These are bare grey lines or sentences the spec's own
   rule ("never 'No X configured yet.' as a bare grey line") may cover:
   - `FileHistory.tsx:161` "No jobs have run for this file yet." is a bare `p`. Its test (`FileHistory.test.tsx:186`)
     asserts the exact text. Changing it means changing that assertion, so I did not.
   - `IntegrationsPage.tsx:651` "Nothing delivered yet." is a bare `p`, with `webhook-deliveries-empty`.
   - `EasyLibraryForm.tsx:85` "No folder chosen yet" is the spec's own placeholder (5.5). Kept.
   - `EncodingStatus.tsx:242` "No nodes yet." is a Banner, not a grey line. Kept.
3. **"Save changes" buttons** (`AppSettingsPage`, `EncodeSettingsPage`, `LibraryEditPage`, `IntegrationsPage` x2,
   `NodesPage`) are not renamed. The spec's Naming rule asks buttons to say the action (its examples are "Save rules"
   and "Add library"), but the spec does not name "Save changes" as a string to replace. Owner call whether to rename
   to "Save settings" / "Save library" / "Save node".
4. **Import placement.** `useDocumentTitle` is imported above the first existing import in each touched file, so it
   sits above `react` or `@tanstack` imports. Lint and tsc pass. It is cosmetic only; say if you want the imports
   reordered.
5. **Route guard loading state.** `guards.tsx` (`RequireReady`, `Loading`) renders before the page mounts and does not
   set a title. A page sets its own title once it renders, so the tab shows the previous title for the moment the guard
   is loading. Out of scope (not a route).

## Summary

1. **Built:** `useDocumentTitle` on all 13 routes (Setup, Login, Dashboard, Libraries, New library, Library detail, Edit,
   Rules, Flow, Nodes, Encode, App, Integrations); the Flow fallback reads "Rules"; the dashboard history empty state
   reads "No activity yet"; `routeTitles.test.tsx` checks all 13 titles.
2. **Skipped:** Playwright (no browser libraries here; CI is the gate). Acceptance item 1 needed no code change (the
   strings were already gone). No Go change, no `api.ts` change, no new dependency, no existing test edited.
3. **Owner calls:** the title wording; four empty-copy lines left for the owner (FileHistory needs an assertion change);
   "Save changes" button names; import placement.
4. **Next task (53, guardrail lint audit) should know:** the lint rule for retired classes (`text-slate-500`, `rose-`,
   `emerald-600`) is not yet added; `FileHistory.tsx:161` and `IntegrationsPage.tsx:651` are the bare grey empty lines
   to decide on; route titles are covered by `routeTitles.test.tsx`, so 53 can reuse it for the single-h1 check.
5. **Not done here:** Playwright run and a screenshot check. CI e2e decides the gate.

## Status

Branch `task/52-copy-sweep` is pushed. The PR is not opened: `gh` is not available for this run, so the supervisor opens
it against `main` and runs the merge gate. Acceptance boxes are ticked (both are covered by unit tests and the grep
above). STATUS.md is `in_progress` with the branch name; set it to `done` with the PR link after CI is green.
