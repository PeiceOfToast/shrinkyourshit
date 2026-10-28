# Task 09: File transfer mode, shared-storage check, installer scripts

## Goal
A node with no access to the library still gets work: the server streams the file to it and takes the result back. Nodes auto-detect whether they can see the library. One-line installers set up a worker as a service on Linux, macOS and Windows, plus Docker.

## Read first
API.md (task 09 endpoints), ARCHITECTURE.md (file access modes, B7), DECISIONS.md (8, 13, B7). Depends on task 08.

## Do
- Access check: `POST /worker/jobs/{id}/access-check` returns `{path, size, sample_offset, sample_sha256}` for the job's source (sample is 1MiB at a random offset). Node resolves the path through its mappings, reads the sample, compares. Result cached per node per library for 10 minutes. In `auto` mode: match means shared, otherwise transfer. The node reports the mode it chose in the progress payload and the server stores it on the job.
- Transfer:
  - `GET /worker/jobs/{id}/source`: streams the source with `Range` support and `ETag` = fingerprint. Node downloads to `<scratch>/<job-id>/source.<ext>`, resumes with `Range` on interruption, verifies size.
  - `PUT /worker/jobs/{id}/result`: chunked upload (8MiB chunks, `Content-Range`), server writes to `<scratch-on-server>/<job-id>/output.<ext>`, final request includes `X-Checksum-SHA256` which the server verifies before applying the result policy (the server performs verify-and-replace in transfer mode, reusing task 05's code).
  - Throughput limit setting per node (MB/s, 0 = unlimited).
- Installer:
  - `GET /install.sh` and `GET /install.ps1` served by the server, templated with the server URL. Scripts: detect OS and arch, download the matching binary from the GitHub release that matches the server's version (URL pattern from STACK.md release section), install to `/usr/local/bin` or `%ProgramFiles%\ShrinkYourShit`, write the data dir config with `role=worker`, `server`, `token`, register and start a service (systemd unit, launchd plist, Windows service via `sc.exe`), print the node name and "waiting for approval" when no token was given.
  - `scripts/installer.sh` and `scripts/installer.ps1` are the sources; the server embeds them.
  - Docker: the task 08 generated Docker command uses `SHRINK_ROLE=worker`, `SHRINK_SERVER`, `SHRINK_TOKEN`, and mounts `/scratch`.
- Update the task 06 wizard "Join" path and the task 08 Add-node dialog to use the real `/install.sh` URL.

## Out of scope
Auto-update of nodes, TLS provisioning, NAS templates (post-MVP).

## Acceptance
- [ ] Integration test: worker with no mapping to the library runs in transfer mode end to end; source arrives with the right size, result is uploaded, checksum verified, original replaced on the server side, job `done` with `access_mode=transfer` recorded.
- [ ] Same worker with a correct mapping passes the access check and the job runs in shared mode with no bytes transferred (assert the source endpoint was not called).
- [ ] Interrupting the download at 50% (test proxy drops the connection) resumes via `Range` and completes; a corrupted upload (flip a byte) fails the checksum, the job fails, the original is untouched.
- [ ] Access check fails when the node's mapping points at a different file of the same size.
- [ ] `installer.sh` passes `shellcheck`; running it in a Linux container with a local fake release server installs the binary, writes config, and the service file is valid (`systemd-analyze verify` or equivalent).
- [ ] `installer.ps1` passes `PSScriptAnalyzer` (run in CI on a Windows runner or skip with a note if unavailable).
- [ ] `GET /install.sh` contains the server's actual URL.
