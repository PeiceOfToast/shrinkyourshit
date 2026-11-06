# Encode profiles and in-app storage: specification (wave v1.11)

Owner direction, 2026-11-05: file mounts are built into the container, the install does the
setup, and everything after that happens in the WebUI. Encode rules stop living on each
library: they become reusable profiles built in Encode settings, and a library just points
at one. `DECISIONS.md` stays read only for agents; the decisions below are recorded here.

## 1. What this wave changes

Two features with one theme: define a thing once, in one place, and let libraries reference it.

- **Storage**: add, mount, test and browse NFS and SMB shares from the WebUI. No compose edits,
  no host fstab, no path typing.
- **Profiles**: the encode graph, its tier, and its output policy move off the library and into
  a named profile on the Encode settings page. A library picks a profile.

## 2. Owner decisions

1. Mounts are handled inside the container. The installer sets the container up once; every
   mount after that is created and managed from the WebUI.
2. Encode rules belong to Encode settings, not to each library page.
3. A library holds a profile reference and follows that profile's flow. The per-library encode
   builder is removed from the library pages.
4. Hermes picks the mount mechanism and the security posture. That choice is section 4.

## 3. Verified current state (main at b1dabcd, 2026-11-06)

- No mount concept exists. Grep for mount/nfs/cifs/smb across `internal/` returns only two
  scanner comments (network mounts cannot be inotify watched, so the scanner polls) and
  `node.go` `path_mappings`, which only translates a server path to a node path.
- Encode settings (`web/src/EncodeSettingsPage.tsx`) holds app wide values: `default_video_codec`,
  `preferred_languages`, `default_rule_tier`, `allow_raw_args`, `worker_concurrency`,
  `scratch_dir`, `default_result_policy`, QSV options, update options.
- The library row owns `target`, `easy`, `flow`, `flow_source`, `rule_tier`, `path`, `watch`,
  `schedule`, `result_policy`, `file_kinds`. Wave v1.10 put the tier on the library.
- `docker/Dockerfile` runtime declares `VOLUME ["/config", "/media", "/scratch"]` and sets
  `SHRINK_DATA_DIR=/config`; `docker/compose.example.yml` mounts host paths as volumes.
- Login is a setting (`store.SettingLoginEnabled`). A default install currently reports
  `login_enabled: false`, which matters in 4.6.

## 4. Part one: storage

### 4.1 Mechanism: in-container mounts, app-managed

The server mounts shares itself, inside the container, using `mount(8)`, and owns their
lifecycle. Requirements in the container: the `nfs-common` and `cifs-utils` packages, the
`SYS_ADMIN` capability, and an AppArmor override so the syscall is permitted. Not
`privileged: true`: add exactly one capability and one security option, nothing else.

Runtime shape:

- Mountpoint convention is `/storage/<name>`, where `name` is a slug unique across storages.
  Libraries then hold `/storage/<name>/Movies` style paths.
- Mounts live in the container's mount namespace, so they disappear when the container
  restarts. The database is the source of truth and the server remounts every enabled storage
  at startup. That is deliberate: it also means the config volume stays portable.
- A watchdog checks `/proc/mounts` on an interval, remounts anything enabled that has silently
  gone away (an NFS share going stale mid-encode is a real failure mode, and a stale mount
  that still looks mounted is worse than one that is reported missing), and records the exact
  `mount(8)` stderr on failure.
- One broken storage never prevents the server from starting. It shows as an error in the UI
  and on the dashboard.

### 4.2 Rejected alternatives, and why

- **Compose volume edits per share** (what the project does today): that is the thing the owner
  is complaining about. Rejected.
- **Docker volume plugins** (local-persist, nfs, rexray): still a compose edit plus a restart,
  plus a third party plugin to trust. Rejected.
- **Host-side helper or systemd unit**: works for a VM, breaks on a NAS appliance, and splits
  the configuration across two machines. Rejected for v1.
- **rclone or FUSE mounts**: needs `/dev/fuse` and still needs privileges, adds a translation
  layer and latency on a media path. Rejected as the primary mechanism; noted as a possible
  later path for cloud remotes.

### 4.3 Data model

```sql
CREATE TABLE storages (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,        -- slug, also the mountpoint: /storage/<name>
  protocol    TEXT NOT NULL,               -- nfs | smb
  source      TEXT NOT NULL,               -- host:/export  or  //host/share
  options     TEXT NOT NULL DEFAULT '',    -- mount -o string, validated in 4.4
  username    TEXT NOT NULL DEFAULT '',    -- smb only
  domain      TEXT NOT NULL DEFAULT '',    -- smb only
  secret_enc  TEXT NOT NULL DEFAULT '',    -- smb password, encrypted at rest
  read_only   INTEGER NOT NULL DEFAULT 0,
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
```

Secrets: the SMB password is encrypted with a key generated on first run into
`/config/storage.key` (0600). Operators who want their own key can set `SHRINK_STORAGE_KEY`.
The password is never returned by any API, is never logged, and is written to a temporary
credentials file (0600, in the scratch dir) that is deleted right after mount, because passing
`pass=` on the command line exposes it in `ps`.

### 4.4 Validation and failure rules

- `options` is a comma separated allowlist: `vers`, `nfsvers`, `soft`, `hard`, `timeo`,
  `retrans`, `port`, `rw`, `ro`, `nosuid`, `nodev`, `noexec`, `noatime`, `actimeo`, `uid`,
  `gid`, `file_mode`, `dir_mode`, `iocharset`, `sec`, `_netdev`. Anything else is a 400 with a
  field error. Shell metacharacters, newlines and a second `-o` are rejected outright.
- `source` is validated per protocol: `host:/path` for NFS, `//host/share` for SMB. No spaces.
- Defaults that reflect this lab's own history: NFS defaults to `vers=4.2,soft,timeo=50,retrans=2`.
  The version is 4.2 and not 3 because NFSv3 locking needs `rpc.statd` on the client, and the
  shipped container runs neither rpcbind nor statd. A `vers=3` mount there fails with
  `mount.nfs: rpc.statd is not running but is required for remote locking` (real-hardware run,
  2026-11-06, task 80). v4.2 carries its own locking and mounts with neither daemon. `vers=3`
  stays allowed when an operator sets it, and the help text says what it needs. The soft mount is
  kept because a hard mount inside a container turns a NAS hiccup into an unkillable process, and
  because the NZBGet incident on 2026-11-05 was a soft NFS mount producing a `.nfs` silly
  rename that failed a move with EBUSY. Those defaults are shown and editable, with the
  reasoning in the help text rather than hidden.
- Mount, unmount and test are serialised per storage with a per-storage lock so two clicks
  cannot race.
- `test` mounts, stats the mountpoint, writes and removes a probe file when not read only, then
  unmounts, and returns the timings.

### 4.5 Image and compose, exactly

- `docker/Dockerfile` runtime stage: install `nfs-common` and `cifs-utils` from the existing
  Debian bookworm base, same apt pattern already used for the ffmpeg packages. Create
  `/storage` and add it to `VOLUME`.
- `docker/compose.example.yml`:
  ```yaml
  cap_add:
    - SYS_ADMIN
  security_opt:
    - apparmor=unconfined
  ```
  with a comment explaining that this is what lets the container mount shares, that it is
  narrower than `privileged: true`, and that `apparmor=unconfined` is required because the
  default profile blocks `mount(2)`.
- The installer and `START-HERE`/README material must state the requirement, because a user who
  copies a compose file from an older release gets a clear error instead of a mystery.
- `SHRINK_MANAGE_MOUNTS=false` disables the feature entirely, for operators who do not want the
  capability. The UI then explains why the add button is disabled.

### 4.6 The security catch, and the gate

Mounting a share gives this process kernel-level authority to attach arbitrary network
resources and read and write everything it can reach as root in the container. That is a
different risk class from everything else the app does. Therefore:

- **Storage write endpoints (create, update, delete, mount, unmount, test) require authentication
  to be enabled.** If `login_enabled` is false, those endpoints return 403 with a message telling
  the operator to turn on login first. Read-only status may stay open so the UI can explain.
- The library path picker requires authentication as well.
- An optional `storage_allowed_hosts` setting (comma separated host or CIDR list) restricts which
  hosts may be mounted at all. Empty means no restriction, but the UI recommends setting it.
- When mounts exist and login is disabled, the dashboard shows a standing warning. When login is
  disabled, the Storage page renders read only with the reason and a link to App settings.

### 4.7 Workers and other nodes

v1 mounts on the server node only, which is where the scanner and the built-in worker run. The
storage record reserves a per-node override table in a later wave, and a worker that cannot see
a library path must fail the job with an error that names the storage, instead of reading an
empty directory and reporting success. Silent empty directories are the failure mode that
loses files.

## 5. Part two: encode profiles

### 5.1 Model

```sql
CREATE TABLE profiles (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  description   TEXT NOT NULL DEFAULT '',
  tier          TEXT NOT NULL,              -- basic | moderate | advanced
  graph         TEXT NOT NULL,              -- canonical flow graph JSON
  result_policy TEXT NOT NULL,              -- replace | move | copy
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
ALTER TABLE libraries ADD COLUMN profile_id TEXT REFERENCES profiles(id);
```

The tier moves from the library to the profile. `libraries.rule_tier` and `libraries.flow` stay
in place, unused, for one release so a rollback does not lose data, then they are dropped.

### 5.2 Migration

1. For every library with a stored graph, create a profile named after the library unless an
   identical graph already has a profile (compare a sha256 of the canonical graph JSON).
2. Set `libraries.profile_id`, carry the library's `rule_tier` and `result_policy` onto the
   profile.
3. A library with no graph gets the profile named by `default_rule_tier`'s default seed plus a
   note in the migration log. Nothing is deleted.

### 5.3 Edit semantics

- Editing a profile changes behaviour for every library using it. That is the point, and it must
  be visible: the profile editor shows "used by N libraries" and the save confirmation lists
  them.
- A Duplicate action exists so someone can diverge without altering shared behaviour.
- Deleting a profile is refused while any library references it.
- No per library overrides in v1. Keeping the profile absolute is what makes the mental model
  clean; if overrides are wanted later they become an explicit feature, not a silent one.

### 5.4 What lives where

| On the profile | On the library |
|---|---|
| graph (steps, conditions, scaling, rate control) | storage and subpath |
| tier and its representability rules | watch on or off |
| result policy (replace, move, copy) | schedule |
| name and description | file kinds, enabled |

### 5.5 UI

- `/encode` becomes the profile list: name, tier badge, used-by count, result policy, last
  edited. Selecting one opens the existing editor (Basic form, Moderate tree, Advanced canvas)
  bound to the profile instead of a library.
- The tier control and the "not fully representable" banner from v1.10 move to the profile page.
- The library form replaces the embedded builder with a profile dropdown plus a plain language
  summary of what the selected profile does, rendered from `StepTypes()` and the existing
  describe helpers. Library detail shows the inherited profile name, a link, and a warning when
  the library has no profile.
- Any library pointing at a profile whose tier cannot express its own graph gets a dashboard
  warning naming both.

## 6. Task order

| Task | Scope |
|---|---|
| 64 | Storage table, encryption at rest with the config key file, store CRUD |
| 65 | Storage API: validation, option allowlist, source checks, per-storage locking |
| 66 | Mount executor: nfs and smb mount, cifs credentials file, unmount, test, startup remount, watchdog, status |
| 67 | Dockerfile packages, compose capability and security options, installer and README notes, `SHRINK_MANAGE_MOUNTS` |
| 68 | Storage page in the WebUI plus the authentication gate and the dashboard warning |
| 69 | Storage browser and the library path picker |
| 70 | Profiles table, migration from libraries with graph hashing and dedupe, `library.profile_id` |
| 71 | Profiles API: CRUD, usage counts, duplicate, delete guard, representability on the profile |
| 72 | Encode settings becomes the profile list and hosts the editor |
| 73 | Library form and detail: profile dropdown, inherited summary, warnings; remove the embedded builder |
| 74 | Result policy moves to the profile; deprecate `libraries.flow`, `easy`, `rule_tier`, `result_policy` with docs and a follow up removal task |
| 75 | Verify on real hardware: mount NFS and SMB from the WebUI, encode one file from a share, switch a library's profile and prove the behaviour changes; docs, ROADMAP, STATUS |

## 7. Wave rules

Branch `task/NN-slug` from `main`, one PR, CI green, `make test`, `make lint`, `npm run build`
and `npx playwright test` green before ticking the checklist. `DECISIONS.md` untouched. Every
existing `data-testid` survives unless this spec says otherwise. Any changed assertion goes in
`tasks/NN-notes.md` with file, line, old, new and why. Migrations are forward only and must be
safe to run on a database created by v1.10.
