# Task 27: Full job log

## Goal
Every job keeps its complete log, not a 64 KB tail, and you can read or download it from the app. Survives a worker crash, a node reboot and a container restart.

## Read first
`internal/worker/runner.go` (`tailBuffer`, the failure and completion paths), `internal/worker/client.go` (`CompleteInput`, `Fail`), `internal/server/queue.go` (`MaxLogTail`, `capLogTail`, `Complete`, `Fail`), `internal/server/dto.go` (`Job`), `internal/api/api.go` (worker routes and job routes), `internal/config/config.go` (where the data directory comes from), `web/src/FileHistory.tsx`, `web/src/api.ts`, `web/src/dialog.tsx`.

Today the only log is `LogTail`, capped at 64 KB by `queue.go:31`, and it exists only on a finished job. A running job shows nothing, a crashed worker loses everything, and there is no way to see why an encode was slow or which encoder it picked.

## Do

### 1. Capture the whole thing, on the worker

`internal/worker/runner.go`: the `tailBuffer` stays (the failure message still needs a tail), but feed the same stderr stream into a new `logSink` that:

- Keeps a byte count and buffers up to 32 KB.
- Sends a chunk with `POST /api/v1/worker/jobs/{id}/log` when the buffer fills, when 5 seconds have passed since the last flush, and once more on completion or failure. Body `{"seq": 0, "data": "<utf-8 string>"}` where `seq` increases by one per job and starts at 0.
- Never blocks the encode: sends happen on a goroutine and a failed send is logged at debug and dropped. A log upload problem must never fail a job.
- Interleaves non ffmpeg lines with a `sys:` prefix so the log reads like a job log rather than raw stderr: encoder chosen and any fallback reason, the resolved policy, the sources probing found, the result policy outcome, bytes before and after, and the health check verdict once task 24 lands.
- Truncates a single chunk at 32 KB, and stops the sink entirely after 8 MB of job log with one final `sys: log truncated at 8 MiB` line.

### 2. Store it as a file, not in the database

New `internal/server/joblog.go`. Logs live on disk, because ffmpeg stderr is large and the database is a single SQLite file:

- Path: `<data dir>/logs/jobs/<job id>.log`. The data dir comes from the config package, and in Docker it sits on the `/config` volume, so logs survive a container replace. Create the directory at startup with `0o750`.
- Append only, `O_APPEND`, one `Write` per chunk, with a mutex per job id so two chunks cannot interleave. Refuse a chunk that would push the file past 8 MB and write the truncation line once.
- Retention, run at server start and once an hour: delete job logs older than 14 days, then the oldest beyond 500 files. Never delete the log of a job still running or queued. Log the count removed.
- Deleting a library or a file deletes its job logs; do it in the same place the job rows are pruned, and make it safe when the file is already gone.

### 3. API

```
POST /api/v1/worker/jobs/{id}/log      worker key, chunk append
GET  /api/v1/jobs/{id}/log             admin, text/plain; charset=utf-8
GET  /api/v1/jobs/{id}/log/download    admin, Content-Disposition attachment
```

- `GET /api/v1/jobs/{id}/log` accepts `?offset=<bytes>` and returns `X-Log-Offset`, `X-Log-Bytes` (total size) and `X-Log-Truncated: true|false`. With no offset it returns up to the last 128 KB, newest first is wrong for a log, so return the last 128 KB in order with `X-Log-Offset` naming where that window starts. This is what lets the viewer follow a running job without re-reading megabytes.
- A missing log answers 404 with the existing error shape, not an empty 200.
- `Job` in `internal/server/dto.go` gains `log_bytes int64` and `has_log bool`, so lists can show a log button without probing.
- Unknown job id answers 404. The worker endpoint validates the node owns the job, the same way the complete and fail endpoints already do.

### 4. Viewer in the web UI

- `web/src/JobLogDialog.tsx`: a `dialog.tsx` based viewer with monospace text in a scroll box, an "auto scroll" toggle that follows the end while the job runs, a Copy button, a Download button that hits the download endpoint, and a header line with the job id, file name, status and byte count.
- While a job is `running` or `queued`, poll `GET /api/v1/jobs/{id}/log?offset=<last byte read>` once a second and append. Stop polling when the job reaches a terminal status, and stop when the dialog closes.
- Opened from `web/src/FileHistory.tsx` rows and from the live job line on the nodes page (`NodeJobLine.tsx`), by a `Log` action that meets the 44 px rule from task 20. Keyboard reachable, Escape closes it.
- `web/src/api.ts`: `getJobLog`, `getJobLogDownloadURL` and the chunk post is worker only, so it stays out of the client. Types follow the existing style.

### 5. Tests
- `internal/worker/runner_test.go`: chunk boundaries produce several `seq` values in order; a send failure does not fail the job; the sink stops at 8 MB with exactly one truncation line.
- `internal/server/joblog_test.go`: appends in order, concurrent appends come out whole, the 8 MB cap holds, retention deletes by age and by count and never touches a running job, `?offset=` returns the right window and the right headers.
- `internal/api/worker_test.go`: a chunk for a job owned by another node is rejected.
- `web/src/JobLogDialog.test.tsx`: renders fetched content, appends a polled chunk, download link points at the right path, and stops polling when the job is terminal.

## Out of scope
Streaming with SSE or websockets, a global jobs page, log search, sending logs to a file on the host outside the data dir, per-line timestamps inside ffmpeg output, any change to `DECISIONS.md`.

## Acceptance
- [ ] A finished job has a complete log on disk under `<data dir>/logs/jobs/`, and the API returns it in order with a correct `X-Log-Bytes`.
- [ ] A job killed mid encode (`docker kill` the worker container during a real encode) still has every chunk written before the kill, proven by reading the file afterwards. Paste the byte counts in the PR body.
- [ ] A 9 MB log is capped at 8 MB with exactly one truncation line and the job still ends `done`.
- [ ] `?offset=` returns only the new bytes and the viewer's second poll appends instead of duplicating.
- [ ] Retention never deletes the log of a running job, proven by a test that sets an old mtime on a running job's log.
- [ ] `Job` responses carry `has_log` and `log_bytes`, and the viewer opens from a file's job history.
- [ ] `make test`, `make test-integration` and `make lint` are green.
