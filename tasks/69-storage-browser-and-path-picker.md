# Task 69: Browse a storage and pick a library path

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.6. The library form from the v1.9 wave (`web/src/LibraryForm` or the equivalent), and the
`/api/v1/storages/{id}/browse` route from task 65.

## Scope
Create `web/src/StorageBrowser.tsx`, `web/src/StorageBrowser.test.tsx`.
Modify the library form and its tests, `web/e2e/library.spec.ts`.

## Do
1. Directory browser: given a storage, list directories only, with breadcrumbs and an up control.
   No file listing, no download, no preview.
2. The library path field becomes a picker: choose a storage, then walk to a folder. A manual
   text entry stays available behind a disclosure, and a typed path that resolves outside any
   mounted storage is refused with an explanation.
3. Show free space and a mounted or unmounted badge for the chosen storage before the library is
   saved.
4. The browse endpoint requires login, so the picker links to the fix when it is off.

## Acceptance
- [x] Playwright picks a folder inside a storage and creates a library with that path.
- [x] A hand typed path outside every mounted storage is refused with a clear message.
- [x] A directory the process cannot read produces a readable error, not an empty list.
- [x] `npm run build` and `npx playwright test` green.

## Notes
Write the plan and any changed assertion in `tasks/69-notes.md`.
