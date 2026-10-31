# SPDX-License-Identifier: AGPL-3.0-only
# syntax=docker/dockerfile:1

# Multi-stage build per STACK.md: node:20 builds the web app, golang:1.23
# builds a static binary, debian:bookworm-slim + jellyfin-ffmpeg runs it.
# jellyfin-ffmpeg ships NVENC, QSV and VAAPI, so one image covers all v1
# encoders (decision B8).

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
# amd64 only: the image has never been built or tested for arm64. The
# pinned jellyfin-ffmpeg deb does have an arm64 build on the same channel
# (https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/), so a
# multi-arch image is possible; see docs/container-architectures.md.
FROM debian:bookworm-slim AS runtime
ARG DEBIAN_FRONTEND=noninteractive
# jellyfin-ffmpeg comes from the official deb, not the jellyfin apt repo
# (the bookworm repo no longer ships the package and the old key URL 404s).
# The exact filename is pinned even though latest-7.x is a rolling channel;
# the sha256 check fails the build on mismatch. apt installs the local deb
# and resolves its dependencies from Debian bookworm.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gosu \
 && curl -fsSL -o /tmp/jellyfin-ffmpeg7.deb \
      https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/amd64/jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb \
 && echo "0cf62bc2423822c9ec7a38dbc8f526d9a58671bd01843daa7817ece35619fc1c  /tmp/jellyfin-ffmpeg7.deb" \
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
RUN chmod 0755 /usr/local/bin/entrypoint.sh

VOLUME ["/config", "/media", "/scratch"]
EXPOSE 8765

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["serve"]
