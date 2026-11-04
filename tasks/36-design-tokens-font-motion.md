# Task 36: Design tokens, font, motion

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`web/src/index.css`, `web/tailwind.config.js`, `web/package.json` and `package-lock.json` (add `@fontsource-variable/inter`), `web/src/main.tsx` (font import), `STACK.md` (add a "Fonts" row: Inter Variable via @fontsource, self-hosted, OFL-1.1), `tasks/36-notes.md` (dependency justification, copied from section 8).

## Acceptance

- [x] All variables from 2.9 exist and the Tailwind config matches it.
- [x] New `src/tokens.test.ts` imports `tailwind.config.js` and asserts that the colour keys `canvas, surface, raised, line, fg, accent, focus, success, info, warning, danger`, the font sizes `caption, body, heading, title` and the durations `fast, base, slow` exist.
- [x] The focus ring is sky-300; the reduced-motion rule is present; React Flow variables are set (verify the names against the installed CSS).
- [x] `npm run build` passes; the bundle includes one Inter woff2 (latin).
- [x] No component file changed apart from `main.tsx`.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/36-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/36-design-tokens-font-motion` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/36-notes.md`.
