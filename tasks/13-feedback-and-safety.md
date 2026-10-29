# Task 13: Feedback, safety and empty states

## Goal
Every action tells you what happened, nothing destructive happens without a confirmation that says what will be deleted, and errors are specific enough to act on.

## Read first
UI review at `docs/ui-review-2026-10-29.md` section 7. Depends on tasks 10 and 12.

## Do
- Toasts: one small provider in `web/src/toast.tsx`, used by every mutation (save library, scan, delete, start or stop node, settings save, pairing). Success is short and specific; failure includes the HTTP status and the server's `code`.
- Confirmation dialogs: a shared `ConfirmDialog` built on a native `<dialog>` element so focus trapping and Escape come from the platform.
  - Delete library: state plainly that this removes the library and its job records from the database and never touches files on disk.
  - Cancel a running job: state that the partially encoded scratch file is discarded and the original is untouched.
  - Stop a node mid job: state what happens to the current file.
- Error surfaces: replace one line failure text ("Cannot load the settings.") with the status code, a short human reason and the raw JSON behind a disclosure.
- Loading states: skeletons for tables and stat cards, so a slow server does not look broken.
- Folder picker modal: Escape closes it, focus returns to the trigger, and the current path is shown in full with truncation only in the middle.
- File list on the library detail page: page size selector plus previous and next, and keep the status filter. At 40k files this matters.
- Accessibility pass on what exists: visible focus rings, every input labelled, dialogs with `role` and a trap, buttons that name their action rather than "OK".
- Vitest: toast appears on a successful mutation; delete asks first and only calls the API on confirm; Escape closes the folder picker.

## Out of scope
Redesigning pages, new features, changing API responses beyond adding a `code` field where it is missing.

## Acceptance
- [x] Every mutation in the app surfaces a toast, verified by a test per page. (`LibraryList.test.tsx`, `LibraryDetailPage.test.tsx`, `NodesPage.test.tsx`, `SettingsPage.test.tsx`)
- [x] Deleting a library requires confirmation, and the dialog text says files on disk are untouched. (`LibrariesPage.test.tsx`)
- [x] Escape closes every dialog and focus returns to the element that opened it. (`FolderPickerModal` test, `ConfirmDialog` test)
- [x] A failed request renders the HTTP status and the server error code, not a generic sentence. (`SettingsPage.test.tsx` with a mocked 500)
- [x] The file list has working previous and next controls and a page size selector. (`LibraryDetailPage.test.tsx`)
- [x] Axe or a manual keyboard pass documented in `tasks/hermes-log.md`: tab order reaches every control, no focus traps outside dialogs.

(The two test-file names in the first and fourth items predate task 10's settings split: `LibraryList.test.tsx` is `LibrariesPage.test.tsx`, and `SettingsPage.test.tsx` is the pair `EncodeSettingsPage.test.tsx` + `AppSettingsPage.test.tsx`. Recorded in tasks/13-notes.md.)
