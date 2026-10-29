# Task 17: In app update check and update UI

## Goal
The app tells you a new version exists, shows what changed, and offers the right action for how it is installed. Docker users get the exact commands, binary users get a button, nobody gets surprise restarts.

## Read first
API.md (task 17), ARCHITECTURE.md (update path). Depends on task 16 and task 10 for the App settings page.

## Do
- API:
  - `GET /api/v1/version` returns `{version, commit, date, runtime: "binary"|"container", update_check_enabled, channel}`.
  - `GET /api/v1/update/status` returns `{current, latest, update_available, release_url, release_notes, published_at, self_update_supported, reason}` where `reason` explains why self update is unavailable (container, read only install dir, permissions).
  - `POST /api/v1/update/check` forces a refresh, rate limited to once a minute.
  - `POST /api/v1/update/apply` starts the update for binary installs and streams progress on the existing events channel as `update.progress`, `update.succeeded`, `update.failed`. Refused for container installs with `409 {"code":"self_update_unsupported"}` and the instruction text.
- UI, App settings page, About card:
  - Current version, commit, build date, runtime, and a Check now button.
  - When an update exists: a banner with the new version, publish date, a link to the release notes, and the action that matches the runtime.
    - Binary: an Update button with progress, a warning that the app restarts, and a link to release notes.
    - Container: the exact `docker compose pull` and `docker compose up -d` lines in a copyable block, plus the image name.
  - A checkbox for automatic checks (on by default) and a channel selector. Automatic apply is not offered at all: this app never updates itself without a click.
  - A dashboard warning entry when an update is available, consistent with the other warnings from task 10.
- Never block startup on a network call. The update check runs in the background after start and on demand.
- Failure handling: no network, rate limit, or a release with no matching asset each produce a specific, quiet message in About and nothing anywhere else.

## Out of scope
Auto apply on a schedule, staged rollouts, telemetry about which versions are in use.

## Acceptance
- [ ] `GET /api/v1/version` reports the right runtime for a binary and inside the container. (`TestVersionEndpoint`)
- [ ] `GET /api/v1/update/status` reports `update_available` correctly against a fixture release, and `self_update_supported` is false in a container with a reason that names the fix. (`TestUpdateStatus`)
- [ ] `POST /api/v1/update/apply` is refused with 409 and the instruction text inside a container, and starts a streamed update for a binary install. (`TestApplyRefusedInContainer`, `TestApplyEmitsProgressEvents`)
- [ ] The About card renders the version, the runtime, and the right action for each runtime. (`AboutCard.test.tsx`)
- [ ] The app starts and serves normally with no network access; the update card shows a quiet failure and nothing else changes. (`TestOfflineStartup`)
- [ ] No path applies an update without an explicit click, verified by a test that asserts no apply call happens on load. (`AboutCard.test.tsx`)
