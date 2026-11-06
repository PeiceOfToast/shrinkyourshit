# Task 66: Mount executor, startup remount and watchdog

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.1 and 4.4. `internal/worker/access.go` for how paths are probed, `internal/server/scanner.go`
for where a filesystem error is handled.

## Scope
Create `internal/mount/mount.go`, `internal/mount/mount_test.go`.
Modify the server startup wiring, `internal/server/dto.go` (status fields), the dashboard warning
feed if it already has a warnings type.

## Do
1. `mount.Mount(ctx, storage)`: build the argument vector (never a shell), write a 0600 cifs
   credentials file when the protocol is smb, run `mount -t nfs` or `mount -t cifs`, delete the
   credentials file in a defer, and return stderr verbatim on failure.
2. `Unmount`, `Test` (mount, stat, write and remove a probe file unless read only, unmount,
   report timings), and `Status` reading `/proc/mounts` for `/storage/<name>`.
3. Startup: remount every enabled storage, log one line each, never fail the server if one fails.
4. Watchdog: on an interval, compare desired against `/proc/mounts`, remount a missing mount with
   exponential backoff, and record the last error. A stale entry that still appears mounted is
   reported by a stat with a short timeout, because a hang must not block the loop.
5. Tests use a fake runner, so nothing needs real privileges in CI.

## Acceptance
- [x] Argument vectors asserted exactly for nfs and smb, including the credentials file path.
- [x] A mount failure surfaces stderr and leaves the server running, proven by a test.
- [x] The credentials file does not exist after mount returns, success or failure.
- [x] Watchdog remounts a mount that vanished and backs off on repeated failure.
- [x] `make test` green.

## Notes
Write the plan and the exact argument vectors in `tasks/66-notes.md`.
