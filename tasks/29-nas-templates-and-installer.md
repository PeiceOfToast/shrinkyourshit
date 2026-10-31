# Task 29: NAS deploy templates and installer polish

## Goal
Someone on Unraid, Synology or TrueNAS can install this in minutes from a checked-in file, and a node can be uninstalled as cleanly as it was installed. Every template is validated by a test so they cannot rot.

## Read first
`docker/compose.example.yml`, `docker/Dockerfile`, `docker/entrypoint.sh` (PUID/PGID and the volume layout), `scripts/installer.sh` (the node installer and its flags), `scripts/test-installer.sh`, the `installer-check` target in `Makefile`, `internal/api/installer_test.go`, `README.md`.

Today there is one compose example buried in `docker/` and a node installer with no way back out. The people this replaces (Tdarr) mostly run on NAS boxes.

## Do

### 1. `deploy/` tree

```
deploy/README.md                 which file for which box, and why
deploy/generic/docker-compose.yml
deploy/synology/docker-compose.yml
deploy/truenas/docker-compose.yml
deploy/unraid/shrinkyourshit.xml
```

Every compose file uses the same contract, differing only where the platform forces it:
- image `ghcr.io/peiceoftoast/shrinkyourshit:0.1.2` with the tag pinned, and a comment saying how to pin a newer one.
- container port `8765`, published as `8765:8765`.
- volumes `/config`, `/media`, `/scratch` mapped to platform-appropriate host paths (`/mnt/user/appdata/...` on Unraid, `/volume1/docker/...` on Synology, `/mnt/tank/apps/...` on TrueNAS).
- `PUID`/`PGID` as environment with the platform's usual default and a one line comment on how to find the right value (`id -u`, `id -g`).
- `restart: unless-stopped`, and `--stop-grace-period` style notes where the platform supports it.
- No `privileged`, no host networking, no Docker socket mount.

`deploy/unraid/shrinkyourshit.xml` is a normal Community Applications template: `Name`, `Repository`, `Network` bridge, `Config` entries for port and the three paths with `Mode: rw` for `/media` and `/config`, `Icon`, `Overview` written in plain language, and `DateInstalled`/`TemplateURL` left for the CA tool to fill.

### 2. A validator that makes the templates real

New `deploy/deploy_test.go` (package `deploy`), part of `make test`, asserting for every compose file:
- it parses as YAML,
- the image is the ghcr repo with an explicit tag that is not `latest`,
- the port mapping includes `8765`,
- the three volumes and the two env vars are present,
- nothing privileged and no docker socket mount.

And for the Unraid XML: it parses as XML, `Repository` matches the ghcr repo, and the three `Config` paths exist with the right modes.

Each failure message names the file and the missing key, so a future edit that breaks a template fails `make test` instead of shipping.

### 3. `scripts/installer.sh`: a way out
Add:
- `--uninstall` — stop and disable the service, remove the unit file, remove the binary, print what it did. Refuses to touch the data dir.
- `--purge` — with `--uninstall`, also remove the data dir, and require the flag explicitly rather than asking, so it works non-interactively.
- `--prefix DIR` as an alias that sets both `--install-dir` and `--service-dir` for a user local install without root.
- Every unknown flag prints usage and exits 2 (some do not today).

Extend `scripts/test-installer.sh` for the new flags using the existing fake release server: uninstall removes the unit and the binary, `--purge` also removes the data dir, uninstall on a machine that never installed exits 0 with a clear message, and no flag touches `--server` validation. Keep `make installer-check` (shellcheck) clean.

### 4. Docs
- `docs/installing.md`: one page. A table of platform to template file, the volume contract, why `/media` must be writable (results replace files in place), PUID/PGID for each platform, the port, and the fact that a paired node starts **stopped** and must be started from the nodes page.
- `README.md`: a Quick start section linking `docs/installing.md`, three lines per path (compose, binary installer, NAS template).
- `CHANGELOG.md`: entries under `[0.1.3] - unreleased`.

## Out of scope
Publishing to the Unraid Community Applications feed or any NAS app store (a hosting decision, not a code change), a Kubernetes chart, a Windows service installer, changes to the container entrypoint logic beyond what the templates need.

## Acceptance
- [ ] The four template files exist and `make test` runs `deploy_test.go` green against all of them.
- [ ] Deliberately breaking one template (wrong port, `latest` tag, privileged) fails the test with the file name in the message. Paste that failure in the PR body, then restore the file.
- [ ] `docker compose -f deploy/generic/docker-compose.yml config` and the Synology and TrueNAS files all validate locally.
- [ ] `scripts/installer.sh --uninstall` removes the unit and binary and leaves the data dir; `--purge` removes it too. Proven by `scripts/test-installer.sh`.
- [ ] `--prefix` works without root and the tests cover it. `make installer-check` is clean.
- [ ] `docs/installing.md` exists with the platform table and the writable `/media` explanation, and the README links it.
- [ ] `make test` and `make lint` are green.
