# SPDX-License-Identifier: AGPL-3.0-only
# syntax=docker/dockerfile:1

# Multi-stage build per STACK.md: node:20 builds the web app, golang:1.23
# builds a static binary, debian:bookworm-slim + jellyfin-ffmpeg runs it.
# jellyfin-ffmpeg ships NVENC, QSV and VAAPI, so one image covers all v1
# encoders (decision B8). The runtime is built for linux/amd64 and
# linux/arm64; the jellyfin-ffmpeg deb is pinned per architecture.

ARG GO_VERSION=1.23
ARG NODE_VERSION=20

# --- web build ---
FROM node:${NODE_VERSION} AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

# --- go build ---
FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/webui/dist
# The three build identity args match the -X names in cmd/shrinkyourshit.
# `make docker` and the release workflow pass the same values the release
# binaries get, so /api/v1/version agrees with `shrinkyourshit version`.
ARG VERSION=v0.0.0
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/shrinkyourshit ./cmd/shrinkyourshit

# --- runtime ---
# The jellyfin-ffmpeg deb is pinned per architecture: TARGETARCH (set by
# buildx from the target platform) picks the URL and sha256 from the four
# ARGs below. `amd64` and `arm64` are supported; anything else fails the
# build naming the arch — never default silently to amd64.
# Both debs come from the official channel, not the jellyfin apt repo (the
# bookworm repo no longer ships the package and the old key URL 404s). The
# exact filename is pinned even though latest-7.x is a rolling channel;
# the sha256 check fails the build on mismatch. apt installs the local deb
# and resolves its dependencies from Debian bookworm.
# The arm64 investigation notes and the CI proof live in
# docs/container-architectures.md.
FROM debian:bookworm-slim AS runtime
ARG DEBIAN_FRONTEND=noninteractive
ARG TARGETARCH
ARG FF_DEB_AMD64_URL=https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/amd64/jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb
ARG FF_DEB_AMD64_SHA256=0cf62bc2423822c9ec7a38dbc8f526d9a58671bd01843daa7817ece35619fc1c
ARG FF_DEB_ARM64_URL=https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb
ARG FF_DEB_ARM64_SHA256=9de240f98cc49db7ebc649c72c37d3ef154170d37ec2fbc68483cc84744804bb
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gosu \
 && case "$TARGETARCH" in \
      amd64) FF_DEB_URL="$FF_DEB_AMD64_URL"; FF_DEB_SHA256="$FF_DEB_AMD64_SHA256";; \
      arm64) FF_DEB_URL="$FF_DEB_ARM64_URL"; FF_DEB_SHA256="$FF_DEB_ARM64_SHA256";; \
      *) echo "jellyfin-ffmpeg is not pinned for TARGETARCH '$TARGETARCH'; supported: amd64, arm64" >&2; exit 1;; \
    esac \
 && curl -fsSL -o /tmp/jellyfin-ffmpeg7.deb "$FF_DEB_URL" \
 && echo "$FF_DEB_SHA256  /tmp/jellyfin-ffmpeg7.deb" \
      | sha256sum -c - \
 && apt-get install -y --no-install-recommends /tmp/jellyfin-ffmpeg7.deb \
 && rm -f /tmp/jellyfin-ffmpeg7.deb \
 && apt-get purge -y curl \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

# The deb puts the binaries in /usr/lib/jellyfin-ffmpeg with the bundled
# shared libraries in ./lib, but sets no RPATH and ships no ld.so.conf
# entry, so ffmpeg only runs with LD_LIBRARY_PATH set.
ENV PUID=1000 \
    PGID=1000 \
    LD_LIBRARY_PATH=/usr/lib/jellyfin-ffmpeg/lib \
    SHRINK_DATA_DIR=/config \
    SHRINK_SCRATCH_DIR=/scratch \
    SHRINK_FFMPEG_PATH=/usr/lib/jellyfin-ffmpeg/ffmpeg

COPY --from=build /out/shrinkyourshit /usr/local/bin/shrinkyourshit
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh \
 && mkdir -p /config/plugins \
 && chown -R "${PUID}:${PGID}" /config/plugins

VOLUME ["/config", "/media", "/scratch"]
EXPOSE 8765

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["serve"]
