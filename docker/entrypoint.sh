#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Drops privileges to PUID:PGID when started as root, the linuxserver-style
# convention (decision B9). Never chowns the whole media tree; only the data
# dir gets a recursive chown.
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ] && [ "$PUID" != "0" ]; then
  getent group "$PGID" >/dev/null 2>&1 || groupadd -g "$PGID" shrink
  id -u "$PUID" >/dev/null 2>&1 || useradd -u "$PUID" -g "$PGID" -s /usr/sbin/nologin shrink
  for dir in /config /media /scratch; do
    mkdir -p "$dir"
    chown "$PUID:$PGID" "$dir"
  done
  chown -R "$PUID:$PGID" /config
  exec gosu "$PUID:$PGID" /usr/local/bin/shrinkyourshit "$@"
fi

exec /usr/local/bin/shrinkyourshit "$@"
