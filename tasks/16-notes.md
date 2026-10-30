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
- Local proof records go in this file (hermes-log.md is PM-owned).

## Summary (written at the end)

- Built:
- Skipped:
- Next task should know:
