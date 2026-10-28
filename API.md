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
