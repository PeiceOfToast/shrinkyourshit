# Task 53: Guardrail lint and audit

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`web/eslint.config.js` (a `no-restricted-syntax` regex rule on `className` literals for the retired classes in 2.2), `web/e2e/mobile.spec.ts` (add the new routes plus a single-h1 check), `web/e2e/layout.spec.ts`, `ARCHITECTURE.md` (the UI routes section: a one-paragraph pointer to this spec and the token rule).

## Acceptance

- [x] `npm run lint` fails on a sample `text-slate-500` (prove it in a throwaway commit, then revert). Proof: `fa5d9cf` fails lint, `f94e5be` reverts it.
- [x] The audit covers `/`, `/libraries`, `/libraries/new`, `/libraries/:id`, `/libraries/:id/edit`, `/libraries/:id/rules`, `/libraries/:id/flow`, `/nodes`, `/encode`, `/app` and `/integrations` at 320, 390, 414 and 768. `e2e/mobile.spec.ts` passed at all four widths.
- [x] Zero raw palette classes remain outside `index.css` and the `App.tsx` mirror. Grep is in `tasks/53-notes.md`.

Order rationale:
- Tokens (36) and classes (37) change nothing structural but make every later diff small.
- Primitives (38, 39) come before the shell, because the shell uses Icon, Switch and Badge.
- The shell (40) comes before pages, because every page screenshot changes with it.
- Pages go from highest traffic (dashboard, libraries) to least, except 41 first: it fixes the most visible form problem and exercises FormSection early.
- 47 waits for the owner.
- 52 and 53 lock it in.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/53-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/53-guardrail-lint-audit` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/53-notes.md`.
