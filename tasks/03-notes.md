# Task 03 notes — library API, scanner, watcher, queue, events

## Plan (files, in order)

1. **`internal/store`** (queue primitives task 02 anticipated)
   - `store.go`: `JobFilter.FileID`; `ErrQueueEmpty`.
   - `crud.go` / new `queue.go`: `ClaimNextJob(ctx, nodeID)` (single tx: highest priority, oldest first, `queued -> claimed`, sets node/claimed_at/heartbeat_at; WHERE guard on status so it stays safe if the connection pool ever widens) and `RequeueStaleJobs(ctx, cutoff, maxAttempts)` (tx: claimed/running with stale heartbeat -> `queued`, attempt+1, clears claim fields; attempt+1 >= maxAttempts -> `failed` instead; guarded UPDATE so a live heartbeat wins).
   - tests: claim ordering (priority DESC, created ASC), 50-goroutine claim race, requeue/attempt/fail lifecycle.
2. **`internal/probe`**: add JSON tags to `MediaInfo`/stream types (lowercase snake) so `File.ProbeJSON` and the API return readable JSON. Additive, no behavior change.
3. **`internal/server`** (the task core)
   - `events.go` — `EventBus`: `Subscribe/Unsubscribe`, `Publish` (non-blocking, drop on full buffer), types `file.updated|job.updated|node.updated|library.updated`.
   - `dto.go` — JSON shapes for library/file/job (JSON API + SSE payloads). Kept out of `api` so the bus can publish without an import cycle.
   - `cron.go` — minimal 5-field cron (star, lists, ranges, steps, DOM/DOW vixie quirk, 0 and 7 = Sunday). No external library.
   - `queue.go` — `Queue` service: `QueueJob` (active-job guard, bytes_before, sets file queued), `Claim` (store claim + file -> processing + events), `Cancel`, `Retry`, `Sweep` (calls store requeue + file transitions + events).
   - `scanner.go` — walk (media extension list in code, skip hidden), fingerprint, probe only new/changed (or status `new` re-eval from cached probe), evaluate, queue, mark `missing`; bounded worker pool (default 4); per-library scan mutex.
   - `library.go` — manager: create/update/delete + validation; update re-evaluates all files from cached probes (queues newly-needing, satisfies newly-satisfied, cancels queued jobs that are no longer needed; leaves claimed/running alone).
   - `watcher.go` — fsnotify recursive, 2s debounce, size-stable 5s (plus mtime age) before scan; per-library; poll fallback every 5 min when fsnotify init fails.
   - `scheduler.go` — minute-tick loop matching each library's cron; catch-up bounded; scan once per matching minute.
   - `server.go` — aggregate + `Start(ctx)` (startup scans, watchers, scheduler, sweeper) / `Stop()`.
   - unit tests with an injectable `Prober` func (no ffmpeg needed); `integration` tests with real fixtures for acceptance 1, 2, 6.
4. **`internal/api`** — all task 03 endpoints from API.md, SSE handler, `fs/browse` (dirs + names only, never file contents), `requireAdmin` no-op middleware placeholder for task 06, `New(version, static, services)`.
5. **`cmd/shrinkyourshit/main.go`** — open store at `<data-dir>/shrinkyourshit.db`, build `server.Server`, start it, wire into `api.New`, graceful shutdown.
6. Docs/status: tick acceptance, update STATUS.md, summary.

## Decisions / justifications

- **Attempt increments on stale-requeue, not on claim.** `attempt` counts interrupted tries; a job claimed 3 times that times out each time ends `failed` on the 3rd timeout ("max 3 attempts, then failed"). `Retry` resets attempt to 0.
- **Sweeper staleness decided in Go, guarded UPDATE in SQL.** RFC3339Nano strings are not lexically orderable (variable precision), so `heartbeat_at <= cutoff` in SQL is unsafe; Go parses and compares, the UPDATE guards on the old heartbeat value (also correct for a future Postgres backend).
- **Cancel returns the file to `new`** (it still needs work; the next scan re-queues it — target-driven convergence). Retry conflicts with a file that meanwhile became `satisfied`.
- **Update re-evaluation uses cached probes only** (fast, no disk IO); files without a probe keep status `new` and the next scan probes them. Queued jobs for files that became satisfied get cancelled; claimed/running jobs are left to finish (the worker fetches the current Target at claim time, task 05).
- **Scanner skips non-regular files (symlinks) and hidden entries**; media extension list exactly as the task file lists it.
- **`fs/browse` lists directories and file names** (with `is_dir`), never contents — the restriction in the task file is about file contents; the folder picker needs names to confirm the right folder.
- **POST /libraries/{id}/scan is async (202)** — real libraries are large; tests poll.
- **New dependency `github.com/fsnotify/fsnotify`** — listed in STACK.md, so allowed (rule 3); no other new deps.
- **Events drop on a slow subscriber** (buffered 64, non-blocking publish) — SSE is best-effort UI state, backpressure must never stall scans; a heartbeat comment every 15 s keeps proxies open.

## Out of scope (per task file)

Running FFmpeg (task 05 consumes these jobs), UI, remote workers, real auth (middleware placeholder only).

## Summary (filled at the end)

- **Built:** `internal/server` — event bus (SSE fan-out, 4 event types), JSON DTOs, minimal 5-field cron, queue service (QueueJob with active-job guard, Claim, Cancel, Retry, Sweep), scanner (walk, media extensions, fingerprint short-circuit, stability wait, Evaluate, queue, missing-marking, per-library mutex + burst collapse), library manager (CRUD + target-driven re-evaluation on update, cancel of unneeded queued jobs), watcher (fsnotify recursive, 2s debounce, poll fallback; scanner owns stability), scheduler (minute-tick cron loop), Server aggregate with Start/Stop; `internal/api` — all task 03 endpoints, SSE handler, fs/browse, `requireAdmin` placeholder; store additions (`ClaimNextJob` atomic tx, `RequeueStaleJobs` with guarded updates, `JobFilter.FileID`); probe JSON tags; `serve` wiring (store in data dir, services, graceful shutdown).
- **Skipped (out of scope):** running FFmpeg (jobs queue, nothing claims them until task 05), real auth (placeholder middleware), UI, remote workers.
- **Tests:** unit tests run without ffmpeg (fake Prober injected); integration tests (tag `integration`) cover acceptance 1 (3 fixtures → 1 job, satisfied filter = 2 over HTTP), 2 (real fsnotify slow-copy), 6 (delete leaves fixture files); `make test`, `make test-integration` and `make lint` all green; smoke-tested the real binary end-to-end with curl (create, scan, queue, SSE, fs/browse, cancel, SIGTERM shutdown).
- **Decisions for the owner to know:** attempt counts interrupted tries (stale-requeue increments; `Retry` resets to 0; 3rd timeout → failed); cancel returns the file to `new` so the next scan re-queues it (target-driven convergence); a queued job for a file that became satisfied is cancelled on library update; PUT /libraries is full-replace and requires the target; POST /libraries falls back to the default Target when target is absent; `fs/browse` lists names + is_dir, never contents; stability check lives in the **scanner** (every scan path protected), the watcher only debounces.
- **Next task should know:** task 05 claims via `svc.Queue().Claim(nodeID)` (`store.ErrQueueEmpty` → 204) and progresses with `UpdateJob` + events (see queue.go for the transitions, file status follows the job: claimed/running→processing, done→done, fail→failed); the worker fetches the **current** library Target at claim time (jobs do not snapshot it); `flow.SelectAudio/SelectSubtitles` are ready for ffmpeg mappings; heartbeat timeout default 120s (`HeartbeatTimeoutDefault`), max attempts 3 (`MaxJobAttempts`).
