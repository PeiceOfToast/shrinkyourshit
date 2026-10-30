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

- Backend commits landed one per unit: last scan time (migration 0009,
  store, scanner, DTO, docs), files search/sort/encoder, file jobs endpoint.
- `TestLibraryFilesSearchAndSort` needed two expectation fixes: the seeded
  sizes sort inside→alpha, and stable sorting keeps store order on ties
  (documented in the test).
- Web: `StatusChip` extracted and shared by the list and the detail header;
  card variants of list testids carry the `-card` suffix (task 12 pattern).
- `easyModel.STATUS_LABELS` gained the job-only labels (claimed, running,
  cancelled) so the history panel speaks plain language too.
- `make test` exit 0 (Go + 153 web tests in 23 files), `make lint` exit 0
  (golangci 0 issues, eslint 0 warnings, prettier clean).
- `make test-integration`: one flake — `TestNodeStartTakesEffectOnHeartbeat`
  timed out at 64 s during the full-suite run (VM saturated by the many
  concurrent server+worker+ffmpeg stacks); it passes in isolation (4.5 s)
  and the whole worker integration package passes on re-run (113 s). Same
  timing-test behaviour the repo has seen before; not a regression — the
  task touches nothing on the claim path.
- e2e: this VM cannot launch chromium (`libatk-1.0.so.0` missing, no
  sudo/apt — the standing limitation tasks 10, 12 and 13 recorded), so the
  first verdict came from CI — and it caught a real bug: the detail page
  overflowed by 109 px at tablet (877 > 768). A temporary diagnostic walk
  in the layout suite named the offender from the CI log: the
  `sr-only` span in the Actions `<th>` — `position: absolute` with no
  positioned ancestor, so its containing block is the document and it
  paints at the table's far edge (right = 877) OUTSIDE the
  `overflow-x-auto` clip. The old table carried the same latent bug; the
  new Before/After/Ratio/Encoder columns pushed the table wide enough to
  expose it. Fix: `relative` on both scroll wrappers (detail page and
  libraries list), so the span clips inside its scroll container. The
  diagnostic also showed the search label collapsing to 9 px at tablet
  (flex basis 0 + min-w-0 under squeeze); fixed with `min-w-[12rem]` so
  the field wraps instead of vanishing. After the fix all seven CI jobs
  are green, e2e included — the pixel-true 390/768/1280 check on both
  library pages is verified by CI run 41004113513.
- `LibraryDetailPage` plain-language block reuses the server's
  `POST /flows/preview {graph}` — the same renderer the easy screen uses,
  so the UI keeps no second copy (task 14 rule).

## Summary

- Built: last scan time end to end (migration 0009, scanner stamp,
  `LibraryDTO.last_scan_at`); `q`/`sort`/`dir` server side on the files
  endpoint with per-page `last_job_encoder`; `GET /files/{id}/jobs`; the
  libraries list as a table with status chips (cards under sm); the detail
  header summary (counts, saved, plain-language target, last scan); the
  file table with before/after/ratio/encoder and a job-log link; the
  FileHistory panel with attempts, fallbacks and log disclosures; e2e
  layout coverage for both library pages.
- Skipped: charts over time, bulk actions, flow editors (out of scope);
  no SQL-level file filtering (handler-side, the documented home-lab
  pattern); job history capped at the newest 100 per file.
- Next task should know: `FileItem.last_job_encoder` and
  `Library.last_scan_at` are optional JSON fields (omitempty); the
  list/detail chips share `StatusChip`; the stores' `SetLibraryScanTime`
  is deliberately not part of `UpdateLibrary`.
- Verification: make test 0, make lint 0, integration re-run green after
  one load flake; CI run 41004113513 green on all seven jobs including
  e2e at 390/768/1280 for both library pages (after the sr-only fix).
- PR: #18 https://github.com/PeiceOfToast/shrinkyourshit/pull/18

## For the next task

The `sr-only`-inside-a-table-header pattern paints an absolutely
positioned span at the table's far edge and widens the page whenever the
table's min-content exceeds the viewport — any new table with an sr-only
Actions column must sit in a `relative` scroll wrapper (`relative
overflow-x-auto`). Toolbar rows built from selects also squeeze flexed
inputs to zero under pressure; give such inputs a `min-w-[…]` so they wrap
instead of collapsing.
