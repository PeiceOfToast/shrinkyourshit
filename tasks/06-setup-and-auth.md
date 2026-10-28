# Task 06: First-run setup and admin auth

## Goal
A fresh server shows a setup wizard. The user creates the single admin account or turns login off. After that, the API is protected (or open) accordingly.

## Read first
API.md (task 06 endpoints, auth conventions), ARCHITECTURE.md (settings keys, security), DECISIONS.md (16), STACK.md (argon2). Depends on task 03.

## Do
- `internal/api` auth:
  - `GET /setup/status`, `POST /setup` (only while `setup_complete` is false; afterwards 409).
  - `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`. Sessions: random 32-byte token, stored hashed in a `sessions` table (add migration), 30-day expiry, cookie per API.md. Rate-limit login to 5 attempts per minute per IP.
  - Admin middleware (replace the task 03 placeholder): if `login_enabled` is false, pass; else require a valid session. `/setup/*`, `/auth/login`, `/health`, `/pair*`, and `/worker/*` (node auth) are excluded.
  - `GET /settings`, `PUT /settings`: `worker_concurrency`, `scratch_dir`, `default_result_policy`, `login_enabled`, `preferred_languages`, `default_video_codec` (hevc|av1|h264), password change (requires current password when login is enabled).
- `/web`:
  - Route guard: if `setup_complete` is false, every route redirects to `/setup`. If login is enabled and no session, redirect to `/login`.
  - Setup wizard, 3 steps: (1) "Make this the server" or "Join an existing server" (join is a disabled card with text "coming soon" until task 08); (2) login on/off, username and password with confirmation; (3) summary and finish.
  - Login page.
  - Settings page with the fields above and a "Turn login off/on" toggle with confirmation.
  - App shell: left nav (Dashboard, Libraries, Nodes, Settings), top bar with version and logout. Dashboard and Libraries can be placeholders; task 07 fills them.

## Out of scope
Easy-mode screen, dashboard content, node pages, multi-user, SSO.

## Acceptance
- [ ] Fresh data dir: `GET /setup/status` reports incomplete; any UI route redirects to `/setup`; completing setup with login enabled then calling `GET /libraries` without a cookie returns 401.
- [ ] Setup with login disabled: `GET /libraries` works with no cookie; `GET /auth/me` reports `login_enabled=false`.
- [ ] `POST /setup` after completion returns 409.
- [ ] Wrong password returns 401; sixth attempt within a minute returns 429.
- [ ] Logout invalidates the session; expired sessions are rejected.
- [ ] Password hashes are argon2id (test inspects the stored prefix) and never appear in any API response or log.
- [ ] Turning login on from settings when it was off requires setting a password in the same request.
- [ ] Vitest tests: wizard step navigation and validation, route guard redirects.
