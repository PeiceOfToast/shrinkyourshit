#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
#
# Install check for the POSIX installer (task 09, extended by task 29).
# Runs the real scripts/installer.sh against a fake release server, then
# verifies the binary, the config and the service file, the uninstall and
# purge path, the flag error handling and a root-less --prefix install.
# Made for a throwaway container: point FAKE_RELEASE_URL at a local HTTP
# server that serves the release layout
# (e.g. <base>/v0.0.0-test/shrinkyourshit-linux-amd64.tar.gz).
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
# Restart=always is the required setting (task 16): a self update exits 0
# after the swap and expects systemd to bring the new binary right back.
grep -q '^Restart=always$' "$UNIT"
grep -q 'WantedBy=multi-user.target' "$UNIT"
if command -v systemd-analyze >/dev/null 2>&1; then
	systemd-analyze verify "$UNIT"
fi

# 4. Uninstall (task 29) removes the unit and the binary, keeps the data
#    dir, and needs no --server.
sh "$REPO/scripts/installer.sh" \
	--uninstall \
	--install-dir "$WORK/bin" \
	--data-dir "$WORK/data" \
	--service-dir "$WORK/systemd" \
	>"$WORK/uninstall.log"
cat "$WORK/uninstall.log"
[ ! -e "$WORK/bin/shrinkyourshit" ]
[ ! -e "$UNIT" ]
[ -f "$WORK/data/config.yaml" ]
grep -q 'Kept the data dir' "$WORK/uninstall.log"

# 5. --uninstall --purge also removes the data dir, with no prompt.
sh "$REPO/scripts/installer.sh" \
	--server "$SERVER" \
	--version "$VERSION" \
	--release-url "$FAKE" \
	--install-dir "$WORK/bin" \
	--data-dir "$WORK/data" \
	--service-dir "$WORK/systemd" \
	--token test-token
[ -f "$WORK/data/config.yaml" ]
sh "$REPO/scripts/installer.sh" \
	--uninstall --purge \
	--install-dir "$WORK/bin" \
	--data-dir "$WORK/data" \
	--service-dir "$WORK/systemd" \
	>"$WORK/purge.log"
cat "$WORK/purge.log"
[ ! -e "$WORK/bin/shrinkyourshit" ]
[ ! -d "$WORK/data" ]

# 6. Uninstall on a machine that never installed exits 0 with a clear
#    message.
NOTHING="$(mktemp -d)"
rc=0
sh "$REPO/scripts/installer.sh" \
	--uninstall \
	--install-dir "$NOTHING/bin" \
	--data-dir "$NOTHING/data" \
	--service-dir "$NOTHING/systemd" \
	>"$WORK/nothing.log" 2>&1 || rc=$?
cat "$WORK/nothing.log"
[ "$rc" -eq 0 ]
grep -q 'Nothing to uninstall' "$WORK/nothing.log"

# 7. Every unknown flag prints usage and exits 2.
rc=0
sh "$REPO/scripts/installer.sh" --definitely-not-a-flag \
	>"$WORK/badflag.log" 2>&1 || rc=$?
grep -q '^Usage:' "$WORK/badflag.log"
[ "$rc" -eq 2 ]

# 8. --prefix is a user-local install that needs no root. When this
#    harness is root, prove it: drop to an unprivileged user for the whole
#    install -> uninstall -> purge cycle. Without root available the same
#    checks still run as the current user.
PWORK="$(mktemp -d)"
TEST_USER=""
if [ "$(id -u)" = "0" ] && command -v useradd >/dev/null 2>&1; then
	useradd -M -s /bin/sh -d "$PWORK" shrinkcheck 2>/dev/null || true
	if id shrinkcheck >/dev/null 2>&1; then
		TEST_USER=shrinkcheck
		chown -R "$TEST_USER" "$PWORK"
	fi
fi
run() {
	# $@: one command line, paths without spaces (mktemp output).
	if [ -n "$TEST_USER" ]; then
		su -s /bin/sh "$TEST_USER" -c "$*"
	else
		sh -c "$*"
	fi
}
PREFIX="$PWORK/sys"
run "sh $REPO/scripts/installer.sh --prefix $PREFIX --server $SERVER --version $VERSION --release-url $FAKE --data-dir $PWORK/data --token test-token"
[ -x "$PREFIX/shrinkyourshit" ]
[ -f "$PREFIX/shrinkyourshit.service" ]
grep -q "ExecStart=$PREFIX/shrinkyourshit serve" "$PREFIX/shrinkyourshit.service"

run "sh $REPO/scripts/installer.sh --uninstall --prefix $PREFIX --data-dir $PWORK/data"
[ ! -e "$PREFIX/shrinkyourshit" ]
[ ! -e "$PREFIX/shrinkyourshit.service" ]
[ -d "$PWORK/data" ]

run "sh $REPO/scripts/installer.sh --uninstall --purge --prefix $PREFIX --data-dir $PWORK/data"
[ ! -d "$PWORK/data" ]

printf 'install check passed\n'
