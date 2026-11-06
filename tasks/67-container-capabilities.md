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
- [x] A container built from the new Dockerfile has both mount helpers present, proven by
      `docker run --rm <image> sh -c 'command -v mount.nfs mount.cifs'`. Proof: CI run
      https://github.com/PeiceOfToast/shrinkyourshit/actions/runs/42165909121, docker job (amd64), step
      "Network storage helpers and capability report (task 67)". arm64 helper presence is not checked in CI (see notes).
- [x] With `cap_add` removed, the server starts and answers health (proven by the `storage-missing` CI step in run
      42165909121). The Storage page explanation of the missing capability is task 68.
- [x] README and the installer both name the capability and the login gate.
- [x] `make test` and the docker build job green. Proof: docker job green in run 42165909121; `go test -count=1 ./...`
      and `make test-web` exit 0 on the build box (see notes).

## Notes
Write the plan, the image size delta and the exact error text in `tasks/67-notes.md`.
