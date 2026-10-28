# ShrinkYourShit

Free, open-source, self-hosted media compression and conversion for home media servers. No paywalled features. Ever.

Think Tdarr or FileFlows, minus the paywall, with a UI simple enough that you can point it at a folder and be done.

## Why this exists

- FileFlows limits the free tier to 1 node, 30 flow elements and SQLite only, and puts file server, reporting, SSO and more behind a license.
- Tdarr caps unmapped nodes at 10MB video files unless you pay for Pro.
- Unmanic is GPL-3 but is largely unmaintained and has a dated UI.

ShrinkYourShit is AGPL-3.0. Anyone can use, modify and host it, but nobody can close it.

## What it does (v1 goal)

- Watch folders and probe every file. If it already matches your target, skip it (never reprocess).
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

## Building from source

Requires Go 1.23+, Node 20, and make. `make build` compiles the web UI into the binary; `make test`, `make lint`, `make cross` (linux, windows, macOS — arm64 and amd64), `make docker`. For development, `make dev` runs Vite on :5173 with the Go API on :8765 behind a proxy.

## License

AGPL-3.0. Funding: optional GitHub Sponsors. Built for the author's home lab first, then shared.
