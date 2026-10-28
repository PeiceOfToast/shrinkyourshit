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
ARG VERSION=v0.0.0
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/shrinkyourshit ./cmd/shrinkyourshit

# --- runtime ---
FROM debian:bookworm-slim AS runtime
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gpg gosu \
 && curl -fsSL https://repo.jellyfin.org/keys/jellyfin.asc \
      | gpg --dearmor -o /usr/share/keyrings/jellyfin.gpg \
 && echo "deb [signed-by=/usr/share/keyrings/jellyfin.gpg] https://repo.jellyfin.org/debian bookworm main" \
      > /etc/apt/sources.list.d/jellyfin.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends jellyfin-ffmpeg \
 && apt-get purge -y curl gpg \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*

ENV PUID=1000 \
    PGID=1000 \
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
