# Task 47 notes: Edit page becomes properties only

Spec: `docs/design/ui-redesign-2026-11-04.md` 5.7, 8.3, 8.4 items 1 and 2.

## Owner decision (spec 8.4)

STATUS.md says "owner OK pending; default yes". The spec recommends yes for item 1 (Edit loses the rule
controls) and item 2 (Remove library moves to the Edit page's Danger zone). No reply was recorded, so I
took the default: yes to both. If the owner says no, this PR is the thing to revert.

## Plan (as executed)

1. `LibraryEditPage.tsx`: drop the sliders, the Exact settings panel and the Target fields. Add FormSection
   Rules (summary plus `btn` "Change rules" link), then the action bar, then FormSection Danger zone
   (`btnDangerGhost` "Remove library" with `library-delete-<id>`, and the existing ConfirmDialog ids).
2. `LibrariesPage.tsx`: remove the Remove button from the row and card, its dialog and its mutation.
3. Tests: Edit-page tests rewritten for properties only; the slider save and the Steps-tier reachability
   tests added to `RulesPage.test.tsx`; the delete test moved from `LibrariesPage.test.tsx` to
   `LibraryEditPage.test.tsx`.
4. Verify: `npm run test`, `npm run lint`, `npm run format:check`, `npx tsc --noEmit`, `npm run build`,
   `go test ./...`, Playwright against a freshly built binary.

## Save body: the one behaviour decision

The server requires `target` or `easy` on every PUT (`libraryParams`: "target is required on update"),
so the page cannot send only name, path, watch and kinds. The page resends the rule the library already has:

- `flow_source === 'easy'` and `easy` set: send `easy` (the stored values, via `parseEasy`).
- otherwise: send `target: lib.target` (the stored Target, unchanged).

Why not the other two options:

- Always `easy`: `build()` regenerates the flow from the easy object on every easy save. A library saved on
  Easy and later moved to Steps still has a stale `easy` column, so an Edit save would wipe its hand-made steps.
- Always `target`: clears the `easy` column, so an Easy library would lose its slider settings.

A stored target equal to the stored one keeps the flow (`build()` only regenerates on a change), so the
Steps and Advanced path does not touch their flow. The old page sent `defaultTarget(lib)`, which normalises
some fields; sending `lib.target` as stored avoids a spurious regeneration. The test
"never sends a leftover easy object when the Steps tier owns the flow" pins this.

## Changed and removed assertions

Format: original file:line (in `HEAD`), old to new, why.

`web/src/LibraryEditPage.test.tsx`
- :68 "reopens the easy sliders ... saves the easy object" removed. Its slider assertions (:85-88) and
  the quality and subtitles clicks (:90-91) moved to `RulesPage.test.tsx` ("the Easy tier sliders save the
  easy object and no target"). Its save assertion (:97-104) is kept in the new "resends the stored easy
  object unchanged" test, with the value now the stored one (quality 3, remove subtitles true, audio 2)
  instead of a changed one. Why: the sliders leave Edit (spec 5.7).
- :108 "exposes the raw Target only inside the collapsed Advanced panel" removed. Its codec, container,
  reencode, languages and save-body checks (:127-155) are replaced by the Steps-tier test in
  `RulesPage.test.tsx` ("the Steps tier reaches the codec, quality and container settings"), which checks
  codec options, the quality default and the container options on the step editor. The languages and
  subtitle-mode checks are not reproduced there: the RulesPage test palette has no step that carries them,
  so no test in this PR shows them reachable on the Steps tier.
- :169 "shows only the advanced editor for a hand-tuned library" removed. Its intent (no easy editor, a
  save sends the target) is covered by "resends the stored target and no easy object for a Steps library"
  (:111) and by "has Library, File types, Rules and Danger zone ... no rule controls" (the `easy-edit` absence).
- :254 "names the exact panel 'Exact settings' and labels options in plain language" removed. Its
  plain-language label intent (`audio` option labels, :264-293) moved to `RulesPage.test.tsx` ("the Easy tier
  names its audio choices in plain language"), which checks the stop labels that now sit on the Rules page.
  The subtitle and container label lists (:283-296) have no reachable equivalent on the Edit page; they are
  part of the Exact panel that is removed. The heading check moves to the section-title test.
- :295 "keeps Save and Cancel in a sticky action bar and gives the sections their titles": kept as "keeps the
  file-type help line and the Save and Cancel bar sticky" (the sticky check and the help line are unchanged).
  The section-title list changed from `Library, File types, Exact settings` to `Library, File types, Rules,
  Danger zone` (the spec's new sections). Why: spec 5.7.
- File-kind tests (:192-251 in HEAD): no assertion changed. The fixture helper gained a `flowSource`
  parameter and `flow_source`, defaulting to 'steps'.

`web/src/LibrariesPage.test.tsx`
- :273 "deletes only after confirmation, promising the files stay on disk" removed from this file. It moved
  to `LibraryEditPage.test.tsx` ("removes the library from the Danger zone after confirmation only"), with
  the same ids (`library-delete-lib-1`, `confirm-delete`, `confirm-delete-cancel`, `confirm-delete-yes`),
  the same dialog copy assertions, and the same single-DELETE check. Why: Remove leaves the row (spec 8.4 item 2).
- New: "offers no Remove on the row: removing moved to the Edit page (task 47)" checks that neither the row nor
  the card button exists, and that the Edit link is still there.

`web/src/RulesPage.test.tsx`
- No existing assertion changed. Three tests added (see above) under "RulesPage holds the rule controls moved
  from Edit (task 47)".

## Test ids

- Kept: `library-edit-save`, `library-edit-cancel`, `library-edit-name`, `library-edit-watch`,
  `library-edit-kind-*`, `library-edit-danger`, `library-delete-<id>` (the Remove trigger keeps its id on the
  Edit page, spec 8.2), `confirm-delete`, `confirm-delete-yes`, `confirm-delete-cancel`.
- Removed with the sliders and the panel (spec says they go): `easy-edit`, `advanced-panel`,
  `advanced-fields`, `advanced-codec`, `advanced-reencode`, `advanced-quality`, `advanced-container`,
  `advanced-audio-mode`, `advanced-languages`, `advanced-subtitles-mode`.
- Removed with the row action: `library-delete-<id>-card`.
- Added: `library-edit-rules`, `library-edit-rules-summary`, `library-edit-rules-link`.

## Not changed, on purpose

- No Go change. No `api.ts` change. `DECISIONS.md`, `ARCHITECTURE.md` and `API.md` untouched. No new dependency.
- `web/src/EasyLibraryForm.tsx:6` (a comment mentioning the Exact settings panel on the easy screen) and
  `web/src/easyPlainLanguage.test.ts` (a comment saying codec names are allowed "only in the Advanced panel")
  are stale comments. Out of scope; task 53's copy sweep is the place to fix them.
- `e2e/mobile.spec.ts` (the Edit page 44 px audit) runs unchanged and passes.

## Verification

- `npm run test` (web): 58 files, 396 tests passed. `make test` exit 0.
- `npm run lint` (`eslint --max-warnings 0`): clean after the rulesSummary export was removed (a react-refresh warning).
- `npm run format:check`: clean. `npx tsc --noEmit`: clean. `npm run build`: clean.
- `go test ./...`: all packages `ok` (no Go changes).
- Playwright (`npx playwright test`), against a binary rebuilt with the new embed (`internal/webui/dist` copied
  from `web/dist`, `go build`), with `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib SHRINK_E2E_PORT=8733`:
  61 passed, 6 skipped (the same file-table skips as tasks 45 and 46).
- `make test` and `make lint` run with the VM's own `make` and `golangci-lint` (in `/home/mb/.local/bin`): both exit 0.

## Summary

1. **Built:** the Edit page is properties only on spec 5.7: Library, File types, a Rules summary with a
   "Change rules" link, the action bar, and a Danger zone with Remove library (same ConfirmDialog test ids).
   The Libraries row no longer has Remove.
2. **Skipped:** nothing in the acceptance list. The Exact-panel subtitle and container label checks have no
   reachable equivalent on the Edit page and were not reproduced (see the changed-assertions list).
3. **Next tasks should know:** the Edit save resends the stored rule (easy when the easy tier owns the flow,
   the stored target otherwise) because the API needs one and an easy save regenerates the flow. Task 48+
   must not assume Edit can change rules. Stale comments are listed above for task 53.
4. **Owner decisions:** spec 8.4 items 1 and 2 were taken at their recommended default (yes), with no reply recorded.
5. **Known gaps:** no test shows the languages and subtitle-mode settings reachable on the Steps tier (see
   the changed-assertions list). The Danger zone's placement on a phone is covered only by the existing Playwright suite.
