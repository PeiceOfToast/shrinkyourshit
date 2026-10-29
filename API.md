# API

Shared contract for all tasks. Add endpoints here when a task introduces them; do not rename existing ones.

## Conventions

- Base path `/api/v1`. JSON request and response bodies. `Content-Type: application/json`.
- Errors: HTTP status plus body `{"error": "human readable", "code": "snake_case_code"}`.
- IDs are strings (UUID v4).
- Timestamps are RFC 3339 UTC.
- Lists return `{"items": [...], "total": n}`. Pagination with `?limit=&offset=` where noted.
- Two auth modes:
  - **Admin (UI):** session cookie `sys_session`, HttpOnly, SameSite=Lax. When login is disabled, every admin endpoint is open.
  - **Node (workers):** header `Authorization: Bearer <node-key>`. The built-in worker uses a key generated at first run and stored in the data dir.
- Admin endpoints never accept node keys and node endpoints never accept session cookies.
- Events: `GET /api/v1/events` is Server-Sent Events. Each event has `event: <type>` and `data: <json>`. Types: `file.updated`, `job.updated`, `node.updated`, `library.updated`.

## Endpoints by task

### Task 01

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/health` | none | `{"status":"ok","version":"v0.0.0"}` |

### Task 03

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/libraries` | admin | list |
| POST | `/api/v1/libraries` | admin | create: `{name, path, target, watch, schedule?}` |
| GET | `/api/v1/libraries/{id}` | admin | get |
| PUT | `/api/v1/libraries/{id}` | admin | update, triggers re-evaluation of files |
| DELETE | `/api/v1/libraries/{id}` | admin | delete DB records only |
| POST | `/api/v1/libraries/{id}/scan` | admin | start a scan now |
| GET | `/api/v1/libraries/{id}/files` | admin | paginated, filter `?status=` |
| GET | `/api/v1/jobs` | admin | paginated, filter `?status=` |
| GET | `/api/v1/jobs/{id}` | admin | includes log tail |
| POST | `/api/v1/jobs/{id}/cancel` | admin | |
| POST | `/api/v1/jobs/{id}/retry` | admin | |
| GET | `/api/v1/events` | admin | SSE |
| GET | `/api/v1/fs/browse?path=` | admin | server-side directory listing for the folder picker |

### Task 05

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/worker/heartbeat` | node | `{capabilities, running_job_ids}` |
| POST | `/api/v1/worker/claim` | node | returns one job or 204 |
| POST | `/api/v1/worker/jobs/{id}/progress` | node | `{percent, speed, eta_seconds, encoder}` |
| POST | `/api/v1/worker/jobs/{id}/complete` | node | `{output_path, bytes_after, encoder, fallback_reasons[]}` |
| POST | `/api/v1/worker/jobs/{id}/fail` | node | `{error, log_tail}` |
| GET | `/api/v1/worker/jobs/{id}` | node | job spec, target, source path, access mode |

Node auth: `Authorization: Bearer <node-key>`, SHA-256-hashed and looked up
in the nodes table; the node must be `approved` and `enabled`. Anything
else gets `401 {"code":"unauthorized"}`. A node touching a job another node
owns gets `403 {"code":"not_job_owner"}`; a transition on a job that is no
longer claimed/running gets `409 {"code":"job_not_active"}`.

Request/response details beyond the table:

- `POST /api/v1/worker/heartbeat` — response `{"concurrency": n}`
  (`worker_concurrency` from settings, default 1). `capabilities` is the
  node capabilities object `{encoders: [{name, codecs: []}], cpu_cores,
  ffmpeg_version}`, stored verbatim; `running_job_ids` refreshes the
  heartbeat of this node's claimed/running jobs so a long encode without
  progress is not swept.
- `POST /api/v1/worker/claim` — 200 with the JobDTO, or 204 when nothing is
  queued. The job's file moves to `processing`.
- `GET /api/v1/worker/jobs/{id}` — the job spec:

  ```json
  {
    "id": "…", "file_id": "…", "library_id": "…", "attempt": 0,
    "target": { …Target… },
    "source_path": "/node/side/path.mkv",
    "source_size": 123, "source_mtime": 1700000000,
    "fingerprint": "123:1700000000:abc…",
    "access_mode": "shared",
    "result_policy": "replace_if_smaller",
    "bytes_before": 123
  }
  ```

  `source_path` is the node-side path: the server applies the node's path
  mappings (`[{"server_path": "/media", "node_path": "/mnt/nas"}]`,
  longest prefix wins, identity when empty). `access_mode` is the node's
  effective mode; `auto` resolves to `shared` until the shared-storage
  check arrives with task 09.
- `POST /api/v1/worker/jobs/{id}/progress` — response
  `{"status": "<job status>"}`; the worker polls it and treats `cancelled`
  as the kill signal.
- `POST /api/v1/worker/jobs/{id}/complete` — also accepts
  `skipped_reason` (set when the result policy kept the original, e.g. the
  output was not smaller) and `log_tail` (stored on the job, capped at
  64KiB). `fallback_reasons` is a string array (`"nvenc: <error>"`),
  matching the JobDTO shape. `output_path` is the final path of the
  produced file in the library (the extension may have changed to the
  target container). On complete the server records the output fingerprint
  and probe, sets the file `satisfied` (`done` with `bytes_saved=0` for a
  skip) and emits `job.updated` and `file.updated`.
- `POST /api/v1/worker/jobs/{id}/fail` — the job becomes `failed`, the file
  `failed`; `log_tail` is capped at 64KiB.

### Task 06

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/setup/status` | none | `{setup_complete, login_enabled}` |
| POST | `/api/v1/setup` | none, only while setup incomplete | `{mode: "server", login_enabled, username?, password?}` |
| POST | `/api/v1/auth/login` | none | sets cookie |
| POST | `/api/v1/auth/logout` | admin | |
| GET | `/api/v1/auth/me` | admin | `{username, login_enabled}` |
| GET | `/api/v1/settings` | admin | |
| PUT | `/api/v1/settings` | admin | concurrency, scratch dir, result policy, login on/off, password change |

### Task 07

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/stats` | admin | totals: files, satisfied, queued, bytes saved, per-library breakdown |

### Task 08

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/nodes` | admin | list with status and capabilities |
| POST | `/api/v1/nodes/tokens` | admin | create one-time pairing token, returns token and the generated install commands |
| POST | `/api/v1/nodes/{id}/approve` | admin | |
| POST | `/api/v1/nodes/{id}/revoke` | admin | |
| PUT | `/api/v1/nodes/{id}` | admin | rename, enable/disable, path mappings |
| POST | `/api/v1/pair` | none | `{token?, name, os, arch, capabilities}`; returns `{node_id, node_key}` on token match, or `{node_id, status:"pending"}` |
| GET | `/api/v1/pair/{node_id}` | none | poll pending approval; returns node key once approved |

### Task 09

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/worker/jobs/{id}/access-check` | node | `{library_id}` returns `{path, size, sample_offset, sample_sha256}` for the shared-storage check |
| GET | `/api/v1/worker/jobs/{id}/source` | node | streams the source file, supports `Range` |
| PUT | `/api/v1/worker/jobs/{id}/result` | node | upload result; supports resumable chunks via `Content-Range`; final request includes `X-Checksum-SHA256` |
