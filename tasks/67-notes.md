# Task 67 notes: container packages, capabilities and install notes

## Plan (written before implementation)

Files, in order:
1. `docker/Dockerfile`: `nfs-common` and `cifs-utils` on the existing runtime apt line; `/storage` created and
   added to `VOLUME`.
2. `docker/entrypoint.sh`: `storage_report` (one line; checks `mount.nfs`, `mount.cifs`, CapBnd bit 21 and the AppArmor
   profile), `/storage` in the chown loop, and a `SHRINK_ENTRYPOINT_LIB=1` seam so the functions can be tested by sourcing.
3. `docker/compose.example.yml`: `cap_add: [SYS_ADMIN]` and `security_opt: [apparmor=unconfined]` with comments.
4. `deploy/*/docker-compose.yml` (generic, synology, truenas) and `deploy/unraid/shrinkyourshit.xml`: the same two
   settings. The NAS templates are what users actually copy, so the compose lines must be there too. Scope note below.
5. `deploy/deploy_test.go`: tests that every template carries the two settings, and that README and
   `docs/installing.md` name the capability and the login gate.
6. `README.md`, `docs/installing.md`, `STACK.md` (runtime packages, volumes).
7. `.github/workflows/ci.yml`: one step in the existing docker job proving the helpers exist in the built image.
8. `make test`, `make lint`, `scripts/check-spdx.sh`. Tick what is proven; leave the rest open.

## Scope decisions (the owner may redirect)

- **The deploy templates and the Unraid XML are in this task.** The task's scope names `docker/` and "installer
  scripts", but the NAS templates are the install files users copy (`docs/installing.md` points at them). Without the two
  settings there, the feature does not work for NAS users. Also `deploy/` is covered by the spec's "compose lines" item.
- **CI step added to the docker job.** It is the only place the image can be proven here. Docker is not installed on
  the build box (`docker: command not found`), so acceptance item 1 and the docker build job must be proven by CI.
- **`installer` means the container install path.** `scripts/installer.sh` and `installer.ps1` install the worker node
  binary only (`--server`, `--token`). They never touch the container, so they have nothing to say about compose
  capabilities. The container install text lives in `docs/installing.md` and the deploy templates, and that is where the
  requirement and the login gate are stated. Recorded as a gap, not a change to the worker installer.
- **`SHRINK_MANAGE_MOUNTS` is read by the entrypoint only.** `internal/config` already parses it (`cfg.ManageMounts`),
  but nothing in `cmd/` reads it yet, so the mount executor still starts whatever the value. Wiring it is a Go change
  outside this task's scope. Open question, see below.

## Why the entrypoint checks CapBnd, not CapEff

The entrypoint runs as root only until `gosu` drops to PUID. After that, CapEff is empty for a non-root user. The
bounding set (`CapBnd`) is what Docker sets from `cap_add`, and it survives the drop. `mount(8)` is setuid root
(`/usr/bin/mount` and `/bin/mount` are `-rwsr-xr-x root root` on this box and in Debian's util-linux), so an exec as
PUID gets root euid and its permitted set is bounded by `CapBnd`, which includes SYS_ADMIN when cap_add grants it.
The reasoning was checked by reading the bits and by the failure below, not by a container run.

## Verified here (no Docker)

- Without SYS_ADMIN in the bounding set, `mount -t tmpfs` fails with exit 32 and the stderr line
  `mount: <path>: permission denied.` (reproduced with `unshare -Urm` and `setpriv --bounding-set -sys_admin`,
  util-linux 2.39.3). This is the text the storage's `error` field carries (task 66 stores mount stderr), and it is
  what a user sees when they copy a pre-v1.11 compose file and then add a storage.
- `mount` run as a non-root user without setuid prints `must be superuser to use mount` (`/usr/bin/mount` as mb). So the
  setuid bit is load-bearing, and the CI step below asserts it.

## Image size delta (estimate, not measured)

`docker build` is not runnable here. Estimate from `apt-get -s install --no-install-recommends nfs-common cifs-utils`
on Ubuntu 24.04 package sizes (not bookworm): nfs-common 884 KiB, cifs-utils 330 KiB, libwbclient0 148 KiB,
libtalloc2 83 KiB, so about 1.4 MiB installed for the new packages, plus a few libraries that may already be in the base.
Expect roughly 1.5 to 2 MiB uncompressed. The real number comes from the docker job; the CI step below prints
`docker image ls` size so the PR can record it.

## Exact error text (entrypoint, when something is missing)

`shrinkyourshit: network storage is unavailable: <problems>; add cap_add SYS_ADMIN and security_opt apparmor=unconfined to the compose file, then recreate the container. The app still starts.`
where `<problems>` is one or more of `mount.nfs not installed`, `mount.cifs not installed`, `SYS_ADMIN not granted`,
`AppArmor confines this container`. Ready line:
`shrinkyourshit: network storage is ready (mount.nfs and mount.cifs found, SYS_ADMIN granted)`.
Disabled line (`SHRINK_MANAGE_MOUNTS` false, 0 or off):
`shrinkyourshit: network storage is off (SHRINK_MANAGE_MOUNTS=false)`.

## Acceptance status

To be filled in at the end of the run.

## Open questions for the owner

1. `SHRINK_MANAGE_MOUNTS=false` does not yet stop the mount executor in `cmd/shrinkyourshit`. Wire it there
   (skip `mounts.Run` and `api.WithStorageMounter`) in a follow-up, or in task 68?
2. The server-side "Storage page explains the missing capability" (acceptance item 2) is the Storage page, task 68.
   Until then the only visible text is the storage `error` field and the entrypoint log line.
3. `mount.cifs` and `mount.nfs` running from a setuid `mount` as PUID is the working assumption. It is proven by the
   CI step only for the helpers and the setuid bit, not by a real mount. A real NFS or SMB mount in the container is
   task 75's job.
