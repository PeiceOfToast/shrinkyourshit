# Real-hardware mount evidence, 2026-11-06

Run by Hermes against the lab itself, not a simulation. Source of tasks 79, 80 and 81.

Build under test: `shrinkyourshit:main-6f25a45`, version string `v1.11-verify`,
commit `6f25a45bcb00c18383960651b30f19d96bf9cc3c` (origin/main with tasks 64 to 74
merged). Config backed up to `/home/mb/sys/config.pre-v111-20261106-143726` before
the container swap. The container was replaced in place with the task 67 flags:
`--cap-add SYS_ADMIN`, `--security-opt apparmor=unconfined`, plus
`-v /home/mb/sys/storage:/storage`.

## Environment

TrueNAS `10.10.1.50` exports `/mnt/DataSSD/SSD` to `10.10.0.0/20`, so the SSD pool
is a real NFS target. Test data was a generated 30 second 1920x1080 H.264 clip at
20.2 Mbps, 75,936,737 bytes, placed at
`/mnt/DataSSD/SSD/shrink-test/in/shrink-test-1080p.mp4`.

## What passed

1. **Image contents.** `mount.nfs` at `/usr/sbin/mount.nfs` and `mount.cifs` at
   `/usr/sbin/mount.cifs` both present (cifs-utils `2:7.0-2`). `CapEff` =
   `00000000a82425fb`, so `CAP_SYS_ADMIN` (bit 21) is effective.
2. **Auth gate.** With `login_enabled=false`, `POST /api/v1/storages` returned 403
   `login_required` with "Turn on login in App settings before you manage storage."
   `GET /api/v1/storages` stayed open. Matches spec 4.6.
3. **Option validation.** `options: "vers=4.2;rm -rf /"` returned 400 with "the
   options cannot contain spaces or shell characters such as ; | & $ or quotes".
4. **Storage record.** Created `ssdpool`, protocol `nfs`, source
   `10.10.1.50:/mnt/DataSSD/SSD`, mountpoint `/storage/ssdpool`, default options
   `vers=3,soft,timeo=50,retrans=2`.
5. **Mount state detection.** After a mount, `GET /storages/{id}` reported
   `mounted: true`, read from `/proc/mounts`. Correct.
6. **Browse.** `GET /storages/{id}/browse?path=/storage/ssdpool/shrink-test`
   returned the real listing plus `free_bytes: 3956832993280` and
   `total_bytes: 3957270249472` from the SSD pool. A relative path is rejected with
   "path must be absolute".
7. **Library on the share.** Created library `SSD Test` at
   `/storage/ssdpool/shrink-test/in`. Scan returned 202 and produced one job.
8. **Real encode through the share.** Job `66b5b630` ran on the built-in node,
   `encoder_used: cpu`, no fallbacks, speed 1.3x: `bytes_before: 75936737` ->
   `bytes_after: 25089834`, 67 percent smaller. On the pool the file is now
   `shrink-test-1080p.mkv` and the original mp4 is gone, which is the `replace`
   result policy working against a network share. Result streams:
   `hevc (Main), yuv420p, 1920x1080, 24 fps` with `aac (LC), 44100 Hz, mono`.
   Duration preserved at `00:00:30.02`.
9. **Bad source fails without taking the server down.** A storage pointing at
   `10.10.9.99` returns 409 `mount_failed` on mount, and `/health` stayed 200.

## Bug 1: the executor cannot mount at all as shipped

`POST /storages/{id}/test` and `/mount` both returned:

```
{"error":"mount.nfs: failed to apply fstab options","code":"mount_failed"}
```

The server process runs as `shrink` (uid 1000), not root. `mount.nfs` is a setuid
root helper, and when an unprivileged process calls it, it restricts itself to
fstab style user options and refuses everything else. `CAP_SYS_ADMIN` in the
container does not help, because the code shells out to `mount(8)` instead of
calling `mount(2)`.

Running the same command as root inside the same container got past this point,
which confirms the cause:

```
docker exec -u root shrinkyourshit mount -t nfs -o vers=4.2 10.10.1.50:/mnt/DataSSD/SSD /storage/ssdpool
```

Consequence: as shipped, no storage added in the WebUI can ever mount. Fix is task
79: call the mount syscall in process and keep the capability in the process.

## Bug 2: the default NFS options need a daemon the container does not run

As root, `mount -t nfs -o vers=3,soft,timeo=50,retrans=2` (the app default) failed:

```
mount.nfs: rpc.statd is not running but is required for remote locking.
```

NFSv3 needs `rpc.statd` for locking, and the image runs neither rpcbind nor statd.
`vers=4.2` mounted first try. Defaulting to NFSv3 was the author's design choice in
spec 4.4 and it is wrong for a container. Fix is task 80.

## Still not verified anywhere

- The SMB leg end to end with credentials: mount, read, write, wrong password. The
  `dperson/samba` test container never came up healthy, so this has not been proven
  on real hardware by anyone.
- Remount on container restart, and the watchdog restoring a mount that vanished.
- Migration against a copy of a v1.10 database.

Task 81 covers the first two.

## Side effect left in place

Login was enabled to exercise the storage endpoints. Admin user `admin`, password in
`/home/mb/sys/.verify-admin-password` (mode 0600) on `10.10.2.210`. It should be
changed in App settings, or login turned back off, before that box is used normally.
