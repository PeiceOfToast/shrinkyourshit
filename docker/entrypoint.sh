#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Drops privileges to PUID:PGID when started as root, the linuxserver-style
# convention (decision B9). Never chowns the whole media tree; only the data
# dir gets a recursive chown.
#
# Network storage (task 67): before the drop, one line says whether managed
# shares can mount: the helpers are installed, the container was granted
# SYS_ADMIN, and no AppArmor profile confines it. It never exits. A missing
# capability leaves the app running; the mount error shows on the storage.
set -e

# cap_bit_set HEX BIT succeeds when bit BIT of the hex capability mask is set.
cap_bit_set() {
  [ $((0x$1 >> $2 & 1)) -eq 1 ]
}

# storage_report prints one line about managed network storage. The bounding
# set (CapBnd) is read, not the effective set: after the drop to PUID the
# effective set is empty, but mount(8) is setuid root and regains what the
# bounding set allows.
storage_report() {
  case "${SHRINK_MANAGE_MOUNTS:-true}" in
  false | FALSE | False | 0 | off | OFF | Off)
    echo "shrinkyourshit: network storage is off (SHRINK_MANAGE_MOUNTS=false)"
    return 0
    ;;
  esac

  problems=""
  for helper in mount.nfs mount.cifs; do
    if ! command -v "$helper" >/dev/null 2>&1; then
      problems="${problems:+$problems; }$helper not installed"
    fi
  done
  # Under set -e a failing substitution in an assignment would stop the
  # script, so the fallback is explicit: an unreadable status is "not granted".
  bnd="$(sed -n 's/^CapBnd:[[:space:]]*//p' /proc/self/status 2>/dev/null)" || bnd=""
  if [ -z "$bnd" ] || ! cap_bit_set "$bnd" 21; then
    problems="${problems:+$problems; }SYS_ADMIN not granted"
  fi
  if grep -q '(enforce)\|(complain)' /proc/self/attr/current 2>/dev/null; then
    problems="${problems:+$problems; }AppArmor confines this container"
  fi

  if [ -z "$problems" ]; then
    echo "shrinkyourshit: network storage is ready (mount.nfs and mount.cifs found, SYS_ADMIN granted)"
  else
    echo "shrinkyourshit: network storage is unavailable: $problems; add cap_add SYS_ADMIN and security_opt apparmor=unconfined to the compose file, then recreate the container. The app still starts."
  fi
  return 0
}

main() {
  PUID="${PUID:-1000}"
  PGID="${PGID:-1000}"

  storage_report

  if [ "$(id -u)" = "0" ] && [ "$PUID" != "0" ]; then
    getent group "$PGID" >/dev/null 2>&1 || groupadd -g "$PGID" shrink
    id -u "$PUID" >/dev/null 2>&1 || useradd -u "$PUID" -g "$PGID" -s /usr/sbin/nologin shrink
    for dir in /config /media /scratch /storage; do
      mkdir -p "$dir"
      chown "$PUID:$PGID" "$dir"
    done
    chown -R "$PUID:$PGID" /config
    # Keep CAP_SYS_ADMIN across the privilege drop. Docker grants the container
    # the capability when it starts as root, but gosu/su then clear permitted
    # and effective capabilities for the new uid, so the server could never call
    # mount(2) even though the container "has" SYS_ADMIN. setpriv raises it as an
    # ambient capability, which survives execve for a non-root uid.
    # Losing this is invisible until the first mount fails with EPERM.
    if command -v setpriv >/dev/null 2>&1 &&
       setpriv --reuid="$PUID" --regid="$PGID" --init-groups \
         --inh-caps=+sys_admin --ambient-caps=+sys_admin true >/dev/null 2>&1; then
      exec setpriv --reuid="$PUID" --regid="$PGID" --init-groups \
        --inh-caps=+sys_admin --ambient-caps=+sys_admin \
        /usr/local/bin/shrinkyourshit "$@"
    fi
    exec gosu "$PUID:$PGID" /usr/local/bin/shrinkyourshit "$@"
  fi

  exec /usr/local/bin/shrinkyourshit "$@"
}

# SHRINK_ENTRYPOINT_LIB=1 loads the functions without starting the server;
# deploy/deploy_test.go uses it to test storage_report and cap_bit_set.
if [ "${SHRINK_ENTRYPOINT_LIB:-0}" != "1" ]; then
  main "$@"
fi
