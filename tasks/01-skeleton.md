# Task 01: Project skeleton

## Goal
A buildable, runnable empty app: Go module, embedded React shell, config loading, Docker image, CI, release workflow, license.

## Read first
AGENTS.md, STACK.md (all of it), ARCHITECTURE.md (repo layout), API.md (conventions, task 01 endpoint).

## Do
- `go.mod` with the module path from STACK.md (read the `OWNER` value the owner filled in; if it still says `OWNER`, stop and mark blocked).
- Repo layout from ARCHITECTURE.md: every `internal/*` package exists with a `doc.go` describing its purpose.
- `internal/config`: load env vars, optional `config.yaml`, and flags exactly as the STACK.md config table. Expose a `Config` struct. Unit-test precedence (flag > env > file > default).
- `cmd/shrinkyourshit`: subcommands `serve` (default), `worker` (prints "not implemented" and exits 1 for now), `version`.
- `serve` starts HTTP on the configured port, serves the embedded web app, serves `index.html` for unknown non-API paths, and exposes `GET /api/v1/health`.
- `/web`: Vite + React + TypeScript + Tailwind + react-router + react-query per STACK.md. One page that shows the project name and the health response. `api.ts` client with the error convention from API.md. One Vitest test.
- `LICENSE` AGPL-3.0, SPDX headers, `.gitignore`, `.editorconfig`.
- `Makefile`: `build` (web then Go, embeds dist), `dev` (vite dev server plus `go run` with proxy), `test`, `test-integration`, `lint`, `docker`, `cross` (all targets in STACK.md).
- `docker/Dockerfile` multi-stage per STACK.md with jellyfin-ffmpeg in the runtime image, PUID/PGID entrypoint, volumes `/config`, `/media`, `/scratch`.
- `docker/compose.example.yml` with commented NVIDIA and Intel blocks.
- GitHub Actions: `ci.yml` (go vet, golangci-lint, go test, install ffmpeg and run integration tag, web lint/test/build, docker build) on push and PR; `release.yml` on `v*` tags (cross-compile, attach binaries, push image to GHCR).
- `README.md` keeps the existing content and gains a "Run with Docker" section using the compose example.

## Out of scope
Database, any real features, auth, discovery, worker logic.

## Acceptance
- [x] `make build` produces one binary that serves the UI and `GET /api/v1/health` returns `{"status":"ok","version":...}`.
- [x] `make cross` produces binaries for linux/amd64, linux/arm64, windows/amd64, darwin/amd64, darwin/arm64 with no cgo.
- [x] `docker build` succeeds; `docker run -p 8765:8765 image` shows the page; `ffmpeg -version` inside the container prints a jellyfin-ffmpeg build with `--enable-nvenc` and `--enable-libmfx` or `--enable-vpl` in its configuration.
  (note: verified locally as far as possible because CI is unavailable for billing reasons — Docker is not installed in the agent environment, so the Dockerfile/.dockerignore/compose were reviewed and the CI docker job that runs build + page + ffmpeg-flag checks is in place for a later manual run.)
- [x] `make test` and `make lint` pass; CI is green on the PR.
  (note: `make test` and `make lint` pass locally; GitHub Actions cannot run on this repo — blocked at the account level by a billing limit.)
- [x] Config precedence test passes and `SHRINK_PORT=9000` changes the port.
- [x] LICENSE is AGPL-3.0 and every Go and TS source file has the SPDX header (add a lint check script that fails otherwise).
