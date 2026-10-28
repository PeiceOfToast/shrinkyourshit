# Decisions Log

Source of truth. Agents must not change this file. If a decision blocks a task, stop and write the question in the task's notes file.

## Part A: Decisions from the scoping session (owner-approved)

| # | Topic | Decision |
|---|-------|----------|
| 1 | Why not fork Unmanic | Unmanic is not maintained and its UI is not simple enough for the masses. New project. |
| 2 | License | AGPL-3.0. Must stay open even when hosted as a service. |
| 3 | Funding | Optional GitHub Sponsors. Project is for the owner first, shared second. |
| 4 | Scope | Media only for v1 (video, audio, subtitles). Core must be generic (task/plugin based, not hardcoded to video) so other file types can be added later. |
| 5 | Day-one tasks | Transcode, remux, audio track strip/select, subtitle strip/extract, audio conversion, health check, rename/move. Plus **file detection**: probe each file, skip if it already matches the target preference, and remember what was processed. |
| 6 | Rule definition | All three tiers on one engine: easy mode (5-step slider + checkboxes + folder), step stack, advanced visual flow. Lower tiers generate the same flow format the advanced editor edits. |
| 7 | Distributed | Distributed from v1. Every install auto-detects hardware. GPU encoder first, CPU fallback on absence or failure. |
| 8 | File access | Both: shared-storage mounts when a node has them, server-to-node file transfer otherwise. Node always works on a local scratch copy and writes the result back. Path mapping handled automatically. |
| 9 | Plugin format | Declarative YAML/JSON plugins with FFmpeg templates, plus a sandboxed script escape hatch (runtime decided later). Chosen by Claude on the owner's request. |
| 10 | Plugin distribution | Bundled curated set plus user-added git repos, cached locally for offline use. No central hosted dependency. |
| 11 | Backend language | Go. Single static binary, easy cross-compile, FFmpeg does the heavy work as a subprocess. Chosen by Claude on the owner's request. |
| 12 | Frontend / DB | React plus React Flow, embedded in the Go binary. SQLite for v1 behind a DB layer that keeps Postgres possible later. Chosen by Claude on the owner's request. |
| 13 | Deployment | Docker image first. Native binaries and NAS templates (Unraid, Synology, TrueNAS) to follow. One-line setup, detect and go. Same binary everywhere: first launch discovers a server, otherwise becomes the server. WebUI generates the one-line command to add a node (e.g. `installer.sh --server 192.168.1.1`). Node pairing needs admin approval or a token. |
| 14 | Encoders | v1: NVENC, Intel (QSV/VAAPI), CPU (x264/x265/SVT-AV1). Later: AMD (AMF/VAAPI), Apple VideoToolbox. Encoder layer must be modular. |
| 15 | Audience | Home media servers (Plex/Jellyfin/Emby users). Simplicity first. No multi-user features in v1. |
| 16 | Auth | Single admin account, with an option to disable login, chosen during first-run server setup. Node pairing stays token-based regardless. |
| 17 | Integrations | v1: REST API plus webhooks. Standalone, finds files by watching folders. Sonarr/Radarr and Plex/Jellyfin/Emby refresh come later as optional add-ons built on the API. |
| 18 | Team / timeline | Solo, built with coding agents in a home lab. Work is split into small, self-contained tasks. No date. Owner tests in their own home lab. |
| 19 | Name | ShrinkYourShit. GitHub org, Docker Hub name and domain availability not yet checked. |

## Part B: Defaults chosen during doc review (Claude's picks, owner can override)

These were needed so agents do not each pick differently. Change any of them before task 01 starts; after that, changing one is a migration.

| # | Topic | Default | Why |
|---|-------|---------|-----|
| B1 | Easy-mode codec | HEVC (H.265) in MKV. AV1 and H.264 selectable in settings, not on the easy screen. | Widest hardware decode support on TVs and phones. Slider changes quality only, not codec. |
| B2 | Satisfied rule | A file that is already in the target codec is **not** re-encoded by default, even at a higher bitrate than the slider would produce. Option `reencode_if_same_codec` (off by default) overrides. | Avoids generational quality loss and surprise re-encodes of a whole library. Audio and subtitle rules still apply. |
| B3 | Easy mode in the MVP | Easy mode compiles to a `Target` struct, not yet to a flow graph. The flow engine arrives post-MVP and will wrap a `Target` as a single-action flow. | Keeps MVP small. Decision 6 still holds for the final product. |
| B4 | Result policy default | Replace original only if the output is smaller and verified. | Safest for a home library. |
| B5 | SQLite driver | `modernc.org/sqlite` (pure Go, no cgo). | Needed for static binaries and Windows/macOS/ARM cross-compile. |
| B6 | Events | Server-Sent Events at one endpoint. No WebSocket. | Simpler, works through reverse proxies. |
| B7 | Shared-storage detection | Read-only check: server picks an existing library file, node must read the same path and match size plus partial hash. No marker files written to the library. | Never write into the user's media folder for bookkeeping. |
| B8 | FFmpeg in Docker | jellyfin-ffmpeg (ships NVENC, QSV, VAAPI). Path override via `SHRINK_FFMPEG_PATH`. | One build that covers all v1 encoders. |
| B9 | Docker volumes | `/config` (data dir), `/media` (libraries), `/scratch` (temp). `PUID`/`PGID` supported. | Matches Unraid and linuxserver conventions users already know. |
| B10 | Task granularity | 9 MVP tasks instead of 7: auth/setup split from easy-mode UI, node pairing split from file transfer/installer. | Each task fits one agent loop. |
