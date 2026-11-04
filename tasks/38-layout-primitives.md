# Task 38: Layout primitives

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

new `Icon.tsx`, `PageHeader.tsx`, `Card.tsx`, `FormSection.tsx`, `EmptyState.tsx`, `Banner.tsx`, `Disclosure.tsx`, each with `*.test.tsx`; `PageContainer.tsx` (max-w-6xl and the default gap).

## Acceptance

- [ ] PageHeader renders one h1, breadcrumb `nav[aria-label=Breadcrumb]` with links, a skeleton when `title` is undefined, and actions in order.
- [ ] Field wires `aria-describedby` and `aria-invalid`.
- [ ] EmptyState renders title, body and action.
- [ ] Banner has `role="status"` for info and success and `role="alert"` for danger.
- [ ] Disclosure toggles via the keyboard.
- [ ] Every page still renders (the existing page tests pass with the new PageContainer).

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/38-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/38-layout-primitives` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/38-notes.md`.
