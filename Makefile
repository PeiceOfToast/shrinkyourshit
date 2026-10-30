# ShrinkYourShit build recipes. See STACK.md for the pinned toolchain.
BINARY := shrinkyourshit
BIN_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo v0.0.0)
LDFLAGS := -s -w -X main.version=$(VERSION)
IMAGE ?= ghcr.io/peiceoftoast/shrinkyourshit:dev
EMBED_DIST := internal/webui/dist

CROSS_TARGETS := \
	linux/amd64 \
	linux/arm64 \
	windows/amd64 \
	darwin/amd64 \
	darwin/arm64

.PHONY: all build dev web web-deps placeholder test test-go test-web \
	test-integration e2e lint lint-go lint-web spdx installer-check docker \
	cross release-cross clean

all: build

# placeholder creates a minimal web UI dist when none exists so the go:embed
# in internal/webui compiles in a fresh clone before the first Vite build.
# It never overwrites a real build.
placeholder:
	@if [ ! -f $(EMBED_DIST)/index.html ]; then \
		mkdir -p $(EMBED_DIST); \
		printf '<!doctype html>\n<html lang="en">\n  <head>\n    <meta charset="utf-8" />\n    <title>ShrinkYourShit</title>\n  </head>\n  <body>\n    <p>Placeholder web UI. Run <code>make build</code> to build the real UI into this folder.</p>\n  </body>\n</html>\n' > $(EMBED_DIST)/index.html; \
	fi

web-deps: web/node_modules/.keep

web/node_modules/.keep: web/package-lock.json
	cd web && npm ci --no-fund --no-audit && touch node_modules/.keep

# web builds the UI into web/dist, then copies it into the embed location.
web: web-deps
	cd web && npm run build
	rm -rf $(EMBED_DIST)
	cp -r web/dist $(EMBED_DIST)

build: web
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/shrinkyourshit

# dev runs the Vite dev server (port 5173) and the Go server (port 8765).
# Vite proxies /api to 8765, so open http://localhost:5173.
dev: placeholder
	@bash -eu -o pipefail -c 'trap "kill 0" EXIT; cd web && npm run dev & go run ./cmd/shrinkyourshit serve'

test: test-go test-web

test-go: placeholder
	go test ./...

test-web: web-deps
	cd web && npm run test

# Integration tests (build tag `integration`) need ffmpeg/ffprobe and skip
# with a clear message when they are absent.
test-integration: placeholder
	go test -tags=integration ./...

# e2e (task 12): Playwright layout suite at phone/tablet/desktop widths
# against the real Go binary serving the built web app. `make web` builds
# the UI into the embed, the binary is compiled fresh, and the suite's
# global setup starts the server with throwaway dirs and a seeded library.
# First run downloads the chromium headless shell; system deps that
# chromium needs must be present (see README).
e2e: web
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/shrinkyourshit
	cd web && npx playwright install chromium
	cd web && npx playwright test

lint: lint-go spdx lint-web

lint-go: placeholder
	go vet ./...
	golangci-lint run

lint-web: web-deps
	cd web && npm run lint && npm run format:check

# spdx fails when a Go or TypeScript source file is missing its license header.
spdx:
	./scripts/check-spdx.sh

# installer-check runs shellcheck over the POSIX installer (CI installs
# shellcheck; locally install it first, e.g. from your package manager).
installer-check:
	shellcheck scripts/installer.sh scripts/test-installer.sh

docker:
	docker build -f docker/Dockerfile --build-arg VERSION=$(VERSION) -t $(IMAGE) .

# cross produces static binaries for every target in STACK.md (no cgo).
# The embedded UI is the placeholder unless `make web` ran first; the release
# workflow always builds the web UI before cross-compiling.
cross: placeholder
	@set -e; \
	for target in $(CROSS_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=; [ "$$os" = windows ] && ext=.exe; \
		echo "== $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags '$(LDFLAGS)' \
			-o $(BIN_DIR)/$(BINARY)-$$os-$$arch$$ext ./cmd/shrinkyourshit; \
	done

# release-cross is the local release dry run (task 16): the same build,
# packaging and verification the release workflow performs, without
# uploading anything. `make release-cross VERSION=vX.Y.Z` pins the version.
# Output lands in bin/release.
release-cross: placeholder
	./scripts/release.sh build "$(VERSION)" $(BIN_DIR)/release
	./scripts/release.sh package $(BIN_DIR)/release
	./scripts/release.sh verify $(BIN_DIR)/release

clean:
	rm -rf $(BIN_DIR) web/coverage
