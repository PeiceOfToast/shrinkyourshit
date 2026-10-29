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
  configured mode; in task 09 `auto` is passed through and the worker
  resolves it with the shared-storage access check.
- `POST /api/v1/worker/jobs/{id}/progress` — response
  `{"status": "<job status>"}`; the worker polls it and stops the encode
  when the status is terminal (`cancelled`, `failed`, `done`). A job that
  left the active state while it was encoding answers 200 with its terminal
  status (not 409), so the worker always learns it should stop.
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

Request/response details beyond the table:

- `GET /api/v1/setup/status` — `{setup_complete, login_enabled, username?}`; `username` only present when login is enabled.
- `POST /api/v1/setup` — `{mode: "server", login_enabled, username?, password?}`. `mode` other than `"server"` is 400 (joining arrives with task 08). Login enabled requires `username` and `password`. Response is the same shape as the status endpoint with `setup_complete: true`, status 201. After completion the endpoint answers `409 {"code":"setup_complete"}`.
- `POST /api/v1/auth/login` — `{username, password}`. Success sets the `sys_session` cookie (HttpOnly, SameSite=Lax, Path=/, 30-day Max-Age) and returns `{setup_complete, login_enabled: true, username}`. Wrong credentials are `401 {"code":"invalid_credentials"}` (never says which part was wrong). The sixth failed attempt within one minute from one IP is `429 {"code":"too_many_attempts"}`; only failed attempts count.
- `GET /api/v1/auth/me` — same shape as the status endpoint; 401 when login is enabled and the session is missing, unknown or expired. 200 with `login_enabled: false` when login is off.
- `GET /api/v1/settings` —
  `{"worker_concurrency": 1, "scratch_dir": "", "default_result_policy": "replace_if_smaller", "login_enabled": false, "admin_username": "", "preferred_languages": ["eng"], "default_video_codec": "hevc"}`. Never contains `admin_password_hash`.
- `PUT /api/v1/settings` — any subset of the GET fields, plus `login_enabled` (`bool`),
  `password` and `current_password`. Turning login on requires `password` in the
  same request (and `admin_username` when none is stored). A password change
  requires `current_password` to verify when login is enabled (`401
  {"code":"invalid_credentials"}` otherwise). Invalid values (concurrency out
  of range, unknown result policy or codec) are `400 {"code":"invalid_request"}`.
  Response is the updated settings document.

### Task 07

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/stats` | admin | totals: files, satisfied, queued, bytes saved, per-library breakdown |

Request/response details beyond the table:

- `POST /api/v1/libraries` and `PUT /api/v1/libraries/{id}` (task 03
  endpoints) accept either a full `target` or an `easy` object
  `{quality: 1..5, remove_subtitles: bool, audio: 1..4}`; sending both is
  `400 {"code":"invalid_request"}`. The server compiles `easy` with the
  current settings (`default_video_codec`, `preferred_languages`) and stores
  both the compiled Target and the raw easy input (a JSON column on the
  library). An update that carries only `target` clears the stored easy
  input. Out-of-range slider values are `400 {"code":"invalid_request"}`.
  On create an empty `name` defaults to the library path's base name (the
  easy screen has no name field).
- `LibraryDTO` gains `easy`: the stored easy input when the library was
  saved from the easy screen, the described input when the Target is
  easy-shaped, otherwise `null`.
- `GET /api/v1/stats` returns totals plus a per-library breakdown; every
  total equals the sum of the breakdown:

  ```json
  {
    "files": 6, "new": 1, "satisfied": 2, "queued": 1, "processing": 0,
    "done": 1, "failed": 1, "missing": 0, "bytes_saved": 550,
    "libraries": [
      {"library_id": "…", "name": "Movies", "files": 4, "new": 0,
       "satisfied": 2, "queued": 1, "processing": 0, "done": 1, "failed": 0,
       "missing": 0, "bytes_saved": 500}
    ]
  }
  ```

- `GET /api/v1/jobs` accepts `?order=desc` (newest first; default oldest
  first) for the dashboard's recent history.

### Task 08

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/nodes` | admin | list with status and capabilities |
| POST | `/api/v1/nodes/tokens` | admin | create one-time pairing token, returns token and the generated install commands |
| POST | `/api/v1/nodes/{id}/approve` | admin | |
| POST | `/api/v1/nodes/{id}/revoke` | admin | |
| PUT | `/api/v1/nodes/{id}` | admin | rename, enable/disable, path mappings |
| DELETE | `/api/v1/nodes/{id}` | admin | remove a node (the Reject action); its in-flight jobs return to the queue |
| POST | `/api/v1/pair` | none | `{token?, name, os, arch, capabilities}`; returns `{node_id, node_key}` on token match, or `{node_id, status:"pending"}` |
| GET | `/api/v1/pair/{node_id}` | none | poll pending approval; returns node key once approved |
| GET | `/api/v1/setup/servers` | none | servers discovered on the LAN for the wizard's join card; `{"servers":[]}` once setup is complete |

Request/response details beyond the table:

- Node keys and pairing tokens are 32 random bytes, base64url (43
  characters). Only SHA-256 hashes are stored (nodes.key_hash,
  pairing_tokens.token_hash): the keys are high-entropy, so argon2id
  would add nothing. A node's raw key exists in exactly three places:
  the response that delivered it, the `nodes.key_once` column until the
  node retrieves it once, and the node's own key file. Neither the key
  nor the token is ever logged or included in an event payload.
- `POST /nodes/tokens` —
  `{"token", "expires_at", "commands": {"docker", "shell", "powershell"}}`.
  The token is single use and valid for 24h. The commands are built from
  the request's own origin (`Host` header; scheme from
  `X-Forwarded-Proto`, then TLS, then http) so a node reaches the server
  the same way the admin's browser does.
- `POST /nodes/{id}/approve` — pending → approved; a fresh key is
  generated and waits in `key_once` for the node's first poll. Not
  pending → `409 {"code":"not_pending"}`.
- `POST /nodes/{id}/revoke` — status `revoked`, key hash and one-time key
  deleted; the node's claimed/running jobs return to `queued`
  (attempt+1, failed at the attempt cap). Idempotent.
- `PUT /nodes/{id}` — any subset of `{name, enabled, access_mode,
  path_mappings}`. Unknown access modes and malformed mappings are
  `400 {"code":"invalid_request"}`. `path_mappings` is the JSON array
  `[{"server_path", "node_path"}]` (both non-empty); `[]` clears it.
- `POST /pair` — 201. A valid token creates the node approved and returns
  `{node_id, status:"approved", node_key}`. No (or empty) token creates a
  pending node and returns `{node_id, status:"pending"}`. A used, expired
  or unknown non-empty token is `401 {"code":"invalid_token"}` — a wrong
  token never silently downgrades to a pending request. `name` is
  required (`400` when missing); `capabilities` is stored verbatim.
- `GET /pair/{id}` — `{node_id, status}` where status is `pending`,
  `approved` (with `node_key` on the first retrieval only, without it
  afterwards) or `revoked`. Unknown id → 404.
- `GET /setup/servers` — `{"servers": [{"id", "name", "url", "version"}]}`;
  `id` is the server's built-in node id so the wizard can hide its own
  announcement from the list.
- `POST /setup` with `mode:"join"` — `{server, token?}`: records the
  decision (role `worker` and the server URL in the data-dir config.yaml,
  via the process) and marks setup complete; response
  `{setup_complete: true, mode: "join", server}` with status 201. The
  live process then switches to worker mode and runs the pairing flow.
  Missing or unparsable server URL is `400`; after setup
  `409 {"code":"setup_complete"}`.

### Task 09

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/worker/jobs/{id}/access-check` | node | `{library_id}` returns `{path, size, sample_offset, sample_sha256}` for the shared-storage check |
| GET | `/api/v1/worker/jobs/{id}/source` | node | streams the source file, supports `Range` |
| PUT | `/api/v1/worker/jobs/{id}/result` | node | upload result; supports resumable chunks via `Content-Range`; final request includes `X-Checksum-SHA256` |
| GET | `/install.sh` | none | the POSIX installer script, templated with this server's URL and version |
| GET | `/install.ps1` | none | the PowerShell installer script, same templating |

Request/response details beyond the table:

- `POST /api/v1/worker/jobs/{id}/access-check` — the job must be claimed by
  this node (`409`/`403` otherwise, like every job endpoint). The optional
  body's `library_id` must match the job's library when present
  (`400 invalid_request` otherwise). The response's `path` is the node-side
  path (the node's path mappings are applied server-side, exactly like the
  job spec's `source_path`); `size` is the source size; `sample_offset` is a
  random offset and `sample_sha256` the SHA-256 of the 1 MiB starting there
  (short tail of the file at the end). The node reads the same bytes through
  its mapping, compares, and caches the outcome per library for 10 minutes.
  In `auto` mode match means shared, anything else transfer; the node
  reports the mode it chose in the progress payload and the server stores it
  on the job.
- `GET /api/v1/worker/jobs/{id}/source` — `http.ServeContent` semantics:
  `Range`/`If-Range` supported (interrupted downloads resume), `ETag` is
  the quoted file fingerprint, `Last-Modified` the file mtime.
- `PUT /api/v1/worker/jobs/{id}/result` — one chunk per request. Headers:
  `Content-Range: bytes <start>-<end>/<total>` (strict, no wildcards; chunk
  bodies beyond 64 MiB are `400`) and, on the final chunk only,
  `X-Checksum-SHA256` over the whole result. The server writes chunks at
  their offset into `<server-scratch>/uploads/job-<id>/output.<ext>.part`:
  repeats of already-received ranges overwrite their region, a chunk past
  the received prefix is `409 {"code":"upload_gap"}` (the node restarts from
  byte 0). On the final chunk the server verifies size and checksum;
  `422 {"code":"checksum_mismatch"}` discards the upload and the original is
  untouched. Success answers `204` per chunk and `200 {"received": total}`
  for the final one, after which the verified output waits at
  `output.<ext>` for the completion call. The extension follows the
  library's target container.
- `POST /api/v1/worker/jobs/{id}/complete` gains `access_mode`
  (`shared` or `transfer`). In transfer mode the server applies the result
  policy itself from the verified upload (reusing the shared result-policy
  code) before recording, so the worker never touches the library.
- `POST /api/v1/worker/jobs/{id}/progress` gains `access_mode` (the mode
  the node resolved for this job); the server stores it on the job.
- `GET /api/v1/worker/jobs/{id}` (job spec) — `access_mode` is the node's
  configured mode; `auto` is passed through and resolved by the worker with
  the access check. The spec also carries `throughput_limit_mbps` (the
  node's transfer cap; 0 = unlimited).
- `PUT /api/v1/nodes/{id}` gains `throughput_limit_mbps` (non-negative
  integer; anything else is `400 {"code":"invalid_request"}`). It appears in
  `GET /api/v1/nodes` responses as `throughput_limit_mbps`.
- `GET /install.sh` / `GET /install.ps1` — the installer sources
  (`scripts/installer.sh`, `scripts/installer.ps1`) with the request's
  server URL (`X-Forwarded-Proto` aware, same as the pairing commands) and
  the server's version substituted, so a node downloads the release binary
  matching the server it joins.

### Task 10

No new endpoints. Route and shell changes only:

- The settings endpoints stay the same; the UI splits them across `/encode` and `/app`.
- `GET /api/v1/stats` and `GET /api/v1/jobs` gain nothing; the dashboard now also reads `GET /api/v1/nodes` to decide whether to show the "nothing can run" warning.

### Task 11

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/nodes/{id}/start` | admin | set `desired_state=running` |
| POST | `/api/v1/nodes/{id}/stop` | admin | body `{drain: bool}`; `drain=true` sets `draining`, `false` sets `stopped` |
| POST | `/api/v1/nodes/{id}/drain` | admin | set `desired_state=draining` |
| POST | `/api/v1/nodes/pause-all` | admin | stop every node now, remember prior intent |
| POST | `/api/v1/nodes/resume-all` | admin | restore the remembered intent per node |

Details:

- `POST /api/v1/worker/heartbeat` responses gain `desired_state`, `concurrency`, and `effective_state`. This is how a worker learns it was started or stopped, so a stopped node makes no claim calls at all.
- `GET /api/v1/nodes` responses gain `desired_state`, `effective_state`, `concurrency`, and `current_job_id`.
- `PUT /api/v1/nodes/{id}` accepts `concurrency` (integer 1 to 16; anything else is `400 {"code":"invalid_request"}`).
- `enabled` stays the hard off switch enforced by node auth. `desired_state` is the operator control. A claim needs `status=approved`, `enabled=true` and `desired_state=running`.
- A fresh install, and any completed setup wizard that did not explicitly start the node, leaves `desired_state=stopped`.

### Task 12

No new endpoints. The Playwright suite drives the built web app through the Go server, so it uses the same routes as a browser.

### Task 13

- Error responses keep their shape, and every 4xx and 5xx path must carry a stable `code` field. The UI renders the status plus `code` plus the message, so a missing `code` is a bug.
- No other contract change.

### Task 14

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/libraries/{id}/flow` | admin | the stored graph plus its `source` |
| PUT | `/api/v1/libraries/{id}/flow` | admin | body `{source, graph}`; validates before saving |
| POST | `/api/v1/flows/validate` | admin | body `{graph}`; `200 {"valid":true}` or `400 {"code":"invalid_flow","errors":[{path,message}]}` |
| GET | `/api/v1/flows/step-types` | admin | the palette: step type id, label, description, parameter schema |

Details:

- Library responses gain `flow` and `flow_source` alongside the existing `target` and `easy`.
- An invalid graph never saves; validation errors name the field path so the UI can attach them to a row or a node.
- When a library has both `target` and `flow`, `flow` is authoritative and a mismatch is logged at warn level. `target` remains for compatibility with easy mode clients.

### Task 15

No new endpoints. The canvas uses `GET` and `PUT /api/v1/libraries/{id}/flow`, `POST /api/v1/flows/validate` and `GET /api/v1/flows/step-types`, so the server stays the only place that decides whether a graph is valid.

### Task 16

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/version` | none | `{version, commit, date, runtime}` where runtime is `binary` or `container` |

- CLI only beyond that endpoint: `shrink version`, `shrink update --check`, `shrink update`, `shrink update --rollback`.
- Releases publish `checksums.txt` beside the archives. The updater refuses any asset whose sha256 does not match or is missing from that file.
- Settings added: `update_check` (bool, default true), `update_channel` (`stable` or `prerelease`, default `stable`), `backup_before_update` (bool, default true).

### Task 17

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/version` | none | extended with `update_check_enabled` and `channel` |
| GET | `/api/v1/update/status` | admin | `{current, latest, update_available, release_url, release_notes, published_at, self_update_supported, reason}` |
| POST | `/api/v1/update/check` | admin | force a refresh, rate limited to once a minute, `429 {"code":"rate_limited"}` beyond it |
| POST | `/api/v1/update/apply` | admin | binary installs only; `409 {"code":"self_update_unsupported"}` with the pull instructions in a container |

Details:

- Progress streams on the existing events channel as `update.progress`, `update.succeeded` and `update.failed`.
- The status endpoint never blocks on the network at startup; the first answer may be `{update_available: false, reason: "not checked yet"}`.
- Nothing in this API applies an update on its own. There is no auto apply endpoint and none is planned.
