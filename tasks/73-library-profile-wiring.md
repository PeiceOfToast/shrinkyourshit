# Task 73: Library pages point at a profile

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 5.4 and 5.5, and the library form and detail pages from the v1.9 wave.

## Scope
Modify the library form, the library detail page, their tests, `web/e2e/library.spec.ts`,
`web/src/api.ts`.

## Do
1. The library form loses the embedded encode builder. In its place: a profile dropdown, plus a
   plain language summary of what the selected profile does, rendered from `StepTypes()` and the
   existing describe helpers.
2. The form still carries storage and subpath, watch, schedule, file kinds and enabled.
3. Library detail shows the inherited profile name, a link to Encode settings, and a warning when
   the library has no profile.
4. Any library whose profile has unrepresentable steps raises a dashboard warning naming the
   library and the profile.
5. Existing libraries keep working: the migration in task 70 already gave each one a profile.

## Acceptance
- [ ] Playwright creates a library by choosing a profile and a folder, and the detail page shows
      the profile name.
- [ ] Switching a library from one profile to another changes the summary and, in the task 75
      verification, changes what the encoder actually does.
- [ ] No library page renders an embedded encode builder anywhere.
- [ ] `npm run build` and `npx playwright test` green.

## Notes
Write the plan and any changed assertion in `tasks/73-notes.md`.
