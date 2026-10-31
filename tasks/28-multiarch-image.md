# Task 28: Multi-arch container image (linux/amd64 and linux/arm64)

## Goal
The published image runs on ARM, not just x86. Option A from `docs/container-architectures.md`, implemented and proven by an actual arm64 boot in CI.

## Read first
`docs/container-architectures.md` (the written answer this task implements), `docker/Dockerfile` (the pinned `jellyfin-ffmpeg` deb and the three build args), `.github/workflows/release.yml` (prepare, binaries, docker jobs), `docs/releasing.md`.

The image is amd64 only today because nobody ever built or booted an arm64 image, **not** because the package is missing: `https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb` returns 200 at the same version as the amd64 pin. The arm64 release binaries already ship.

## Do

### 1. `docker/Dockerfile`: pick the pin by target arch
- `ARG TARGETARCH` in the runtime stage.
- Two `ARG`s for the deb URL and sha256, amd64 and arm64, and one selection rule: `amd64` and `arm64` are supported, anything else fails the build with a message naming the arch. Do not default silently to amd64.
- Compute the arm64 sha256 yourself and check it into the file, do not leave a placeholder: download the .deb, `sha256sum`, paste. Both digests stay pinned exactly as they are now, because a rolling channel that silently changes FFmpeg would change every user's output.
- Keep everything else identical: same ffmpeg path (`/usr/lib/jellyfin-ffmpeg/ffmpeg`), same ldflags, same build args for `VERSION`, `COMMIT`, `BUILD_DATE`.

### 2. `release.yml`: build and push both platforms
- `docker/setup-qemu-action` and `docker/setup-buildx-action` before the build.
- `docker/build-push-action` with `platforms: linux/amd64,linux/arm64`.
- **Proof, not assumption.** Add a step after the build, before the push, that loads the arm64 image and runs it under emulation:

```yaml
- name: Smoke test the arm64 image
  run: |
    docker run --rm --platform linux/arm64 -d --name sys-arm64 \
      -e PUID=1000 -e PGID=1000 -v /tmp/armcfg:/config <image>
    for i in $(seq 1 30); do
      if curl -fsS http://127.0.0.1:8765/api/v1/health; then ok=1; break; fi
      sleep 2
    done
    docker logs sys-arm64 | tail -50
    test "${ok:-0}" = "1" || { docker rm -f sys-arm64; exit 1; }
    docker exec sys-arm64 /usr/lib/jellyfin-ffmpeg/ffmpeg -hide_banner -version | head -2
    docker rm -f sys-arm64
```
Publish the port mapping the app actually uses. This step is the whole point of the task: an image that boots and answers `/api/v1/health` under arm64 emulation. Keep the existing version-endpoint smoke check for amd64.

### 3. Docs
- `docs/container-architectures.md`: rewrite the "what works today" and "options" sections to describe what is now true, keep the investigation notes, and move Option A from recommendation to implemented. Keep the AMF/VideoToolbox facts out of it, they are a different document.
- `README.md`: the image line says amd64 **and** arm64, with the one caveat if any.
- `CHANGELOG.md`: under `[0.1.3] - unreleased`, an entry naming the user-visible change.
- `docs/releasing.md`: step 7 gains a check that both platforms are present in the published manifest:
  `docker buildx imagetools inspect ghcr.io/peiceoftoast/shrinkyourshit:latest` must list `linux/amd64` and `linux/arm64`.

### 4. Do not
- Do not change the ffmpeg version or the channel.
- Do not add a third architecture.
- Do not weaken the existing version-endpoint smoke check.

## Out of scope
Regenerating the release binaries (they already cover arm64), a native arm64 runner, Windows containers, tagging a release (the owner does that).

## Acceptance
- [x] `docker buildx build --platform linux/amd64,linux/arm64` succeeds locally or in CI, and both digests appear in `docker buildx imagetools inspect` of the locally built tag. — CI run 41222785022 (PR #29), `docker-arm64` job: multi-platform build pushed to the local proof registry; `imagetools inspect` printed `Platform: linux/amd64` and `Platform: linux/arm64`.
- [x] The arm64 image starts under emulation, answers `GET /api/v1/health`, and its bundled ffmpeg runs. Paste the health JSON and the ffmpeg banner from the CI log into the PR body. — Same run, `Smoke test the arm64 image (boot under emulation)` step: `{"status":"ok","version":"v0.0.0-ci"}`, `ffmpeg version 7.1.4-Jellyfin ...`, `built with gcc 12 (Debian 12.2.0-14)`; pasted into the PR body.
- [x] The amd64 image still reports `commit` and `build_date` from the build args, and the existing smoke check still passes. — Same run, `docker` job `Image reports version, commit and build date` step green; the version-endpoint check is unchanged in `release.yml` and `ci.yml`.
- [x] An unsupported `TARGETARCH` fails the Docker build with a message naming the arch. Proven by building with `--build-arg TARGETARCH=riscv64` and pasting the failure. — Same run, `docker` job `Unsupported TARGETARCH fails the build` step: build failed with exit 1 and the step's grep gate confirmed the message names `riscv64`; pasted into the PR body.
- [x] Both sha256 digests for the two debs are in `docker/Dockerfile` and match a fresh `sha256sum` of each downloaded file. — Both debs downloaded and hashed twice locally (amd64 `0cf62bc2…`, arm64 `9de240f9…`, stable), digests checked into the Dockerfile, and each build runs `sha256sum -c -` on the downloaded deb (green in both the `docker` and `docker-arm64` jobs).
- [x] `docs/container-architectures.md`, `README.md`, `CHANGELOG.md` and `docs/releasing.md` describe the new reality. — All four updated on this branch; the doc-assertion tests in `internal/update/release_hygiene_test.go` pin the new facts and pass.
- [x] `make test` and `make lint` are green. — Run locally on the dev VM: `go test ./...` (unit + integration) and the web vitest suite pass, `make lint` (go vet, golangci-lint, eslint, prettier) passes.
