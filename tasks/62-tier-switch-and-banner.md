# Task 62: Tier switch, downgrade banner and read only rendering

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec sections 2 decision 4, 6 and 7 (tier switch). The tier endpoint from task 59, and the
library detail page.

## Scope

Modify: the library detail page component, `web/src/App.tsx` if routing changes,
`web/src/api.ts` helpers, `web/src/EncodeSettingsPage.tsx` (the `default_rule_tier` and
`allow_raw_args` controls), their tests, `web/e2e/library.spec.ts`.

## Do

1. Three way tier control on the library detail page. Changing tier calls the tier endpoint.
2. When the response's `unrepresentable` list is non empty, show a banner that names each step and
   the lowest tier that can edit it, and state plainly that the flow was not modified.
3. While that list is non empty, the lower tier editor renders read only: fields disabled, a clear
   reason, and a link to the tier that can edit it. Save must be impossible, not merely discouraged.
4. Encode settings page gains `default_rule_tier` and the `allow_raw_args` switch, in the existing
   rule tier section, with validation feedback before submit.
5. Every mutation gets a toast, per the UI contract.

## Out of scope

No graph rewriting of any kind. If a step cannot be represented, it stays in the graph untouched.

## Acceptance

- [ ] An advanced graph with a `raw_args` step, downgraded to Basic, shows the banner naming that
      step, and the Basic form is disabled with a visible reason.
- [ ] The same library upgraded back to Advanced shows an editable canvas, with the step still
      present and unchanged.
- [ ] Playwright test covers the round trip downgrade then upgrade and asserts the step survived.
- [ ] `npm run build`, `npx playwright test` and `make lint` green.

## Notes

Write the plan and any changed assertion in `tasks/62-notes.md`.
