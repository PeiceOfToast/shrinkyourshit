# Task 49 notes: Encode and App settings plus AboutCard

Spec: `docs/design/ui-redesign-2026-11-04.md` 5.11 and 4.3, with 2 to 4 as the reference (3.6 FormSection and FormActions,
3.7 Banner and ErrorBox, 2.10 control strings, 4.1 Checkbox and CodeBlock).

## Plan

1. `AboutCard.tsx`: becomes the "About and updates" FormSection (testid `about-card` kept). dl rows, short commit with
   `title`, date via `formatDateTime`, Checkbox for the automatic-check toggle, `Banner tone="info"` for the update box,
   `CodeBlock copy` for the container commands, `Field` plus `select max-w-xs` for the channel.
2. `EncodeSettingsPage.tsx`: PageHeader; FormSection "Library defaults" (languages help "Three-letter ..."), FormSection
   "Intel Quick Sync extras" with the QSV Banner when no node reports `qsv`, then the save error and FormActions
   "Save changes".
3. `AppSettingsPage.tsx`: PageHeader; FormSection "Server" (number `w-32`, scratch folder with help), then the save
   error and FormActions directly under it; FormSection "Login" (same flows and testids, danger confirm uses `btnDanger`);
   then `AboutCard`.
4. Tests: add new tests for each acceptance item; change only the two assertions the spec changes (listed below).
5. `web/e2e/mobile.spec.ts`: one Playwright check that no `pre` on /encode or /app clips at 390 px.

## Changed assertions

Format: file:line (original line in `HEAD`), old to new, why.

- `web/src/AboutCard.test.tsx:90-92` `toHaveTextContent('2026-01-02T03:04:05Z')` to
  `toHaveTextContent(formatDateTime('2026-01-02T03:04:05Z'))`. Why: the About date is now formatted with `formatDateTime`
  (spec 4.1, acceptance item 5). The new assertion imports the same formatter, so it does not hard-code the locale.
- `web/src/AppSettingsPage.test.tsx:173` (the 44 px loop) `getAllByRole('checkbox')` removed from the `min-h-11` loop and
  replaced by a loop asserting `h-11` and `w-11` on each checkbox. Why: the update toggle is now a `Checkbox`, whose 44 px
  transparent input is sized with `h-11 w-11` and has no `min-h-11` (spec 4.1 Checkbox markup, acceptance item 7). The
  intent is unchanged: every checkbox is a 44 px touch target.

No assertion was deleted. Text changes that no test asserted: "Save settings" to "Save changes", "Hardware extras (Intel
Quick Sync)" to "Intel Quick Sync extras", "Scratch folder for temporary work" to "Scratch folder", the update box and
the Login buttons' danger colour.

## New tests (acceptance coverage)

- Item 1 (order): `EncodeSettingsPage.test.tsx` h2 order `Library defaults`, `Intel Quick Sync extras`;
  `AppSettingsPage.test.tsx` h2 order `Server`, `Login`, `About and updates`.
- Item 2 (help): `EncodeSettingsPage.test.tsx` exact text "Three-letter language codes, comma separated (for example eng, deu)."
- Item 3 (Save under Server): `AppSettingsPage.test.tsx` DOM order Server section, Save, Login (`compareDocumentPosition`).
- Item 4 (w-32): `AppSettingsPage.test.tsx` `worker-concurrency` has `w-32`. See the layout caveat below.
- Item 5 (About commit and date): `AboutCard.test.tsx` short hash text, full hash in `title`; the date test above.
- Item 6 (container code block): `AboutCard.test.tsx` the `pre` inside `container-instructions` has `whitespace-pre-wrap`;
  `mobile.spec.ts` Playwright check on /encode and /app at 390 px (see the caveat below).
- Item 7 (Checkbox): `AboutCard.test.tsx` `update-check-toggle` is `type=checkbox` with `h-11 w-11`.
- Item 8 (QSV banner): `EncodeSettingsPage.test.tsx` shown when the nodes report only `cpu`, hidden when one reports `qsv`.

## Judgement calls (not in the spec)

- **No help text on "Files processed at once".** The spec makes the help conditional on `worker_concurrency` being the
  built-in node's default. It is not: nothing reads it at runtime (see the owner questions). I left the label as the spec
  says and added no help.
- **`Banner neutral` is `tone="info"`.** Banner has no `neutral` tone (tones: info, success, warning, danger; spec 3.7 lists
  the same four). `info` is the calm tone; no Banner code was changed.
- **No Reset button.** Spec 3.6 says "Cancel" or "Reset" when a form is dirty. Spec 5.11 names only "Save changes" for
  these pages, and neither page tracks dirty state today, so I did not add one.
- **Save error placement.** The save error renders above the action bar, per 3.7 (mutation errors where the button is).
- **Loading and error states** use `SkeletonCard` and `ErrorBox onRetry` (3.7). The PageHeader renders in every state.
- **Login flows unchanged.** Same testids, same confirm steps. Only the danger button changed to `btnDanger`, which removes
  the `!bg-red-600` override (2.10: no `!important` overrides remain).
- **Scratch folder label** is "Scratch folder", as spec 5.11 writes it.

## Owner questions (not blocking this task's acceptance)

1. **`worker_concurrency` has no runtime reader.** `internal/server/settings.go` saves it and `NodeRegistry.Concurrency`
   (`internal/server/node.go:346`) reads it, but nothing calls `Concurrency`. Workers use the per-node cap
   (`node.Concurrency`, the Nodes page "Parallel jobs"). So "Files processed at once" on App settings may do nothing.
   Either wire it to the built-in node's default or remove it from the page. Spec 5.11 asks for confirmation before the
   help text is written. I did not change Go (task rule: no Go changes here).
2. **`w-32` does not win over `w-full`.** `input` (2.10) carries `w-full`. In the built CSS `.w-32` (offset 7645) comes
   before `.w-full` (offset 7894), so `w-full` wins and the number input renders full width, not 8rem. The acceptance
   item is met as literally written (the class is present), but the visual is not. NodesPage's throughput input
   (`${input} w-32`, `NodesPage.tsx:289`) has the same problem. Fix options: an owner
   decision on `input`/`w-32` in `uiControls.ts` (frozen by 2.10), or a spec edit.
3. **Spec 5.11 says `Banner neutral`, but Banner has no neutral tone.** See the judgement call above.

## Verification

- `web`: `npm run test` 58 files, 407 tests passed (was 399 at task 48; +8 new, 0 deleted).
- `npx tsc --noEmit`: clean. `npm run lint`: clean (`--max-warnings 0`). `npm run format:check`: clean (Prettier wrote
  five touched files).
- `npm run build`: clean. Built dist copied to `internal/webui/dist` and `bin/shrinkyourshit` rebuilt with `go build`
  (both gitignored; `make` is not installed on this machine, so the Makefile steps were run by hand).
- `go test ./...`: all packages ok. No Go source changed.
- `npx playwright test` against the rebuilt binary: 62 passed, 6 skipped. The 6 skips are the file-table tests, which
  skip because ffmpeg is not installed here. The new `code blocks at phone width` test passes.
- Not done: a visual check in a real browser at 1440 and 390 px. The e2e suite covers layout and touch targets only.
  The container `pre` wrap is not exercised in e2e (the e2e server is a binary install and never renders the container
  commands), so the Playwright check passes vacuously for that block; the unit test is the real check for it.
- No `api.ts` change, no `DECISIONS.md` change, no new dependency.

## Summary

1. **Built:** the Encode settings and App settings pages and AboutCard on spec 5.11 and 4.3. FormSections in spec order;
   Save under Server; "Three-letter" help; QSV notice when no node reports Quick Sync; About uses shortHash, formatDateTime,
   Checkbox, CodeBlock copy, and Banner info. Danger confirm uses `btnDanger`.
2. **Skipped:** the conditional worker_concurrency help text (the setting has no runtime reader, see owner question 1);
   the Reset button (not named in 5.11); the browser visual check.
3. **Next tasks should know:** `input w-32` does not render 8rem because `w-full` comes later in the CSS (owner question
   2). The same is true of NodesPage's throughput input. The "Banner neutral" in the spec is `info` here. Task 53's
   touch-target audit should keep the Checkbox assertion as `h-11`/`w-11`, not `min-h-11`.
