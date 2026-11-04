# Task 40: App shell and EncodingStatus

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`AppShell.tsx`, new `EncodingStatus.tsx` and test, `AppShell.test.tsx`, `e2e/layout.spec.ts` (nav labels), `e2e/mobile.spec.ts` (only if a selector referenced the header).

## Acceptance

- [x] No `<header>` at `md+`.
- [x] The rail is sticky and keeps `w-56`/`w-16` and the `sys.rail` behaviour.
- [x] The rail footer holds EncodingStatus, logout (`data-testid=logout`) and version (`data-testid=version`).
- [x] Nav labels are Dashboard, Libraries, Nodes, Encode settings, Integrations, App settings.
- [x] `job-indicator` shows "1 running" for one running job.
- [x] The switch shows on = running and calls pauseAll/resumeAll.
- [x] The mobile top bar is 56 px with the menu, brand and compact pill, without the label "Pause all encoding".
- [x] The drawer contains nav, the EncodingStatus switch and logout.
- [x] A skip link exists.
- [x] Test updates: `AppShell.test.tsx:174-175, 220, 287-288` and `e2e/layout.spec.ts:25-26` ('Dash' → 'Dashboard', 'Library' → 'Libraries').

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/40-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/40-app-shell-encoding-status` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/40-notes.md`.
