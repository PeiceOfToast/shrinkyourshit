#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
#
# ShrinkYourShit worker installer (task 09). Detects OS and architecture,
# downloads the matching release binary, installs it, writes the data-dir
# config (role=worker, server, token) and registers a service
# (systemd on Linux, launchd on macOS).
#
# The server serves this file at /install.sh with __SHRINK_SERVER_URL__ and
# __SHRINK_VERSION__ replaced, so the copy an admin pastes knows where to
# join and which release to fetch. The raw source works too when --server
# and --version are given.
#
# Usage:
#   curl -fsSL http://server:8765/install.sh | sh -s -- --server http://server:8765 --token TOKEN
#   sh installer.sh --server http://server:8765 [--token TOKEN] [--node-name NAME]
#                   [--data-dir DIR] [--install-dir DIR] [--service-dir DIR]
#                   [--prefix DIR] [--version vX.Y.Z] [--release-url URL]
#                   [--no-service]
#   sh installer.sh --uninstall [--purge] [--data-dir DIR]
#                   [--install-dir DIR] [--service-dir DIR]
#
# --uninstall removes the service and the binary but never the data dir;
# add --purge to remove the data dir too (no confirmation prompt).
#
# shellcheck shell=sh

set -eu

SCRIPT_VERSION="__SHRINK_VERSION__"
SERVER="__SHRINK_SERVER_URL__"
RELEASE_URL_DEFAULT="https://github.com/PeiceOfToast/shrinkyourshit/releases/download"

fail() {
	printf 'installer: %s\n' "$1" >&2
	exit 1
}

# Every flag mistake (unknown option, missing value, bad combination)
# prints usage and exits 2, so scripts can tell a usage error (2) from a
# runtime failure (1).
usage_fail() {
	printf 'installer: %s\n\n' "$1" >&2
	usage >&2
	exit 2
}

usage() {
	cat <<EOF
Usage: installer.sh --server URL [options]
       installer.sh --uninstall [options]

Install (join a server as a worker node):
  --server URL        the ShrinkYourShit server to join (required unless
                      the script was served by one)
  --token TOKEN       one-time pairing token (optional: without it the
                      node waits for approval in the server UI)
  --node-name NAME    display name of this node (default: hostname)
  --data-dir DIR      worker data dir (default: ~/.shrinkyourshit)
  --install-dir DIR   where the binary goes (default: /usr/local/bin)
  --service-dir DIR   write the service file here instead of the system
                      location and skip activation (for checks)
  --prefix DIR        shorthand for --install-dir DIR --service-dir DIR
                      (a user-local install that needs no root)
  --version vX.Y.Z    release tag to download (baked in by the server)
  --release-url URL   release download base (override for tests)
  --no-service        do not register or start a service

Remove (needs no --server; the data dir is only touched by --purge):
  --uninstall         stop and disable the service, remove the service
                      file and the binary, print each step
  --purge             with --uninstall: also remove the data dir (the
                      database and settings). No confirmation prompt.
EOF
}

# --- flags -----------------------------------------------------------------

TOKEN=""
NODE_NAME=""
DATA_DIR=""
INSTALL_DIR="/usr/local/bin"
SERVICE_DIR=""
RELEASE_URL="$RELEASE_URL_DEFAULT"
NO_SERVICE=0
UNINSTALL=0
PURGE=0

while [ $# -gt 0 ]; do
	case "$1" in
	--server)
		[ $# -ge 2 ] || usage_fail "--server needs a value"
		SERVER="$2"
		shift 2
		;;
	--server=*)
		SERVER="${1#*=}"
		shift
		;;
	--token)
		[ $# -ge 2 ] || usage_fail "--token needs a value"
		TOKEN="$2"
		shift 2
		;;
	--node-name)
		[ $# -ge 2 ] || usage_fail "--node-name needs a value"
		NODE_NAME="$2"
		shift 2
		;;
	--data-dir)
		[ $# -ge 2 ] || usage_fail "--data-dir needs a value"
		DATA_DIR="$2"
		shift 2
		;;
	--install-dir)
		[ $# -ge 2 ] || usage_fail "--install-dir needs a value"
		INSTALL_DIR="$2"
		shift 2
		;;
	--service-dir)
		[ $# -ge 2 ] || usage_fail "--service-dir needs a value"
		SERVICE_DIR="$2"
		shift 2
		;;
	--prefix)
		[ $# -ge 2 ] || usage_fail "--prefix needs a value"
		INSTALL_DIR="$2"
		SERVICE_DIR="$2"
		shift 2
		;;
	--prefix=*)
		INSTALL_DIR="${1#*=}"
		SERVICE_DIR="${1#*=}"
		shift
		;;
	--version)
		[ $# -ge 2 ] || usage_fail "--version needs a value"
		SCRIPT_VERSION="$2"
		shift 2
		;;
	--release-url)
		[ $# -ge 2 ] || usage_fail "--release-url needs a value"
		RELEASE_URL="$2"
		shift 2
		;;
	--no-service)
		NO_SERVICE=1
		shift
		;;
	--uninstall)
		UNINSTALL=1
		shift
		;;
	--purge)
		PURGE=1
		shift
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		usage_fail "unknown option $1"
		;;
	esac
done

# --purge is only ever an addition to --uninstall: it deletes the data
# dir, so it must be asked for explicitly (no prompt, no default).
if [ "$PURGE" -eq 1 ] && [ "$UNINSTALL" -eq 0 ]; then
	usage_fail "--purge only makes sense together with --uninstall"
fi

# --- platform --------------------------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
Linux) GOOS="linux" ;;
Darwin) GOOS="darwin" ;;
*) fail "unsupported OS $OS (this installer covers Linux and macOS; Windows uses install.ps1)" ;;
esac
case "$ARCH" in
x86_64 | amd64) GOARCH="amd64" ;;
aarch64 | arm64) GOARCH="arm64" ;;
*) fail "unsupported architecture $ARCH (want amd64 or arm64)" ;;
esac

# --- uninstall (task 29) ---------------------------------------------------
# Runs before the --server validation on purpose: removing a node has
# nothing to do with the server it used to talk to. The data dir is only
# touched by --purge, which the caller must pass explicitly.

uninstall_all() {
	removed=0
	BIN_PATH="$INSTALL_DIR/shrinkyourshit"

	# 1. Stop and disable the service, remove its file.
	case "$GOOS" in
	linux)
		SYSTEM_UNIT="/etc/systemd/system/shrinkyourshit.service"
		if [ -f "$SYSTEM_UNIT" ]; then
			if command -v systemctl >/dev/null 2>&1; then
				if systemctl stop shrinkyourshit.service >/dev/null 2>&1; then
					printf 'Stopped the shrinkyourshit service.\n'
				fi
				if systemctl disable shrinkyourshit.service >/dev/null 2>&1; then
					printf 'Disabled the shrinkyourshit service.\n'
				fi
			else
				printf 'systemctl not found; assuming the service is not running.\n'
			fi
			rm -f "$SYSTEM_UNIT" || fail "could not remove $SYSTEM_UNIT (re-run with sudo)"
			printf 'Removed %s\n' "$SYSTEM_UNIT"
			systemctl daemon-reload >/dev/null 2>&1 || true
			removed=1
		fi
		if [ -n "$SERVICE_DIR" ]; then
			UNIT="$SERVICE_DIR/shrinkyourshit.service"
			if [ -f "$UNIT" ]; then
				rm -f "$UNIT" || fail "could not remove $UNIT"
				printf 'Removed %s\n' "$UNIT"
				removed=1
			fi
		fi
		;;
	darwin)
		PLIST="$HOME/Library/LaunchAgents/dev.shrinkyourshit.worker.plist"
		if [ -f "$PLIST" ]; then
			if launchctl unload "$PLIST" >/dev/null 2>&1; then
				printf 'Unloaded the launchd agent.\n'
			fi
			rm -f "$PLIST" || fail "could not remove $PLIST"
			printf 'Removed %s\n' "$PLIST"
			removed=1
		fi
		;;
	esac

	# 2. Remove the binary.
	if [ -e "$BIN_PATH" ]; then
		rm -f "$BIN_PATH" || fail "could not remove $BIN_PATH (re-run with sudo)"
		printf 'Removed %s\n' "$BIN_PATH"
		removed=1
	fi

	# 3. The data dir is never touched without --purge.
	if [ -z "$DATA_DIR" ]; then
		DATA_DIR="${HOME:-.}/.shrinkyourshit"
	fi
	if [ "$PURGE" -eq 1 ]; then
		if [ -e "$DATA_DIR" ]; then
			rm -rf "$DATA_DIR" || fail "could not remove the data dir $DATA_DIR"
			printf 'Removed the data dir %s\n' "$DATA_DIR"
			removed=1
		fi
	elif [ -d "$DATA_DIR" ]; then
		printf 'Kept the data dir %s (pass --purge to remove it too).\n' "$DATA_DIR"
	fi

	if [ "$removed" -eq 0 ]; then
		printf 'Nothing to uninstall: no ShrinkYourShit binary, service file or data dir was found.\n'
	else
		printf 'Uninstall finished.\n'
	fi
}

if [ "$UNINSTALL" -eq 1 ]; then
	uninstall_all
	exit 0
fi

# A served copy has the server URL and release version baked in; the raw
# source still carries the placeholder text, which cannot be a URL or a
# release tag — in that case --server (and --version) are required.
case "$SERVER" in
http://* | https://*) ;;
*)
	usage >&2
	fail "--server is required (e.g. --server http://192.168.1.10:8765)"
	;;
esac
case "$SCRIPT_VERSION" in
v[0-9]*) ;;
*)
	fail "no release version known: serve this script from the server (GET /install.sh), or pass --version vX.Y.Z"
	;;
esac

# --- download and install --------------------------------------------------

ASSET_URL="$RELEASE_URL/$SCRIPT_VERSION/shrinkyourshit-$GOOS-$GOARCH.tar.gz"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

printf 'Downloading ShrinkYourShit %s for %s/%s...\n' "$SCRIPT_VERSION" "$GOOS" "$GOARCH"
if command -v curl >/dev/null 2>&1; then
	curl -fsSL -o "$TMP_DIR/binary.tar.gz" "$ASSET_URL" || fail "download failed: $ASSET_URL"
elif command -v wget >/dev/null 2>&1; then
	wget -qO "$TMP_DIR/binary.tar.gz" "$ASSET_URL" || fail "download failed: $ASSET_URL"
else
	fail "need curl or wget to download the release"
fi

mkdir -p "$TMP_DIR/unpack"
tar -xzf "$TMP_DIR/binary.tar.gz" -C "$TMP_DIR/unpack"
BIN_SRC="$TMP_DIR/unpack/shrinkyourshit-$GOOS-$GOARCH"
[ -f "$BIN_SRC" ] || fail "the release archive did not contain shrinkyourshit-$GOOS-$GOARCH"

mkdir -p "$INSTALL_DIR"
INSTALL_PATH="$INSTALL_DIR/shrinkyourshit"
mv "$BIN_SRC" "$INSTALL_PATH"
chmod 0755 "$INSTALL_PATH"
printf 'Installed %s\n' "$INSTALL_PATH"

# --- configuration ---------------------------------------------------------

if [ -z "$NODE_NAME" ]; then
	NODE_NAME="$(hostname 2>/dev/null || uname -n)"
fi
if [ -z "$DATA_DIR" ]; then
	DATA_DIR="${HOME:-.}/.shrinkyourshit"
fi
mkdir -p "$DATA_DIR"
CONFIG="$DATA_DIR/config.yaml"
{
	printf 'role: worker\n'
	printf 'server: %s\n' "$SERVER"
	printf 'node_name: %s\n' "$NODE_NAME"
	if [ -n "$TOKEN" ]; then
		printf 'token: %s\n' "$TOKEN"
	fi
} >"$CONFIG"
chmod 0600 "$CONFIG"
printf 'Wrote %s (role: worker, server: %s)\n' "$CONFIG" "$SERVER"

# --- service ---------------------------------------------------------------

install_systemd_unit() {
	# $1 = unit file path
	#
	# Restart=always is required, not on-failure: `shrinkyourshit update`
	# exits 0 after swapping the binary and expects systemd to bring the
	# new build straight back up (task 16). With on-failure a clean exit
	# would leave the service stopped after every update.
	cat >"$1" <<EOF
[Unit]
Description=ShrinkYourShit worker
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$INSTALL_PATH serve
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
}

install_launchd_plist() {
	# $1 = plist path
	cat >"$1" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>dev.shrinkyourshit.worker</string>
	<key>ProgramArguments</key>
	<array>
		<string>$INSTALL_PATH</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>$DATA_DIR/worker.log</string>
	<key>StandardErrorPath</key>
	<string>$DATA_DIR/worker.log</string>
</dict>
</plist>
EOF
}

# --service-dir: write the service file to a scratch location and skip
# activation entirely (used by the install checks).
if [ -n "$SERVICE_DIR" ]; then
	mkdir -p "$SERVICE_DIR"
	case "$GOOS" in
	linux)
		install_systemd_unit "$SERVICE_DIR/shrinkyourshit.service"
		printf 'Wrote %s/shrinkyourshit.service (not activated)\n' "$SERVICE_DIR"
		;;
	darwin)
		install_launchd_plist "$SERVICE_DIR/dev.shrinkyourshit.worker.plist"
		printf 'Wrote %s/dev.shrinkyourshit.worker.plist (not activated)\n' "$SERVICE_DIR"
		;;
	esac
elif [ "$NO_SERVICE" -eq 1 ]; then
	printf 'Skipping service registration (--no-service).\n'
elif [ "$GOOS" = "linux" ] && [ -d /run/systemd/system ]; then
	if [ "$(id -u)" -eq 0 ]; then
		install_systemd_unit /etc/systemd/system/shrinkyourshit.service
		if command -v systemctl >/dev/null 2>&1; then
			systemctl daemon-reload || true
			systemctl enable --now shrinkyourshit.service || fail "could not start the shrinkyourshit service; check journalctl -u shrinkyourshit"
			printf 'Service shrinkyourshit is enabled and started.\n'
		else
			printf 'Unit written to /etc/systemd/system/shrinkyourshit.service (systemctl not found).\n'
		fi
	else
		printf 'Not root: run the worker manually with "%s serve", or re-run the installer with sudo.\n' "$INSTALL_PATH"
	fi
elif [ "$GOOS" = "darwin" ]; then
	PLIST="$HOME/Library/LaunchAgents/dev.shrinkyourshit.worker.plist"
	mkdir -p "$HOME/Library/LaunchAgents"
	install_launchd_plist "$PLIST"
	launchctl unload "$PLIST" 2>/dev/null || true
	launchctl load "$PLIST" || printf 'Could not load the launchd agent; start it with: launchctl load "%s"\n' "$PLIST"
	printf 'launchd agent installed (%s).\n' "$PLIST"
else
	printf 'No supported service manager found; start the worker manually with "%s serve".\n' "$INSTALL_PATH"
fi

# --- what happens next -----------------------------------------------------

printf '\nNode name: %s\n' "$NODE_NAME"
if [ -n "$TOKEN" ]; then
	printf 'The node pairs with the server using its token and starts working.\n'
else
	printf 'The node is asking to join %s; approve it in the server UI\n' "$SERVER"
	printf '(Nodes page). It is waiting for approval.\n'
fi
