# Task 27: Full job log, in the database

## Goal
Every job keeps its complete log as structured rows in SQLite, not a 64 KB tail and not files on disk. You can read, filter and download it from the app, and it survives a worker crash, a node reboot and a container replace.

## Read first
`internal/worker/runner.go` (`tailBuffer`, the failure and completion paths), `internal/worker/client.go` (`CompleteInput`, `Fail`), `internal/server/queue.go` (`MaxLogTail`, `capLogTail`, `Complete`, `Fail`), `internal/server/dto.go` (`Job`), `internal/api/api.go` (worker routes and job routes), `internal/store/migrations/0009_library_last_scan.sql` (migration style) and the rest of `internal/store` (how the neighbouring entities are written, pruned and deleted), `web/src/FileHistory.tsx`, `web/src/api.ts`, `web/src/dialog.tsx`.

Today the only log is `LogTail`, capped at 64 KB by `queue.go:31`, and it exists only once a job has finished. A running job shows nothing, a crashed worker loses everything, and there is no way to see why an encode was slow or which encoder it picked.

**Owner decision, this task:** logs live in SQLite, as lines with a level and a status, not as log files. One thing to back up, filterable without parsing, and no directory of orphaned files to prune. Do not create a `logs/jobs/` tree anywhere.

## Do

### 1. Migration `internal/store/migrations/0012_job_logs.sql`

```sql
CREATE TABLE job_log (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id   TEXT NOT NULL,
    seq      INTEGER NOT NULL,
    at       TEXT NOT NULL,
    level    TEXT NOT NULL,               -- info | warn | error
    source   TEXT NOT NULL,               -- ffmpeg | sys
    message  TEXT NOT NULL,
    repeated INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX job_log_job ON job_log(job_id, id);
CREATE INDEX job_log_at  ON job_log(at);
```

`seq` is the chunk order from the worker, so out of order arrivals can be sorted and gaps noticed. `repeated` collapses consecutive identical messages, which is what stops an ffmpeg warning loop from eating the table.

Store methods and interface entries next to the neighbouring entities: `AppendJobLog(jobID string, lines []JobLogLine) error`, `ListJobLog(jobID string, afterID int64, level string, limit int) ([]JobLogLine, error)`, `CountJobLog(jobID string) (int64, error)`, `PruneJobLog(olderThan time.Time, maxTotal int) (int64, error)`, `DeleteJobLog(jobID string) error`.

### 2. Capture on the worker

`internal/worker/runner.go`: the `tailBuffer` stays, because the failure message still needs a tail. In addition:

- Split the ffmpeg stderr stream into lines and send them as batches: `POST /api/v1/worker/jobs/{id}/log` with `{"seq": 0, "lines": [{"level": "info", "source": "ffmpeg", "message": "..."}]}`. Flush when 200 lines accumulate, when 5 seconds pass since the last flush, and once more on completion and on failure.
- A single message is capped at 4 KB and marked `[truncated]` when cut. Consecutive identical messages increment `repeated` instead of inserting a new row.
- Interleave `source: "sys"` lines so the log reads like a job log rather than raw stderr: encoder chosen, any fallback reason as `warn`, the resolved policy, the streams probing found, the result policy outcome, bytes before and after, and the health check verdict once task 24 lands.
- Stop at 2,000 lines for the job and append one final `error` line `log truncated at 2000 lines`.
- Sending is best effort on its own goroutine. A failed upload is logged at debug and dropped, and never fails the job.

### 3. Store and retention

- Per job cap 2,000 lines, enforced on append, so one pathological file cannot flood the database.
- Retention: delete rows older than 14 days, then oldest first beyond 500,000 total rows. Run at server start and hourly. Never delete rows belonging to a job that is queued or running.
- After a prune run `PRAGMA incremental_vacuum` so the space actually comes back, and switch the database to incremental auto vacuum in this migration if it is not already set. Report the file size before and after a prune test in the PR body.
- Deleting a job, a file or a library deletes its log rows in the same place the job rows go, and is safe when they are already gone.

### 4. API

```
POST /api/v1/worker/jobs/{id}/log      worker key, append a batch
GET  /api/v1/jobs/{id}/log             admin, JSON list
GET  /api/v1/jobs/{id}/log/download    admin, text/plain attachment
```

- `GET /api/v1/jobs/{id}/log` takes `?after_id=<id>&level=<info|warn|error>&limit=<n>`, default limit 500 and max 2,000, ordered by `id`. The response is `{"lines": [...], "total": n, "has_more": bool}` so a viewer can follow a running job without re-reading the whole log.
- `GET .../log/download` renders the same rows as plain text, one line per row as `<at> <level> <source> <message>`, with `Content-Disposition: attachment; filename="job-<id>.log"`.
- A job with no log rows answers 200 with an empty list on the JSON route. 404 only when the job itself does not exist.
- The worker route validates that the node owns the job, the same way complete and fail already do.
- `Job` in `internal/server/dto.go` gains `log_lines int` and `has_log bool`.

### 5. Viewer in the web UI

- `web/src/JobLogDialog.tsx`: a `dialog.tsx` based viewer. Monospace rows with the time, a level chip and the message. Level filter chips (all, warnings, errors), an "auto scroll" toggle that follows while the job runs, Copy, and Download.
- While a job is `running` or `queued`, poll `GET /api/v1/jobs/{id}/log?after_id=<last id>` once a second and append. Stop polling at a terminal status, and when the dialog closes.
- Opened from `web/src/FileHistory.tsx` rows and from the live job line on the nodes page (`NodeJobLine.tsx`), by a `Log` action that meets the 44 px rule from task 20. Keyboard reachable, Escape closes it.
- `web/src/api.ts`: `getJobLog` and `jobLogDownloadURL`, types in the existing style.

### 6. Tests
- `internal/worker/runner_test.go`: lines arrive in order with increasing `seq`; identical consecutive lines collapse into `repeated`; the 2,000 line cap stops the stream and appends the truncation line once; an upload failure does not fail the job.
- `internal/store` tests: append and read back in order, `after_id` and `level` filters, the per job cap, prune by age and by count leaving running jobs alone, and log rows disappearing with their job.
- `internal/api/worker_test.go`: a batch for a job owned by another node is rejected.
- `internal/api` test: `?after_id=` returns only newer rows, and `has_more` is right at the limit boundary.
- `web/src/JobLogDialog.test.tsx`: renders rows, appends a polled batch, the level filter hides info rows, the download link is correct, polling stops at a terminal status.

## Out of scope
Log files on disk, SSE or websocket streaming, a global jobs page, full text search across jobs, per line timestamps inside ffmpeg output, any change to `DECISIONS.md`.

## Acceptance
- [ ] `job_log` exists, a finished job has its complete log as rows, and `GET /api/v1/jobs/{id}/log` returns them in order with a correct `total`.
- [ ] A job killed mid encode (`docker kill` the worker container during a real encode) still has every batch written before the kill. Paste row counts in the PR body.
- [ ] An ffmpeg warning loop produces one row with `repeated` incremented, not thousands of rows.
- [ ] A job is capped at 2,000 lines with exactly one truncation line, and the job still ends `done`.
- [ ] `after_id` returns only new rows and the viewer's second poll appends instead of duplicating.
- [ ] Retention deletes by age and by count and never touches a running job's rows, proven by a test that backdates rows on a running job.
- [ ] Database file size before and after a prune test is reported in the PR body, and `PRAGMA incremental_vacuum` ran.
- [ ] `Job` responses carry `has_log` and `log_lines`, and the viewer opens from a file's job history.
- [ ] `make test`, `make test-integration` and `make lint` are green.
