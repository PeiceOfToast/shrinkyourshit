# Task 01 notes — project skeleton

## Plan (files, in order)

1. **Go scaffold**
   - `go.mod` (module `github.com/PeiceOfToast/shrinkyourshit`, go 1.23)
   - `internal/{api,server,worker,encoders,flow,probe,store,discovery,config}/doc.go` — every package from the ARCHITECTURE.md layout
   - `internal/config/config.go` + `config_test.go` — env (`SHRINK_*`), optional `<data-dir>/config.yaml`, flags; precedence flag > env > file > default
   - `internal/api/api.go` + tests — `GET /api/v1/health`, static UI serving with index.html fallback, JSON 404 for unknown `/api/*`
   - `cmd/shrinkyourshit/main.go` — `serve` (default), `worker` (stub, exit 1), `version`
   - `web/embed.go` — `go:embed all:dist`; `web/dist/index.html` committed as build placeholder
2. **Web app** — Vite 5, React 18, TS strict, Tailwind 3, react-router v6, react-query v5, `api.ts`, home page (name + health), Vitest + RTL test, ESLint flat + Prettier
3. **Build tooling** — `Makefile` (build, dev, test, test-integration, lint, docker, cross, clean), `scripts/check-spdx.sh`
4. **Docker** — `docker/Dockerfile` (node:20 → golang:1.23 → debian:bookworm-slim + jellyfin-ffmpeg + gosu), `docker/entrypoint.sh` (PUID/PGID), `docker/compose.example.yml`
5. **CI** — `.github/workflows/ci.yml` (vet, golangci-lint, go test, ffmpeg + integration tag, SPDX check, web lint/test/build, docker build), `.github/workflows/release.yml` (`v*` tags: cross-compile, attach binaries, push GHCR)
6. **Docs** — README "Run with Docker" section
7. Status updates, acceptance ticks, summary

## Decisions / justifications

- **New dependency `gopkg.in/yaml.v3`**: needed to parse the optional `config.yaml` required by STACK.md; no YAML parser is listed in STACK.md (one-line justification per rule 3).
- **`web/dist/index.html` placeholder committed**: `go:embed all:dist` fails to compile when the dir is missing; placeholder keeps `go build`/`go test` working before the first web build. `.gitignore` ignores everything else in `web/dist`.
- **Docker defaults come from image ENV** (`SHRINK_DATA_DIR=/config`, `SHRINK_SCRATCH_DIR=/scratch`), not from an in-code `/.dockerenv` check — deterministic and testable.
- **Config file may set `data_dir`**: file is looked up in the data dir resolved from flag/env/default; if the file moves the data dir it is not re-read at the new location (documented in code).
- **GHCR/compose image name lowercased** (`ghcr.io/peiceoftoast/shrinkyourshit`) — Docker image names must be lowercase.
- **golangci-lint pinned to v1.64.8** in CI to match the local install; default linters, no config file.
- **No ffmpeg integration tests in this task**: nothing in task 01 needs ffmpeg (probe/encoders arrive in tasks 02/04). The `make test-integration` target and the CI step exist and pass; later tasks add real tests under the `integration` tag.

## Environment notes

- Docker is **not installed** in this environment, so the `docker build` acceptance item is verified by the CI `docker` job instead; jellyfin-ffmpeg config flags cannot be checked locally. Everything else is verified locally.
- Local toolchain: go 1.27.3 (go.mod pinned to 1.23), node 20, golangci-lint v1.64.8, ffmpeg/ffprobe 7.0.2 present.

## Summary (filled at the end)

- **Built:** Go module + all nine `internal/*` packages with doc.go; `internal/config` (flag > env > file > default, unit-tested, incl. `~` expansion); `GET /api/v1/health` + embedded-UI serving with index.html fallback and JSON 404s; CLI `serve`/`worker`/`version`; web app (Vite 5, React 18, TS strict, Tailwind 3, router, react-query, `api.ts`, Vitest+RTL tests, ESLint+Prettier); Makefile (build/dev/test/test-integration/lint/docker/cross); SPDX check script; multi-stage Dockerfile + entrypoint + compose example; ci.yml and release.yml; README Docker section.
- **Skipped (out of scope):** database, real features, auth, discovery, worker logic. No ffmpeg integration tests — nothing in task 01 needs ffmpeg; the target and CI step exist for tasks 02+.
- **Environment:** Docker not installed → docker acceptance is covered by the CI `docker` job (build, page, ffmpeg NVENC/QSV flag checks). GitHub Actions is blocked at the account level (billing), so no green run could be observed; the ci.yml `report` job posts results as a PR comment for later runs. golangci-lint upgraded from the preinstalled v1.64.8 to v2.15.0 (the old build cannot parse Go 1.27 export data); CI pins v2.15.0 to match.
- **Next task should know:** `web/` carries a stub `go.mod` — keep it, it stops Go tooling from walking `web/node_modules`; the UI is embedded from `internal/webui/dist`, refreshed by `make web` (and `make placeholder` creates a stub in fresh clones); version is injected via `-X main.version=`; a `codex review` was requested on PR #1 — check for late findings before merging.
- **Codex findings:** P1 (Dockerfile copied the built UI to the wrong embed path) was fixed in f9bde2b; P2 (`~` in `SHRINK_DATA_DIR`/`--data-dir` not expanded before the config-file lookup) fixed with a regression test in 5f63d90.
