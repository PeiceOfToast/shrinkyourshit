# Task 13 notes: Feedback, safety and empty states

## Plan

Files touched, in order:

1. Shared primitives (new): `web/src/toast.tsx` (provider + `useToast`), `web/src/dialog.tsx`
   (`ModalDialog` on a native `<dialog>`: showModal, Escape, focus restore, backdrop click),
   `web/src/ConfirmDialog.tsx` (on ModalDialog), `web/src/ErrorBox.tsx` (status + code +
   raw JSON disclosure), `web/src/skeleton.tsx`.
2. `api.ts`: `ApiError` keeps the raw response body (for the disclosure).
3. `test/setup.ts`: minimal `showModal`/`close`/`show` polyfill (the pinned jsdom 25 has
   none at all — verified on the VM).
4. `index.css`: one global `:focus-visible` ring + `dialog::backdrop` colour.
5. `easyModel.ts`: `truncateMiddle` (folder picker footer; start and end visible, middle elided).
6. Pages: toasts on every mutation, shared ConfirmDialog for delete-library / cancel-job /
   stop-node / reject-node, ErrorBox for load and save failures, skeletons for tables and
   stat cards, file-list pagination (page size + prev/next, status filter kept),
   FolderPickerModal and the node dialogs rebuilt on ModalDialog, drawer focus return.
7. Backend: the two 405 paths in `internal/api/api.go` answered plain text with no `code`;
   they now use `writeError(..., "method_not_allowed", ...)`. Every other 4xx/5xx already
   went through `writeError`/`a.internal` (grep verified) — no other contract change.
8. Tests per the acceptance list, mapped to the real file names (see mismatches below).

## Doc mismatches (recorded, not blocking)

- The task's acceptance names `LibraryList.test.tsx` and `SettingsPage.test.tsx`. Task 10
  split `SettingsPage` into `EncodeSettingsPage` + `AppSettingsPage` and the library list
  page is `LibrariesPage`. The acceptance intent (a test per page; a settings page tested
  with a mocked 500) is implemented in `LibrariesPage.test.tsx`,
  `EncodeSettingsPage.test.tsx` and `AppSettingsPage.test.tsx`.
- "Cancel a running job" had no caller in the UI (`cancelJob` in api.ts was unused). The
  cancel affordance lives on the library detail file rows for queued/processing files with
  a job id, with the required confirmation text. The dashboard stays read-only (task 10).

## Decisions inside the task's freedom

- Toasts: success auto-dismisses after 4 s, failure after 7 s; failure text is
  `"<Action> (<status>, <code>): <server message>"`. Login/setup keep their inline error
  text (form-adjacent, specific: 429 vs 401) and additionally toast on failure.
- Forms (library edit, easy form, node edit) show the persistent ErrorBox next to the
  submit button *and* toast on failure; the dialog stays open.
- Native `<dialog>`: `showModal()` provides the focus trap and the Escape `cancel` event in
  real browsers; the component additionally listens for keydown Escape (the path jsdom can
  drive) and restores focus to the previously focused element on close, which also covers
  browsers without showModal. jsdom gets a three-line polyfill in test setup so the
  components run against the real code path.
- Node dialogs and the nav drawer do not close on backdrop click when they hold form state
  (`dismissOnBackdrop=false`); confirmations and the folder picker do.
- Retry-failed fetches failed files server-side (`?status=failed&limit=1000`) instead of
  only the visible page, so pagination cannot hide failed files from the retry action.

## Out of scope (not built)

No page redesign, no new features, no API response changes beyond the 405 `code` field,
no axe-core dependency (manual keyboard pass documented in tasks/hermes-log.md instead).

## Summary (written at the end)

- Built: toast provider used by every mutation; ConfirmDialog (native `<dialog>`) on
  delete-library, reject-node, stop-node and cancel-job with the required "what happens"
  wording; ErrorBox (status + code + raw JSON disclosure) replacing one-line failures;
  skeletons for tables and stat cards; file-list pagination with page size + prev/next;
  folder picker Escape + focus return + middle truncation; global focus-visible ring;
  405 error bodies now carry `code`.
- Skipped: no UI redesign, no new endpoints beyond the code field, no axe dependency.
- Test notes: jsdom has no dialog implementation, so `test/setup.ts` polyfills
  showModal/show/close; Escape is driven via keydown (the component keeps a keydown
  listener for exactly this). Acceptance file names `LibraryList.test.tsx` /
  `SettingsPage.test.tsx` map to `LibrariesPage.test.tsx` and the two settings page tests.
- Environment: Playwright e2e cannot launch on this VM (chromium system libs missing —
  the same limitation tasks 10 and 12 recorded), so the layout suite runs in CI only;
  the keyboard pass is structural + unit-tested, recorded in tasks/hermes-log.md.
- Next task should know: `useToast()` requires `<ToastProvider>` (main.tsx mounts it once,
  above the router); test render helpers must wrap it too. `ModalDialog` in
  `web/src/dialog.tsx` is the one wrapper to reuse for any future dialog (task 14/15), and
  `truncateMiddle` lives in easyModel.ts.

## Status

Done. PR #14: https://github.com/PeiceOfToast/shrinkyourshit/pull/14
