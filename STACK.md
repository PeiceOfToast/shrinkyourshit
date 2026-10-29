# Stack

Pinned so every task uses the same tools. Do not introduce alternatives for anything listed here.

## Backend

| Item | Choice |
|------|--------|
| Language | Go 1.23 or newer |
| Module path | `github.com/PeiceOfToast/shrinkyourshit` (set by the owner; agents read it from `go.mod`) |
| HTTP | Standard library `net/http` with Go 1.22+ method routing (`mux.HandleFunc("GET /api/v1/...")`). No router framework. |
| JSON | `encoding/json` |
| Database | SQLite via `modernc.org/sqlite` (pure Go). All access through the `store.Store` interface. |
| Migrations | Plain SQL files embedded with `go:embed`, applied in order, tracked in a `schema_migrations` table. No migration library. |
| File watching | `github.com/fsnotify/fsnotify` plus periodic polling fallback |
| mDNS | `github.com/grandcat/zeroconf` (service type `_shrinkyourshit._tcp`) |
| Password hashing | `golang.org/x/crypto/argon2` (argon2id) |
| Logging | `log/slog`, JSON in Docker, text in dev |
| Config | Environment variables with `SHRINK_` prefix, optional `config.yaml` in the data dir, flags override both. Use `github.com/spf13/pflag` only; no viper. |
| CLI | Subcommands: `shrinkyourshit serve` (default), `shrinkyourshit worker`, `shrinkyourshit version`. Implemented with the standard library. |
| Process execution | `os/exec` for ffmpeg and ffprobe. Always pass `-progress pipe:1 -nostats` for encodes and parse key=value lines. |
| Tests | `go test`, standard `testing` package, `github.com/stretchr/testify` allowed for assertions. Integration tests that need ffmpeg use build tag `integration` and are skipped if ffmpeg is missing. |
| Lint | `go vet` and `golangci-lint` with the default linters |

## Frontend

| Item | Choice |
|------|--------|
| Build | Vite 5, React 18, TypeScript strict |
| Styling | Tailwind CSS 3. No component library. |
| Routing | `react-router-dom` v6 |
| Data | `@tanstack/react-query` v5. One `api.ts` client that wraps `fetch` and throws on non-2xx. |
| Events | native `EventSource` against `/api/v1/events` |
| Flow editor (post-MVP) | `@xyflow/react` (React Flow 12) |
| Tests | Vitest plus React Testing Library |
| Lint/format | ESLint plus Prettier, defaults |
| Embedding | `web/dist` embedded into the Go binary with `go:embed`; Go serves `index.html` for unknown paths so client routing works. |

## Config reference

| Env var | Flag | Default | Meaning |
|---------|------|---------|---------|
| `SHRINK_ROLE` | `--role` | `both` | `server`, `worker`, or `both` |
| `SHRINK_PORT` | `--port` | `8765` | HTTP port (server) |
| `SHRINK_DATA_DIR` | `--data-dir` | `/config` in Docker, `~/.shrinkyourshit` otherwise | DB, settings, plugin cache, node key |
| `SHRINK_SCRATCH_DIR` | `--scratch-dir` | `/scratch` in Docker, `<data-dir>/scratch` otherwise | Per-job temp space, should be fast local disk |
| `SHRINK_SERVER` | `--server` | empty | Server URL for a worker, e.g. `http://192.168.1.10:8765` |
| `SHRINK_TOKEN` | `--token` | empty | One-time pairing token for a new worker |
| `SHRINK_NODE_NAME` | `--node-name` | hostname | Display name of this node |
| `SHRINK_FFMPEG_PATH` | `--ffmpeg` | auto-detect | Path to ffmpeg; ffprobe assumed beside it |
| `SHRINK_LOG_LEVEL` | `--log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `SHRINK_DISCOVERY_PORT` | `--discovery-port` | `8766` | UDP broadcast port for LAN discovery |
| `SHRINK_DISCOVERY_MDNS` | `--discovery-mdns` | `true` | Use mDNS for LAN discovery |
| `SHRINK_DISCOVERY_BROADCAST` | `--discovery-broadcast` | `true` | Use UDP broadcast for LAN discovery |
| `PUID` / `PGID` | | `1000` | Docker only: run as this user/group |

## Docker

- Multi-stage Dockerfile: `node:20` builds the web app, `golang:1.23` builds the binary, runtime is `debian:bookworm-slim` plus `jellyfin-ffmpeg` from the Jellyfin apt repository.
- Image published to `ghcr.io/PeiceOfToast/shrinkyourshit` with tags `latest` and the semver tag.
- Volumes: `/config`, `/media`, `/scratch`.
- GPU: compose example includes commented blocks for NVIDIA (`runtime: nvidia`, `NVIDIA_VISIBLE_DEVICES=all`) and Intel (`devices: - /dev/dri:/dev/dri`).

## Release

- Semantic version tags `vX.Y.Z`. `shrinkyourshit version` prints the tag injected with `-ldflags`.
- GitHub Actions on tag: cross-compile linux/amd64, linux/arm64, windows/amd64, darwin/amd64, darwin/arm64, attach to the release, push the Docker image.

## Binary and service names

- Binary: `shrinkyourshit`
- systemd unit / launchd label / Windows service: `shrinkyourshit`
- mDNS service: `_shrinkyourshit._tcp`
- User agent and node key header: see API.md
