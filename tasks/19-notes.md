# Task 19 notes: library list and detail

## Plan

Read `docs/ui-review-2026-10-29.md` sections 4 and 8, `API.md` (tasks 03, 07,
19), ARCHITECTURE.md, DECISIONS.md, STACK.md. Dependencies: 13 and 18 (done).

Files to touch, in order:

### Backend

1. **Last scan time (store).** The detail header must show the last scan
   time, but nothing records it today. Add `libraries.last_scan_at` via
   migration `0009_library_last_scan.sql` (`TEXT NOT NULL DEFAULT ''`),
   `Library.LastScanAt` in the store, a focused `SetLibraryScanTime(ctx, id,
   time)` store method (so the scanner never rewrites a whole library row
   from a stale copy), and a scanner hook that stamps it when `ScanLibrary`
   finishes without error. `LibraryDTO` gains `last_scan_at` (RFC 3339,
   omitted when never scanned). ARCHITECTURE.md data model updated in the
   same commit (AGENTS rule 5); API.md task 19 gains a bullet.
2. **Files search and sort (API).** `GET /api/v1/libraries/{id}/files` gains
   `q` (case-insensitive substring of the file's basename), `sort`
   (`name`, `size`, `saved`) and `dir` (`asc`, `desc`); unknown values are
   `400 invalid_request`; empty `sort` keeps the existing created_at order
   (dir ignored, so existing clients are unaffected). Filtering and sorting
   happen in the handler over the loaded slice — the same in-memory pattern
   the stats service documents for home-lab scale, and the browser never
   re-sorts. `FileDTO` gains `last_job_encoder` (best-effort job lookup for
   the page's files only, like task 18's per-job file lookup).
3. **Per-file job history (API).** `GET /api/v1/files/{id}/jobs`: 404 for an
   unknown file, otherwise the existing `JobFilter` with `FileID` set,
   oldest first, same status/order/limit/offset parsing as `GET /jobs`.

### Frontend

4. **api.ts**: `FileItem.last_job_encoder`, `Library.last_scan_at`,
   `getFiles` gains `q`/`sort`/`dir`, new `getFileJobs`.
5. **LibrariesPage**: real table on `sm` and up (name links to detail,
   files, satisfied/queued/failed/done chips, bytes saved, Scan now, Edit,
   Remove), stacked cards under `sm` with the task 12 pattern. Update
   LibrariesPage.test.tsx to the new shape.
6. **LibraryDetailPage**: header summary (per-status counts + bytes saved
   from `/stats`, plain-language target rendered from
   `POST /flows/preview {graph}` — the easy mode description path, last
   scan time). File table gains size before (size + bytes_saved), size
   after, ratio, encoder used and a job-log link. Search box and sort
   selector wired server-side; pagination kept.
7. **FileHistory.tsx** (new): native-dialog panel per file over
   `GET /files/{id}/jobs` — attempts, status, encoder, fallback reasons,
   bytes before/after, log tail behind a `<details>` disclosure; empty and
   error states per task 13.
8. **e2e**: `/libraries/{id}` added to the layout suite at 390/768/1280
   (skips when the seeded library is absent), `/libraries` already covered.

### Verification

`make test`, `make lint`, `make test-integration` (ffmpeg present on this
VM), `make e2e` if chromium can be installed here (the VM had no browser in
past tasks; if it cannot start, record that and let CI's e2e job decide).

## Progress log

(written at the end)

## Summary

(written at the end)
