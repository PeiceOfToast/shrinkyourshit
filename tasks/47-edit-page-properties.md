# Task 47: Edit page becomes properties only (needs owner OK, see 8.4)

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`LibraryEditPage.tsx`, `LibraryEditPage.test.tsx`, `LibrariesPage.tsx` (Remove leaves the row, if not done in 43), `RulesPage.test.tsx`.

## Acceptance

- [ ] The Edit page has FormSections Library, File types, Rules (summary plus link) and Danger zone.
- [ ] The easy sliders and the Exact settings panel are gone from Edit.
- [ ] Their tests move to `RulesPage.test.tsx` or are replaced by Steps-tier tests proving the same settings are reachable.
- [ ] Remove library works from the Danger zone with the same ConfirmDialog test ids.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/47-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/47-edit-page-properties` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/47-notes.md`.
