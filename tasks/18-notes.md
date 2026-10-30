# Task 18 notes — node switch, live job line, per node totals

## Plan (files, in order)

1. `internal/store/migrations/0008_job_bytes_so_far.sql` — `jobs.bytes_so_far
   INTEGER NOT NULL DEFAULT 0`.
2. `internal/store/store.go` — `Job.BytesSoFar`; new `NodeStat` type
   (`FilesDone`, `BytesSaved`).
3. `internal/store/crud.go` / `queue.go` / `interface.go` — carry the new
   column through jobColumns/scan/create/update/UpdateJobIfActive, reset it
   on requeue; new `NodeJobStats(ctx)` store method: one `GROUP BY node_id`
   query, `SUM(MAX(bytes_before - bytes_after, 0))` over `status='done'`.
4. `internal/server/dto.go` — `JobDTO` gains `file_name`, `file_path`
   (resolved from file_id) and `bytes_so_far`; `NewJobDTO` takes the file
   (nil-safe); `NodeDTO` gains `Stats *NodeStatsDTO` (list responses only,
   like `CurrentJobID`).
5. `internal/server/queue.go` — `ProgressParams.BytesSoFar` stored on
   Progress; a `jobDTO` helper resolves the file for event publishes;
   Complete stamps `bytes_so_far = bytes_after`; Retry/RequeueNodeJobs
   reset it.
6. `internal/api/worker.go` / `handlers.go` — progress body gains
   `bytes_so_far`; job DTO responses enriched with the file lookup.
7. `internal/api/nodes.go` — `listNodes` calls `NodeJobStats` exactly once
   and sets every node's stats (zeros when no finished jobs).
8. `internal/worker/client.go` + `runner.go` — `ProgressInput.BytesSoFar`;
   the report loop `os.Stat`s the scratch output once per tick (default
   2s) and reports it; stat failure reports 0, never kills the tick.
9. `web/src/api.ts` — `Job.file_name/file_path/bytes_so_far`,
   `Node.stats`.
10. `web/src/Switch.tsx` — accessible switch (`role="switch"`,
    `aria-checked`, 44px target).
11. `web/src/NodeJobLine.tsx` — one condensed live line: name (truncated,
    full path in title), percent bar (`aria-valuenow`), speed `1.8x`, ETA
    via `formatEta`, size `before → so far` via `formatBytes`; one line at
    `sm`+, two under `sm`.
12. `web/src/NodesPage.tsx` — switch drives start / stop(drain:true);
    secondary "Stop now" keeps the confirm dialog with `drain:false`;
    card footer `N files, X saved` + concurrency stepper; page header
    Pause all / Resume all; 5s `refetchInterval` poll fallback next to the
    SSE invalidation.
13. Tests: Go `TestJobDTOCarriesFileName`, `TestProgressStoresBytesSoFar`,
    `TestWorkerReportsBytesSoFar`, `TestNodeStatsAggregate`; web
    `NodesPage.test.tsx` (switch, stop now, pause/resume, stats footer) and
    `NodeJobLine.test.tsx`; Playwright already covers `/nodes` at 390/768/1280.
14. Docs: API.md task-18 section (already written at planning time; extend
    with the zero-stats shape), ARCHITECTURE.md Job data-model line gains
    `bytes_so_far`.

## Design decisions (filling doc gaps)

- Pause all / Resume all disabled state: the UI cannot see the server's
  pause snapshot, so it uses what a card shows: Pause all is enabled when
  any approved node is running/draining; Resume all is enabled when
  approved nodes exist and all of them are stopped. A node manually
  restarted after a pause makes resume-all look redundant (edge case,
  harmless — the server answers `{resumed: 0}` or restores the rest).
- `bytes_so_far` for a finished job is stamped to `bytes_after` on
  complete, so a done job's line shows the final size instead of a stale
  tick.
- The store's stats aggregate is one SQL statement (`GROUP BY node_id`);
  `TestNodeStatsAggregate` counts `NodeJobStats` calls through a wrapping
  store to prove the handler makes exactly one, never one per node.
- Freshness: SSE invalidation stays the fast path; the poll fallback is a
  5s `refetchInterval` on the nodes/running-jobs queries. Server state is
  re-fetched whole, so "last write wins" holds by construction (the
  refetch that lands last carries the latest `heartbeat_at`).
- The switch maps `desired_state === 'running'` to on. Draining shows off
  (it is a stop in progress); turning it on calls start, which the
  lifecycle model already allows from draining.

## Summary

- Built the task-18 node control surface: an accessible on/off switch per
  card (on = start, off = graceful `drain:true` stop, "Stop now" = the old
  confirmed `drain:false`), one condensed live line per busy node (name
  with full path on hover, percent bar, speed, ETA, `before → so far`),
  per node `files_done`/`bytes_saved` in the card footer from one grouped
  store query, and page-level Pause all / Resume all that disable when
  they would be a no-op.
- Backend: migration 0008 adds `jobs.bytes_so_far`; the worker stats its
  scratch output once per progress tick and reports it; the server keeps
  the last positive value, stamps it to `bytes_after` on complete and
  resets it on requeue/retry; every job DTO/event now carries
  `file_name`/`file_path` resolved from `file_id` (one file lookup,
  nil-safe).
- Tests: 4 new Go tests (`TestJobDTOCarriesFileName`,
  `TestProgressStoresBytesSoFar`, `TestWorkerReportsBytesSoFar`,
  `TestNodeStatsAggregate` — the last counts stats queries through a
  wrapping store to prove one grouped query), `NodeJobLine.test.tsx`,
  and reworked `NodesPage.test.tsx` (14 tests: switch, stop now,
  pause/resume, footer totals). Full `go test ./...`, `go test
  -tags=integration ./...` (ffmpeg present) and web suites green; Playwright
  passes all /nodes tests at 390/768/1280 (run locally after extracting
  chromium's missing system libs to a user dir — no sudo in this env).
- Skipped (out of scope): library page changes (task 19), scheduling
  policy, queue reordering. Pause/resume disabled states are UI
  heuristics — the server's pause snapshot is not exposed (recorded
  above); `job.updated` SSE still refreshes the live line through the
  existing invalidation, with a 5s `refetchInterval` fallback.
- Next task (19) should know: job DTOs now always carry
  `file_name`/`file_path`/`bytes_so_far` (events included), so the file
  list's per-file job history can use `GET /api/v1/jobs?file_id=…`
  payloads as-is; node list responses always include `stats`, zeros
  included.
