# Task 44: Library detail plus FileHistory and JobLogDialog

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`LibraryDetailPage.tsx`, `FileHistory.tsx`, `JobLogDialog.tsx`, `NodeJobLine.tsx` (colours only), and tests.

## Acceptance

- [x] Header actions are Rules, Edit and Scan now (primary).
- [x] Summary Card with four numbers and the rules summary.
- [x] Labels above toolbar controls.
- [x] Retry hidden at 0.
- [x] Pager hidden when `total <= perPage`.
- [x] Status "Shrunk" for satisfied with savings.
- [x] Encoder words.
- [x] `cardsBelow="md"`.
- [x] The dialogs use DialogHeader/Body/Footer.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/44-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/44-library-detail` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/44-notes.md`.
