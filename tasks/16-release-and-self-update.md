# Task 16: Release artifacts and self update

## Goal
Users update without rebuilding. Every tagged release publishes verifiable binaries and a container image, and a binary install can update itself in place. Today the release workflow exists and has never been run, so nothing is published, and there is no update code in the product at all.

## Read first
ARCHITECTURE.md (update path). STACK.md (release, Docker). API.md (task 16). Depends on task 12 so the release gate includes the layout tests.

## Do
- `.github/workflows/release.yml`:
  - Add a `checksums` job that writes `checksums.txt` with the sha256 of every archive and attaches it to the release. Without this the updater cannot verify a download.
  - Make `docker` depend on `binaries` and `checksums`, so an image never publishes from a broken release.
  - Embed the commit and build date with `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`.
  - Add a release dry run that can be triggered by hand (`workflow_dispatch`) so the pipeline can be proven without publishing: it builds and verifies but skips the upload and the push.
  - Attest build provenance with `actions/attest-build-provenance` so a user can verify where a binary came from.
  - OCI labels on the image: `org.opencontainers.image.source`, `.licenses=AGPL-3.0-only`, `.version`, `.revision`. A redistributed image must state its licence and point at the source for the exact tag it was built from.
  - After the first push, the ghcr package is private by default even when the repo is public. Set the package visibility to public and enable inherited access from the repository so later pushes stay public. Pulling without auth is the whole point of publishing.
- `internal/update`:
  - `Latest(ctx, channel)` reads `https://api.github.com/repos/PeiceOfToast/shrinkyourshit/releases/latest` (or the releases list for prerelease), with an on disk cache in the data dir, a 6 hour TTL, and ETag support so a check costs no API budget when nothing changed.
  - Asset selection by `runtime.GOOS` and `runtime.GOARCH` from the archive names the release workflow produces.
  - Download to the scratch dir, verify sha256 against `checksums.txt`, extract, and refuse to continue on any mismatch or missing entry.
  - Replace the running executable atomically: write the new binary beside the old one, fsync, `rename` over it, and keep the previous copy as `<binary>.prev` for rollback.
  - Refuse to self update when running in a container: detect `/.dockerenv` or `SHRINK_IN_CONTAINER=1` and return a typed error carrying the exact `docker compose pull` instruction for the user.
  - Back up the SQLite database to `<data-dir>/backups/pre-update-<version>.db` before applying, and keep the last three.
- Restart behaviour: if the process is managed by systemd, exit 0 after the swap and let `Restart=always` bring it back on the new binary; otherwise `syscall.Exec` the new binary with the same arguments. Document the required unit setting in README and in the installer.
- CLI:
  - `shrink version` prints version, commit, build date and runtime (binary or container).
  - `shrink update --check` prints current and latest, changed or not, and exits 0.
  - `shrink update` performs the update, honouring the container refusal.
  - `shrink update --rollback` restores `<binary>.prev`.
- Settings: `update_check` (bool, default true), `update_channel` (`stable` or `prerelease`, default stable), `backup_before_update` (default true). Nothing auto applies an update by default.

## Out of scope
Delta or binary diff updates, signing keys beyond GitHub artifact attestation, self updating a container image from inside the container.

## Acceptance
- [ ] A release built from a `v*` tag contains five archives plus `checksums.txt`, and every checksum verifies. (`TestReleaseArtifacts` against the dry run output, or the run recorded in `tasks/hermes-log.md`)
- [ ] `shrink version` prints the injected version, commit and date from a build made with the release flags. (`TestVersionOutput`)
- [ ] `internal/update` selects the correct asset for linux/amd64, linux/arm64, windows/amd64, darwin/amd64 and darwin/arm64. (`TestAssetSelection`)
- [ ] A tampered download fails verification and the running binary is untouched. (`TestChecksumMismatchAborts`)
- [ ] A successful update swaps the binary, keeps `.prev`, and `--rollback` restores the previous version and runs it. (`TestSelfUpdateAndRollback`)
- [ ] Inside a container the updater refuses with the pull instruction and never touches files. (`TestContainerRefusesSelfUpdate`)
- [ ] The database is backed up before an update and at most three backups are kept. (`TestPreUpdateBackupRetention`)
- [ ] README documents the systemd `Restart=always` requirement and the three update paths: binary self update, `docker compose pull`, and rebuilding from source.
