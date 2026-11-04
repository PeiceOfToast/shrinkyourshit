# Task 49: Encode and App settings plus AboutCard

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`EncodeSettingsPage.tsx`, `AppSettingsPage.tsx`, `AboutCard.tsx`, and tests.

## Acceptance

- [x] FormSections in the order of 5.11.
- [x] The language help says "Three-letter".
- [x] The App Save sits directly under Server.
- [x] Number input `w-32`.
- [x] The About commit is `shortHash` with a `title`; the date via `formatDateTime`.
- [x] Container instructions in `CodeBlock` with `whitespace-pre-wrap` (Playwright at 390: `pre` scrollWidth ≤ clientWidth).
- [x] The update toggle is a `Checkbox`.
- [x] The QSV "none of your nodes" Banner when no node reports QSV.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/49-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/49-encode-app-settings` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/49-notes.md`.
