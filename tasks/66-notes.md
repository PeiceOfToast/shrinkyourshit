# Task 66 notes: mount executor, startup remount and watchdog

## Plan (written before implementation)

Files, in order:
1. `internal/mount/mount.go` (new): `Executor`, `Options`, `Runner` (os/exec in production, a fake in tests),
   argument builders, the cifs credentials file, `Mount`, `Unmount`, `Test`, `Status`, `Startup`, `Watch`, `Run`.
2. `internal/mount/mount_test.go` (new): fake runner, temp dirs, fake `/proc/mounts`, fake clock.
3. `internal/api/storage.go`: exported `CheckStorageHost(setting, protocol, source)` so the executor's
   gate runs the same allowlist rule the API uses (the parser is unexported in package api).
4. `cmd/shrinkyourshit/mounts.go` (new): adapter that makes `*mount.Executor` satisfy `api.StorageMounter`
   (the interface returns `api.StorageState`), and the gate closure that reads `storage_allowed_hosts`.
5. `cmd/shrinkyourshit/main.go`: build the executor after `svc.Start`, `go Run(ctx)`, pass
   `api.WithStorageMounter`.
6. `API.md` / `ARCHITECTURE.md`: short note that the status fields are now filled, and that the
   executor is in `internal/mount`. No endpoint or enum changes.
7. Tick the checklist, update `tasks/STATUS.md`, write the summary.

`internal/server/dto.go` needs no change: the storage DTO (with `mounted` and `error`) lives in
`internal/api/storage.go`, which task 65 already wired. The dashboard has no warnings type
(grep for "warning" in `internal/server` and `internal/api` finds nothing relevant), so the
dashboard warning stays with task 68.

## Design decisions (the owner may redirect)

- **Mount package does not import api.** `mount` depends on `store` only. The API seam is
  satisfied by an adapter in `cmd/`. This keeps the executor usable from the watchdog and startup
  without the HTTP layer.
- **Own per-storage lock.** The API already locks per storage for its own calls. The executor also
  takes its own per-storage mutex inside Mount, Unmount, Test and the watchdog, because the watchdog
  and startup never go through the API. The two locks never nest in the other direction, so there
  is no cycle.
- **Allowlist gate inside the executor.** Startup and the watchdog bypass the API handlers, so a host
  removed from `storage_allowed_hosts` would still be remounted at boot. The executor therefore calls
  an injected `Gate` before every mount-side action (Mount, Test, and so every watchdog remount).
- **Deterministic credentials path.** `<scratch>/cifs-<storage id>.cred`. A stale file from a crash is
  removed first. Creation uses `O_CREATE|O_EXCL` with mode 0600. A deterministic path lets the tests
  assert the exact argument vector. The per-storage lock means two mounts cannot share it.
- **Stale mounts are reported, not remounted.** The watchdog stats a mounted path with a short timeout
  (5 s). A timeout or error goes to `Status.Error` and the log. It does not unmount automatically,
  because a lazy unmount under a hung NFS client is a decision for the owner (see open questions).
- **Backoff.** Per storage: base 10 s, doubling, capped at 10 min, reset on success. Startup counts as a
  failure, so the watchdog does not immediately repeat it.
- **Test on an already mounted storage** probes in place and does not unmount, because a second mount
  on the same mountpoint would stack. The probe file is `.shrinkyourshit-probe`, written and removed
  in the share root, only when the storage is not read only.
- **Timings** are logged at info level (`mount_ms`, `stat_ms`, `probe_ms`, `unmount_ms`). The interface
  returns only an error, as task 65 defined it, so the API response stays `{ok, error?}`.
- **Disabled storages** are ignored by startup and the watchdog. A disabled storage that is still
  mounted is left alone (see open questions).
- **Tools.** `mount` and `umount` are called by absolute name from PATH, never through a shell. Both
  are looked up with `exec.LookPath` at call time, so a missing binary becomes a readable error on
  the storage rather than a crash.

## Exact argument vectors

Mountpoint `M` = `<root>/<name>`, default root `/storage`. `R` = read only flag.
Options are assembled in this order: credentials (smb only), the stored `options` split on commas,
then `ro` when `R` is set and the list does not already hold `ro`.

| Case | Program | Argument vector |
|---|---|---|
| NFS | `mount` | `-t nfs -o <opts> <source> M` |
| SMB | `mount` | `-t cifs -o credentials=<scratch>/cifs-<id>.cred[,<opts>] <source> M` |
| Unmount | `umount` | `M` |

Examples with the test fixtures:
- NFS `host:/export`, options `vers=3,soft,timeo=50,retrans=2`, not read only:
  `mount -t nfs -o vers=3,soft,timeo=50,retrans=2 nas:/export /storage/media`
- SMB `//nas/media`, options `vers=3.0`, read only, scratch `/scratch`:
  `mount -t cifs -o credentials=/scratch/cifs-<id>.cred,vers=3.0,ro //nas/media /storage/media`
  with the credentials file holding `username=...`, `password=...` and `domain=...` (domain line only when
  set). Its mode is 0600 while mount runs and it is gone when Mount returns.

## Gate results (2026-11-06, branch task/66-mount-executor)

- `go test -count=1 ./...` exit 0 (cmd, api 465 s, server 467 s, store 205 s, mount, worker and the rest all ok).
- `go test -count=20 ./internal/mount/` green (flake check on the timing tests).
- Mutation check: with `lock` replaced by a no-op, `TestConcurrentMountsOfOneStorageSerialise` fails (2 in flight). Restored, it passes.
- `go vet ./...` clean. `golangci-lint run` (v2.15.0): 0 issues on the changed packages. `scripts/check-spdx.sh` exit 0.
- `make test-web` (npm run test): 65 files, 474 tests pass. No web file changed.
- `-race` could not run: the build box has no gcc, so cgo is off. Not run.
- `gofmt -l` lists `installer_test.go` and `nodes_test.go`, which I did not touch.
- Not run: `make test-integration` (no ffmpeg path changed) and the Playwright suite (no UI change).

## Build notes

- Credentials path is `<scratch>/cifs-<id>.cred`. `mkdir -p` on the scratch dir runs before the write, so a fresh install
  without a scratch dir still mounts SMB.
- `Test` on an already mounted storage probes in place and leaves it mounted.
- The mount executor is started in `cmd/shrinkyourshit/main.go` after `svc.Start`, with `go mounts.Run(ctx)`, and passed to
  the API with `api.WithStorageMounter(mountAdapter{mounts})`.
- `api.CheckStorageHost` is exported so the executor's gate runs the same allowlist rule as the handlers.
- Not done here: `SHRINK_MANAGE_MOUNTS` (task 67). The executor runs whenever the server runs.

## Summary

1. Built: `internal/mount/mount.go` (Mount, Unmount, Test, Status, Startup, Watch, Run, argument builders, the
   0600 cifs credentials file, bounded stat, backoff) and `mount_test.go` (31 test functions, fake runner, fake mount table,
   fake clock). `cmd/shrinkyourshit/mounts.go` (adapter and host gate) with `mounts_test.go`. Startup wiring in
   `main.go`. `api.CheckStorageHost` exported. API.md and ARCHITECTURE.md notes.
2. Skipped: `SHRINK_MANAGE_MOUNTS` (task 67), the dashboard warning (the dashboard has no warnings feed yet, task 68),
   and `dto.go` (the storage DTO with `mounted`/`error` lives in `internal/api/storage.go`, already wired in task 65).
3. Task 67 should know: the executor needs `mount` and `umount` in the image, plus `CAP_SYS_ADMIN` and the AppArmor
   override. Without them every mount returns the `mount` error text, and the storage shows it in `error`.
4. Open for the owner: the open questions below.

## Open questions for the owner

1. A stale mount (stat times out) is reported in `error` and never remounted. Should the watchdog lazily
   unmount (`umount -l`) and remount it after N failed checks? That risks an unmount under a live encode.
2. A storage disabled while mounted stays mounted. Should disabling unmount it? The API does not do
   that today, and the spec does not say.
3. Test on a storage that is already mounted probes in place. Confirm that is the wanted behaviour.
4. Timings from `Test` are logged, not returned, because the API contract is `{ok, error?}`. Confirm that is enough.
