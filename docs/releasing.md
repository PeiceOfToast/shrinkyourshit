# Releasing

The exact procedure for cutting a ShrinkYourShit release. Copy the commands,
run them in order, and stop at the first failure.

- Release workflow: `.github/workflows/release.yml`
- Container image: `ghcr.io/peiceoftoast/shrinkyourshit`
- Changelog: `CHANGELOG.md`
- Container architectures: `docs/container-architectures.md`

Nothing here is automated by an agent. AGENTS.md forbids build agents from
creating tags or publishing releases; the owner runs this list.

## 1. Bump the version where it lives

The version is **not stored in a source file**. There is no version constant to
edit, and `web/package.json` (`"version": "0.0.0"`) is not used for releases.
The only source is the git tag:

- `Makefile`: `VERSION ?= $(shell git describe --tags --always --dirty ...)`
  feeds local builds.
- `.github/workflows/release.yml`: the `prepare` job exports
  `version=${GITHUB_REF_NAME}` (the tag name) and passes it to both the
  binaries and the image.

So "bumping the version" means choosing the next semantic version tag, for
example `v0.1.2`, and using that exact string in steps 2 and 5. No commit is
needed for this step.

## 2. Update `CHANGELOG.md`

Move the entries under `## [0.1.2] - unreleased` to a new heading for the
version being released, with the release date:

```markdown
## [0.1.2] - 2026-11-01
```

Leave a fresh `## [0.1.3] - unreleased` section at the top, and update the
comparison links at the bottom:

```markdown
[0.1.3]: https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.1...v0.1.2
```

Commit that with `git commit -m "release: v0.1.2"` and push to `main`. Each
entry must name what a user sees change, not the commit title.

## 3. Run the pipeline on demand

`workflow_dispatch` is a **dry run**: it builds and verifies everything but
uploads no release assets and pushes no image. It does run the Docker build and
the `/api/v1/version` smoke check, so a broken image fails here.

```sh
gh workflow run release.yml --ref main
gh run watch
```

## 4. Confirm all five archives build

The `binaries` job must be green for every target:

```text
linux/amd64  linux/arm64  windows/amd64  darwin/amd64  darwin/arm64
```

The Windows build is not optional. v0.1.0 shipped without archives because
`windows/amd64` failed to compile after the image had already been built;
`go test` and the `cross` CI job now catch that, but check it here too.

## 5. Tag

An annotated tag is the release trigger:

```sh
git tag -a v0.1.2 -m "ShrinkYourShit v0.1.2"
git push origin v0.1.2
```

The tag push runs the same pipeline with publishing enabled: assets are
attached to the GitHub release and both image tags are pushed.

## 6. Verify the GitHub release

The release page must carry five archives plus `checksums.txt`. Download the
Linux archive, verify it and run it:

```sh
tmp="$(mktemp -d)"
gh release download v0.1.2 \
  --pattern 'shrinkyourshit-linux-amd64.tar.gz' \
  --pattern 'checksums.txt' \
  --dir "$tmp"
cd "$tmp"
sha256sum -c checksums.txt --ignore-missing
tar -xzf shrinkyourshit-linux-amd64.tar.gz
./shrinkyourshit-linux-amd64 version
```

`version` must print `v0.1.2`, a real commit, a UTC build date and
`runtime: binary`.

## 7. Verify both image tags pull anonymously

Log out first, so a cached credential cannot hide a private package:

```sh
docker logout ghcr.io
docker pull ghcr.io/peiceoftoast/shrinkyourshit:0.1.2
docker pull ghcr.io/peiceoftoast/shrinkyourshit:latest
```

Both must succeed without a login. `0.1.2` is the semver tag without the `v`;
that is what `docker/metadata-action` produces. If the package was recreated,
one manual step is needed once: package settings -> visibility public ->
inherited repository access (see the note at the end of `release.yml`).

Then verify the manifest list carries both architectures:

```sh
docker buildx imagetools inspect ghcr.io/peiceoftoast/shrinkyourshit:latest
```

The output must list `linux/amd64` **and** `linux/arm64` under
`Manifests`. The release workflow checks the same thing right after the
push; if a platform is missing, rebuild before announcing the release.

## 8. Verify the container's build identity, then update

The image must report the tag's full commit, not `none`:

```sh
git rev-parse v0.1.2
curl -fsS http://localhost:8765/api/v1/version | python3 -m json.tool
```

`commit` must equal `git rev-parse v0.1.2`, `version` must be `v0.1.2`, and
`date` must be a timestamp, not `unknown`. If `commit` is `none`, the image was
built without the `COMMIT` build arg: do not announce the release, rebuild.

Then update a real install:

```sh
docker compose pull
docker compose up -d
```

## 9. Rollback

The image tags are the rollback point. Retag `latest` from an earlier published
version; do not delete the release page unless it is actively harmful. Mark it
as a pre-release instead, so nobody installs it by accident.

```sh
docker buildx imagetools create \
  --tag ghcr.io/peiceoftoast/shrinkyourshit:latest \
  ghcr.io/peiceoftoast/shrinkyourshit:0.1.1
docker buildx imagetools inspect ghcr.io/peiceoftoast/shrinkyourshit:latest
```

For a binary install, `shrinkyourshit update --rollback` restores the previous
binary that self update kept as `shrinkyourshit.prev`.

## FFmpeg pin maintenance

The runtime image installs a specific `jellyfin-ffmpeg` `.deb` instead of an
apt package, so the build is reproducible and covers NVENC, QSV and VAAPI
(decision B8). The pin lives in `docker/Dockerfile`, runtime stage, as two
ARG pairs, one per architecture:

- amd64 URL:
  `https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/amd64/jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb`
  sha256 `0cf62bc2423822c9ec7a38dbc8f526d9a58671bd01843daa7817ece35619fc1c`
- arm64 URL:
  `https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb`
  sha256 `9de240f98cc49db7ebc649c72c37d3ef154170d37ec2fbc68483cc84744804bb`

`latest-7.x` is a rolling channel. Jellyfin can rotate or remove the file at
any time. That is intentional: a missing file makes `curl -fsSL` fail, and a
replaced file fails `sha256sum -c`. **The build fails loudly by design.**
Never remove the checksum to get a build through, and never switch to
`latest`; a silent FFmpeg change would change every user's encode output.

To bump the pair, list the channel, fetch the new file and hash it (replace
the filename below with the new one the listing prints), for each
architecture:

```sh
for arch in amd64 arm64; do
  curl -fsS "https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/$arch/" \
    | grep -o "jellyfin-ffmpeg7_[^\"]*_bookworm_$arch.deb" | sort -u
  curl -fsSLO "https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/$arch/<new filename>"
  sha256sum <new filename>
done
```

Update both URL and digest pairs together in `docker/Dockerfile` — and bump
both architectures in the same commit, because the image must keep shipping
one ffmpeg version everywhere — then prove it:

```sh
docker build -f docker/Dockerfile \
  --build-arg VERSION=v0.0.0-test \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t shrinkyourshit:ffmpeg-pin .
docker run --rm --entrypoint /usr/lib/jellyfin-ffmpeg/ffmpeg shrinkyourshit:ffmpeg-pin -version
docker buildx build -f docker/Dockerfile --platform linux/arm64 \
  --build-arg VERSION=v0.0.0-test \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t shrinkyourshit:ffmpeg-pin-arm64 .
```

On a machine without Docker, push the branch and let the `docker` job in
`.github/workflows/ci.yml` build it. Both the URL and the digest belong in the
same commit as the changelog entry that mentions the bump.

The same channel also publishes an arm64 deb with the same version and
filename suffix; that is exactly the arm64 pin already in the Dockerfile,
so both architectures are bumped the same way (URL plus sha256). See
`docs/container-architectures.md`.
