# Task 37: Control classes and formatting helpers

Part of the v1.9 UI redesign. The specification is `docs/design/ui-redesign-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below and sections 2 to 6 as the reference, plus `AGENTS.md`, `DECISIONS.md`,
`STACK.md` and `ARCHITECTURE.md`.

Where the spec gives a class string, use that exact string.

## Scope

`web/src/uiControls.ts`, `uiControls.test.ts`, new `web/src/format.ts` and `format.test.ts`, `easyModel.ts` (re-export `formatBytes` and `formatEta`), new `useDocumentTitle.ts` and test.

## Acceptance

- [x] Exports exactly as in 2.10. The test asserts every control string contains `min-h-11` and contains none of `slate-`, `emerald-`, `red-`, `rose-`, `!`.
- [x] `format.test.ts` covers bytes (0, 551 000, 15 185 350, 1.2e12), counts (43655 → "43,655" with `en-US` forced), relative times (5 s, 4 min, 3 h, 2 d), `shortHash`, and `encoderLabel` (cpu, qsv, vaapi, nvenc, videotoolbox, unknown passthrough).
- [x] Existing `easyModel.test.ts` passes unchanged.

## Rules for this wave

- Test updates: you may change an existing assertion only where the spec changes the asserted text
  or class, and you must list each changed assertion in `tasks/37-notes.md`
  (file:line, old to new, why). Never delete an assertion without a replacement that checks the
  same intent.
- Every existing `data-testid` survives unless the spec says otherwise.
- No Go changes and no `api.ts` request or response shape changes. `DECISIONS.md` is read-only.
  If a task genuinely needs a new API field, tick nothing, record the question in the notes file,
  set STATUS.md to `blocked` and stop.
- Branch `task/37-control-classes-format-helpers` from `main`, one PR, CI green. `make test`, `make lint`,
  `npm run build` and `npx playwright test` green before you tick the checklist.

## Notes

Write the plan, the list of changed assertions and the 5-line summary in `tasks/37-notes.md`.
