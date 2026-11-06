# Task 75 notes: verify profiles and storage on real hardware

## Run 2026-11-06 (blocked before any proof)

### Plan (unchanged for the rerun)
1. `git switch -c task/75-verify-wave` from main at e7fdfab.
2. Re-check the supervisor test bed: `lxc list`, bring up static IPv4 in t75 (10.239.201.100) and
   t75nas (10.239.201.101), confirm ganesha and smbd are running.
3. Build `bin/shrinkyourshit`, create privileged container t75app (10.239.201.102) with the three
   lxc config settings, install ffmpeg cifs-utils nfs-common, push and run the binary as root.
4. Proofs 1 to 5 from `tasks/75-verify-wave.md`, recording real output here.
5. `go test ./...`, `go vet ./...`, `golangci-lint run`, the spdx script, `cd web && npm test`,
   `npx eslint`, `npx prettier --check`, with exit codes.

## Run 2026-11-06, second attempt (evidence)

Branch `task/75-verify-wave` from main at 6f25a45. Everything below is real output from this VM, copied from
the terminal. Nothing in Go or TypeScript was changed by this task.

### Test bed (re-checked before use)

- `lxc list`: `t75` RUNNING 10.239.201.100 (ganesha + samba), `t75nas` RUNNING 10.239.201.101. `t75` config
  confirmed: `security.privileged: "true"`, `security.syscalls.deny_default: "false"`, `raw.apparmor` with the
  nfs, nfs4, cifs and umount rules, disk device `/home/mb/t75export -> /export-host`. `systemctl is-active
  nfs-ganesha smbd` in t75: `active active`.
- New container `t75app` (image 12a0de07e168, ubuntu 24.04) with the same three settings, static
  10.239.201.102, `apt-get install --no-install-recommends ffmpeg cifs-utils nfs-common sqlite3`
  (ffmpeg 6.1.1-3ubuntu5, `/usr/sbin/mount.nfs`, `/usr/sbin/mount.cifs`).
- App binary: `cd web && npm run build` (exit 0), `cp -r web/dist internal/webui/dist`,
  `go build -trimpath -o bin/shrinkyourshit ./cmd/shrinkyourshit` (exit 0). Pushed to
  `t75app:/usr/local/bin/shrinkyourshit`, run as root by systemd-run:
  `shrinkyourshit serve --data-dir /var/lib/sys --scratch-dir /scratch --port 8765 --ffmpeg /usr/bin/ffmpeg`.
  The default mount root `/storage` was used. Log: `worker: hardware detected encoders=[cpu] ffmpeg=/usr/bin/ffmpeg`.
- The WebUI was driven from the host by a Playwright script (chromium from `web/node_modules`,
  `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`) against http://10.239.201.102:8765. The script and its
  screenshots are in `/tmp/t75pw` on the VM and are not committed.

### 3. Auth gate (login disabled), done first on the fresh install

`POST /api/v1/setup {"mode":"server","login_enabled":false}` -> 201. Then curl with no cookie:

```
POST     /storages                  403 {"error":"Turn on login in App settings before you manage storage. Storage can reach your network, so it stays locked until login is on.","code":"login_required"}
PUT      /storages/x                403 (same body)
DELETE   /storages/x                403 (same body)
POST     /storages/x/test           403 (same body)
POST     /storages/x/mount          403 (same body)
POST     /storages/x/unmount        403 (same body)
GET      /storages/x/browse         403 (same body)
GET      /storages                  200 {"items":[],"total":0,"host_policy":{"restricted":false,...}}
GET      /health                    200 {"status":"ok","version":"v0.0.0"}
```

Those seven are every storage write route registered in `internal/api/api.go:200-208` (browse is the library
path picker, gated by spec 4.6); only the read-only list stays open. In the UI the Storage page rendered
`storage-readonly`: "Read only: login is off ... turn on login in App settings before you add, test or mount a
share. Go to App settings", and the Add share button was disabled (`isDisabled() === true`).
Login was then turned on with `PUT /api/v1/settings {"login_enabled":true,"admin_username":"admin","password":...}` -> 200,
and every later UI step logs in through the real login form.

### 1. NFS and SMB added and mounted from the WebUI

UI steps (Add share dialog -> Save, then the row's Mount button), toasts as rendered:

```
nfs1: name nfs1, NFS, source 10.239.201.100:/export/media, options vers=4.1,soft,timeo=50,retrans=2
  TOASTS: ["Share “nfs1” saved."]
  TOASTS: ["Mounted at /storage/nfs1."]
  ROW: "nfs1 · NFS · 10.239.201.100:/export/media · /storage/nfs1 · Mounted · 0 libraries"
smb1: name smb1, SMB, source //10.239.201.100/media, options (empty), user smbtest, password set
  TOASTS: ["Share “smb1” saved."]
  TOASTS: ["Mounted at /storage/smb1."]
  ROW: "smb1 · SMB · //10.239.201.100/media · /storage/smb1 · Mounted · 0 libraries"
```

SMB options are empty because the app refuses `vers=3.1.1` (gap G1 below): the UI shows the field error
`the value of "vers" is not allowed` under Mount options. With no `vers` the kernel negotiated 3.1.1, as the
mount table shows.

Mount table in t75app (`grep " /storage/" /proc/mounts`):

```
10.239.201.100:/export/media /storage/nfs1 nfs4 rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,soft,proto=tcp,timeo=50,retrans=2,sec=sys,clientaddr=10.239.201.102,local_lock=none,addr=10.239.201.100 0 0
//10.239.201.100/media /storage/smb1 cifs rw,relatime,vers=3.1.1,cache=strict,upcall_target=app,username=smbtest,uid=0,noforceuid,gid=0,noforcegid,addr=10.239.201.100,file_mode=0755,dir_mode=0755,soft,nounix,serverino,mapposix,rsize=4194304,wsize=4194304,bsize=1048576,echo_interval=60,actimeo=1,closetimeo=1 0 0
```

`mount | grep /storage`:

```
10.239.201.100:/export/media on /storage/nfs1 type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,soft,proto=tcp,timeo=50,retrans=2,sec=sys,clientaddr=10.239.201.102,local_lock=none,addr=10.239.201.100)
//10.239.201.100/media on /storage/smb1 type cifs (rw,relatime,vers=3.1.1,cache=strict,upcall_target=app,username=smbtest,uid=0,noforceuid,gid=0,noforcegid,addr=10.239.201.100,file_mode=0755,dir_mode=0755,soft,nounix,serverino,mapposix,rsize=4194304,wsize=4194304,bsize=1048576,echo_interval=60,actimeo=1,closetimeo=1)
```

Mountpoints, read and write (in t75app, then on the host's export directory):

```
== read ==
$ cat /storage/nfs1/probe.txt      -> hostfile
$ cat /storage/smb1/probe.txt      -> hostfile
== write ==
$ echo "written-via-nfs-$(date +%s)" > /storage/nfs1/t75-app-nfs.txt
$ echo "written-via-smb-$(date +%s)" > /storage/smb1/t75-app-smb.txt
/storage/nfs1:
drwxrwxr-x 2 nobody nogroup 4096 Nov  6 14:30 .
-rw-rw-r-- 1 nobody nogroup    9 Nov  6 14:15 probe.txt
-rw-r--r-- 1 root   root      27 Nov  6 14:30 t75-app-nfs.txt
-rwxr--r-- 1 root   root      27 Nov  6 14:30 t75-app-smb.txt
/storage/smb1:
drwxr-xr-x 2 root root    0 Nov  6 14:30 .
-rwxr-xr-x 1 root root    9 Nov  6 14:15 probe.txt
-rwxr-xr-x 1 root root   27 Nov  6 14:30 t75-app-nfs.txt
-rwxr-xr-x 1 root root   27 Nov  6 14:30 t75-app-smb.txt
== host: /home/mb/t75export/media ==
-rw-r--r-- 1 root root   27 Nov  6 14:30 t75-app-nfs.txt
-rwxr--r-- 1 root root   27 Nov  6 14:30 t75-app-smb.txt
written-via-nfs-1793975420
written-via-smb-1793975420
```

The UI Test button on both: `TOASTS: ["Test passed for “nfs1”."]`, `TOASTS: ["Test passed for “smb1”."]`; log
`storage test passed storage=nfs1 mount_ms=0 stat_ms=0 probe_ms=27 unmount_ms=0` (already mounted, so the test
stats and writes a probe without remounting).

Secret handling, checked after the SMB mount: the SMB password appears in `ps -eo args` 0 times, and in files
under `/var/log/sys.log`, `/scratch` and `/var/lib/sys` 0 times (the pattern was assembled in a variable so the
check's own command line could not match). `/var/lib/sys/storage.key` is `-rw-------`. `/scratch` is empty after
the mount, so the credentials file was removed.

### 2. Failure paths (each shown in the UI, health 200 after each)

| Case | What was done | What the UI showed | `GET /health` after |
|---|---|---|---|
| Wrong host | share `badhost`, NFS `10.239.201.199:/export/media`, `vers=4.1,soft,timeo=10,retrans=1`, Mount | row state `Error` with `mount.nfs: Connection timed out for 10.239.201.199:/export/media on /storage/badhost` | `200 {"status":"ok"}` |
| Wrong credential | share `badcred`, SMB, user smbtest, wrong password, Mount | toast `Mounting the share failed (409, mount_failed): mount error(13): Permission denied Refer to the mount.cifs(8) manual page (e.g. man mount.cifs) and kernel log messages (dmesg)`, row `Error` with the same text | `200` |
| Option outside allowlist | Add share with options `vers=4.1,nolock` | toast `Saving the share failed (400, invalid_request): the storage settings are not valid`; under Mount options: `"nolock" is not an allowed option; allowed options are _netdev, actimeo, dir_mode, file_mode, gid, hard, iocharset, nfsvers, noatime, nodev, noexec, nosuid, port, retrans, ro, rw, sec, soft, timeo, uid, vers` | `200` |
| Shell characters | Add share with options `vers=4.1;touch /tmp/pwned` | the same toast; field error `the options cannot contain spaces or shell characters such as ; | & $ or quotes; write them as a comma separated list like soft,ro`. `ls /tmp/pwned` in t75app: `No such file or directory` | `200` |
| Share disappears | `lxc exec t75 -- systemctl stop smbd` (14:36:32) while smb1 was mounted, then Test | toast `Testing “smb1” failed: stat /storage/smb1: the share did not answer within 5s`; row `Error` with the same text. `ls /storage/smb1` in t75app: `Host is down` | `200` |
| Share comes back | `systemctl start smbd`, Test | `Test passed for “smb1”.`, row `Mounted`, `cat /storage/smb1/probe.txt` -> `hostfile` | `200` |

Other option refusals, via curl with the session cookie (all 400 `invalid_request`):
`vers=4.1,credentials=x` -> `"credentials" is not an allowed option; ...`, `vers=4.1,soft,hard` -> `soft and hard cannot be used together`,
`vers=4.1 -o exec` -> the shell characters message, `vers=4.1,credentials=/etc/shadow` -> `the options may only contain letters, digits and . _ - = ,`.

Server log for the failures (the watchdog retried the enabled broken shares and recorded mount(8)'s stderr):

```
level=WARN msg="storage mount failed" storage=badhost error="mount.nfs: Connection timed out for 10.239.201.199:/export/media on /storage/badhost"
level=WARN msg="storage watchdog remount failed" storage=badhost error="mount.nfs: Connection timed out ..."
level=WARN msg="storage watchdog remount failed" storage=badcred error="mount error(13): Permission denied\nRefer to the mount.cifs(8) manual page ..."
level=WARN msg="storage test failed" storage=smb1 error="stat /storage/smb1: the share did not answer within 5s"
```

The server stayed up through all of it: health 200 each time, and at the end `systemctl show sys` gave
`MainPID=3797 NRestarts=0 ActiveEnterTimestamp=Fri 2026-11-06 14:28:13 UTC` (before the first storage was added),
with exactly one `server listening` line in the log. Observations on
these paths are in G2 to G4 below.

### 4. Encode through a profile on the NFS share

Setup (API, session cookie): profile `T75 profile A` = duplicate of `Default (basic)`, then
`PUT /profiles/{id}` with `result_policy: keep_both` (so the source on the share is kept and each run is
comparable). Library `T75 NFS Movies`, path `/storage/nfs1/Movies`, `profile_id` = profile A -> 201,
`deprecated: []`. Built-in node started (`POST /nodes/{id}/start` 200). Source made with ffmpeg in t75app:
`ffmpeg -f lavfi -i testsrc=size=1920x1080:rate=25 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 30 -c:v mpeg4 -q:v 2 -c:a aac -ac 2 -shortest`
(6410788 bytes, `mpeg4` + `aac`), copied onto the share as `Movies/runN.mkv` before each run, then
`POST /libraries/{id}/scan` -> 202.

The job log still does not contain the ffmpeg argv (task 63 gap 1, unchanged). The command lines below were
captured from `/proc/<pid>/cmdline` of the real ffmpeg process by a 0.1 s poller in t75app
(`/var/log/ffmpeg-argv.log`), and matched with ffmpeg's own echo in the job log.

**Run 1, profile `T75 profile A` as created** (graph: `if_codec_not {codec: hevc}` -> `transcode_video {codec: hevc, quality: 3}`, policy keep_both):

```
/usr/bin/ffmpeg -hide_banner -nostdin -progress pipe:1 -nostats -y -i /scratch/job-1834029f-4144-4372-9c71-33cef6921588/source.mkv -map 0:V:0 -map 0:a:0 -c:v libx265 -crf 24 -preset medium -c:a:0 copy /scratch/job-1834029f-4144-4372-9c71-33cef6921588/output.mkv
job log: Stream #0:0 -> #0:0 (mpeg4 (native) -> hevc (libx265)) ... x265 [info]: Rate Control / qCompress : CRF-24.0 / 0.60
job 1834029f done, access_mode shared, encoder cpu, 6410788 -> 840562 bytes, output /storage/nfs1/Movies/run1.shrunk.mkv
```

**Run 2, after editing profile A** (`PUT /profiles/{id}` 200, now `if_codec_not {codec: h264}` ->
`transcode_video {codec: h264, quality: 5}`, `used_by_names: ["T75 NFS Movies"]`; the library was not touched):

```
/usr/bin/ffmpeg -hide_banner -nostdin -progress pipe:1 -nostats -y -i /scratch/job-f5a617c9-50a9-4c89-922a-1ef39489d9d3/source.mkv -map 0:V:0 -map 0:a:0 -c:v libx264 -crf 18 -preset medium -c:a:0 copy /scratch/job-f5a617c9-50a9-4c89-922a-1ef39489d9d3/output.mkv
job log: Stream #0:0 -> #0:0 (mpeg4 (native) -> h264 (libx264))
job f5a617c9 done, 6410788 -> 1539214 bytes, output /storage/nfs1/Movies/run2.shrunk.mkv
```

**Run 3, library switched to another profile** `T75 profile B` (duplicate of Default, `transcode_video {codec: hevc, quality: 1}`, keep_both),
`PUT /libraries/{id} {"profile_id": B}` 200:

```
/usr/bin/ffmpeg -hide_banner -nostdin -progress pipe:1 -nostats -y -i /scratch/job-3ece7827-0a81-4f96-b730-f0b5dbb853a7/source.mkv -map 0:V:0 -map 0:a:0 -c:v libx265 -crf 28 -preset medium -c:a:0 copy /scratch/job-3ece7827-0a81-4f96-b730-f0b5dbb853a7/output.mkv
job log: Stream #0:0 -> #0:0 (mpeg4 (native) -> hevc (libx265)) ... Rate Control / qCompress : CRF-28.0 / 0.60
job 3ece7827 done, 6410788 -> 783280 bytes, output /storage/nfs1/Movies/run3.shrunk.mkv
```

So the flow that ran followed the profile: editing the profile changed the codec and CRF (libx265 crf 24 ->
libx264 crf 18), and switching the library's profile changed it again (libx265 crf 28). The library detail
page shows `Encode profile: T75 profile B · Basic ... Only when the video is not HEVC: Encode the video as HEVC
at quality 1 of 5`. The library was created with the API, not the library form; the profile edits were also
API calls (`PUT /profiles/{id}`), not the profile editor UI.

Side effects in the same folder are recorded as G5 and G6 (keep_both outputs are rescanned as sources, and a
re-queue overwrote an earlier keep_both output). Full job list from that library:

```
created  finished id       status file                                         before  after
14:39:17 14:39:32 1834029f done   /storage/nfs1/Movies/run1.mkv                6410788 840562   run 1 (A, hevc)
14:39:49 14:39:57 79a560c7 done   /storage/nfs1/Movies/run1.shrunk.mkv         840562  1722621  run 1's output, picked up as a source
14:39:49 14:40:02 f5a617c9 done   /storage/nfs1/Movies/run2.mkv                6410788 1539214  run 2 (A edited, h264)
14:40:15 14:40:29 548d9fea done   /storage/nfs1/Movies/run1.mkv                6410788 783280   re-queued by the library switch; wrote over run1.shrunk.mkv
14:40:15 14:40:43 feafe4a9 done   /storage/nfs1/Movies/run2.mkv                6410788 783280   re-queued by the library switch; wrote over run2.shrunk.mkv
14:40:21 14:40:56 5827416b done   /storage/nfs1/Movies/run1.shrunk.shrunk.mkv  1722621 791647
14:40:21 14:40:57 852fd6a9 failed /storage/nfs1/Movies/run2.shrunk.mkv         1539214 0        "source changed since the job was queued; the next scan will re-evaluate it"
14:40:22 14:41:10 3ece7827 done   /storage/nfs1/Movies/run3.mkv                6410788 783280   run 3 (B, hevc q1)
```

### 5. Migration on a copy of a v1.10 database

v1.10 = `690cff2` (`pm: task 63 merged and logged, v1.10 wave complete`); there is no v1.10 git tag. Built with
`git worktree add /tmp/t75v110 690cff2`, the current `web/dist` copied into its embed dir (the v1.10 UI was not
used; only its API), `go build -o /tmp/shrink-v110` (exit 0). Run in t75app with its own data dir
`/var/lib/v110`, login off. Libraries created through the v1.10 API: `Movies` (easy q3), `TV` (easy q3, then
`result_policy: keep_both`), `Music videos` (easy q5 remove_subtitles audio 2, then `PUT /tier moderate`).
Stopped cleanly, copied (`sha256 be61367d...` for both original and copy), and the new binary was run on the
copy's own data dir `/var/lib/mig`.

Copy 1, before (sqlite3 on the copy):

```
schema_migrations: 16|0016_library_tier.sql
libraries: 3
profiles table: absent
name|rule_tier|result_policy|flow_len|flow_source
Movies|basic|replace_if_smaller|751|easy
Music videos|moderate|replace_if_smaller|755|easy
TV|basic|keep_both|751|easy
(Movies.flow_json = TV.flow_json: 1)
```

Boot log: `library profile created library=Movies profile=Movies`, `... library=TV profile=TV`,
`... library="Music videos" profile="Music videos"`, then the task 74 legacy-columns warning. Health 200.

Copy 1, after:

```
schema_migrations: 18|0018_profiles.sql
libraries: 3
profiles: 3
libs_without_profile: 0
library|profile|tier|result_policy
Movies|Movies|basic|replace_if_smaller
Music videos|Music videos|moderate|replace_if_smaller
TV|TV|basic|keep_both
```

Expected: TV's graph is identical to Movies but its policy differs, and `BackfillProfiles`
(`internal/store/profiles.go:178`) reuses a profile only when hash, tier and policy all match, so 3 profiles is
right and TV keeps its keep_both. To prove the dedupe itself, a fourth library `Kids` identical to Movies was
added on the v1.10 server and a second copy migrated (`/var/lib/mig2`):

```
BEFORE copy 2: schema_migrations 16|0016_library_tier.sql, libraries 4, profiles table count 0
  Kids|basic|replace_if_smaller|751   Movies|basic|replace_if_smaller|751
  Music videos|moderate|replace_if_smaller|755   TV|basic|keep_both|751
boot log: library profile created library=Movies profile=Movies / library=TV profile=TV /
          library="Music videos" profile="Music videos" / library profile assigned library=Kids profile=Movies
AFTER copy 2: schema_migrations 18|0018_profiles.sql, libraries 4, profiles 3, libs_without_profile 0
  Kids|Movies|basic|replace_if_smaller
  Movies|Movies|basic|replace_if_smaller
  Music videos|Music videos|moderate|replace_if_smaller
  TV|TV|basic|keep_both
```

Library count unchanged (3 -> 3, 4 -> 4), profile count as expected (3 and 3, with Kids deduped onto Movies),
every library has a profile (0 without), tier and policy carried. The v1.10 original was not touched:
`sha256 8558d9ea...` for both `/var/lib/v110/shrinkyourshit.db` and the copy after the run.

### Checks (make is not installed; recipes run by hand)

| Recipe | Command | Exit |
|---|---|---|
| test-go | `go test ./...` | 0 (15 packages ok) |
| test-web | `cd web && npm run test` | 0 (69 files, 524 tests) |
| lint-go | `go vet ./...` | 0 |
| lint-go | `golangci-lint run` | 0 (`0 issues.`) |
| spdx | `./scripts/check-spdx.sh` | 0 |
| lint-web | `cd web && npm run lint` (eslint) | 0 |
| lint-web | `cd web && npm run format:check` (prettier) | 0 (`All matched files use Prettier code style!`) |
| web build | `cd web && npm run build` | 0 |

### Gaps and findings (recorded, not fixed: this task's scope is docs only)

- **G1. SMB `vers=3.1.1` is refused by the option check.** `versionRE` in `internal/api/storage.go:141` is
  `^[0-9]{1,2}(?:\.[0-9]{1,2})?$`, so `3.1.1` gives `400 {"path":"options","message":"the value of \"vers\" is
  not allowed"}`. This samba (min protocol SMB3) refuses `vers=3.0` ("Dialect not supported by server", per the
  supervisor's test bed), so the only working SMB setup here is empty options; the kernel then negotiates
  `vers=3.1.1` (see the mount table). Recorded in README as a known limit. Owner call: allow three part
  versions for `vers` on SMB.
- **G2. A wrong NFS host blocks the mount call for about two minutes.** `soft,timeo=10,retrans=1` does not
  bound it; mount.nfs retries a foreground mount for its default 2 minutes (`retry` is not on the allowlist).
  The first attempt's server log: clicked about 14:30:45, `storage mount failed` at 14:32:50. A second
  Playwright run waited 240 s for the mount toast and got none (the request was still pending when the browser
  closed); the row showed `Error` with mount(8)'s message from the stored state. The server was not affected.
  Owner call: a server side deadline on mount, or `retry=0` for NFS.
- **G3. The watchdog does not detect a stale share that is still in `/proc/mounts`.** With smbd stopped at
  14:36:32, `GET /storages` still said `smb1 mounted: true, error: ''` more than two minutes later (watchdog
  interval 30 s), and nothing was logged until the manual Test at 14:36:39. Spec 4.1 asks the watchdog to catch
  "a stale mount that still looks mounted". It recovered by itself when smbd came back (CIFS reconnect), and
  Test then passed.
- **G4. The dashboard does not show a broken storage.** With `badcred` in `Error`, the dashboard said
  "All caught up." `web/src/DashboardPage.tsx:344` renders only the storage login warning. Spec 4.1 says a
  broken storage "shows as an error in the UI and on the dashboard"; it shows on the Storage page only.
- **G5. keep_both outputs are scanned as new sources.** `run1.shrunk.mkv` was queued by the next scan and
  became `run1.shrunk.shrunk.mkv`, then `run1.shrunk.shrunk.shrunk.mkv`. Every scan of a keep_both library
  re-encodes the previous outputs whenever they do not satisfy the current profile.
- **G6. A re-queue under keep_both overwrote an earlier keep_both output.** Switching the library's profile
  (`PUT /libraries/{id}` at 14:40:15) re-queued the finished `run1.mkv` and `run2.mkv`, and those jobs wrote to
  `run1.shrunk.mkv` and `run2.shrunk.mkv` again: `run2.shrunk.mkv` went from h264 1539214 bytes (run 2) to
  hevc 783280 bytes. These were files the app made, not originals, so AGENTS rule 10 was not broken, but a
  user's file in the library was replaced with no new name. The profile edit (`PUT /profiles/{id}`) did not
  re-queue finished files while the library switch did; spec 5.3 does not say which is intended.
- **G7. ffmpeg auto-detect fails when `--ffmpeg` is not set.** With ffmpeg at `/usr/bin/ffmpeg` on PATH, the
  first start logged `worker: hardware detection failed; encodes will fail until ffmpeg is usable
  err="encoders: ffmpeg  is not usable: encoders: listing ffmpeg encoders failed: exec: no command: "`. STACK.md
  says empty means auto-detect. The Docker image sets the path, so container installs are unaffected; a native
  install may not be. Every run in this task passed `--ffmpeg /usr/bin/ffmpeg`.
- **G8. The job log still has no ffmpeg argv** (task 63 gap 1). The command lines above come from
  `/proc/<pid>/cmdline`.
- **G9. Not exercised: the Docker capability set.** This VM has no Docker, so the app ran as root in a
  privileged LXD container with AppArmor mount rules, not under `cap_add: [SYS_ADMIN]` plus
  `security_opt: [apparmor=unconfined]`. The compose posture in spec 4.5 is not proven here.
- **G10. Not exercised: the NFS default options** (`vers=3,soft,timeo=50,retrans=2`). nfs1 used `vers=4.1`, the
  version the test bed proved with ganesha. Library creation and the profile edits were API calls, not the
  library form or the profile editor; storage add, mount, test and every failure path went through the WebUI.

## Checks (bookkeeping rerun, 2026-11-06)

make is not installed on this VM, so each `make` target was run by hand with the same commands.
Go test results were served from the Go test cache for unchanged packages (`(cached)`).

checks: go test ./... exit 0; go vet ./... exit 0; golangci-lint run exit 0 (0 issues); ./scripts/check-spdx.sh exit 0; web npm run test exit 0 (69 files, 524 tests); web npm run lint exit 0; web npm run format:check exit 0.
