# Task 27 notes: full job log, in the database

## Plan

1. `internal/store/migrations/0012_job_logs.sql` — the task's table plus an
   `ON DELETE CASCADE` FK on `job_id` (see "Deviations") so deleting a job,
   file or library takes its log rows with it, exactly like the jobs.
2. `internal/store/joblog.go` — `JobLogLine` type, constants and the five
   methods (`AppendJobLog`, `ListJobLog`, `CountJobLog`, `PruneJobLog`,
   `DeleteJobLog`), per-job cap 2000 rows enforced on append, prune
   protecting queued/claimed/running jobs, then `PRAGMA incremental_vacuum`.
3. `internal/store/sqlite.go` — `PRAGMA auto_vacuum = INCREMENTAL` before
   migrations, one VACUUM when an existing database has not switched modes
   yet (see "Deviations").
4. `internal/server/dto.go` + `queue.go` — `JobDTO` gains `log_lines` and
   `has_log`, stamped from one indexed COUNT per job payload in the API's
   jobDTO; `Queue.AppendJobLog` is the ownership-checked append the worker
   route uses.
5. `internal/server/joblog.go` — hourly retention loop (one pass at start):
   delete older than 14 days, then oldest-first beyond 500k rows, never
   touching queued/claimed/running jobs' rows.
6. `internal/api` — `POST /api/v1/worker/jobs/{id}/log`,
   `GET /api/v1/jobs/{id}/log`, `GET /api/v1/jobs/{id}/log/download`; the
   read response also carries the job's live `status` so the viewer stops
   polling when the job goes terminal. `docs/openapi.yaml` updated (the
   anti-drift test enforces it); API.md and ARCHITECTURE.md updated in the
   same commit.
7. `internal/worker/joblog.go` — `jobLogSink`: line splitting, 4 KB message
   cap with `[truncated]`, consecutive-repeat collapsing, the 2000-row cap
   with exactly one truncation line, batch flushing (200 lines / 5 s /
   final flush before complete/fail), best-effort upload goroutine. The
   runner interleaves `sys` lines (probe summary, resolved target and
   policy, encoder chosen, fallback reasons as warn, result policy outcome,
   bytes before/after, health check verdict). `stderrTee` feeds both the
   64 KiB tail and the structured log.
8. `web/src` — `api.ts` (`getJobLog`, `jobLogDownloadURL`), the
   `JobLogDialog` viewer (level chips, auto scroll, Copy, Download, 1 s
   after_id polling, stops at a terminal status or on close), and the 44 px
   Log action on the FileHistory rows and the nodes page live job line.
9. Tests: store, api (worker ownership, after_id/level/has_more, cap,
   download, DTO fields), worker (batching/dedupe/cap/upload failure,
   runner lifecycle sys lines, tee), web (JobLogDialog), plus integration
   tests over a real ffmpeg encode (streaming and mid-encode kill).

## Deviations (recorded, not decision changes)

- The task's migration SQL block has no foreign key on `job_id`; I added
  `job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE` so "deleting
  a job, a file or a library deletes its log rows in the same place the job
  rows go" holds through the same cascade the jobs ride. `DeleteJobLog`
  still exists for the explicit case and is a no-op when rows are gone.
- The task says "switch the database to incremental auto vacuum in this
  migration". The migration runner wraps each file in a transaction and a
  `PRAGMA auto_vacuum` inside a transaction is silently ignored (verified
  empirically); on a non-empty database the mode change also only applies
  after a rebuild. So the pragma runs in `store.New` before migrations
  (fresh databases get the mode from the start) and existing databases get
  one VACUUM on the first open that finds the mode unset.
- `PRAGMA incremental_vacuum(0)` only frees one page per statement step in
  the modernc driver, so the store drains the pragma's result set to the
  end (measured: 18,845,696 → 147,456 bytes in the prune test).
- The download format is `<at> <level> <source> <message>` as specified;
  a collapsed repeat adds ` (xN)` after the message so a warning loop's
  count survives the download.
- The read endpoint's response carries the job's live `status` next to
  `lines/total/has_more` so the viewer can stop polling at a terminal
  status without a second request per poll.

## Known issues (pre-existing, not this task's)

- `TestE2ETransferModeReplacesOriginalOnServer` (worker integration) and
  `TestNodeDrainFinishesCurrentJob` are timing-flaky on this machine —
  verified failing on the base commit (02c2f75) at the same rate as on this
  branch. The first is a race in the test: it observes the job row `done`
  (polling the store directly) before the API handler reaches
  `Transfer().Discard` after `Queue().Complete` returns. Not touched here
  (out of scope).

## Summary (5 lines)

- Built: `job_log` rows in SQLite (migration 0012), store methods with the
  2000-row cap, age+count retention protecting active jobs and reclaiming
  space via incremental vacuum, worker batch upload of ffmpeg+sys lines
  (4 KB cap, repeat collapsing, 2000-row cap with one truncation line,
  best-effort upload), three API routes plus `log_lines`/`has_log` on every
  job response, and a JobLogDialog viewer with filters, polling and
  download opened from file history and the nodes page.
- Skipped: log files on disk, SSE/websocket streaming, global jobs page,
  full text search — all out of scope. The old 64 KiB tail and `log_tail`
  stay as the failure message carrier.
- Acceptance tests: all ticked items have tests; `make test`,
  `make test-integration` and `make lint` pass (see the flake note above
  for the two pre-existing timing races).
- Numbers for the PR: kill test 49 rows before the kill / 49 after; prune
  test 18,845,696 → 147,456 bytes.
- Next task should know: `web/src/api.ts` Job now carries `log_lines` and
  `has_log`; every job DTO consumer gets the count for free; the log read
  response's `status` field is how a follower stops.
