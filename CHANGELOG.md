# Changelog

All notable user-visible changes to ShrinkYourShit. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Newest first.

## [0.1.2] - unreleased

### Added

- Library refresh integrations: Jellyfin, Emby and Plex refresh, and Sonarr
  and Radarr rescan, after a job succeeds. Configured in the UI; API keys are
  never returned by the API.
- Health check and move as first-class flow steps, with validation and editor
  support.
- Signed webhooks with Discord and Slack shaping, a delivery log with retry
  info, and an OpenAPI document served by the API.
- A full job log for every job, stored as structured rows (level, source,
  message) rather than a truncated tail, batched from the worker while the
  job runs, filterable by level in the UI and downloadable as text.
- The release procedure is written down, the changelog is now hand-written,
  and the arm64 container question has a documented answer.

### Changed

- The flow canvas is view-only on phones; the full editor is for larger
  screens. All canvas chrome is at least 44px.
- Dashboard stat tiles scale with the viewport, and the navigation rail
  collapses itself below the large breakpoint (with a remembered override).
- The Playwright mobile audit covers every route at 320, 390, 414 and 768px.

### Fixed

- The container image now reports the same commit and build date as the
  release binaries. `GET /api/v1/version` inside Docker used to answer
  `commit: none` because the image only carried the version.
- Every pull request now proves the image's version endpoint before merge.

## [0.1.1] - 2026-10-30

### Added

- Release archives for five targets: Linux amd64/arm64, macOS amd64/arm64 and
  Windows amd64, each with version, commit and build date embedded, plus
  `checksums.txt` and build provenance. v0.1.1 is the first release with
  downloadable archives because it fixes the Windows compile error that
  blocked v0.1.0.
- Self update: `shrinkyourshit update`, `update --check` and
  `update --rollback`, with sha256 verification, an atomic binary swap, the
  previous binary kept for rollback, and a database backup before the swap.
  Inside a container it refuses and prints the `docker compose` commands.
- A five-page UI shell: Dashboard, Libraries, Nodes, Encode settings and App
  settings, with a read-only dashboard.
- Explicit node control: start, stop, graceful drain, pause all, a live job
  line with file name, progress, speed and ETA, and per-node totals.
- Library views: per-library savings, per-file before/after with ratio and
  encoder, server-side search and sort, and per-file job history.
- Rule tiers: the flow schema, a step stack editor, and a visual flow canvas.
- A feedback layer with confirmations, useful error detail and empty states.
- In-app update checks and update status, with the update channel setting.
- Mobile groundwork: every interactive control is at least 44px tall, library
  and file lists render as cards below the small breakpoint, and a Playwright
  audit at 320, 390, 414 and 768px fails CI on overflow or a small control.

### Fixed

- Windows/amd64 builds compile again, which unblocked the release archives.

### Known limits

- The container image is `linux/amd64` only. See
  [docs/container-architectures.md](docs/container-architectures.md).
- The navigation rail still needs a manual collapse below the large
  breakpoint, and the dashboard stat cards are not yet fluid. Those land in
  0.1.2.

## [0.1.0] - 2026-10-29

### Added

- First public release. Watch folders, probe every file, and skip anything
  that already matches the target.
- Transcode video (HEVC, AV1, H.264), remux containers, convert audio, strip
  or extract subtitles, strip or pick audio tracks, and rename/move after
  processing.
- Hardware detection per machine: GPU encoder first, CPU fallback when no GPU
  exists or a GPU encode fails.
- Distributed installs: every install is the same binary and can be the
  server, a worker, or both, with LAN discovery and token-based pairing.
- Three rule tiers on one engine: easy mode, step stack, and the flow format
  the advanced editor uses.
- First-run setup with an optional login, and the easy-mode web UI.

### Known limits

- The downloadable archives were missing: the Windows/amd64 binary did not
  compile at release time. Fixed in 0.1.1.
- The container image was `linux/amd64` only.
- Mobile layout and explicit node start/stop arrived in 0.1.1.

[0.1.2]: https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/PeiceOfToast/shrinkyourshit/releases/tag/v0.1.0
