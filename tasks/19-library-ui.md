# Task 19: Library list and detail

## Goal
The library pages become the place the owner actually manages work: savings at a glance on the list, and a detail page that shows what happened to each file, with search, sorting and per file job history.

## Read first
`docs/ui-review-2026-10-29.md` sections 4 and 8, `API.md` (task 03, 07 and 19). Depends on tasks 13 and 18.

## Do
- List page `web/src/LibrariesPage.tsx`:
  - Each row shows files, satisfied, queued, failed and bytes saved from the per library breakdown in `GET /api/v1/stats`.
  - Status chips, a Scan now action per library, and the library name links to detail.
  - Under `sm` the table becomes cards, using the same pattern as task 12.
- Detail page `web/src/LibraryDetailPage.tsx`:
  - Header summary: file counts by status, bytes saved, the current target described in plain language (reuse the easy mode description path), and the last scan time.
  - File table adds size before, size after, ratio, encoder used, and a link that opens the job log.
  - Server side search by name and sort by size, savings or name. Extend the existing files endpoint with `q`, `sort` and `dir`, validated (`400 invalid_request` on an unknown sort), never sorted in the browser.
  - Keep pagination, add page size and a real total count.
- Per file job history:
  - New `GET /api/v1/files/{id}/jobs` using the existing `JobFilter` shape (`FileID` is already supported).
  - UI panel per file: attempts, status, encoder, fallback reasons, bytes before and after, and the log tail behind a disclosure.
- Empty and error states per task 13 conventions: one line plus a link, errors with status and code.

## Out of scope
Charts over time, per file bulk actions, the flow editors.

## Acceptance
- [ ] The library list shows per library counts and bytes saved, matching `GET /api/v1/stats`. (`LibrariesPage.test.tsx`)
- [ ] The detail header shows status counts, bytes saved, the plain language target and the last scan time. (`LibraryDetailPage.test.tsx`)
- [ ] The file table shows before, after, ratio, encoder and a working job log link. (`LibraryDetailPage.test.tsx`)
- [ ] `q`, `sort` and `dir` filter and order server side, an unknown sort returns 400, and paging reports the real total. (`TestLibraryFilesSearchAndSort`)
- [ ] `GET /api/v1/files/{id}/jobs` returns that file's jobs oldest first and 404s for an unknown file. (`TestFileJobsEndpoint`)
- [ ] The per file panel renders attempts, encoder, fallback reasons and the log tail. (`FileHistory.test.tsx`)
- [ ] Playwright passes at 390, 768 and 1280 on `/libraries` and `/libraries/{id}` with no overflow.
- [ ] `API.md` documents the new query parameters and the file jobs endpoint.
