# Installing

One page: pick your platform, point it at your media, start it. ShrinkYourShit
ships as a Docker image, a plain binary, and ready-made files for the common
NAS systems.

## Which file for my box

| Platform | Use this | Where it lives |
|----------|----------|----------------|
| Any Linux PC, VM or NAS with plain Docker | generic compose file | `deploy/generic/docker-compose.yml` |
| Synology with Container Manager | Synology compose file | `deploy/synology/docker-compose.yml` |
| TrueNAS SCALE | TrueNAS compose file | `deploy/truenas/docker-compose.yml` |
| Unraid | Community Applications template | `deploy/unraid/shrinkyourshit.xml` — install from the Apps tab |
| Linux or macOS machine joining a server as a worker | one-line installer | Nodes page → Add node |
| Windows machine joining a server as a worker | PowerShell installer | Nodes page → Add node |

The compose files are copies of one contract that differ only in paths and
default user ids, and a test (`deploy/deploy_test.go`, run by `make test`)
keeps them honest — an edit that breaks one fails the build.

## The three folders every install maps

| Container path | What it holds | Notes |
|----------------|---------------|-------|
| `/config` | settings, database, the node's identity | your platform's app-data folder |
| `/media` | your media libraries | **must be read/write** — see below |
| `/scratch` | temp space while a file is being worked on | fast local disk is best |

**Why `/media` must be writable:** a finished result replaces the original
file in place, and by default it only does that when the new file really is
smaller (the result policy, changeable in the app). Replacing in place is a
rename inside the folder, so the folder itself must be writable too — a
read-only mount leaves every job failed and every original untouched.

## PUID / PGID

The container writes its files as this user and group. Match it to the owner
of your media folder, or the files it writes will belong to someone else.
Find yours by running `id -u` and `id -g` over SSH as the user who owns the
folders.

| Platform | Template default | Who that is |
|----------|------------------|-------------|
| generic  | 1000 / 1000 | the first regular Linux user |
| Synology | 1024 / 100  | DSM's `docker` user, `users` group |
| TrueNAS  | 568 / 568   | the `apps` user SCALE runs apps as |
| Unraid   | 99 / 100    | `nobody` / `users`, the Unraid convention |

## The port

The web UI and API listen on **8765**, published as `8765:8765`. Open
`http://<machine>:8765` after starting. Note that LAN discovery does not
cross the Docker bridge, so worker nodes always join using the server's
address from the Nodes page — auto-discovery is for binary installs.

## Updating

The templates pin an image tag (`0.1.2` today) instead of `:latest`, so
updates are deliberate:

1. Find the newest release on the [releases page](https://github.com/PeiceOfToast/shrinkyourshit/releases).
2. Change the tag in your compose file (or the Unraid template's Repository
   field) to that release number.
3. Run `docker compose pull && docker compose up -d` — on Unraid, use the
   docker tab's update button.

Your data lives in `/config`; nothing else needs to move.

## Adding a worker node

Nodes page → Add node → copy the one-line command → paste it on the other
machine. No root on that machine? Add `--prefix DIR` to the command and the
binary and service file land inside `DIR` instead of the system folders.

After pairing, a node starts **stopped** — open the Nodes page and press
Start before it takes work. A stopped node only sends heartbeats; it never
encodes until started.

## Removing it again

**Container install:** stop and remove the container (`docker compose down`
on the compose platforms, the docker tab on Unraid). Your data stays in the
mapped folders; delete the mapped `/config` folder to purge it.

**Binary node:**

```sh
sh installer.sh --uninstall          # stops and removes the service and the binary, keeps your data
sh installer.sh --uninstall --purge  # also removes the data dir (settings + database)
```

`--uninstall` never touches the data dir unless `--purge` is passed, and
neither flag asks questions, so both are safe to run from scripts. An
uninstall on a machine that never installed exits successfully with a note.
