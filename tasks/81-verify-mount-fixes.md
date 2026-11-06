# Task 81: Verify the mount fixes on real hardware

Closes the loop on tasks 79 and 80. This task must run against the real NAS and a
real container, not a simulated share, because the two defects it verifies were
invisible in simulation and only appeared on real hardware
(`tasks/real-hardware-mount-evidence-2026-11-06.md`).

`DECISIONS.md` is read only.

## Read first

`tasks/real-hardware-mount-evidence-2026-11-06.md`, then tasks 79 and 80 and their
notes. The spec is `docs/design/encode-profiles-and-storage-2026-11-06.md`.

## Preconditions

- Tasks 79 and 80 are merged on `main`.
- A container image built from that commit, run with `--cap-add SYS_ADMIN` and
  `--security-opt apparmor=unconfined`, with `/storage` bind mounted.
- An NFS export reachable from the container. In this lab: TrueNAS `10.10.1.50`
  exports `/mnt/DataSSD/SSD` to `10.10.0.0/20`.
- A real SMB server. If one is not available, that acceptance box stays unticked
  and the task ends BLOCKED on that point rather than claiming it. Do not substitute
  a fake for this one.

## Do and check

1. **App-driven NFS mount, no root shell.** Create a storage from the API or the
   Storage page, press Mount, and confirm it mounts without any `docker exec -u root`
   assistance. This is the whole point of task 79. Confirm the storage row reports
   `mounted: true` and a browse of the mountpoint lists the share.
2. **Default options produce a working mount.** Create the storage with empty
   options, confirm the saved record shows `vers=4.2,soft,timeo=50,retrans=2`, and
   confirm the mount succeeds. This is task 80's defect.
3. **SMB end to end.** Mount an SMB share with a username and password, browse it,
   write a file through it, read it back, and confirm the credentials file is gone
   from the scratch directory afterwards. Then set a wrong password and confirm the
   failure is a clean `mount_failed` with a message naming the credentials problem,
   and that the server stays healthy.
4. **Restart remount.** Restart the container with the shares unmounted, and confirm
   the startup pass remounts every enabled storage with no WebUI action. Record the
   log lines.
5. **Watchdog.** With a share mounted, unmount it inside the container so the app
   still believes it should be mounted. Confirm the watchdog restores it within one
   interval, and record the log line and the elapsed time.
6. **Failure isolation.** Point a storage at a host that does not exist, confirm a
   clean `mount_failed`, and confirm `/health` stays 200 and other mounted shares
   keep serving.
7. **Encode over the app-mounted share.** Scan a library on the mounted share, run
   one job, and confirm a real byte reduction with the result written to the share.
8. **Restore the box.** Undo any test state: remove the test storages and libraries,
   and report what login or password state was left behind.

## Acceptance

- [ ] A storage mounts from the WebUI or API against the real NAS with no manual root intervention, and the evidence shows the exact request and response.
- [ ] An empty options field yields `vers=4.2,soft,timeo=50,retrans=2` and that mount succeeds.
- [ ] SMB mounts, reads and writes through a real server; the credentials file is absent afterwards; a wrong password fails cleanly.
- [ ] A container restart remounts enabled storages with no operator action, proven by log output.
- [ ] The watchdog restores a mount removed underneath it, with the log line and the elapsed time recorded.
- [ ] A bad host fails cleanly and does not disturb healthy mounts or `/health`.
- [ ] One encode over the app-mounted share shows a real byte reduction.
- [ ] Test state removed, and the final login and password state of the box stated.
- [ ] `tasks/81-notes.md` holds every command and its output, and names anything still unverified.

## Notes

If any step fails, this task stays in_progress and the notes carry the exact
failure text plus a proposed fix task number. Do not tick a box that did not pass.
