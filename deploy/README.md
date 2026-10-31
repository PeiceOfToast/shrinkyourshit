# Deploy templates

Ready-made install files so a NAS box runs ShrinkYourShit in minutes. Full
walkthrough (volumes, PUID/PGID, updates, adding a node):
[docs/installing.md](../docs/installing.md).

## Which file for which box

| Your box | Use | Why this one |
|----------|-----|--------------|
| Any Linux PC, VM or NAS with plain Docker | [generic/docker-compose.yml](generic/docker-compose.yml) | Relative `./` paths next to the compose file; no platform assumptions |
| Synology with Container Manager | [synology/docker-compose.yml](synology/docker-compose.yml) | DSM paths (`/volume1/docker/...`) and the DSM `docker` user (1024/100) |
| TrueNAS SCALE | [truenas/docker-compose.yml](truenas/docker-compose.yml) | Pool paths (`/mnt/tank/apps/...`) and the `apps` user (568/568) |
| Unraid | [unraid/shrinkyourshit.xml](unraid/shrinkyourshit.xml) | A Community Applications template: install from the Apps tab, no YAML |

For a machine **joining** an existing server as a worker node (no Docker
needed), use the one-line command the Nodes page generates instead — it runs
[scripts/installer.sh](../scripts/installer.sh).

## What every compose file guarantees

- The image is pinned to a release (`0.1.2` today) — never `:latest`, so an
  upgrade is a deliberate tag change plus `docker compose pull`.
- Port `8765` is the web UI and API.
- The container sees `/config` (settings + database), `/media` (your
  libraries — writable, results replace files in place) and `/scratch`
  (temp space).
- `PUID`/`PGID` set the files' owner; check yours with `id -u` and `id -g`.
- `restart: unless-stopped`, no `privileged`, no host networking, no Docker
  socket.

These rules are enforced by [deploy/deploy_test.go](deploy_test.go): an edit
that breaks a template fails `make test` instead of shipping.
