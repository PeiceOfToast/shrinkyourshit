# Task 67: Container packages, capabilities and installer notes

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.2, 4.5, 4.6. `docker/Dockerfile`, `docker/compose.example.yml`, `docker/entrypoint.sh`,
the installer scripts under `scripts/`, and `README.md`.

## Scope
`docker/Dockerfile`, `docker/compose.example.yml`, `docker/entrypoint.sh`, installer scripts,
`README.md`, `docs/` install documentation, `STACK.md` if it names the runtime packages.

## Do
1. Install `nfs-common` and `cifs-utils` in the runtime stage with the same apt pattern already
   used for the pinned ffmpeg packages, and confirm the image size delta is stated in the notes.
2. Create `/storage` and add it to `VOLUME`.
3. Compose: `cap_add: [SYS_ADMIN]` and `security_opt: [apparmor=unconfined]`, each with a comment
   saying what it buys and that it is narrower than `privileged`.
4. Entrypoint: when `SHRINK_MANAGE_MOUNTS` is true, verify `mount.nfs` and `mount.cifs` exist and
   that `/proc/self/status` shows the capability; log one clear line either way. Never exit.
5. Installer and README: state the requirement, show the compose lines, and give the error the
   user sees when they copied an old compose file. Include the login requirement from spec 4.6.

## Acceptance
- [ ] A container built from the new Dockerfile has both mount helpers present, proven by
      `docker run --rm <image> sh -c 'command -v mount.nfs mount.cifs'`.
- [ ] With `cap_add` removed, the server starts and the Storage page explains the missing
      capability instead of failing to boot.
- [ ] README and the installer both name the capability and the login gate.
- [ ] `make test` and the docker build job green.

## Notes
Write the plan, the image size delta and the exact error text in `tasks/67-notes.md`.
