# Task 75: Verify profiles and storage on real hardware

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
The whole spec. Verification is evidence, not a self report.

## Scope
`tasks/75-notes.md`, `README.md`, `ARCHITECTURE.md`, `API.md`, `ROADMAP.md`, `tasks/STATUS.md`.

## Do
1. Mount a real NFS share and a real SMB share from the WebUI against a live target on this lab,
   capture the `mount` table and the mountpoints, and prove read and write (write only where the
   share is not read only).
2. Prove the failure paths: a wrong host, a wrong credential, an option outside the allowlist,
   and a share that disappears, each showing the error in the UI and none of them taking the
   server down.
3. Prove the auth gate: with login disabled, every write endpoint returns 403.
4. Create a library on a mounted share, encode one real file through it with a profile, and prove
   the flow that ran came from the profile by changing the profile and re-running.
5. Prove the migration on a copy of a v1.10 database: library count unchanged, profile count as
   expected, every library has a profile.
6. Docs, ROADMAP and STATUS updated.

## Acceptance
- [ ] The mount table and both mountpoints are quoted in the notes with their real output.
- [ ] The encode proof includes the ffmpeg command line and the profile it came from.
- [ ] The migration proof includes row counts before and after.
- [ ] `make test` and `make lint` green.

## Notes
`tasks/75-notes.md` is the evidence file for this wave.
