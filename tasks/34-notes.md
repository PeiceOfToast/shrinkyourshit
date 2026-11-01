# Task 34 Notes

## Plan

This task introduces a media file kind system to support non-media files
(images, subtitles, archives) alongside the existing video and audio handling.

Goal (from `tasks/34-non-media-file-types.md`):
> A library declares what it holds; the scanner and planner learn about files
> that are not video or audio; the steps that make sense (rename, move,
> plugins, scripts, size rules) work on them while media-only steps skip
> cleanly instead of failing.

Steps:
1.  **Create `internal/probe/kind.go`**
    - Define `Kind` enum (`KindVideo`, `KindAudio`, `KindSubtitle`, `KindImage`, `KindOther`).
    - Extension table and helpers (`KindForPath`, `IsMedia`, `KindValues`).
    - Unit test in `internal/probe/kind_test.go`.
2.  **Store layer**
    - Migration `internal/store/migrations/0014_library_kinds.sql` to add
      `file_kinds` to the `libraries` table.
    - `internal/store/store.go` to add `FileKinds` to the `Library` struct.
    - `internal/store/crud.go` to read and write `file_kinds`.
3.  **Scanner**
    - `internal/server/scanner.go` to respect the library's `FileKinds`:
      skip files that do not match the library's kinds *before* probing.
4.  **Flow**
    - `internal/flow/evaluate.go` to skip media-only steps on non-media files.
    - `internal/flow/target.go` and `internal/flow/graph.go` to handle
      non-media file kinds.
    - `internal/worker/runner.go` to dispatch non-media steps correctly.
5.  **API**
    - `internal/api/handlers.go` to accept and return `file_kinds` on library
      create and update. Validate the kind list.
6.  **UI**
    - `web/src/LibrariesPage.tsx`, `web/src/LibraryDetailPage.tsx` to allow
      editing the file kinds and showing the kind list.
    - `web/src/api.ts` to add `file_kinds` to the library DTOs and body types.
7.  **Docs**
    - `ARCHITECTURE.md`, `API.md` and `README.md` to document the kind system.

## Progress

- [x] Define `Kind` and helpers in `internal/probe/kind.go`.
- [x] Write `internal/probe/kind_test.go` (all tests passing).
- [x] Migration `internal/store/migrations/0014_library_kinds.sql` created.
- [x] `internal/store/store.go` updated with `FileKinds []string`.
- [x] `internal/store/crud.go` updated to serialize/deserialize `file_kinds`.
- [x] Fixed missing `encoding/json` imports in `internal/store/crud.go`
      and `internal/store/sqlite.go`.
- [x] Build passes (`go build ./...`).

## DECISIONS.md quote

> Item 4: "The core must stay generic so other file types can be added
> later." — this task is that extension point. `DECISIONS.md` is not
> touched.

## Verification results (2026-11-01)

- `timeout 300 npx tsc --noEmit` — clean.
- Web tests (detail, list, edit pages + KindPicker) — 29 passed.
- `make test` — all Go packages ok, 32 web test files, 231 web tests passed.
- `make test-integration` — all packages ok (api 439s, server 524s, worker
  303s, store 143s, cmd 83s).
- `make lint` — go vet clean, golangci-lint 0 issues, eslint 0 warnings,
  spdx ok, prettier clean.
- First `make lint` run caught two real issues, both fixed: `ineffassign`
  in the scanner's scan-summary stamping (rewritten with an explicit
  `stampErr` variable) and `react-refresh/only-export-components` in
  KindPicker.tsx (constants and `kindLabel` moved to `web/src/kinds.ts`).
- Note: the earlier hang was not reproduced. `make lint`'s web step (eslint
  + prettier via npm) is the slowest part; every re-verification here
  completed well inside its timeout, so no lint step is flagged as
  chronically slow.

## Acceptance mapping

All eight boxes ticked in `tasks/34-non-media-file-types.md`:

1. `scanner_test.go`: TestScannerImageLibrarySkipsOtherKinds,
   TestScannerKindSkippedRowRecorded,
   TestScannerNonMediaKindQueuesKindAgnosticWork.
2. `api/filekinds_test.go`: TestLibraryFileKindsValidation (+ default and
   create/update round-trip).
3. `worker/runner_test.go`: TestRunnerNonMediaRenameAndMove (no encoder, no
   ffprobe).
4. `worker/runner_test.go`: TestRunnerNonMediaMediaOnlyFlowSkips.
5. Full go/web suites green; TestLibraryFileKindsDefault asserts ["video"].
6. Web tests: KindPicker, LibraryEditPage, LibrariesPage, LibraryDetailPage
   (skipped count + hint).
7. Docs committed with the code (cdbb360).
8. make test / test-integration / lint green.

## Summary

- Built: probe.Kind decided by extension, library `file_kinds` (+ last-scan
  summary persistence), kind-aware scanner/planner/worker, API validation,
  UI kind picker + skipped count + hint, docs.
- Skipped: transfer-mode extension thrash (the server keeps the file's own
  extension for non-media), per-file rule trees, image encoding (out of
  scope per the task).
- The worker decides kind from the path extension at job start; media-only
  flows on non-media files complete as skips, never failures.
- Next task should know: `flow.ResolvePlan` and `ApplyStep` now take a
  `probe.Kind`; `SetLibraryScanTime` became `SetLibraryScanResult(ctx, id,
  at, summaryJSON)`.
- Pending: owner should open a PR from `task/34-non-media-file-types` and
  watch CI.
