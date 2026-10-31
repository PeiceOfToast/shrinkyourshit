# Container architectures

What the project publishes today, why the container is `linux/amd64` only, and
the one option recommended to the owner. Nothing in this document is
implemented yet; it is the written answer task 26 owed.

## What works today

- **Container image:** `ghcr.io/peiceoftoast/shrinkyourshit`, published for
  `linux/amd64` only. The tags are `latest` and the semver version.
- **Binaries:** five release archives, including `linux/arm64`. An ARM64
  machine can run ShrinkYourShit from the binary archive today. Only the
  container image is missing for ARM64.

## Why the image is amd64 only

Not because the FFmpeg package is unavailable. The exact pinned
`jellyfin-ffmpeg` version also exists for arm64 on the same channel:

```text
https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb
HTTP 200, 15,555,600 bytes
```

That URL was checked on 2026-10-31, and the arm64 directory listing contains
that exact filename. The amd64 pin in `docker/Dockerfile` and the arm64 deb are
the same version from the same rolling channel.

The image is amd64 only because **it has never been built or tested for
arm64**. There is no arm64 build job, no arm64 CI runner, and no record of the
image starting and encoding on an ARM64 machine. Publishing an untested
architecture would be a claim the project cannot back, so the Dockerfile and
the README say amd64 only.

An earlier Dockerfile comment claimed the deb had no arm64 build. That was
wrong and has been corrected.

## The two honest options

### Option A — build a multi-arch image with the existing arm64 deb (recommended)

Add `linux/arm64` to the release build with `docker/build-push-action` and
`platforms: linux/amd64,linux/arm64`, and make the Dockerfile choose the
per-architecture package URL and sha256 (the arm64 `.deb` is on the same
channel). Keep the checksum check identical: a rotation still fails the build
loudly.

What must be true before publishing:

- The arm64 image builds in CI (an emulated build is enough to prove it
  assembles; a real arm64 runner is better).
- The version endpoint smoke check passes on arm64, the same one task 26 added
  for amd64.
- At least one real encode runs on an ARM64 host, and the encoder list is
  recorded. ARM64 users will not get Intel QSV; expect CPU encoders, with
  VAAPI only on boards that expose it. That is a different quality and speed
  baseline than the amd64 image and must be stated, not implied.

Cost: a longer Docker build, a larger release job, and one ARM64 machine in
the test loop. No new dependency and no change to the amd64 image.

### Option B — stay amd64 only

Keep the container `linux/amd64` and point ARM64 users at the
`linux/arm64` binary archive, which already ships. Zero new risk, no ARM64
test host needed, and no second architecture to keep verified. Cost: Docker
users on a Raspberry Pi, an ARM NAS or Apple Silicon cannot use the image
without emulation, which is exactly the audience most likely to want a
small, low-power node.

A third path exists but is worse: switch the arm64 image to the distro
`ffmpeg`. It is available everywhere, but it has a different encoder set and a
different quality baseline from jellyfin-ffmpeg, so amd64 and arm64 users would
silently get different output. Do not do this while the jellyfin-ffmpeg arm64
deb exists.

## Recommendation

Choose **Option A**: build the arm64 image with the existing pinned
jellyfin-ffmpeg arm64 deb, and publish it only after the arm64 version smoke
check and one real ARM64 encode pass. It keeps one image definition, one
encoder stack and one quality baseline across architectures.

This task does not implement it. The choice changes what encoders an ARM64
user actually gets, so it belongs to the owner.
