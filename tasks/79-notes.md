# Task 79 notes

## Plan (written before the code)

Files touched, in order:

1. `internal/mount/mounter.go` (new, platform neutral): `Mounter` interface, the
   flag and dropped option tables, `translateOptions`, `BuildMountSpec` (replaces
   `MountArgs`), `MountError` with the errno messages, `unsupportedMounter`.
2. `internal/mount/mounter_linux.go` (new, `//go:build linux`): `kernelMounter` on
   `golang.org/x/sys/unix` `Mount` and `Unmount`, plus `defaultMounter`.
3. `internal/mount/mounter_other.go` (new, `//go:build !linux`): `defaultMounter`
   returns `unsupportedMounter`.
4. `internal/mount/caps.go` (new): `capEffHasSysAdmin` and the startup check.
5. `internal/mount/mount.go`: drop `Runner`, `CommandError`, `execRunner`, `MountArgs`;
   add `Mounter`, `ProcStatus`, `BusyWait` options; mount and umount go through the
   `Mounter`; the umount EBUSY retry; the per-storage busy flag; `Run` logs the
   capability check.
6. `internal/mount/mount_test.go`: fake `Mounter`; every test that asserted an
   argument vector now asserts a `MountSpec` or a flag set.
7. `internal/api/storage_mount_options_test.go` (new): agreement test between the
   allowlist and the mount package's flag table.
8. `go.mod`: `golang.org/x/sys` moves from indirect to direct.

Order inside the package: mounter.go first (types and translation with tests), then
the Linux and other files, then mount.go, then the test rewrite. Each step builds.

## Dependency

`golang.org/x/sys` is not listed in STACK.md. It is already in the build graph as an
indirect dependency (v0.31.0, pulled in by another module), so this only promotes it
to a direct requirement. Justification: `unix.Mount` and `unix.Unmount` are the only
way to call mount(2) without cgo or a raw syscall table. It is the Go project's own
syscall package and it does not duplicate anything in STACK.md.

## Option translation: decision

The flag table lives in `internal/mount`. `internal/mount` must not import
`internal/api`, so the allowlist cannot be shared directly. Instead,
`internal/api/storage_mount_options_test.go` asserts that every flag option mount
knows is an allowlisted boolean, and that every dropped option the API allows is in
the allowlist. If someone adds a flag to one side only, the test fails.

Kernel flag values are the Linux ABI written as constants in `mounter.go`, not taken
from `x/sys`, so the translation and its table test build and run on every platform.

Deviation from the task table: `rw` is dropped, the same as `defaults` and `_netdev`.
The task table did not list it. `rw` is the kernel default (no MS_RDONLY), and mount(8)
never passes it on, so it is not sent as data. Passing it as data would be the same
class of mistake as the `_netdev` case.

## Errno messages (exact text, asserted by tests)

Prefix for a mount: `mount -t <fstype> <source> at <target>: `. For an unmount:
`umount <target>: `.

| errno | message after the prefix |
|---|---|
| `EPERM`, `EACCES` | `not permitted; the container needs cap_add: SYS_ADMIN and security_opt: apparmor=unconfined` |
| `ENODEV` (mount only) | `no nfs client is available; the image needs the nfs-common package` (cifs: `no cifs client is available; the image needs the cifs-utils package`) |
| `ETIMEDOUT`, `EHOSTUNREACH` | `the server did not answer (<source>); check the host name and that it is reachable from the container` |
| anything else | the errno text, for example `invalid argument` |

Unmount: the first attempt is `umount(target, 0)`. Only `EBUSY` triggers the retry:
wait `BusyWait` (500ms in production), then one `umount2(target, MNT_DETACH)`. The
first attempt never detaches. The second attempt's error is returned as it is.

## Timeouts

`mount(2)` cannot be cancelled. The old `exec` path was killed at `MountTimeout`. A
hung NFS hard mount would otherwise block the watchdog forever. So each kernel call
runs in its own goroutine, and the caller waits at most `MountTimeout`. On timeout
the call keeps running, the storage is marked busy, and no second mount or unmount of
that storage starts until the first returns. This is a behavioural change from the
exec path and is recorded here so the owner can judge it.

## Credentials

The credentials file is still written 0600 and removed in the `defer` of `mountOnce`.
Known gap: on a timeout the defer removes the file while the kernel call may still be
inside the cifs option parser. That window is after `MountTimeout` only. Not changed.

## Non-Linux build

`GOOS=darwin` and `GOOS=windows` builds of `./internal/mount` are checked below.
`unsupportedMounter` returns `in-container mounts need Linux; this build cannot mount
shares`, and a Linux test asserts it.

## Open risk, not fixed here (for the owner)

NFS sources that are host names (not IP addresses) may not resolve. `mount.nfs`
resolved names in userspace before. The kernel client resolves names through the
`dns_resolver` key type, which needs `/sbin/request-key` and keyutils in the image. Not
installed. The lab run used an IP (`10.10.1.50`), so this has not been tested. Fix
would be a userspace lookup that adds `addr=<ip>` to the data string. Out of scope
for 79. Needs a follow-up task before v1.12 relies on host names.

## Docs out of scope

`docs/design/encode-profiles-and-storage-2026-11-06.md` section 4.1 still says
`mount(8)`. Not edited here (task says stay in scope). Follow-up.

## Verification

- `GOOS=darwin`, `GOOS=windows`, `GOOS=freebsd` builds of `./internal/mount/`: OK.
- Full binary: `GOOS=darwin GOARCH=arm64 go build ./cmd/shrinkyourshit` and
  `GOOS=windows GOARCH=amd64 go build ./cmd/shrinkyourshit`: OK.
- `go vet ./internal/mount/` on linux, darwin and windows: OK.
- `go test -count=1 ./internal/mount/ ./internal/api/` (the new agreement test
  included): OK. Package `internal/mount` is green with 0 skips.
- `golangci-lint run ./internal/mount/... ./internal/api/...`: 0 issues. Run with
  the cached Go 1.26.10 toolchain, because the ambient Go 1.27 breaks
  golangci-lint v2.15.0 at type-check time (environment issue, not code).
- `scripts/check-spdx.sh`: exit 0.
- `go test -race ./internal/mount/`: could not run, no `gcc` on this VM, so cgo is off.
- `go test ./...`: `cmd/shrinkyourshit`, `deploy`, `internal/config`, `discovery`,
  `encoders`, `flow`, `mount`, `plugin`, `probe`, `secret`, `store`, `update`, `worker`
  passed. `internal/api` and `internal/server` hit the default 10 minute package
  timeout in the first run (it ran alongside the lint and race checks). They are
  rerun alone with `-timeout 40m`, see the last lines of this file.
- `make test-web` and `make lint-web` were not run: no web file changed.
- `DECISIONS.md` unchanged.

## Live box check (not run here)

This VM has `CapEff` 0000000000000000, so on this box the startup check logs its
error and every mount returns the EPERM message. That is the expected behaviour, but
it is not a substitute for the real-hardware run in task 81.

## Summary (5 lines)

1. Storage mounts now call mount(2) and umount2(2) in process through a `Mounter`
   interface; no `mount` or `umount` process is started.
2. Option translation: ro, noatime, nodev, noexec, nosuid become flags;
   _netdev, defaults and rw are dropped; the rest reaches data in order.
3. Errno messages for EPERM/EACCES, ENODEV, ETIMEDOUT/EHOSTUNREACH and a default;
   EBUSY on unmount retries once with MNT_DETACH after 500ms.
4. Skipped: nothing from the acceptance list. Not verified here: a real NFS or SMB
   mount (task 81), and the open hostname-resolution risk above.
5. Next task (80) changes the NFS default to vers=4.2; it should also check whether
   a host name source still mounts (see Open risk).

## Rerun result (api and server alone, `-timeout 40m`)

- `internal/api`: ok, 606.6s. `internal/server`: ok, 560.8s. Both pass.
- Both packages take about the default 10 minute limit on this VM, so `go test ./...`
  can fail on the default timeout under load. Not caused by task 79 (nothing in those
  packages changed except one new fast test in `internal/api`). Flagged for the owner:
  a `-timeout` in the Makefile would remove the risk. Not changed here.
- `make` is not installed on this VM, so the make targets were run by hand: `go test ./...`, `golangci-lint run`, `scripts/check-spdx.sh`, `go vet`.
