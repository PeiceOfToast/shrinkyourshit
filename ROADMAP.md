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

## v1.9: UI redesign (scheduled)

Spec: `docs/design/ui-redesign-2026-11-04.md`. `web/` only: no Go, no API shape, no `DECISIONS.md`
change. Eighteen tasks in five phases (tokens, primitives, shell, pages, enforcement), run in order.
Each depends on the task before it except where noted.

| Task | File | What it does | Depends on |
|------|------|--------------|------------|
| 36 | `tasks/36-design-tokens-font-motion.md` | Colour, type, spacing and motion tokens in Tailwind, self-hosted Inter, focus and reduced-motion rules | 35 |
| 37 | `tasks/37-control-classes-format-helpers.md` | The shared control class strings and the formatting helpers (bytes, counts, times, hashes, encoder words) | 36 |
| 38 | `tasks/38-layout-primitives.md` | PageHeader, Card, FormSection, EmptyState, Banner, Disclosure and Icon | 37 |
| 39 | `tasks/39-control-primitives.md` | Badge, Checkbox, Tabs, SegmentedControl, CopyButton, CodeBlock and the restyled shared controls | 38 |
| 40 | `tasks/40-app-shell-encoding-status.md` | Desktop top bar removed, rail footer status, one encoding switch for the whole app, 56 px mobile bar | 39 |
| 41 | `tasks/41-library-forms.md` | New and Edit library forms onto FormSection, sliders readable on mobile, exact settings relabelled | 40 |
| 42 | `tasks/42-dashboard.md` | Dashboard onto the primitives: encoding status sentences, real file names in recent activity | 41 |
| 43 | `tasks/43-libraries-list.md` | Libraries list: rows link, only non-zero chips, empty state | 42 |
| 44 | `tasks/44-library-detail.md` | Library detail, file history and job log dialog: header actions, summary card, Shrunk status, honest pager | 43 |
| 45 | `tasks/45-rules-page.md` | Rules page (Easy and Steps) with one header and a neutral tier control shared with Flow | 44 |
| 46 | `tasks/46-flow-page.md` | Flow page shares the Rules header, one Edit flow, readable canvas | 45 |
| 47 | `tasks/47-edit-page-properties.md` | Edit page becomes library properties only; rule controls live on Rules (needs owner OK, spec 8.4) | 45, owner OK |
| 48 | `tasks/48-nodes.md` | Nodes: one lifecycle badge, labelled encoding switch, confirm on revoke | 47 |
| 49 | `tasks/49-encode-app-settings.md` | Encode and App settings plus About: section order, short hash, wrapping container instructions | 48 |
| 50 | `tasks/50-integrations-plugins.md` | Integrations and Plugins: tabs, card headers, empty states, secret panel | 49 |
| 51 | `tasks/51-setup-login.md` | Setup and Login: one heading, visual stepper, 44 px targets | 50 |
| 52 | `tasks/52-copy-sweep.md` | Copy sweep and a document title on every route | 51 |
| 53 | `tasks/53-guardrail-lint-audit.md` | Lint rule against the retired palette, mobile audit over every route at four widths | 52 |

The one new dependency is `@fontsource-variable/inter` (OFL-1.1, self-hosted, about 48 KB), justified
in the spec's section 8.5 and in `tasks/36-notes.md`.

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

## v1.10 encode settings tiers (scheduled 2026-11-04, delivered 2026-11-05)

Three editors over one canonical flow graph, per library, with an app wide default tier.
Specification: `docs/design/encode-settings-tiers-2026-11-04.md`. Tasks 54 to 63.

- Basic keeps the five stop slider and the audio slider; Moderate becomes a real decision tree with
  condition rows and indented children; Advanced stays a canvas over a widened node set.
- The engine gains resolution, CRF and bitrate rate control, speed presets, conditions on
  resolution, audio channels and HDR, per step hardware selection, and one validated raw ffmpeg
  arguments escape hatch.
- A library can be moved down a tier without losing any step: the lower tier renders read only and
  says which tier can edit what it cannot express.

Delivered (task 63, evidence in `tasks/63-notes.md`): three real encodes on the CPU encoder (Basic,
Moderate with a condition, scale and per step hardware, Advanced with `raw_args`), output heights
confirmed by ffprobe, and a downgrade that leaves the stored graph byte-identical. Open owner
questions, not built: the ffmpeg command line is not in the job log (only ffmpeg's own echo and the
`raw_args` token line), and `PUT /libraries/{id}/flow` does not check the library's tier yet.

## v1.11 encode profiles and in-app storage (delivered 2026-11-06)

Specification: `docs/design/encode-profiles-and-storage-2026-11-06.md`. Tasks 64 to 75.

- NFS and SMB shares are added, mounted, tested and browsed from the WebUI. The container gains
  `nfs-common`, `cifs-utils`, `SYS_ADMIN` and an AppArmor override at install time, and the server
  owns the mount lifecycle with a startup remount and a watchdog.
- Mount writes require login to be enabled, because mounting is a different risk class from
  everything else the app does. An optional host allowlist narrows it further.
- Encode rules move off libraries into named profiles on the Encode settings page. A profile owns
  the graph, the tier and the result policy; a library owns storage, path, watch, schedule and
  which profile it follows. The per-library encode builder is removed.

Delivered (task 75, evidence in `tasks/75-notes.md`): an NFS share (ganesha, NFSv4.1) and an SMB share (Samba,
SMB 3.1.1) added and mounted from the WebUI in a lab container, with read and write proved through both
mountpoints. A wrong host, a wrong password, refused options and a share that disappears each show their error
in the UI, and the server stays up. Every storage write route answers `403 login_required` while login is off.
One real file on the NFS share was encoded three times: profile as created gave `libx265 -crf 24`, the edited
profile gave `libx264 -crf 18`, and switching the library to a second profile gave `libx265 -crf 28`. Two copies of
a v1.10 database migrated with library counts unchanged (3, 4), 3 profiles each (identical rules deduped), and
no library left without a profile. Open findings, not built: SMB `vers=3.1.1` is refused by the option
check, a wrong NFS host blocks about two minutes, the watchdog misses a stale share that is still mounted, the
dashboard does not show a broken share, keep_both outputs are rescanned as sources and a re-queue can overwrite
an earlier output, ffmpeg auto-detect fails without `--ffmpeg`, and the Docker `SYS_ADMIN` posture was not
exercised (no Docker on the lab VM).

## v1.12 drop the library rule columns (follow up to v1.11 task 74, scheduled)

- Drop the library columns that v1.11 deprecated, in one forward-only migration: `libraries.flow_json`,
  `libraries.flow_source`, `libraries.target`, `libraries.easy`, `libraries.rule_tier` and
  `libraries.result_policy`. Nothing reads them since task 74; they are a rollback snapshot. `target`
  is on the list because the library API's `target` field is deprecated too. The owner confirms it
  when this task is scheduled. After the drop, a rollback to v1.10 cannot run a library: say so in
  the release notes.
- Remove the deprecated library body fields `target`, `easy` and `result_policy` (ignored since
  task 74, see `API.md` task 74), the `deprecated` list on the save replies, and the
  `PUT /api/v1/libraries/{id}/tier` route (`409 follows_profile` since task 74).
- Remove `legacyLibraryFlow` and the boot migration that reads the columns (`backfillFlows`), once
  every library row has a profile. The boot warning from task 74 (`logLegacyColumns`) goes with them.
