# Task 79: Mount with the mount(2) syscall, not a shelled-out mount(8)

Fix for a real-hardware blocker found on 2026-11-06 (see
`tasks/real-hardware-mount-evidence-2026-11-06.md`). Part of the v1.11 fix queue;
the spec is `docs/design/encode-profiles-and-storage-2026-11-06.md`.

`DECISIONS.md` is read only.

## The defect

Every WebUI-initiated mount fails against a real NAS:

```
mount.nfs: failed to apply fstab options
```

The server process runs as uid 1000. `mount.nfs` is a setuid helper, and when an
unprivileged caller passes options that are not fstab options, it refuses them.
The container has `CAP_SYS_ADMIN` effective (`CapEff 00000000a82425fb`, bit 21
set, verified inside the deployed image), but that capability belongs to the
server process, not to the setuid helper, so it does not help.

The same mount succeeds as root inside the same container:

```
docker exec -u root shrinkyourshit mount -t nfs -o vers=4.2 10.10.1.50:/mnt/DataSSD/SSD /storage/ssdpool
```

That proves the network path, the NAS export and the option string are all fine.
The failure is entirely in how the mount is issued.

## Read first

`internal/mount/mount.go` (whole file), spec 4.1, `internal/api/storage.go` around
`StorageMounter` and the option allowlist.

## Do

1. Replace the `Runner` exec path for mount and unmount with an in-process mount
   syscall so the process's own `CAP_SYS_ADMIN` applies. Introduce a small
   interface in `internal/mount` so tests stay privilege free:

   ```go
   type Mounter interface {
       Mount(source, target, fstype string, flags uintptr, data string) error
       Unmount(target string, flags int) error
   }
   ```

   Production implementation uses `golang.org/x/sys/unix` (`unix.Mount`,
   `unix.Unmount`), which is already an indirect dependency; promote it to a
   direct one in `go.mod`. Build tags: Linux gets the real implementation, other
   platforms compile and return a clear "in-container mounts need Linux" error.

2. Translate the option string. This is the subtle part and the reason a naive
   swap will fail: the option allowlist in `internal/api/storage.go` contains
   names that only `mount(8)` understands, and the kernel will reject them as
   unknown options. Map them instead of passing them through:

   | Option | Handling |
   |---|---|
   | `ro` | `MS_RDONLY` flag, not data |
   | `noatime` | `MS_NOATIME` flag |
   | `nodev` | `MS_NODEV` flag |
   | `noexec` | `MS_NOEXEC` flag |
   | `nosuid` | `MS_NOSUID` flag |
   | `_netdev` | dropped, it is a userspace-only flag |
   | `defaults` | dropped, it is a userspace-only flag |
   | everything else (`vers`, `soft`, `hard`, `timeo`, `retrans`, `sec`, `port`, `actimeo`, `uid`, `gid`, `file_mode`, `dir_mode`, `iocharset`, `credentials`) | passed through in the comma joined data string |

   `internal/mount` must not import `internal/api`, so keep one source of truth:
   either move the option table into a shared place or have `internal/mount`
   carry its own flag map with a test in `internal/api` asserting the two agree.
   State which you chose in the notes.

3. `credentials=<path>` keeps working: the CIFS kernel client parses
   `credentials=` itself, so the 0600 credentials file and its `defer os.Remove`
   stay exactly as they are. No password ever reaches the data string.

4. Unmount: `unix.Unmount(target, 0)`. If that returns `EBUSY`, wait 500ms and
   retry once with `unix.MNT_DETACH`. Anything else is returned as is, verbatim.
   Do not silently force detach on the first attempt.

5. Error reporting: replace the stderr-carrying `CommandError` with an error that
   names what was attempted and translates the common errno:
   - `EPERM`/`EACCES`: name the requirement, `cap_add: SYS_ADMIN` plus
     `security_opt: apparmor=unconfined` on the container.
   - `ENODEV`: the kernel has no client for that filesystem type, name `nfs` or
     `cifs` and the `nfs-common` / `cifs-utils` packages.
   - `ETIMEDOUT`/`EHOSTUNREACH`: the server did not answer, name the source.
   - default: `mount -t <fstype> <source> at <target>: <errno text>`.
   The operator sees this string in the storage row and in the log.

6. Startup capability check: read `CapEff` from `/proc/self/status` and test bit
   21 (`CAP_SYS_ADMIN`). When it is missing, log one clear error naming the two
   container flags at startup, and let mounts fail normally with the `EPERM`
   message above. Never crash the server over it. Skip the check on non-Linux.

7. Everything the executor already guarantees stays: per-storage lock, `/proc/mounts`
   as the source of truth for status, startup remount, watchdog with exponential
   backoff, bounded stat, probe file written and removed in `Test`, credentials
   file removed on every path out.

## Acceptance

- [ ] `internal/mount` no longer executes `mount` or `umount` through `os/exec` on the production path.
- [ ] Tests use a fake `Mounter`; no test needs real privileges, and CI is unchanged.
- [ ] A table test proves the option translation: `ro`, `noatime`, `nodev`, `noexec`, `nosuid` become flags, `_netdev` and `defaults` are dropped, and `vers=4.2,soft,timeo=50,retrans=2` reaches `data` in that exact order.
- [ ] Exactly one argument vector or flag set is asserted for an NFS storage and one for a CIFS storage, including the credentials path.
- [ ] Errno mapping covered by tests for `EPERM`, `ENODEV` and a default case, asserting the operator message text.
- [ ] Capability check covered by a test using a fake `CapEff` value, and it never panics.
- [ ] Non-Linux build compiles and returns the unsupported error; `go build` for a non-Linux `GOOS` is checked in the notes.
- [ ] The credentials file still does not exist after a mount returns, success or failure.
- [ ] `make test` green, `make lint` green, `DECISIONS.md` unchanged.

## Notes

Write the errno table, the flag map decision and the exact messages in
`tasks/79-notes.md`.
