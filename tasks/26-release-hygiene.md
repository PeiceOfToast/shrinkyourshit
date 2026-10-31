# Task 26: Release hygiene

## Goal
A release is a procedure, not a ritual: the container reports the same version, commit and build date as the binaries, the changelog is written by hand at release time, the procedure is documented, and the arm64 question has a written answer instead of a blank.

## Read first
`.github/workflows/release.yml`, `docker/Dockerfile`, `docker-compose.yml` if present, `internal/update/`, `internal/api/update.go`, `cmd/shrinkyourshit` (version wiring), `ROADMAP.md`, `README.md`.

Current state, verified: images `v0.1.0` and `v0.1.1` are published and pull anonymously, and the binaries already report version, commit and build date. The container reports `version` only, so `GET /api/v1/version` answers `commit: none` inside Docker. That gap is the first thing this task closes.

## Do

### 1. Carry the commit and the date into the image
`docker/Dockerfile`: accept `ARG VERSION`, `ARG COMMIT`, `ARG BUILD_DATE` and pass all three to `-ldflags`. Match the exact variable names the Go binary already uses for the binaries build, read them out of the Makefile or the release workflow rather than inventing new ones.

`.github/workflows/release.yml`: pass the three as `build-args` on the docker build (`VERSION` from the tag, `COMMIT` from `github.sha`, `BUILD_DATE` from a UTC timestamp produced in a step so the value is identical to the one the binaries get), and add the same three as OCI labels.

Add a smoke check to the docker job, after the build, before the push: run the image detached, poll `GET /api/v1/version` for up to 30 seconds, and fail the job unless `commit` equals `github.sha` and `build_date` is not `unknown`. Print the JSON it got on failure. This is what stops the gap from silently coming back.

### 2. `CHANGELOG.md`
Keep a Changelog format, newest first, with `## [0.1.2] - unreleased` at the top. Seed it honestly from the git log and the task files: 0.1.0 (first release, binaries failed on windows/amd64), 0.1.1 (five binaries, checksums, provenance, self update, the windows fix), and an unreleased section listing tasks 20 to 22 (44px controls, card lists on phones, fluid stat tiles, rail that collapses itself, the mobile audit at 320/390/414/768) plus whatever this wave adds. Each entry names the user-visible change, not the commit. Link the headings to `https://github.com/PeiceOfToast/shrinkyourshit/compare/...` tags.

### 3. `docs/releasing.md`
The exact procedure, copy-pasteable, in order:
1. Bump the version where it lives (find it, name it exactly).
2. Update `CHANGELOG.md` — move the unreleased entries under the new heading.
3. Run the pipeline on demand: `gh workflow run release.yml --ref main`, then `gh run watch`.
4. Confirm all five archives build, including `windows/amd64`.
5. Tag: `git tag -a vX.Y.Z -m "..."` and push the tag.
6. Verify the release page has five archives plus `checksums.txt`, download the linux archive, `sha256sum -c`, run the binary and ask it `version`.
7. Verify `ghcr.io/peiceoftoast/shrinkyourshit:X.Y.Z` and `:latest` both pull anonymously.
8. Verify the container's `/api/v1/version` reports the tag's commit, then `docker compose pull && docker compose up -d`.
9. Rollback: retag `:latest` from an earlier version tag with `docker buildx imagetools create`, and the release page can be marked pre-release rather than deleted.

Also document the ffmpeg pin maintenance: where the pinned `jellyfin-ffmpeg` deb URL and sha256 live, that the channel rolls, that a rotation breaks the build **loudly by design**, and how to bump the pair (fetch the new deb, recompute the sha, update both lines, rebuild).

### 4. The arm64 answer
Investigate and write `docs/container-architectures.md`, short and factual:
- What works today: `linux/amd64` image, and `linux/arm64` **binaries** already ship.
- Why the image is amd64 only: the pinned `jellyfin-ffmpeg` deb on the channel in use has no arm64 build. Show the URL you checked and what it returned.
- The two honest options: an arm64 image on distro `ffmpeg` (available everywhere, no Jellyfin build, so different encoder coverage and a different quality baseline), or waiting on an arm64 `jellyfin-ffmpeg`.
- Recommend one, do not implement it. The choice belongs to the owner, because it changes what encoders an arm64 user actually gets.

### 5. Same-page tidy
- `README.md`: a short Releases section stating what ships, that the image is amd64 only today, and where the changelog is.
- `ROADMAP.md`: tick nothing by hand, but add the pointer from the v1.6 section to this task file if it is not already there.

## Out of scope
Tagging a release or pushing a tag (the owner does that, not a build agent), publishing anything to a registry, arm64 image implementation, installer polish, NAS templates.

## Acceptance
- [x] `docker/Dockerfile` takes `VERSION`, `COMMIT` and `BUILD_DATE` and the image's `GET /api/v1/version` reports all three. Proven by building locally and printing the JSON in the PR body.
- [x] `release.yml` passes the three args and fails the docker job when the version endpoint reports `commit: none` or a commit that is not `github.sha`.
- [x] `CHANGELOG.md` exists with 0.1.0, 0.1.1 and an unreleased section that matches the actual git history.
- [x] `docs/releasing.md` documents the nine steps and the ffmpeg pin maintenance, with real commands and no placeholders.
- [x] `docs/container-architectures.md` states the arm64 facts with the URL that was checked, and recommends one option without implementing it.
- [x] No tag was created and nothing was published by this task.
- [x] `make test` and `make lint` are green.
