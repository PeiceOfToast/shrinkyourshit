# Task 26 notes — Release hygiene

## Plan

Files to touch, in order:

1. `tasks/STATUS.md` — in_progress. Branch `task/26-release-hygiene`.
2. `docker/Dockerfile` — `ARG VERSION`, `ARG COMMIT`, `ARG BUILD_DATE` in the Go
   build stage, passed as `-X main.version` / `main.commit` / `main.date` (the
   exact names the binary already uses in `cmd/shrinkyourshit/main.go`). Fix the
   stale amd64 comment: the arm64 jellyfin-ffmpeg deb exists.
3. `scripts/release.sh` — accept `COMMIT` and `BUILD_DATE` from the environment
   so the workflow can hand the binaries and the image the same values. Use the
   full commit SHA, matching the image's `COMMIT` build arg.
4. `Makefile` — `make docker` also passes `COMMIT` and `BUILD_DATE`.
5. `.github/workflows/release.yml` — a `prepare` job owns version, commit and
   UTC build date for the whole run; binaries receive `COMMIT`/`BUILD_DATE`;
   the image receives all three as build args and OCI labels. Build with
   `load: true` and `push: false`, smoke test `/api/v1/version` against
   `github.sha`, then push, so a bad image can never reach the registry.
6. `.github/workflows/ci.yml` — the docker job builds with the same three args
   and runs the same smoke check on every PR. Docker is unavailable on the dev
   VM, so this job is the proof for acceptance box 1.
7. `internal/update/release_hygiene_test.go` — file-level tests for the
   Dockerfile args, both workflows, the changelog, the release procedure and
   the arm64 document (one test per acceptance item that can be checked
   statically; the docker jobs are the executable proof for the image itself).
8. `CHANGELOG.md` — Keep a Changelog, newest first, honest against the git log:
   0.1.0 (MVP), 0.1.1 (tasks 10 to 20, release engineering, windows fix),
   0.1.2 unreleased (21 to 26). Task 20 shipped in v0.1.1 (tag `v0.1.1` is
   after commit `ce04748`), so it is listed there, not under unreleased — the
   task file's seed text says 20 to 22, the git log says otherwise.
9. `docs/releasing.md` — the nine steps, including that the version lives only
   in the tag, that `workflow_dispatch` is a dry run and a tag publishes, and
   the jellyfin-ffmpeg pin maintenance (URL + sha256, rolling channel, loud
   failure by design, bump procedure).
10. `docs/container-architectures.md` — verified arm64 facts with the URL that
    was checked, the real reason the image is amd64-only, and one recommended
    option for the owner (no implementation).
11. `README.md` — a short Releases section. `ROADMAP.md` already points at this
    task file, so no change there.

Commands the task requires: `make test`, `make lint`, `git push`, `gh pr create`.
No tag and no registry push.
