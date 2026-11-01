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
