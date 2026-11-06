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

The app also uses `/storage` inside the container for network shares (see
below). You do not map it: the app creates it, and each share you add mounts
under `/storage/<name>`.

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

## Network storage (NFS and SMB)

Shares are added, mounted, tested and deleted from the Storage page, so
nothing has to be typed into compose or fstab after the install. This needs
two settings on the container, and the compose files and NAS templates in
this repo already carry them:

```yaml
cap_add:
  - SYS_ADMIN            # lets the container run mount(8); far narrower than privileged: true
security_opt:
  - apparmor=unconfined  # Docker's default AppArmor profile blocks mount(2)
```

Unraid's template sets the same two flags in its Extra Parameters field
(`--cap-add=SYS_ADMIN --security-opt apparmor=unconfined`).

**Login comes first.** Adding, changing, testing, mounting and deleting a
share are refused (HTTP 403) until login is turned on in App settings. Turn
it on before you add a share.

**Copied an old compose file?** The app still starts without the two lines.
The startup log says so:

```
shrinkyourshit: network storage is unavailable: SYS_ADMIN not granted; add cap_add SYS_ADMIN and security_opt apparmor=unconfined to the compose file, then recreate the container. The app still starts.
```

A share added in that state fails with the mount's own message, as it is
written by `mount(8)`:

```
mount: /storage/media: permission denied.
```

Add the two lines to your compose file and recreate the container
(`docker compose up -d`). Leave them out if you do not want network storage at all.

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
