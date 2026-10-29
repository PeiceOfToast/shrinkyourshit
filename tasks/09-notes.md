# Task 09 notes: file transfer, shared-storage check, installers

## Plan

Branch `task/09-file-transfer-installer` from `main`. Commits, in order:

1. **Refactor**: move the result-policy code (`applyResultPolicy`, `replace`,
   `moveExtracts`, `copyFile`, `humanBytes`) from `internal/worker/policy.go`
   into `internal/server/resultpolicy.go` as exported functions. The server
   needs the same verify-and-replace logic for transfer mode and cannot
   import `internal/worker` (worker imports server). Worker keeps using it
   via `server.ApplyResultPolicy`.
2. **Store/API plumbing**: migration 0005 (`jobs.access_mode`,
   `nodes.throughput_limit_mbps`); `Job.AccessMode`, `Node.ThroughputLimit`;
   JobDTO/NodeDTO fields; PUT /nodes/{id} accepts `throughput_limit_mbps`;
   progress gains `access_mode` (stored on the job); `effectiveAccessMode`
   stops forcing auto→shared (spec says `auto`, the node resolves it); JobSpec
   gains `throughput_limit_mbps`. ARCHITECTURE.md + API.md in the same commit.
3. **Server transfer service + endpoints**: `internal/server/transfer.go`
   (access challenge: 1MiB sample at a random offset; chunked result upload
   with Content-Range, sequential-with-repeat resume, final checksum verify),
   `internal/api/transfer.go` handlers, routes for
   `/worker/jobs/{id}/access-check`, `/source`, `/result`.
4. **Server-side verify-and-replace on complete**: transfer-mode complete
   applies the stored result policy to the uploaded output (commit 1's code),
   then records via Queue.Complete; scratch upload discarded after
   complete/fail/cancel.
5. **Worker: access check + download** (`internal/worker/access.go`,
   `download.go`): per-library 10-minute decision cache; Range-resumable
   download into scratch with size + fingerprint verification.
6. **Worker: result upload + transfer runner path**: 8MiB chunk PUT with
   final `X-Checksum-SHA256`; throughput limiter (no new deps — paced
   reader/writer); runner: auto→access check→shared|transfer, transfer skips
   local policy apply and extracts, reports the chosen mode in progress.
7. **Installers**: `scripts/installer.sh` / `installer.ps1` (shellcheck-clean,
   `--release-url` and `--no-service` overrides for tests), embedded at the
   repo root package and templated with the request's server URL + version;
   `GET /install.sh` and `GET /install.ps1`; shellcheck + container install
   test in CI.
8. **E2E acceptance tests** (full stack over HTTP with fake encoders — no
   ffmpeg needed) + UI: throughput field in the node edit dialog; STATUS.md.

## Decisions taken (within the docs' framing)

- **Who caches the access-check result**: the node caches its shared/transfer
  decision per library for 10 minutes (only the node knows the comparison
  outcome). The endpoint stays per-job; a cache hit skips the round trip.
- **Access-check `path` is node-side** (server applies the node's mappings
  before answering), consistent with `source_path` in the job spec — the
  worker has no mappings of its own to resolve with.
- **Spec access_mode for `auto` nodes** is the literal `auto`; the worker
  resolves it and reports the chosen mode in every progress call; the server
  stores it on the job (new `access_mode` column — ARCHITECTURE.md updated).
- **Complete in transfer mode**: the complete body gains `access_mode`;
  when it is `transfer` the server applies the policy itself from the
  verified upload (this is the authoritative signal; the progress-stored
  mode is informational). `Queue.Complete`'s contract ("records what is
  already true on disk") is preserved: the API handler does the disk work
  first, then records.
- **Upload skip short-circuit**: with `replace_if_smaller` and an output that
  is not smaller, the worker skips the whole upload and completes with
  `skipped_reason` (same semantics as task 05, saves a full upload). The
  server still double-checks when an upload does arrive.
- **Extracts in transfer mode**: not uploaded (the task does not define an
  extract upload channel). The worker skips extract commands in transfer
  mode and logs a warning; UI exposure of `extract_removed` is post-MVP.
  Out-of-scope idea recorded here: chunked extract upload if it ever ships.
- **Throughput limit** lives on the node (`throughput_limit_mbps`, 0 =
  unlimited), enforced worker-side on both transfer directions (paced reads
  and writes), delivered to the worker on the job spec.
- **Orphaned server-side uploads** (node vanished mid-upload, or a cancelled
  upload) are cleaned on complete/fail/cancel; a periodic janitor for
  abandoned jobs is a follow-up idea, not built here.

## Acceptance → test mapping

1. Transfer end-to-end → `internal/worker/transfer_e2e_test.go`
   (`TestE2ETransferModeReplacesOriginalOnServer`; full HTTP stack, fake
   encoders — runs without ffmpeg).
2. Correct mapping → shared, no source bytes → same file,
   `TestE2ESharedModeTransfersNothing` (source-endpoint call counter 0).
3. Resume at 50% + corrupted upload → `TestE2EDownloadResumesAfterInterrupt`
   (the test wire aborts mid-body; the worker resumes via Range) and
   `TestE2ECorruptedUploadFailsAndKeepsOriginal` (a byte flipped in flight
   fails the checksum, the job fails, the original is untouched).
4. Same-size different file → `TestE2ESameSizeDecoyFailsAccessCheck` →
   transfer mode.
5. `installer.sh` shellcheck → `make installer-check` + CI `installer` job;
   container install → CI runs `scripts/test-installer.sh` inside
   `debian:bookworm-slim` against a fake release server; unit file verified
   with `systemd-analyze verify` on the runner (also verified locally).
6. `installer.ps1` PSScriptAnalyzer → CI job `installer-powershell` on
   windows-latest (`Invoke-ScriptAnalyzer -Severity Error -EnableExit`);
   no local pwsh to pre-verify, so CI is the check.
7. `GET /install.sh` contains the server URL →
   `TestInstallShellContainsServerURL` (+ reverse-proxy and .ps1 variants).

Wizard/Add-node "real /install.sh URL" item: both flows already render the
server-generated commands (task 08), so no UI change was needed — the URLs
became real when the endpoints landed. The wizard's join path pairs this
machine directly (POST /setup mode=join) and never shows an install URL.

## Out of scope (recorded, not built)

- Auto-update of nodes, TLS provisioning, NAS templates (task file).
- Extract upload channel for transfer mode (see decisions).
- Periodic janitor for orphaned server-side upload scratch.

## Summary (written at the end of the run)

- Built: transfer mode end to end (server endpoints + verify-and-replace on
  complete; worker access check with 10-min per-library cache, Range-resumable
  download, chunked checksum-verified upload, throughput limit), plus
  templated installer scripts served at /install.sh and /install.ps1 with
  shellcheck + container-install CI.
- Skipped: extract upload in transfer mode, upload-janitor for abandoned
  jobs (notes above), no UI for the throughput limit beyond the node edit
  dialog field.
- Next task should know: `effectiveAccessMode` no longer forces auto→shared —
  anything reading `JobSpec.AccessMode` must handle `auto` (the task-08
  path-mapping integration test was updated for this). The server now owns a
  scratch dir for uploads (`server.Config.ScratchDir`, wired in main);
  `Queue.Complete` stays "record only" — transfer disk work happens in the
  API layer before it is called. New node field `throughput_limit_mbps`
  travels on the job spec; the complete/progress bodies carry `access_mode`.
