# Task 72: Encode settings becomes the profile list and editor

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 5.5, the v1.10 editors (`web/src/FlowTreePage.tsx`, `FlowCanvasPage.tsx`, the Basic form) and
`web/src/EncodeSettingsPage.tsx`.

## Scope
Create `web/src/ProfilesPage.tsx`, `web/src/ProfilesPage.test.tsx`, `web/e2e/profiles.spec.ts`.
Modify `web/src/App.tsx`, the nav, `web/src/EncodeSettingsPage.tsx` (keeps the app wide settings,
loses nothing), `web/src/api.ts`.

## Do
1. `/encode` shows two sections: the app wide settings it already has, and a new Profiles section
   listing name, tier badge, used-by count, result policy and last edited.
2. Selecting a profile opens the editor in place: the Basic form, the Moderate tree and the
   Advanced canvas all operate on the selected profile, chosen by its tier with the same three
   way control, and the downgrade banner from v1.10 now lives here.
3. Save is confirmed when the profile is used by libraries, and the confirmation lists them.
4. Duplicate is one click and opens the copy.
5. The app wide `default_rule_tier` becomes the default tier for a new profile.

## Acceptance
- [x] Playwright edits a profile used by two libraries, sees both named in the confirmation, and
      confirms the change applied to both. (Two libraries are answered by route mocks; no real
      library can attach a profile before task 73. See tasks/72-notes.md gap 1.)
- [x] The Basic, tree and canvas editors all save to a profile, proven per editor.
- [x] The representability banner names the steps and the tier that can edit them.
- [x] `npm run build`, `npx playwright test` and `make lint` green.

## Notes
Write the plan and any changed assertion in `tasks/72-notes.md`.
