# Task 28 notes — Multi-arch container image

## Plan

Files to touch, in order:

1. `docker/Dockerfile` — `ARG TARGETARCH` + two URL/sha256 ARG pairs, a
   `case` selection in the runtime RUN; unknown arch fails naming the arch.
   Arm64 sha256 computed by downloading the deb twice (stable across
   downloads): `9de240f98cc49db7ebc649c72c37d3ef154170d37ec2fbc68483cc84744804bb`.
2. `.github/workflows/release.yml` — setup-qemu + setup-buildx, then
   platform-split builds (amd64 load → existing amd64 smoke check, arm64
   load → arm64 boot smoke test), then the multi-platform build+push and an
   `imagetools inspect` gate that both platforms are in the manifest.
3. `docs/container-architectures.md` — rewrite what-works-today and options.
4. `README.md` — image line: amd64 and arm64.
5. `CHANGELOG.md` — `[0.1.3] - unreleased` entry.
6. `docs/releasing.md` — step 7 manifest check; pin-maintenance section now
   covers both pins.

## Constraint found while planning

`docker/build-push-action` cannot `load: true` a multi-platform build with
the default docker driver, so the workflow builds each platform
individually with `load: true` (arm64 per the task's smoke-test snippet,
amd64 for the existing version smoke check) and only the final build+push
step assembles both platforms into one manifest list. `imagetools inspect`
needs a registry, so it is gated on `push` (release/tag), matching where
the docs put the check.

## Local proof status

No docker daemon on this machine (no `docker` binary, no sudo to install
one), so local buildx proof is impossible. Proof is delegated to CI:
`gh workflow run release.yml --ref task/28-multiarch-image` is the dry run
(builds and smoke-tests both platforms, publishes nothing). Acceptance
boxes are ticked only against that run's output.

## BuildKit note

Verified in buildkit source (`dockerfile2llb/platform.go`, `defaultArgs`):
`--build-arg TARGETARCH=riscv64` overrides the auto-derived TARGETARCH, so
the unsupported-arch acceptance proof works as written.

## Summary

(Built:) Dockerfile picks the pinned jellyfin-ffmpeg deb per TARGETARCH
with both digests checked in; release.yml proves amd64 and arm64 (qemu
boot smoke test) before publishing and verifies the manifest. (Skipped:)
local docker build/run proof — no docker daemon here; CI dry run is the
proof. (Next task should know:) the release dry run is the place to read
arm64 boot logs; `imagetools inspect` only runs on real tag pushes; task
30 still owns recording real ARM64 encoders.
