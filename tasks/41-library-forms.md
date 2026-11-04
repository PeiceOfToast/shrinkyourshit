# Task 41: Library forms (New and Edit, interim)

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`NewLibraryPage.tsx`, `EasyLibraryForm.tsx`, `easyControls.tsx`, `KindPicker.tsx`, `FolderPickerModal.tsx`, `LibraryEditPage.tsx` (5.7 *interim*), `index.css` (`.range`), and their tests.

## Acceptance

- [x] FormSection layout; the field column is capped at `max-w-2xl`.
- [x] "Add library" button; Cancel.
- [x] Slider stop labels: under `sm` only three are visible, while all test ids remain.
- [x] Checkboxes use `Checkbox`.
- [x] The Edit page's Advanced panel becomes "Exact settings" with human option labels (values unchanged; `LibraryEditPage.test.tsx` asserts values, not labels; check).
- [x] Sticky action bar on mobile.
- [x] The mobile audit passes on both routes (added to `mobile.spec.ts` here).

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/41-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/41-library-forms` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/41-notes.md`.
