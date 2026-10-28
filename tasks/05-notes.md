# Task 05 notes (worker job runner)

## Plan

Files touched, in order:

1. `internal/store` — add `GetNodeByKeyHash(ctx, hash)` to the `Store`
   interface + SQLite impl (node auth looks a key up by SHA-256).
2. `internal/server/node.go` — `NodeRegistry`:
   `Authenticate` (hash lookup, approved + enabled), `EnsureBuiltinNode`
   (read/create `<data-dir>/node.key` 0600, find node by key hash, else
   re-key the node named `<host> (built-in)`, else create it; idempotent),
   `Heartbeat` (node + running-job heartbeats, returns `worker_concurrency`
   from settings, default 1), `ResolveNodePath` (server-path → node-path
   via the node's path mappings, identity for the built-in node).
3. `internal/server/queue.go` — `Progress` (claimed→running, percent/speed/
   eta/encoder, heartbeat bump), `Complete` (job done; file per result
   policy: replaced → new path/fingerprint/probe/satisfied; skipped → done +
   bytes_saved=0 + reason; keep_both → satisfied, original row untouched),
   `Fail` (job failed + log tail capped at 64 KiB, file failed). Output
   inspection (fingerprint + probe) is an injectable func on `server.Config`
   so tests never run ffprobe.
4. `internal/api/worker.go` — the six task-05 endpoints with node bearer
   auth middleware (401 for unknown/revoked/disabled keys), ownership checks
   (403), claim 204 on empty queue.
5. `internal/encoders/verify.go` — `VerifyOutput` (media-specific output
   verification lives here, keeping store/queue/worker media-agnostic per
   rule 9): size > 0, video codec == target (or source codec for copy),
   audio/subtitle track counts per the spec selection, duration within 1%
   or 2s of the source.
6. `internal/worker` — `client.go` (worker API client, 401/403/409 typed),
   `progress.go` (-progress key=value parser: out_time_us/out_time_ms as µs
   per ffmpeg's long-standing misnomer, out_time fallback, speed, percent,
   eta; 64 KiB stderr tail), `scratch.go` (job dirs, orphan cleanup on
   start), `policy.go` (replace with `.<name>.shrink-tmp` + fsync + rename,
   extension change + old-file removal only after rename, keep_both
   `<name>.shrunk.<ext>`, not-smaller skip), `runner.go` (one job: spec →
   copy to scratch (mtime preserved so fingerprints match) → probe →
   RunWithFallback with an encoder spy for progress → extracts → verify →
   policy → report; cancel via progress-response status polling every 2s),
   `worker.go` (`Run`: heartbeat loop, concurrency tokens from the
   heartbeat response, claim slots, active-job registry for heartbeats).
7. `cmd/shrinkyourshit/main.go` — `serve` role `both` starts the built-in
   worker in-process against `http://127.0.0.1:<port>` with the key from
   `node.key` (same worker API as remote nodes); `serve --role worker`
   rejected with a pointer to the `worker` subcommand; `worker` subcommand
   requires `SHRINK_SERVER` and an existing `<data-dir>/node.key` (pairing
   is task 08).
8. `API.md` — document the task-05 request/response shapes that were
   unspecified (heartbeat returns `{concurrency}`, progress returns the job
   status for cancel polling, complete accepts `skipped_reason`/`log_tail`,
   claim returns the JobDTO or 204, job-spec shape, mapping entry shape,
   fallback_reasons as string array).
9. Tests: store lookup, registry (auth matrix, builtin idempotency, mapping
   resolution), queue transitions (unit, fake prober), API endpoints
   (unit), progress parser, policy (temp dirs), runner (fake prober + fake
   run), and `internal/worker/integration_test.go` (tag `integration`) for
   the end-to-end acceptance items with real ffmpeg: happy path, corrupt
   fake ffmpeg, forced-larger skip, keep_both, cancel, kill/restart/sweep,
   orphan scratch cleanup, job record fields, 401s.

## Decisions taken inside task scope (not owner decisions)

- Built-in node identity is the key file: `EnsureBuiltinNode` looks up by
  key hash, falling back to the node named `<host> (built-in)` (re-keys it)
  so a lost `node.key` heals instead of spawning duplicate nodes. No new
  settings key (ARCHITECTURE.md fixes that list).
- `worker_concurrency` reaches the worker through the heartbeat response —
  the built-in worker uses the same HTTP API as remote workers, so it
  cannot read settings directly.
- Concurrency slots: fixed pool of `MaxSlots` goroutines gated by tokens;
  the token limit follows the heartbeat's concurrency (shrinks take effect
  after the current job).
- The complete endpoint accepts an optional `skipped_reason` (used when the
  worker keeps the original because the output was not smaller) and
  `log_tail`; `fallback_reasons` is a string array (`"nvenc: <error>"`),
  matching the JobDTO shape from task 03.
- Server-side complete recomputes the output fingerprint and probe (shared
  mode has the file locally; transfer mode arrives in task 09). Duplicate
  file rows that collide with the new path are removed first (the old row
  keeps its ID).
- Worker verifies the source fingerprint before encoding (copy preserves
  the mtime) and fails the job if the file changed since it was queued; the
  next scan re-evaluates and re-queues.
- A job reported failed by a worker is failed outright (attempts only count
  heartbeat timeouts); retry stays manual.
- Cancel races: a complete/fail on a cancelled job gets 409 and the worker
  moves on; disk state self-heals on the next scan.
- Scratch extracts (`output.<lang>.srt`) are moved beside the applied
  output; on a not-smaller skip they are discarded with the output.

## Out-of-scope ideas noted, not built

- Transfer-mode source fetch and result upload (task 09); `auto` access
  mode currently resolves to `shared` until the shared-storage check exists.
- Progress-driven cancel during the copy phase (copy is local and fast);
  cancel is checked around the encode.
- Releasing claimed jobs on graceful worker shutdown (they requeue via the
  heartbeat-timeout sweeper).

## Summary

- Built: worker claim/heartbeat/progress/complete/fail API with node-key
  auth, built-in node + key file, NodeRegistry with path mapping, queue
  Progress/Complete/Fail, output verification in encoders, and the full
  worker (client, claim slots, runner with fallback + progress + extracts,
  verify, result policies, scratch lifecycle, cancel).
- Skipped: transfer mode, pairing, UI, shared-storage access check.
- Next task should know: node auth middleware lives in `internal/api`
  (`nodeAuth`), the built-in node is created by
  `server.NodeRegistry.EnsureBuiltinNode` at serve start, workers learn
  concurrency from the heartbeat response, and `server.Config.Inspect`
  overrides output fingerprinting/probing in tests.
