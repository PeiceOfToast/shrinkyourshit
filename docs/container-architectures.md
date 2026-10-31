# Container architectures

What the project publishes today, how the multi-arch image is built, and
what is still not proven. This is the written answer task 26 owed, updated
by task 28 which implemented its recommended option.

## What works today

- **Container image:** `ghcr.io/peiceoftoast/shrinkyourshit`, published for
  `linux/amd64` and `linux/arm64` as one manifest list, tagged `latest` and
  the semver version. Built by `docker/build-push-action` in the release
  workflow.
- **Binaries:** five release archives, including `linux/arm64`.
- **Build proof:** the release pipeline builds each platform and proves the
  arm64 image, not just assembles it: the arm64 image is loaded onto the
  runner, booted under qemu emulation, must answer `GET /api/v1/health`,
  and its bundled jellyfin-ffmpeg must run — before the manifest list is
  pushed. The amd64 image keeps the version-endpoint smoke check that
  verifies `commit` and `build_date` match the build identity.
- **Per-architecture pins:** `docker/Dockerfile` selects the pinned
  jellyfin-ffmpeg deb by `TARGETARCH`. Both debs are the same version from
  the same rolling channel, each with a checked-in sha256:
  - amd64: `jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb`,
    sha256 `0cf62bc2423822c9ec7a38dbc8f526d9a58671bd01843daa7817ece35619fc1c`
  - arm64: `jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb`,
    sha256 `9de240f98cc49db7ebc649c72c37d3ef154170d37ec2fbc68483cc84744804bb`
  A rotation on either channel fails the build loudly. An architecture
  other than amd64/arm64 fails the build with a message naming it.

## The arm64 package question (investigation notes)

The image was amd64-only for releases v0.1.0–v0.1.2 because nobody had
built or booted an arm64 image, **not** because the package is missing.
The exact pinned `jellyfin-ffmpeg` version also exists for arm64 on the
same channel:

```text
https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb
HTTP 200, 15,555,600 bytes
```

That URL was checked on 2026-10-31, and the arm64 directory listing
contains that exact filename. The amd64 pin in `docker/Dockerfile` and the
arm64 deb are the same version from the same rolling channel. An earlier
Dockerfile comment claimed the deb had no arm64 build; that was wrong and
was corrected before this task.

## How the image is built and proven

The release workflow (`release.yml`, docker job) does this in order:

1. `docker/setup-qemu-action` and `docker/setup-buildx-action` install qemu
   and a buildx builder.
2. The amd64 image is built and loaded; the version-endpoint smoke check
   runs against it (`/api/v1/version` must report the release commit and a
   real build date).
3. The arm64 image is built (the web and Go build stages run under
   emulation for this platform) and loaded.
4. The arm64 image is booted under qemu with a published port, polled on
   `/api/v1/health`, and its bundled
   `/usr/lib/jellyfin-ffmpeg/ffmpeg -version` is executed inside the
   container. Failure here stops the release.
5. One build assembles `linux/amd64,linux/arm64` and pushes the manifest
   list to a local registry on the runner (nothing is published externally),
   and `docker buildx imagetools inspect` must list both platforms — this
   runs on the dry run too.
6. On a real tag push the manifest list is pushed to GHCR, and the same
   inspect check runs against the published tag.

## What is still not proven

- **A real encode on ARM64 hardware.** An emulated boot proves the image
  starts and the binary is arm64; it does not prove encoder behavior on a
  real ARM64 machine. That proof belongs on the owner's ARM64 host (or
  task 30, which owns VAAPI/VideoToolbox work). Until it is recorded:
  **ARM64 users get CPU encoders** — no Intel QSV, and VAAPI only on
  boards that expose it. That is a different quality and speed baseline
  than amd64 and must be stated, not implied.
- Emulated builds are slow; a native arm64 runner would be faster but is
  not required.

## The alternative that was considered

Before this task the options were "build a multi-arch image with the
existing arm64 deb" (chosen, implemented) or "stay amd64 only and point
ARM64 users at the binary archive". A third path — switching the arm64
image to the distro `ffmpeg` — was rejected and still is: it has a
different encoder set and a different quality baseline, so amd64 and
arm64 users would silently get different output. Do not do this while the
jellyfin-ffmpeg arm64 deb exists.
