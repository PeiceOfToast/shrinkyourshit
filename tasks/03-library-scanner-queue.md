# Task 03: Library API, scanner, watcher, queue, events

## Goal
Add a folder as a library, scan it, watch it for changes, and turn files that need work into queued jobs. Expose it all over the API with live events.

## Read first
API.md (conventions, task 03 endpoints), ARCHITECTURE.md (data model, job lifecycle), STACK.md (fsnotify, SSE). Depends on task 02.

## Do
- `internal/server`:
  - Library manager: create/update/delete; update triggers re-evaluation of all files in the library.
  - Scanner: walk the library path, skip hidden files and non-media extensions (list in code: mkv mp4 avi mov m4v ts wmv flv webm mp3 flac aac m4a ogg opus wav), probe files whose fingerprint changed or is new, store, `Evaluate`, queue a job if needed, mark `missing` for files no longer on disk. Concurrency-limited probing (default 4).
  - Watcher: fsnotify, recursive, debounced 2s, wait until size is stable for 5s before probing (handles files still being copied). If the path is a network mount or fsnotify fails, fall back to a poll scan every 5 minutes.
  - Scheduler: optional cron string per library (use a minimal 5-field parser; no external cron library).
  - Queue: persistent in the store. `Claim(nodeID)` is a single transaction that picks the highest-priority oldest `queued` job and sets `claimed`, so two workers never get the same job. Heartbeat-timeout sweeper returns stale jobs to `queued`, increments `attempt`, fails after 3.
- `internal/api`: all task 03 endpoints from API.md, SSE event bus with the four event types, `fs/browse` restricted to directories (no file contents), admin-auth middleware as a no-op placeholder that task 06 fills in.
- Wire `serve` to start the API, scanner, watchers and sweeper.

## Out of scope
Running FFmpeg, UI beyond what is needed to test, remote workers, auth.

## Acceptance
- [x] Integration test: library with 3 fixture files (1 needs work, 2 satisfied) queues exactly 1 job; `GET /libraries/{id}/files?status=satisfied` returns 2.
- [x] Dropping a new file into a watched folder (copied slowly in the test) queues a job only after the copy completes.
- [x] Restarting the server does not duplicate or lose queued jobs.
- [x] 50 goroutines calling `Claim` against 10 queued jobs produce exactly 10 claims, no duplicates.
- [x] Stale `running` job with an old heartbeat returns to `queued` with `attempt` incremented; after 3 attempts it is `failed`.
- [x] Deleting a library removes its DB records and never touches files on disk (test asserts the fixture files still exist).
- [x] SSE client receives `job.updated` when a job is queued and `file.updated` when a file is probed.
- [x] `PUT /libraries/{id}` with a changed Target re-evaluates and queues only files that now need work.
