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

## v1.1: UI and control (scheduled)

Source: `docs/ui-review-2026-10-29.md`, written after the owner ran the MVP on a real library. Done means the app is usable on a phone, the dashboard is read only, the five nav pages exist, and a node never encodes unless a human started it.

| Task | Summary | Depends on |
|------|---------|------------|
| 10 | UI shell, responsive navigation, settings split, dashboard read only | 09 |
| 11 | Node lifecycle: explicit start, stop, drain, pause all | 05, 08, 09 |
| 12 | Responsive tables plus Playwright layout tests in CI | 10 |
| 13 | Feedback, confirmations, error detail, empty states | 10, 12 |

## v1.2: Rule tiers (scheduled)

`DECISIONS.md` item 6 requires all three tiers on one engine. Item B3 deferred the flow engine to post MVP; these two tasks land it.

| Task | Summary | Depends on |
|------|---------|------------|
| 14 | Flow graph schema, easy mode preview, step stack editor | 02, 04, 10 |
| 15 | Visual flow editor on the canvas, read only then editing | 14 |

## v1.3: Ship and update (scheduled)

The release workflow already builds five binaries and a container image on a `v*` tag, but no tag has ever been cut, so nothing is published and every test run today means a full rebuild. These two tasks make updating a pull or a click instead.

| Task | Summary | Depends on |
|------|---------|------------|
| 16 | Release artifacts with checksums, self update command, rollback, container refusal | 12 |
| 17 | In app update check and update UI | 16, 10 |

## After v1.2 (order flexible)

1. Remux-only, audio-only conversion, subtitle extract, health check, rename/move as first-class actions with UI.
2. Git plugin repos and bundled plugin format.
3. AMD (AMF/VAAPI) and Apple VideoToolbox encoders.
4. Native binary installers polish and NAS templates (Unraid, Synology, TrueNAS).
5. Webhooks (Discord, Slack, generic), OpenAPI document.
6. Optional integrations: Sonarr/Radarr, Plex/Jellyfin/Emby refresh.
7. Script escape hatch with sandboxed runtime.
8. Support for non-media file types.

## Not planned for v1

Multi-user accounts, SSO, roles, Postgres, any paid tier.
