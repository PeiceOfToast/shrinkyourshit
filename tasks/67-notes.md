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

| Item | Status | Proof |
|---|---|---|
| Image has `mount.nfs` and `mount.cifs` (`docker run --rm <image> sh -c 'command -v mount.nfs mount.cifs'`) | **not ticked** | No docker on this box. CI docker job runs that exact command plus `test -u $(command -v mount)`. Passes only when the CI run is green. |
| `cap_add` removed: server starts, Storage page explains the missing capability | **not ticked** | Server start without the capability: CI step `storage-missing` checks the log line and `/api/v1/health`. The Storage page text is task 68, so the page half cannot be ticked here. |
| README and installer name the capability and the login gate | ticked | `deploy/deploy_test.go` `TestInstallDocsNameTheCapabilityAndLoginGate` on `README.md` and `docs/installing.md`. "Installer" read as the container install path (see scope decisions). |
| `make test` and the docker build job green | **not ticked** | `go test -count=1 ./...` exit 0 locally (make is not installed here; the same command ran). Web is untouched. Docker job pending on CI. |

The task stays `in_progress` with the branch name. The two unticked items need a docker host or CI, and the Storage page half of item 2 is task 68's.

## Gate results (2026-11-06, branch task/67-container-capabilities)

- `go test -count=1 ./...` exit 0 (every package ok, `internal/api` 454 s, `internal/server` 455 s).
- `go test -count=1 -v ./deploy/` all 8 top-level tests pass, including the new ones.
- `gofmt -l deploy/` clean. `go vet ./deploy/ ./cmd/...` clean. `golangci-lint run ./deploy/...` 0 issues.
- `./scripts/check-spdx.sh` exit 0. No new Go or TypeScript source file was created.
- `sh -n docker/entrypoint.sh` clean. shellcheck is not installed here, so not run.
- `make` is not installed on this box, so `make test` and `make lint` were not run through make. The commands they wrap were.
- `npm run build` and Playwright: not run, no web change.
- Not run: `docker build`, `docker run`, `docker compose config`. The CI docker job covers these on the PR.

## Open questions for the owner

1. `SHRINK_MANAGE_MOUNTS=false` does not yet stop the mount executor in `cmd/shrinkyourshit`. Wire it there
   (skip `mounts.Run` and `api.WithStorageMounter`) in a follow-up, or in task 68?
2. The server-side "Storage page explains the missing capability" (acceptance item 2) is the Storage page, task 68.
   Until then the only visible text is the storage `error` field and the entrypoint log line.
3. `mount.cifs` and `mount.nfs` running from a setuid `mount` as PUID is the working assumption. It is proven by the
   CI step only for the helpers and the setuid bit, not by a real mount. A real NFS or SMB mount in the container is
   task 75's job.

## Build notes

- The entrypoint's `storage_report` got three fixes during testing. `tr` was replaced with `case` patterns because
  a missing `tr` made `set -e` abort the entrypoint. The `sed` substitution gets `|| bnd=""` for the same reason.
  The check for `(enforce)` and `(complain)` in `/proc/self/attr/current` covers an AppArmor profile that is not
  unconfined.
- `mount` denial was reproduced locally without SYS_ADMIN: `mount: /tmp/mtest/t: permission denied.`, exit 32. That
  is the storage `error` text the user sees with an old compose file. Task 66 stores `mount(8)` stderr as written.
- Non-root `mount` without setuid prints `must be superuser to use mount` (checked as user mb). So the setuid bit
  on `/usr/bin/mount` is what lets the PUID user mount. CI asserts it with `test -u`.

## Summary

1. Built: Dockerfile installs `nfs-common` and `cifs-utils`, creates `/storage` and adds it to `VOLUME`. Entrypoint
   `storage_report` (one line, never exits, `SHRINK_ENTRYPOINT_LIB` seam for tests). Compose example, the three NAS
   templates and the Unraid ExtraParams carry `cap_add: SYS_ADMIN` and `security_opt: apparmor=unconfined`.
   README and `docs/installing.md` have a network storage section with the login gate and the old-compose error.
   STACK.md lists the runtime packages. CI docker job proves the helpers, the setuid mount, and both startup reports.
2. Skipped: the Go side of `SHRINK_MANAGE_MOUNTS` (the executor still starts whatever the value; out of this task's
   scope). The Storage page text (task 68). The worker `scripts/installer.sh` and `installer.ps1` were not changed
   because they never install the container. The real image size delta and the docker build were not run here.
3. Next task should know: the `mount` helpers only work if the image's `mount` stays setuid root and compose keeps
   both lines. Confirm on the real image with one NFS or SMB mount (task 75). The `SHRINK_MANAGE_MOUNTS` Go wiring
   is still open (see open questions 1).
4. Owner decisions taken in this task: templates and Unraid XML updated, not only `docker/`. CI step added. The
   "installer" in the acceptance means the container install docs. Image size is an estimate until CI prints it.
5. Open for the owner: the open questions above. Nothing in DECISIONS.md changed.
