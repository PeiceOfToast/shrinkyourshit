#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
#
# Install check for the POSIX installer (task 09). Runs the real
# scripts/installer.sh against a fake release server, then verifies the
# binary, the config and the service file. Made for a throwaway container:
# point FAKE_RELEASE_URL at a local HTTP server that serves the release
# layout (e.g. <base>/v0.0.0-test/shrinkyourshit-linux-amd64.tar.gz).
#
#   REPO_DIR=/repo FAKE_RELEASE_URL=http://127.0.0.1:8099 sh scripts/test-installer.sh

set -eu

REPO="${REPO_DIR:-/repo}"
FAKE="${FAKE_RELEASE_URL:?FAKE_RELEASE_URL is required, e.g. http://127.0.0.1:8099}"
VERSION="v0.0.0-test"
SERVER="http://127.0.0.1:8765"
WORK="$(mktemp -d)"

sh "$REPO/scripts/installer.sh" \
	--server "$SERVER" \
	--version "$VERSION" \
	--release-url "$FAKE" \
	--install-dir "$WORK/bin" \
	--data-dir "$WORK/data" \
	--service-dir "$WORK/systemd" \
	--token test-token

# 1. The binary is installed and executable.
[ -x "$WORK/bin/shrinkyourshit" ]

# 2. The data-dir config says worker + server + token.
grep -q '^role: worker$' "$WORK/data/config.yaml"
grep -q "^server: $SERVER\$" "$WORK/data/config.yaml"
grep -q '^token: test-token$' "$WORK/data/config.yaml"

# 3. The service file is written and structurally valid ("systemd-analyze
#    verify or equivalent"; the container usually has no systemd).
UNIT="$WORK/systemd/shrinkyourshit.service"
grep -q '^\[Unit\]' "$UNIT"
grep -q '^\[Service\]' "$UNIT"
grep -q "ExecStart=$WORK/bin/shrinkyourshit serve" "$UNIT"
grep -q '^Restart=on-failure$' "$UNIT"
grep -q 'WantedBy=multi-user.target' "$UNIT"
if command -v systemd-analyze >/dev/null 2>&1; then
	systemd-analyze verify "$UNIT"
fi

printf 'install check passed\n'
