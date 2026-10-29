# Task 06 notes: first-run setup and admin auth

## Plan

Order of work (small commits):

1. `internal/store`: migration `0002_sessions.sql` (token_hash PK, created_at,
   expires_at), `Session` type, `CreateSession / GetSessionByHash /
   DeleteSession / DeleteExpiredSessions`, plus settings keys
   `preferred_languages` and `default_video_codec`. Store tests.
2. `internal/server/password.go`: argon2id hash/verify in PHC string form
   (`$argon2id$v=19$m=65536,t=2,p=1$...`), golang.org/x/crypto (pinned
   v0.36.0 so the go directive stays 1.23 per CI). Unit tests.
3. `internal/server/auth.go`: `Auth` service — status, CompleteSetup (only
   while incomplete, 409 after), Login (username + argon2 verify), Logout,
   Authenticate (SHA-256 token lookup, 30-day expiry), ChangePassword.
4. `internal/server/settings.go`: typed `Settings` service over the settings
   KV table with defaults (`login_enabled=false`, `preferred_languages=eng`,
   `default_video_codec=hevc`), `Update` validation (concurrency >= 1,
   known result policy, codec hevc|av1|h264, login on => password required
   when no hash exists, password change requires current password).
5. `internal/api/auth.go`: endpoints per API.md task 06 —
   `GET /setup/status`, `POST /setup`, `POST /auth/login` (cookie
   `sys_session`, HttpOnly, SameSite=Lax, Max-Age 30d; login rate limit
   5 failed attempts/min/IP, 6th => 429), `POST /auth/logout`,
   `GET /auth/me`, `GET/PUT /settings`. Real `requireAdmin`: pass when
   login disabled, else valid session. `/setup/*`, `/auth/login`, `/health`
   and the node/worker routes stay unwrapped.
6. `web/src`: api client additions; route guard (incomplete setup => `/setup`,
   login enabled without session => `/login`); setup wizard (3 steps, join
   card disabled "coming soon"); login page; settings page (with login
   on/off confirm); app shell (left nav Dashboard/Libraries/Nodes/Settings,
   top bar version + logout); placeholder Dashboard/Libraries/Nodes.
7. Tests: Go — every acceptance item (setup flow, 401/429, logout/expiry,
   argon2 prefix + hash never in responses/logs, login-on needs password).
   Web — Vitest for wizard steps/validation and guard redirects.
8. ARCHITECTURE.md: add Session to the data model and the two new setting
   keys (same commit, per rule 5). API.md: add request/response details for
   the task 06 endpoints.

## Blocked / questions

None so far.

## Decisions taken inside scope

- Rate limiter counts failed login attempts only (a 5th failure still blocks
  the 6th attempt, which the acceptance item requires).
- Cookie Path=/ so it applies to every /api/v1 route; no Secure flag (TLS is
  optional/reverse-proxy per ARCHITECTURE security section).
- Session tokens: 32 random bytes hex, only SHA-256 stored, like node keys.
- Turning login on from settings with no username stored also requires a
  username (login would be impossible otherwise); password is always required.
- Setup wizard auto-logs the user in when login was enabled (falls back to
  the login page on failure).

## Summary (end of run)

- Built: setup wizard endpoints + argon2id auth + session store + rate
  limiting + settings API; web route guard, wizard, login, settings, shell.
- Skipped: dashboard/libraries content (task 07), join-existing-server mode
  (task 08), nodes page (task 08).
- Next task should know: `internal/server.Auth` + `Settings` services exist;
  `requireAdmin` is real now — admin endpoints 401 without a session when
  login is enabled; `sys_session` cookie; settings keys
  `preferred_languages`/`default_video_codec` ready for task 07 easy mode.
- Watch out: web tests mock `./api`; the guard fetches setup status + /me.
