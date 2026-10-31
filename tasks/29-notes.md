# Task 29 notes — NAS deploy templates and installer polish

## Plan

Files touched, in order:

1. `deploy/` tree — `README.md`, `generic/docker-compose.yml`,
   `synology/docker-compose.yml`, `truenas/docker-compose.yml`,
   `unraid/shrinkyourshit.xml`, `unraid/icon.svg` (the template needs a
   working `Icon`; no logo asset existed anywhere in the repo, so the
   template points at the raw GitHub URL of a small hand-written SVG next
   to it).
2. `deploy/deploy_test.go` — package `deploy`, part of `go test ./...`
   (so `make test` runs it). Uses `gopkg.in/yaml.v3`, already a direct
   dependency (internal/config) — no new dependency. Asserts the compose
   contract (pinned non-latest ghcr tag, 8765:8765, the three volumes,
   PUID/PGID, `restart: unless-stopped`, no privileged / host network /
   docker socket) and the Unraid XML (Repository tag, bridge network,
   non-privileged, port Config Type=Port, the three path Configs with
   `Mode: rw` on /config and /media, PUID/PGID Configs, non-empty
   Overview and Icon). Every failure names the file and the key.
3. `scripts/installer.sh` — `--uninstall`, `--purge` (requires
   `--uninstall`, no prompt), `--prefix DIR` (alias: sets `--install-dir`
   and `--service-dir` to DIR). All flag-shaped errors (unknown option,
   missing value, bad combination) print usage and exit 2; runtime
   failures stay exit 1. Uninstall runs before the `--server` validation
   so it never needs a server URL, tolerates missing systemctl/launchd,
   prints each action, keeps the data dir without `--purge`, and exits 0
   with a clear message when nothing is installed.
4. `scripts/test-installer.sh` — new sections: uninstall removes unit +
   binary and keeps the data dir; reinstall then `--purge` removes the
   data dir too; uninstall on a never-installed machine exits 0; unknown
   flag exits 2 with usage; `--prefix` install → uninstall → purge runs
   as an unprivileged user when the harness is root (proves no root
   needed).
5. CI (`.github/workflows/ci.yml`) — a `docker compose config --quiet`
   step over the three compose files in the `docker` job.
6. Docs — `docs/installing.md`, README quick start, CHANGELOG entries
   under the existing `[0.1.3] - unreleased`.

## Decisions worth knowing

- **PUID/PGID defaults per platform** (the "usual default"): generic
  1000/1000 (image default), Synology 1024/100 (DSM `docker` user,
  `users` group), TrueNAS 568/568 (the SCALE `apps` user), Unraid
  99/100 (`nobody`/`users`). Every template's comment says how to check
  with `id -u` / `id -g`.
- **Image pin**: templates pin `ghcr.io/peiceoftoast/shrinkyourshit:0.1.2`
  (newest release; 0.1.3 is unreleased). The validator accepts any
  explicit tag except `latest`, so a release bump is test-safe.
  `docker/compose.example.yml` keeps `:latest` — predates this task, not
  in scope.
- **`--prefix` semantics**: literally the alias the task names — same
  value for `--install-dir` and `--service-dir`, parsed inline so the
  usual last-flag-wins ordering applies. A prefix install therefore also
  skips service activation (that is what `--service-dir` does), which is
  exactly what a no-root install needs.
- **`--purge` without `--uninstall`** is a usage error (exit 2), not an
  implied uninstall: a data-deleting flag must not have a surprising
  meaning on its own.
- The Unraid `/scratch` Config has no `Mode` attribute (CA defaults Path
  to rw); the test asserts its presence and `Mode=rw` only for `/config`
  and `/media`, exactly as the task words it.

## Environment notes (this dev box)

- `shellcheck` and `docker` are absent, but the static shellcheck 0.10.0
  and docker compose v2.32.4 binaries were downloaded to
  `/tmp/opencode/tools` and both work here without a daemon, so
  `make installer-check` and the `docker compose config` acceptance item
  are proven locally this time (unlike task 28, which had to delegate).
- The repo-root `shrinkyourshit` binary is a gitignored build artifact;
  left alone.

## Acceptance proof (recorded)

Break-the-template run (all against `deploy/generic/docker-compose.yml`,
then restored — full text pasted in the PR body):

- wrong port → `generic/docker-compose.yml: services.ports must publish
  the container port 8765 as 8765 ("8765:8765"), got ["8080:8080"]`
- `latest` tag → `...: services.shrinkyourshit.image must pin a release,
  not :latest`
- `privileged: true` → `...: services.privileged must not be true`

`docker compose config --quiet`: all three files validate (v2.32.4).
`scripts/test-installer.sh` against a fake release server: full run green
(install → uninstall keeps data → purge removes data → never-installed
exit 0 → unknown flag exit 2 → prefix cycle unprivileged).
`make test`, `make lint`, `make installer-check`: green.

## Known issues (pre-existing, not this task's)

The integration suite has load-sensitive timing tests on this machine —
same class task 12/19/27 already recorded:

- `TestE2ETransferModeReplacesOriginalOnServer` and
  `TestIntegrationRevokeRequeuesForBuiltinWorker` (internal/worker)
  failed once under full parallel load and passed in isolation (-count=1,
  -run). The first is documented in 27-notes.md as failing on the base
  commit at the same rate.
- `TestIntegrationWorkerJoinsViaDiscovery` (cmd/shrinkyourshit) failed
  once at 139s under load (its subtests take 8s/38s in isolation) and
  passed in isolation.

This task adds no runtime Go code (deploy/ is test-only), so none of
these can be caused by it. Unit `go test ./...` was green, and every
integration package is green in isolation or on re-run.

## Summary

(Built:) deploy/ with generic+Synology+TrueNAS compose and an Unraid CA
template plus icon, a Go contract test that fails naming file and key,
installer `--uninstall`/`--purge`/`--prefix` with full test coverage in
test-installer.sh, CI compose validation, docs/installing.md, README
quick start and changelog. (Skipped:) CA feed publishing, k8s chart,
Windows installer changes, entrypoint changes — all out of scope. (Next
task:) templates pin 0.1.2; deploy/deploy_test.go accepts any
non-latest tag, so bumping the pin when 0.1.3 ships needs no test edit;
installer exit codes are 2 for flag mistakes and 1 for runtime failures.
