<!-- SPDX-License-Identifier: AGPL-3.0-only -->

# Task 17 notes: in-app update check and update UI

## Plan

Backend (order):

1. `internal/update`: add an optional `Progress func(stage string)` hook to
   `Options` (called at checking / backup / downloading / verifying /
   swapping) and a `ContainerReason()` helper the status endpoint and the
   409 body share. No behavior change otherwise.
2. `internal/server/updatecheck.go` (new): the update-check service.
   - State: last check result (release, checked-at, error), last refresh
     time (rate limiter), apply progress snapshot.
   - `Status(ctx)` — pure snapshot, never touches the network. First answer
     is `update_available: false, reason: "not checked yet"`.
   - `Check(ctx)` — forced refresh, rate limited to once per minute
     (`429 rate_limited` past that); reads the channel from settings;
     network failure becomes a quiet `reason` in the status, not a 5xx.
   - `StartBackgroundCheck(ctx)` — goroutine after a short delay; skipped
     when the `update_check` setting is off. Wired by main, not by
     `Server.Start`, so unit tests never touch the network.
   - `Apply` — refuses containers / unknown-version / non-writable install
     dir synchronously (409 `self_update_unsupported` with the fix in the
     message), otherwise runs `update.Apply` in a goroutine, publishing
     `update.progress` / `update.succeeded` / `update.failed` and calling
     the restart hook after success.
3. `internal/server`: `Config` gains `DataDir` and `Update UpdateOptions`
   (test overrides: container detection, hub base, http client, executable,
   now, restart hook, background delay, rate window); `events.go` gains the
   three update event types.
4. `internal/api`: version endpoint gains `update_check_enabled` and
   `channel`; new routes `GET /api/v1/update/status`,
   `POST /api/v1/update/check`, `POST /api/v1/update/apply` (admin).
5. `cmd/shrinkyourshit/main.go`: pass DataDir + build identity + restart
   hook (`restartSelf` after a short delay so the SSE events flush), start
   the background check after the server starts.
6. `API.md`: task 17 details — event types, `progress` field, status codes,
   rate limit, `update_in_progress`.

Frontend:

7. `web/src/api.ts`: version/update types + calls; `Settings` gains the
   task-16 update fields it was missing.
8. `web/src/events.ts`: update events invalidate `['update-status']`.
9. `web/src/AboutCard.tsx` (new, mounted on the App settings page): version
   rows, Check now, update banner with per-runtime action (Update button +
   restart warning for binary, copyable `docker compose` block for
   container), automatic-checks checkbox, channel selector. Check failures
   are quiet in-card messages, never toasts.
10. `web/src/dashboardWarnings.ts` + `DashboardPage`: "update available"
    warning entry linking to /app.

Tests map to the acceptance list; see the task file. UI: `AboutCard.test.tsx`
covers the card rendering, per-runtime actions, quiet failures and the
no-apply-on-load assertion.

## Questions / decisions

None blocking. Two judgement calls, recorded here for the owner:

- `update_available` is tag-string inequality (same comparison `shrink
  update` uses), so a dev build (`v0.0.0`) shows the banner with the release
  link while `self_update_supported` is false with the rebuild reason.
- The status response carries an optional `progress` snapshot
  (`{active, stage, error}`) so the About card can render update progress
  from the single SSE subscription's invalidations (documented in API.md in
  the same commit).

## Summary (written at the end)

- Built: the update-check service (`internal/server/updatecheck.go`) with a
  never-blocking status snapshot, a once-per-minute forced refresh, one
  background check after start (skipped when `update_check` is off) and an
  apply that streams `update.progress` / `update.succeeded` / `update.failed`
  and restarts the process onto the new binary; three admin endpoints plus
  the extended `/api/v1/version`; the About card (version rows, Check now,
  per-runtime action, automatic-checks checkbox, channel selector) on the
  App settings page; a dashboard "update available" warning.
- Skipped: auto apply on a schedule, staged rollouts, telemetry (out of
  scope). No store migration was needed.
- Tests: 10 Go tests (`internal/api/update_test.go`,
  `internal/server/updatecheck_test.go`, extended `TestVersionEndpoint`)
  and 9 AboutCard tests cover every acceptance item; `make test`,
  `make test-integration` and `make lint` are green.
- What the next task should know: the stage names shared by events and the
  status `progress` field are the single vocabulary for update progress
  (`Stage*` in `internal/update`, plain words in `web/src/updateModel.ts`);
  a successful in-app apply execs the new binary, so anything that runs
  code after `Restart` in main must survive process replacement. The rate
  window and background delay are `server.UpdateOptions` fields for tests.
- Owner note: the first run of a release build pings
  `api.github.com` once in the background; disable with the About card's
  automatic-checks checkbox or the `update_check` setting.
