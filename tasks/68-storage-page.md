# Task 68: Storage page and the authentication gate in the UI

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.6 and section 5 of `docs/design/ui-redesign-2026-11-04.md` for the design system. The
existing `web/src/AppSettingsPage.tsx` and `EncodeSettingsPage.tsx` for form patterns.

## Scope
Create `web/src/StoragePage.tsx`, `web/src/StoragePage.test.tsx`, `web/e2e/storage.spec.ts`.
Modify `web/src/App.tsx` (route `/storage`), the nav, `web/src/api.ts`.

## Do
1. List view: name, protocol, source, mounted state, last error, used-by library count.
2. Add and edit form: protocol, source, options with the allowlist surfaced as hints, read only,
   SMB username, domain and password. Password is write only and never re-displayed.
3. Row actions: test, mount, unmount, delete with a confirm that says what happens to libraries
   pointing into it.
4. When login is disabled or the capability is missing, the page renders read only with the exact
   reason and a link to the fix. This is the visible half of spec 4.6.
5. Dashboard: a standing warning when mounts exist and login is disabled.
6. Mobile first at 390 px, 44 px targets, toast on every mutation, per the design system.

## Acceptance
- [ ] Playwright covers add, test, mount and the read only state when login is off.
- [ ] No secret is ever rendered or returned to the browser, proven by a test that fails if the
      API response carries it.
- [ ] The dashboard warning appears when mounts exist and login is disabled.
- [ ] `npm run build`, `npx playwright test` and `make lint` green.

## Notes
Write the plan and any changed assertion in `tasks/68-notes.md`.
