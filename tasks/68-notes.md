# Task 68 notes: Storage page and the authentication gate

## Plan (written before implementation)

Files, in order:

1. `web/src/api.ts`: storage types (`Storage`, `StorageList`, `StorageBody`, `StorageTestResult`) and client functions for the routes in API.md (task 65 table). No new endpoint.
2. `web/src/storageModel.ts` (pure, tested): form state, body builder, field-error parse from `ApiError.raw`, used-by count (library path under `/storage/<name>`), the delete warning text.
3. `web/src/StoragePage.tsx`: list (DataList + Card), add/edit dialog, row actions (test, mount, unmount, edit, remove with ConfirmDialog), read-only banner with reason and link to `/app` when login is off, host policy banner, 501 handling.
4. `web/src/dashboardWarnings.ts`: optional `storage` input; standing warning when an enabled storage exists and login is off. `DashboardPage.tsx` queries storages and settings and passes them in.
5. `web/src/App.tsx` (route `storage`), `web/src/AppShell.tsx` (nav item "Storage", icon `folder`).
6. Tests: `StoragePage.test.tsx`, `storageModel.test.ts`, dashboardWarnings test case, routeTitles case, `web/e2e/storage.spec.ts`, layout and mobile spec route lists.

## Decisions taken without the owner

- Capability signal: the API has no field that says the mount capability is missing (`SHRINK_MANAGE_MOUNTS` is parsed but not read, per task 67 notes). The page shows read-only for login off, and shows a "not available on this server" notice when the list answers `501 not_implemented`. A missing capability otherwise surfaces as a storage's `error` text. Not invented into the API.
- "Mounts exist" for the dashboard warning means at least one storage with `enabled: true` (enabled storages remount at startup).
- "Used by" count is computed client side from `GET /libraries` paths, because the storage response has no such field.
- Name and protocol are read only on edit (the server refuses changes with a 400 field error).
- Storage list polls every 15 s so mount status from the watchdog shows without a reload.
- NFS add form prefills the spec 4.4 default options (`vers=3,soft,timeo=50,retrans=2`) and says why in the help text.

## Changed assertions (spec section 7 requires each one to be recorded)

The nav gained a "Storage" item after "Nodes". Every assertion that lists the nav labels or the route count changed:

- `web/src/AppShell.test.tsx`: both `navLabels` expectations (drawer and rail) gain `'Storage'` after `'Nodes'`.
- `web/src/App.test.tsx`: the nav test goes from 6 links to 7 (`toHaveLength(6)` becomes `7`), the label list gains `'Storage'`, the route case list gains `['/storage', 'storage-page']`, and the test title changes from "six nav routes" to "seven nav routes".
- `web/src/routeTitles.test.tsx`: one case added, `/storage` titled "Storage · ShrinkYourShit".
- `web/e2e/layout.spec.ts`: `NAV_LABELS` gains `'Storage'`; `/storage` joins the `PAGES` audit.
- `web/e2e/mobile.spec.ts`: `/storage` joins the 390 px `PAGES` audit with `settled` on the list or empty state.

No existing `data-testid` is removed or renamed.

## Test ids (new)

Table and card views both render every row, so card-view action buttons carry a `-card` suffix, as the Integrations page does. Tests and e2e use the table ids. Header Add is `storage-add`; the empty-state Add is `storage-add-empty` (both were `storage-add` at first, which Playwright caught).

## Verification

- `web`: `npx vitest run` 67 files, 506 tests, green. `npx tsc --noEmit` clean, `npm run lint` clean, `npx prettier --check .` clean, `npm run build` green.
- Playwright, final: `npx playwright test` on the rebuilt binary, 136 passed, 4 skipped, exit 0 (phone, tablet and desktop). `e2e/storage.spec.ts` 6 of 6. Earlier runs found two defects, both fixed: a duplicate `storage-add` test id (empty state and header), and the e2e lookups picking the hidden table copy on phone.
- Environment: this box's Chromium headless shell needs `libatk-1.0.so.0`, which is not installed system-wide. The run used `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`, which holds the library. No system packages were changed. Without that path every browser test fails at launch, before any assertion.
- `make lint` exit 0 (go vet, golangci-lint, spdx, web lint and format). `make test` exit 0 at the end: go packages all ok (internal/api 431 s, internal/server 437 s, the slow pair), web 67 files / 506 tests.
- Acceptance: all four boxes ticked in `tasks/68-storage-page.md`, each on the test named above.

## Gaps and open questions for the owner

1. Capability signal. The API has no field that says the mount capability is missing, so the Storage page cannot show "capability missing" as its own reason. The page handles login off (read only, with the reason and a link) and a `501 not_implemented` list response (a notice). A missing capability otherwise appears as the storage's `error` text. Proposed: a `mounts` object on `GET /storages` (for example `{available, reason}`) so the page can say "the container lacks SYS_ADMIN" and disable Add. Needs an API.md change, so not made here.
2. `storage_allowed_hosts` has no writer in the UI or the settings API. The host policy banner tells the operator to set it but cannot take them there. Same open question as task 65.
3. The dashboard warning counts enabled shares as "mounts". Confirm that reading.
4. The Storage list polls every 15 s. SSE would be the cleaner signal, but the storage routes do not emit events yet.

## Summary

1. Built: the Storage page (list, add and edit dialog, test, mount, unmount, remove with a library-aware confirm), the read-only gate when login is off, the host policy notice, the login-off dashboard warning, the `/storage` route and nav item, and the storage API client with its types.
2. Skipped: the capability-missing reason (no API signal, see gap 1), per-node storage (spec 4.7, a later wave), the library path picker (task 69), and the browse listing (task 69).
3. Next task (69, browser and path picker) should know: the storage response has no used-by count, so the library count is derived from library paths in the client; the browse route answers `501` until task 69 wires it; the password is write only and its `(unchanged)` placeholder lives on the form.
4. Test ids: card-view action buttons carry `-card`. Tests and e2e use the table ids. The e2e spec picks the visible copy with `filter({ visible: true })`.
5. Environment: Chromium here needs `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib` for libatk. This is a machine setting, not a repo change.
