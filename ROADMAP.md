# Roadmap

## MVP: "watch a folder, shrink what needs it, on one or more machines"

Done when tasks 01 to 09 pass their acceptance tests and the owner has run it on a real library in the home lab.

| Task | Summary | Depends on |
|------|---------|------------|
| 01 | Project skeleton, CI, Docker, release workflow | none |
| 02 | Probe, store, Target, Evaluate (skip-if-satisfied) | 01 |
| 03 | Library API, scanner, watcher, queue, SSE | 02 |
| 04 | Encoder detection, quality table, command builder, GPU to CPU fallback | 02 |
| 05 | Worker job runner (scratch, encode, verify, replace) | 03, 04 |
| 06 | First-run setup and admin auth | 03 |
| 07 | Easy-mode UI, dashboard, settings | 05, 06 |
| 08 | Node discovery, pairing, nodes UI | 07 |
| 09 | File transfer mode, shared-storage check, installer scripts | 08 |

Tasks 03 and 04 can run in parallel. Tasks 05 and 06 can run in parallel.

## After MVP (order flexible)

1. Remux-only, audio-only conversion, subtitle extract, health check, rename/move as first-class actions with UI.
2. Flow engine: Target becomes a single-action flow; step-stack editor.
3. Git plugin repos and bundled plugin format.
4. Visual flow editor (React Flow).
5. AMD (AMF/VAAPI) and Apple VideoToolbox encoders.
6. Native binary installers polish and NAS templates (Unraid, Synology, TrueNAS).
7. Webhooks (Discord, Slack, generic), OpenAPI document.
8. Optional integrations: Sonarr/Radarr, Plex/Jellyfin/Emby refresh.
9. Script escape hatch with sandboxed runtime.
10. Support for non-media file types.

## Not planned for v1

Multi-user accounts, SSO, roles, Postgres, any paid tier.
