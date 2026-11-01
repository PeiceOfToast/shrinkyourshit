# ShrinkYourShit

Free, open-source, self-hosted media compression and conversion for home media servers. No paywalled features. Ever.

Think Tdarr or FileFlows, minus the paywall, with a UI simple enough that you can point it at a folder and be done.

## Why this exists

- FileFlows limits the free tier to 1 node, 30 flow elements and SQLite only, and puts file server, reporting, SSO and more behind a license.
- Tdarr caps unmapped nodes at 10MB video files unless you pay for Pro.
- Unmanic is GPL-3 but is largely unmaintained and has a dated UI.

ShrinkYourShit is AGPL-3.0. Anyone can use, modify and host it, but nobody can close it.

## What it does (v1 goal)

- Watch folders and probe every file. If it already matches your target, skip it (never reprocess). A library declares what it looks for — video, audio, photos, subtitles or anything else — and files of other kinds are skipped untouched, so a folder of photos can sit next to your movies (task 34).
- Transcode video (HEVC, AV1, H.264), remux containers, convert audio, strip or extract subtitles, strip or pick audio tracks, run health checks, rename/move after processing.
- Auto-detect hardware per machine. GPU encoder first, CPU fallback when no GPU exists or a GPU encode fails.
- Distributed: every install is the same binary and can be a server, a worker, or both. First launch auto-discovers a server on the network, or becomes the server.
- Three ways to define rules on one shared engine:
  1. **Easy mode:** 5-step space-vs-quality slider, remove-subs checkbox, audio slider, pick a folder, go.
  2. **Step stack:** short ordered list of conditional steps.
  3. **Advanced flow:** visual node graph with full FFmpeg control.

## Documents

| File | What it is |
|------|------------|
| [DECISIONS.md](DECISIONS.md) | Every scoping decision, plus defaults chosen during doc review. The source of truth. |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Components, data model, `Target` schema, easy-mode mapping, satisfied rule, file access modes, security. |
| [STACK.md](STACK.md) | Pinned languages, libraries, tools, config, Docker conventions. |
| [API.md](API.md) | HTTP API conventions and endpoint list shared by all tasks. |
| [docs/plugins.md](docs/plugins.md) | The plugin contract (task 31): the manifest, the phases, the stdin JSON, the env contract. |
| [docs/scripts.md](docs/scripts.md) | The sandboxed script escape hatch (task 33): write a wasm plugin, the contract, the limits. |
| [docs/intel-qsv.md](docs/intel-qsv.md) | What Intel Quick Sync does here, the probed extras and the two QSV settings — and what Intel's "Deep Link" covers that a single ffmpeg process cannot reach (task 35). |
| [ROADMAP.md](ROADMAP.md) | MVP definition and what comes after. |
| [AGENTS.md](AGENTS.md) | Rules and loop for coding agents. OpenCode reads this file automatically. |
| [tasks/STATUS.md](tasks/STATUS.md) | Which task is next. Agents update it. |
| [tasks/NN-*.md](tasks/) | One self-contained spec per build step, each with acceptance tests. |

## Before you start (owner checklist)

1. ~~Create the GitHub repo and copy the contents of this folder to the repo root.~~ Done: repo is `PeiceOfToast/shrinkyourshit`, AGENTS.md is at the root.
2. Check availability of the GitHub org/name, Docker Hub or GHCR name, and domain for "shrinkyourshit". Not yet verified.
3. ~~Open `STACK.md` and replace `OWNER` in the Go module path with your GitHub owner name.~~ Done: module path is `github.com/PeiceOfToast/shrinkyourshit`.
4. Install ffmpeg and ffprobe on the machine where agents run tests (integration tests need them; unit tests do not).
5. Point OpenCode at the repo and give it the kickoff prompt from `AGENTS.md`.

## Quick start

- **Docker / compose:** copy [`deploy/generic/docker-compose.yml`](deploy/generic/docker-compose.yml), point `/media` at your library, `docker compose up -d`.
- **NAS (Unraid, Synology, TrueNAS):** ready-made templates in [`deploy/`](deploy/) — pick yours from [docs/installing.md](docs/installing.md).
- **Binary node:** copy the one-line command from the Nodes page, paste it on the other machine (`shrinkyourshit` installer, no Docker needed).

Full walkthrough — volumes, PUID/PGID per platform, the port, updating, and uninstalling — in [docs/installing.md](docs/installing.md).

## Run with Docker

```sh
# Using docker directly (config in ./config, your library at /path/to/media):
docker run -d --name shrinkyourshit \
  -p 8765:8765 \
  -v ./config:/config \
  -v /path/to/your/media:/media \
  -v ./scratch:/scratch \
  ghcr.io/peiceoftoast/shrinkyourshit:latest
```

Or with compose:

```sh
cp docker/compose.example.yml docker-compose.yml
# edit the /media path, then:
docker compose up -d
```

The web UI is at http://localhost:8765. GPU passthrough (NVIDIA NVENC, Intel QSV/VAAPI) is in the commented blocks of the compose example. `PUID`/`PGID` are supported by the image entrypoint.

## Releases

Every `vX.Y.Z` tag publishes:

- five binary archives — Linux amd64/arm64, macOS amd64/arm64, Windows amd64 — plus `checksums.txt` and build provenance, on the [GitHub releases page](https://github.com/PeiceOfToast/shrinkyourshit/releases);
- `ghcr.io/peiceoftoast/shrinkyourshit:X.Y.Z` and `:latest`, with the version, commit and build date baked in so the app's About card and `GET /api/v1/version` agree with the binaries.

The container image is published for **linux/amd64 and linux/arm64**. The
arm64 image is boot-tested under emulation in CI, but a real encode on
ARM64 hardware has not been recorded yet: on ARM64 expect CPU encoders,
not Intel QSV ([why, and the ARM64 caveats](docs/container-architectures.md)). User-visible changes are listed in [CHANGELOG.md](CHANGELOG.md), and the release procedure is in [docs/releasing.md](docs/releasing.md).

## Updating

Three install shapes, three update paths. `shrinkyourshit version` always tells you which one you are on (`runtime: binary` or `runtime: container`). Nothing auto-applies an update — checking is on by default, applying is always a human click or command.

**Binary (self update).** A binary install replaces itself in place:

```sh
shrinkyourshit update --check     # report current and latest, change nothing
shrinkyourshit update             # download, verify, swap, restart
shrinkyourshit update --rollback  # back to the previous binary
```

Every download is verified against the release's `checksums.txt` — a mismatch or a missing entry aborts with the running binary untouched. Before the swap the database is backed up to `<data-dir>/backups/pre-update-<version>.db` (the last three are kept, or turn it off with the `backup_before_update` setting), and the previous binary is kept as `<binary>.prev` for `--rollback`. The update feed follows the `update_channel` setting (`stable` or `prerelease`).

**systemd requirement: `Restart=always`.** A binary self update exits 0 after the swap and expects the service manager to bring the new build straight back up. With `Restart=on-failure` a clean exit would leave the service stopped after every update. The installer writes the right setting; check yours with `systemctl cat shrinkyourshit`:

```ini
[Service]
ExecStart=/usr/local/bin/shrinkyourshit serve
Restart=always
RestartSec=5
```

**Container.** The image is published to `ghcr.io/peiceoftoast/shrinkyourshit` on every `v*` tag. A container never updates itself from inside — `shrinkyourshit update` refuses with the exact commands instead:

```sh
docker compose pull && docker compose up -d
```

**From source.** `git pull`, then `make build`. `make release-cross VERSION=vX.Y.Z` is the local dry run of the release pipeline (cross compiles all five targets, packages archives, writes and verifies `checksums.txt`) without publishing anything.

## Building from source

Requires Go 1.23+, Node 20, and make. `make build` compiles the web UI into the binary; `make test`, `make lint`, `make cross` (linux, windows, macOS — arm64 and amd64), `make docker`. For development, `make dev` runs Vite on :5173 with the Go API on :8765 behind a proxy.

## End-to-end layout tests

`make e2e` runs the Playwright layout suite (task 12) at three viewports — phone 390x844, tablet 768x1024 and desktop 1280x800. Every page is asserted to have no horizontal overflow, a shell that covers the viewport, and a reachable nav (drawer on phones, left rail on tablet and desktop). The tests drive the real Go binary serving the built web app — never the Vite dev server: the harness builds the binary, starts it with throwaway data and scratch dirs, completes first-run setup with login off, generates one small test clip with ffmpeg and creates a library over it through the API. Failing runs leave a screenshot per page per viewport in `web/test-results`.

Requirements: Go, Node 20, ffmpeg in `PATH`, and a Chromium the Playwright cache can run (`npx playwright install chromium`; on Linux the first run may need `npx playwright install-deps chromium` for system libraries, which wants root).

Useful overrides (all optional):

- `SHRINK_E2E_PORT` — port for the local server (default 8712).
- `SHRINK_E2E_BASE_URL` — drive an already running server (this is how CI runs the suite against the docker image) instead of starting one; `SHRINK_E2E_MEDIA_DIR` places the generated test file, `SHRINK_E2E_LIBRARY_PATH` names the path as that server sees it.
- `SHRINK_E2E_BIN` — the binary to start (default `bin/shrinkyourshit`).
- `SHRINK_E2E_FFMPEG` — ffmpeg binary for the test clip (default `ffmpeg` from `PATH`; without ffmpeg the file-table tests are skipped with a note).

## License

AGPL-3.0. Funding: optional GitHub Sponsors. Built for the author's home lab first, then shared.
