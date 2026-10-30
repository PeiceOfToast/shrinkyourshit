# Task 16 notes: release artifacts and self update

## Plan

Files to touch, in order:

1. **Windows build fix** — `internal/discovery/discovery.go:315` calls
   `syscall.SetsockoptInt(int(fd), …)`, which does not compile for
   windows/amd64 (the fd is a `syscall.Handle` there; this is what killed
   the v0.1.0 binaries job). Split `enableBroadcast` into
   `broadcast_unix.go` (`//go:build unix`, real setsockopt) and
   `broadcast_windows.go` (no-op: Windows refuses SO_BROADCAST on a bound
   socket anyway and loopback probes never need it). Prove all five
   targets with `make release-cross` locally and a new `cross` job in
   ci.yml so it stays proven on every PR.
2. **`scripts/release.sh`** — single source of truth for the release
   layout, used by the workflow, `make release-cross` and
   `TestReleaseArtifacts`: `build <version> <dist>` (cross compile with
   `-X main.version/-X main.commit/-X main.date`), `package <dist>`
   (archive each binary: `.tar.gz` unix, `.zip` windows via python3
   zipfile — the local VM has no `zip` binary), `verify <dist>`
   (re-hash every archive against `checksums.txt`).
3. **`.github/workflows/release.yml`** — `workflow_dispatch` dry run
   (builds and verifies, skips upload and push); separate `checksums`
   job that writes `checksums.txt` and attaches it with the archives on
   a tag; docker `needs: [binaries, checksums]`, push only on a tag;
   OCI labels `source`, `licenses=AGPL-3.0-only`, `version`, `revision`;
   attest build provenance; ghcr package visibility note (manual
   setting, cannot be set from the workflow).
4. **`internal/update`** — `Latest(ctx, opts)` (GitHub API, per-channel
   on-disk cache in the data dir, 6h TTL, ETag revalidation), asset
   selection for the five targets, download + sha256 verify + extract
   (refuse on any mismatch or missing entry), atomic swap keeping
   `<binary>.prev`, DB backup `<data-dir>/backups/pre-update-<version>.db`
   keeping the last three, typed container refusal carrying the exact
   `docker compose pull` instruction, v0.0.0 refusal. Tests: asset
   selection, checksum mismatch abort, update + rollback, container
   refusal, backup retention, cache/ETag/304.
5. **CLI** — `version` prints version, commit, build date and runtime
   (binary or container); `update --check`, `update`, `update
   --rollback`; restart behaviour: systemd-managed (INVOCATION_ID +
   /run/systemd/system) exits 0 so Restart=always brings the new binary
   back, otherwise exec the new binary with the same args (windows:
   StartProcess).
6. **Settings + API** — `update_check` (default true), `update_channel`
   (stable | prerelease, default stable), `backup_before_update` (default
   true) in store + server settings with PUT validation; `GET
   /api/v1/version` → `{version, commit, date, runtime}` (API.md task 16;
   task 17 adds the update fields). Store gains `Backup` (VACUUM INTO) so
   the CLI can snapshot a live database safely. ARCHITECTURE.md settings
   list updated in the same commit (rule 5). No migration: settings are a
   key/value table.
7. **Docs** — README documents the systemd `Restart=always` requirement
   and the three update paths (binary self update, `docker compose pull`,
   rebuild from source); `installer.sh` unit changes `Restart=on-failure`
   to `Restart=always` with a comment explaining why (self update exits
   0 and relies on the bounce).

Out of scope (per task file): delta updates, signing beyond attestation,
self updating a container image from inside the container, update UI
(task 17).

## Notes

- v0.1.0 release state (hermes log): the docker job published
  ghcr.io/peiceoftoast/shrinkyourshit:v0.1.0 and :latest (public,
  anonymous pull verified), the binaries job died on windows/amd64, so no
  GitHub release or checksums.txt exists yet. The fix below unblocks the
  next tag; the owner then re-tags (the package visibility requirement is
  already satisfied per the log).
- `zip` is absent on the coding VM (no sudo/apt), so archive creation for
  windows uses python3's zipfile (present on the VM and on CI runners).
- `shellcheck` is also absent on the VM; the installer change (one
  heredoc line plus comment) was verified with `sh -n`, the structural
  installer run with a fake release server, and `systemd-analyze verify`
  on the written unit (unit now carries `Restart=always`).
  `scripts/release.sh` is new and checked by `sh -n` and
  TestReleaseArtifacts; CI runs shellcheck over the installers only.

### Local proof records

- Windows build fix: `for t in linux/amd64 linux/arm64 windows/amd64
  darwin/amd64 darwin/arm64; GOOS/GOARCH go build ./...` — all five OK
  (before the fix, windows/amd64 failed at discovery.go:315 exactly as
  the v0.1.0 run did).
- `make release-cross VERSION=v0.1.1-dryrun` (2026-10-30T05:27Z): five
  binaries built with `-X main.version/-X main.commit/-X main.date`, five
  archives (4 tar.gz + 1 zip) plus checksums.txt, `release.sh verify` and
  `sha256sum -c checksums.txt` both OK.
- The dry-run linux/amd64 binary: `shrinkyourshit version` prints
  `version: v0.1.1-dryrun / commit: ba041bf / build date:
  2026-10-30T05:26:57Z / runtime: binary`; `shrinkyourshit update
  --check` reports "no published release on the stable channel" and exits
  (correct: no release exists yet).
- Tests: TestReleaseArtifacts, TestAssetSelection, TestChecksumMismatch
  Aborts, TestSelfUpdateAndRollback, TestContainerRefusesSelfUpdate,
  TestPreUpdateBackupRetention, TestLatestCachesWithETag,
  TestVersionEndpoint (api), TestVersionOutput (cmd), TestSQLiteBackup
  (store) all pass; `make test`, `make lint`, `make test-integration`
  green (ffmpeg 7.0.2 static in ~/.local).

## Summary (written at the end)

- Built: the windows/amd64 build fix (per-OS broadcast socket option in
  internal/discovery, the exact defect that killed the v0.1.0 binaries
  job); scripts/release.sh (build/package/checksums/verify, one place
  owns the release layout); the release workflow dry run
  (workflow_dispatch), separate checksums job, docker gated on binaries +
  checksums, OCI licence/source/version/revision labels and build
  provenance attestation; internal/update (Latest with per-channel cache,
  6h TTL and ETag revalidation, asset selection, sha256-verified download
  and extraction, atomic swap keeping <binary>.prev, DB backup keeping
  the last three, typed container refusal, v0.0.0 refusal); the CLI
  (`version` full identity, `update`, `update --check`,
  `update --rollback`, systemd bounce vs exec restart); settings
  update_check/update_channel/backup_before_update with PUT validation;
  GET /api/v1/version; store Backup (VACUUM INTO) so a live server yields
  a consistent snapshot; README update paths and the installer's
  Restart=always.
- Skipped (out of scope per task): delta updates, signing beyond GitHub
  attestation, container self update, the update UI (task 17), and the
  one-time ghcr package visibility click (already done by the owner at
  v0.1.0 per hermes-log; documented in the workflow footer).
- Verification: make test (Go + 153 web tests), make lint (0 issues),
  make test-integration (ffmpeg present), all five cross targets build,
  make release-cross produced 5 archives + checksums.txt that verify.
- Next task (17, update UI) should know: /api/v1/version returns
  {version, commit, date, runtime} and gains update_check_enabled +
  channel in task 17; the update feed reads settings via the store
  (cmd/shrinkyourshit/updatecmd.go readUpdateSettings is the pattern);
  update.Apply/upd.Rollback take an Options struct with test-injectable
  Executable/Now/APIBase/Backup, which is how the UI should drive them if
  it ever calls Go directly; nothing auto-applies — the UI may only show
  status and ask.
- The v* tag itself is the owner's cut: the next tag runs the new
  pipeline (binaries + checksums + docker in dependency order); the
  workflow_dispatch dry run can prove it any time without publishing.
- The dispatch attempt from the VM failed with HTTP 403: the PAT has no
  actions scope (same block the PM logged on 2026-10-29T02:46:14Z), so
  the dry run needs one click from the owner/PM (Actions tab → release →
  Run workflow, on main after merge, or on this branch before it).
- CI on the PR: run 41009567692 green on all eight jobs (go, web, docker,
  e2e, installer, installer-powershell, report and the new cross job).
  The first run failed only the installer job — scripts/test-installer.sh
  still pinned `Restart=on-failure`; the assertion now expects
  `Restart=always` (task 16's unit requirement), second run green.
