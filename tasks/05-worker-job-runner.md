# Task 05: Worker job runner

## Goal
A worker claims a job through the API, copies the file to local scratch, encodes with fallback, verifies, and safely replaces or keeps the original. The built-in worker and future remote workers share this code and talk only through the worker API.

## Read first
API.md (task 05 endpoints, node auth), ARCHITECTURE.md (job lifecycle, result policies, file access modes), STACK.md (process execution). Depends on tasks 03 and 04.

## Do
- `internal/api`: task 05 worker endpoints. Node auth middleware: `Authorization: Bearer <key>`, key hash looked up in the nodes table, node must be `approved` and `enabled`. At first `serve` start, create the built-in node (`name=hostname (built-in)`, `status=approved`, `access_mode=shared`) and write its key to `<data-dir>/node.key`.
- `internal/worker`:
  - `Run(ctx, cfg)`: detect capabilities (task 04), send heartbeat every 30s, claim loop with concurrency from settings (`worker_concurrency`, default 1).
  - Job steps: resolve the source path (shared mode: apply path mappings; built-in worker: identity), copy to `<scratch>/<job-id>/source.<ext>`, run `RunWithFallback` writing `<scratch>/<job-id>/output.mkv`, stream `-progress` into progress reports every 2s, run any extract-subtitle commands, verify the output, apply the result policy, report `complete` or `fail` with the last 64KiB of ffmpeg stderr.
  - Verify: ffprobe parses the output; duration within 1% or 2s of the source; video codec equals the target; expected audio and subtitle track counts; file size greater than 0.
  - Apply per `result_policy`: `replace_if_smaller` (skip replacement and mark done with `bytes_saved=0` and reason if not smaller), `replace`, `keep_both`. Replacement: write to `<dest-dir>/.<name>.shrink-tmp`, fsync, rename over the original. Extension changes to the target container.
  - Cancel: poll job status in progress responses; on `cancelled`, kill ffmpeg and clean scratch.
  - Scratch cleanup on success, failure, and worker start (remove orphaned job dirs).
- Wire `serve` with role `both` to start the built-in worker in-process using the local key. Wire `worker` subcommand to run against `SHRINK_SERVER` with a key from `<data-dir>/node.key` (pairing comes in task 08; for now the key file must exist).
- On `complete`, the server records the output fingerprint on the file, sets `done`, `satisfied`, and `bytes_saved`, and emits events.

## Out of scope
Remote discovery, pairing, file transfer (tasks 08, 09), UI.

## Acceptance
- [ ] Integration test: library with one h264 fixture, `serve` with role `both`, job completes, output is hevc and smaller, original replaced in place with `.mkv`, file is `satisfied`, a rescan queues nothing.
- [ ] A corrupt output (inject a fake ffmpeg that writes garbage) fails verification; original is unchanged; job is `failed` with a log tail.
- [ ] `replace_if_smaller` with a forced larger output keeps the original and marks the job `done` with `bytes_saved=0`.
- [ ] `keep_both` writes `<name>.shrunk.mkv` beside the original.
- [ ] Killing the worker process mid-job leaves the original intact; after the heartbeat timeout the job returns to `queued` and completes on retry; orphaned scratch is cleaned on worker restart.
- [ ] Cancel via API stops ffmpeg within 5s and the scratch dir is gone.
- [ ] Job record shows `encoder_used`, `bytes_before`, `bytes_after`, and `fallback_reasons`.
- [ ] A request with a revoked or unknown node key gets 401.
