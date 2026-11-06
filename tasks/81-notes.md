# Task 81 notes: real hardware verification of the mount fixes

Run by Hermes on 2026-11-06 against the real NAS, the way the owner asked (option B: fix it
directly rather than only file tasks). Image built from `main` at `0a678da`, deployed onto
`10.10.2.210:8765` as the single `shrinkyourshit` container with `--cap-add SYS_ADMIN
--security-opt apparmor=unconfined`, config backed up to
`/home/mb/sys/config.pre-mountfix-20261106-205848` first.

Result: **the in-app mount feature now works end to end.** Two defects had to be fixed first,
neither of which any test in the repo could see, because the tests fake the kernel.

## What passed

| Check | Result |
|---|---|
| Startup remount, no operator action | `storage startup mount ok` storage=ssdpool on boot |
| Mount from the app (API/UI), NFS over the real pool | `mounted: true`, `/proc/mounts` shows `nfs4 ... vers=4.2 ... soft,timeo=50,retrans=2,addr=10.10.1.50` |
| Empty options field applies the app default | stored as `vers=4.2,soft,timeo=50,retrans=2` and the mount carries `soft,timeo=50` |
| Unmount then mount through the API | `mounted: false` then `mounted: true`, no root shell involved |
| Browse the share | app lists `/storage/ssdpool/shrink-test/in` |
| Watchdog restores a mount removed underneath it | unmounted inside the container, back within 20s, `storage remounted by watchdog` |
| Container restart remounts | restarted, mount present again with only startup action |
| Bad host fails cleanly | 409 `mount_failed` with "the server did not answer", `/health` 200, the good share stayed mounted |
| Real encode over the app-mounted share | `appmount-test.mp4` 30,326,986 -> `appmount-test.mkv` **7,637,663 bytes (75% smaller)**, encoder cpu, source replaced per policy |

Not verified: **SMB end to end.** There is no SMB server in the lab that an agent may use, so
mount/read/write/wrong-password on CIFS remains unproven on real hardware.

## Defect 3: the privilege drop threw CAP_SYS_ADMIN away

`docker exec` as root reported `CapEff 00000000a82425fb`, which is why the container flags were
believed correct. The actual server process is a different story:

```
Uid:    1000 1000 1000 1000
CapEff: 0000000000000000
CapAmb: 0000000000000000
CapBnd: 00000000a82425fb
```

The entrypoint ran `exec gosu 1000:1000 shrinkyourshit`, and gosu clears permitted and effective
capabilities for the new uid. Every `mount(2)` returned EPERM. The app's own startup check
reported it exactly: `network storage needs CAP_SYS_ADMIN, and this container does not have it`.
The check was right and the flags were irrelevant.

Fix: `setpriv --reuid --regid --init-groups --inh-caps=+sys_admin --ambient-caps=+sys_admin`,
which raises the capability as ambient so it survives execve as a non-root uid. Verified in the
image: `CapEff 0000000000200000` on pid 1 as uid 1000. gosu remains the fallback when setpriv is
absent.

## Defect 4: mount(2) needs addr= for NFS

With the capability fixed the error became EINVAL, for every option combination including an
empty one. The kernel's text mount for NFS cannot resolve the server itself: that path needs the
`dns_resolver` upcall and `/sbin/request-key`, which the image does not carry. `mount.nfs` always
did that resolution in userspace and added `addr=` before calling the kernel.

Proved directly with ctypes in a throwaway container, kernel 6.8.0, against the real export:

| call | result |
|---|---|
| `mount("10.10.1.50:/mnt/DataSSD/SSD", t, "nfs", 0, "")` | EINVAL |
| `mount(..., "nfs", 0, "vers=4.2")` | EINVAL |
| `mount(..., "nfs4", 0, "vers=4.2")` | EINVAL |
| `mount(..., "nfs", 0, "addr=10.10.1.50,vers=4.2")` | OK, share readable |
| `mount(..., "nfs", 0, "addr=10.10.1.50,vers=4.2,soft,timeo=50,retrans=2")` | OK |

So it is not a host name problem as `tasks/79-notes.md` guessed: an IP source fails too. Fix:
`internal/mount/nfsaddr.go` resolves the host in userspace (an IP needs no lookup) and appends
`addr=` for nfs and nfs4; CIFS is left alone because it resolves in the kernel.

## Fixes and where they live

PR https://github.com/PeiceOfToast/shrinkyourshit/pull/84 (branch `fix/nfs-addr-and-cap-retention`):
the entrypoint capability retention, `withServerAddr`, and the tests. `internal/mount` is green,
`gofmt` and `go vet` clean.

## Test state left behind on the box

- Storage `ssdpool` exists (NFS `10.10.1.50:/mnt/DataSSD/SSD`), mounted, default options.
- Library `SSD Test` on `/storage/ssdpool/shrink-test/in`, now holding `appmount-test.mkv`
  (7,637,663 bytes) and `shrink-test-1080p.mkv` (25,089,834 bytes).
- That directory was chmod 777 so uid 1000 could write through the mount for the encode test.
- Login is still enabled, admin password in `/home/mb/sys/.verify-admin-password` (0600). The
  owner should change it in App settings.
- The `badhost` test storage was deleted after the failure test.
