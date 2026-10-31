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

## v1.4: Node control and library views (scheduled)

From the second UI planning session. The owner's answers drove this: a real on/off switch per node, one condensed live line per busy node, per node totals, pause all on the nodes page, and the library pages next.

| Task | Summary | Depends on |
|------|---------|------------|
| 18 | Node on/off switch, live job line, per node totals, pause all | 11 |
| 19 | Library list and detail: savings, search, sort, per file job history | 13, 18 |

## v1.5: Mobile portability (scheduled)

The frame was made responsive in v1.1. This makes the interior work on a phone.

| Task | File | What it does |
|------|------|--------------|
| 20 | `tasks/20-mobile-touch-targets.md` | Mobile audit gate in Playwright (320, 390, 414, 768), shared 44px control classes, `DataList` cards below `sm`, five pages migrated |
| 21 | `tasks/21-fluid-dashboard-and-rail.md` | `StatCard` icons with `clamp()` type, 2 to 4 column stat grid, rail auto collapses below `lg` with a remembered override |
| 22 | `tasks/22-flow-canvas-mobile.md` | Flow canvas is view only below `md`, canvas chrome is 44px, audit covers every route, layout rules documented |

Measured baseline and reasoning: `docs/mobile-portability-2026-10-30.md`.

## v1.6: Integrations and release hygiene (scheduled)

Chosen from the unscheduled backlog below, in the owner's order: make the library refresh itself, close the functional gap against Tdarr, make the app legible to other software, then fix the release plumbing.

| Task | File | What it does | Depends on |
|------|------|--------------|------------|
| 23 | `tasks/23-external-integrations.md` | Jellyfin/Emby/Plex refresh and Sonarr/Radarr rescan after a successful job, configured in the UI, keys never returned by the API | 19 |
| 24 | `tasks/24-health-check-and-move.md` | `health_check` and `move` as first-class flow steps with validation, editor support and tests | 14 |
| 25 | `tasks/25-webhooks-and-openapi.md` | Signed webhooks with Discord/Slack shaping, delivery log, and an OpenAPI document gated against the real route table | 23 |
| 26 | `tasks/26-release-hygiene.md` | Commit and build date in the image, a changelog, a written release procedure, and a documented arm64 answer | 16 |
| 27 | `tasks/27-full-job-log.md` | The complete log of every job as structured rows in SQLite (level, source, message), batched from the worker while it runs, filterable and downloadable in the UI | 5, 19 |

Deliberately not in this wave: AMD and Apple encoders (every host in the owner's lab has an Intel iGPU and QSV already works), plugin repos, the script escape hatch, non-media files.

## v1.7: Reach and extensibility (scheduled)

Order chosen for a public tool that replaces Tdarr: run where people actually run it, install in minutes, use their hardware, then let them extend it.

| Task | File | What it does | Depends on |
|------|------|--------------|------------|
| 28 | `tasks/28-multiarch-image.md` | Multi-arch image (amd64 and arm64) with the arm64 deb pinned, proven by an emulated boot in CI | 26 |
| 29 | `tasks/29-nas-templates-and-installer.md` | Unraid, Synology, TrueNAS and generic templates validated by a test, plus installer uninstall and purge | 26 |
| 30 | `tasks/30-vaapi-and-videotoolbox.md` | A real VAAPI backend for AMD and other GPUs, and VideoToolbox on macOS | 4 |
| 31 | `tasks/31-plugins-v1.md` | Plugin manifest, install from a git URL or directory, preflight/postprocess/notify hooks, timeout, output in the job log | 27 |

Everything that was unscheduled after v1.7 is now scheduled in v1.8 below. AMF remains a documented non-goal and is not planned.

## v1.8: The rest of the list (scheduled)

Everything still unscheduled after v1.7 is scheduled here, in the order the roadmap has already
carried it. AMF stays a documented non-goal.

| Task | File | What it does | Depends on |
|------|------|--------------|------------|
| 32 | `tasks/32-plugins-as-flow-steps.md` | A plugin as a first-class flow step: a fourth manifest type `step`, the in-place and result-line contract, a re-probe after the step, and failure and timeout rules that fail the job | 15, 31 |
| 33 | `tasks/33-script-escape-hatch.md` | The script escape hatch from decision 9, sandboxed: a WASI module run by wazero whose only filesystem is the job scratch dir, with no network, a memory ceiling and a wall-clock deadline | 32 |
| 34 | `tasks/34-non-media-file-types.md` | Libraries declare what they hold; a `Kind` decided by extension, and media-only steps that skip with a reason instead of failing | 02, 03, 05, 14 |
| 35 | `tasks/35-intel-qsv-tuning.md` | The QSV features ffmpeg actually exposes: ICQ, look-ahead with extbrc, async depth and low power, each probed, off by default, argument vector unchanged when unsupported | 04, 30 |

Dependency note for 33: it adds `github.com/tetratelabs/wazero` to `STACK.md` with the one-line
justification rule 3 of `AGENTS.md` asks for. Nothing else in this wave adds a dependency.

## After v1.2 (order flexible, superseded by v1.3 to v1.8)

Kept as history: this is the list the waves were cut from, and items 1 to 8 are now scheduled across
v1.3 to v1.8.


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
