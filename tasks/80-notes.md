# Task 80 notes: NFS default becomes vers=4.2

## Plan (as run)

1. `internal/api/storage.go`: change `nfsDefaultOptions`, add the rpc.statd reason to the comment.
2. Go tests: `internal/api/storage_test.go` (exact literal default, allowlist accepts 4.2 and 4.1), `internal/mount/mount_test.go` (fixtures and expected mount vectors move to 4.2; the explicit `vers=3` vector test stays).
3. Web: `web/src/storageModel.ts` (`NFS_DEFAULT_OPTIONS`), the form help text in `web/src/StoragePage.tsx`, a new web test, and the fixtures in `EasyLibraryForm.test.tsx`, `StorageBrowser.test.tsx`, `storageModel.test.ts`, `e2e/storage.spec.ts`, `e2e/library.spec.ts`.
4. Docs that state the default: `API.md`, `docs/openapi.yaml`, `docs/design/encode-profiles-and-storage-2026-11-06.md` section 4.4.
5. Run the gates, record the grep, tick the checklist.

## Before and after

| Place | Before | After |
|-------|--------|-------|
| `internal/api/storage.go` `nfsDefaultOptions` | `vers=3,soft,timeo=50,retrans=2` | `vers=4.2,soft,timeo=50,retrans=2` |
| `web/src/storageModel.ts` `NFS_DEFAULT_OPTIONS` | `vers=3,soft,timeo=50,retrans=2` | `vers=4.2,soft,timeo=50,retrans=2` |

The task said the spec section would describe the v3 default. Section 4.4 now gives the 4.2 default and quotes the observed error `mount.nfs: rpc.statd is not running but is required for remote locking`.

## Why 4.2

NFSv3 locking needs `rpc.statd` on the client. The shipped container runs neither rpcbind nor statd, so a `vers=3` mount fails there. Observed on 2026-11-06 against the real NAS (`10.10.1.50:/mnt/DataSSD/SSD`): v3 fails, v4.2 mounts (see `tasks/real-hardware-mount-evidence-2026-11-06.md`, bug 2). `soft` is kept for the reason in the spec.

## Option allowlist

`versionRE` is `^[0-9]{1,2}(?:\.[0-9]{1,2})?$`. It accepts `4.2`, `4.1`, `3` and `3.0`, so no change was needed for this task. `vers=3` still passes validation when set by an operator. Covered by `TestStorageOptionsAllowlist` (the valid list has `vers=3,soft,...` and the new `vers=4.2,soft,...` and `vers=4.1,hard`) and by the mount vector test `TestBuildMountSpecNFSReadOnlyAddsRoFlag`, which keeps `vers=3` as the data string.

## Tests that pin the default

- `internal/api` `TestStorageCRUDWithLogin` (`storage_test.go`, the create step): asserts the response options and the stored row equal the literal `vers=4.2,soft,timeo=50,retrans=2`. It is a literal, not the constant, so it fails if the constant is changed back.
- `internal/mount` `TestMountRunsExactSpecNFS`: the fake kernel receives `MountSpec{Fstype: "nfs", Source: "nas:/export", Target: ..., Data: "vers=4.2,soft,timeo=50,retrans=2"}`. The chain is therefore: the API saves the literal default (first test), and the executor passes that stored string to `mount(2)` unchanged (this test). The two tests share the literal string. No single test runs both halves, because the API tests use a fake mounter and the mount tests use a fake kernel.
- `web/src/StoragePage.test.tsx` `prefills vers=4.2 and warns that vers=3 needs rpc.statd (task 80)`: the prefilled value, and the help text containing `vers=3 is allowed`, `rpc.statd` and `"rpc.statd is not running"`.
- `web/src/storageModel.test.ts`: `blankStorageForm` starts with the new default.
- `web/e2e/storage.spec.ts` `add a share, test it, then mount it`: the prefilled options at phone, tablet and desktop widths.

## Repo-wide grep

Command: `grep -rn "vers=3" . --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist` after the change, with `vers=3.0` and the history notes removed. Output, in full:

```
ROADMAP.md:201    ... SMB `vers=3.1.1` is refused by the option        (SMB, not the NFS default)
ROADMAP.md:218    - The NFS default options need a daemon ... `vers=3`   (describes the defect, status list, owner-maintained)
API.md:768        ... An empty NFS `options` becomes `vers=4.2,...` (task 80: ... `vers=3` is still accepted when set explicitly).
tasks/80-nfs-version-default.md  (the task spec itself)
tasks/real-hardware-mount-evidence-2026-11-06.md  (evidence)
tasks/75-notes.md  (G1 and G10 history)
docs/design/encode-profiles-and-storage-2026-11-06.md:110,112  (explains why v3 is not the default)
web/src/StoragePage.test.tsx:286,294,295   (new test)
web/src/StoragePage.tsx:571   (help text: "vers=3 is allowed, but it needs rpc.statd")
internal/mount/mount_test.go:281,284,1006  (explicit vers=3 vector and a shell-injection case)
internal/api/storage_test.go:219  (valid list, explicit vers=3)
internal/api/storage.go:175,177  (comment: why v4.2 and that vers=3 stays allowed)
```

Grep for the old default string `vers=3,soft` after the change: the only hits are history notes (`tasks/65-notes.md`, `tasks/66-notes.md`, `tasks/68-notes.md`, `tasks/75-notes.md`, the evidence file, and this task's spec), which record past state and are not edited. No test fixture, i18n string or UI copy still shows `vers=3` as the default.

Left as history, not edited: `tasks/65-notes.md`, `tasks/66-notes.md`, `tasks/68-notes.md`, `tasks/75-notes.md`, `tasks/real-hardware-mount-evidence-2026-11-06.md`. `ROADMAP.md` line 218 describes the defect and says "Fix: task 80". That is an owner-maintained status list, so it was not changed here. The owner can strike it.

## Gates

- `go test -count=1 -timeout 40m ./...`: all packages ok (`internal/api` 463.6s, `internal/server` 438.2s, `internal/store` 220.0s, `internal/mount` 0.34s, the rest ok). Run in the background because the api and server packages take about 7 to 8 minutes each.
- `npm run test` (web): 69 files, 525 tests, all passed (was 524 before task 80; +1 new test).
- `make lint` (`lint-go`, `spdx`, `lint-web`): exit 0. `go vet ./...`, golangci-lint 0 issues, `check-spdx.sh` exit 0, eslint `--max-warnings 0`, prettier check passed.
- `gofmt -l internal/api internal/mount` lists `internal/api/installer_test.go`. That file is not in this diff; it predates task 80 and was not touched.
- Playwright: `e2e/storage.spec.ts` and `e2e/library.spec.ts` with the UI rebuilt by `make build`: 18 passed, 0 failed (phone, tablet, desktop).
- The first Playwright run used a stale embedded bundle (the `make` binary was not on the default PATH, so `make web` had not run). It failed on the prefilled `vers=3` value as expected. After `make build` it passed. Recorded here because it is why the build step matters for e2e.
- `DECISIONS.md` unchanged.
- Live check on the real NAS: not run here. The lab box is the owner's, and task 81 covers real-hardware verification.

## G1 recommendation (not built)

The SMB `vers=3.1.1` rejection is G1 in `tasks/75-notes.md`. The fix is one line in `internal/api/storage.go`: change `versionRE` to `^[0-9]{1,2}(?:\.[0-9]{1,2}){0,2}$`. That accepts `3.1.1`, and it stays a single allowlist pattern shared by `vers` and `nfsvers`. It needs a test for `vers=3.1.1` accepted and `vers=3.1.1.1` refused. This task does not change it, because the task asked for it to be recorded here. Recommend it as its own small task, since it also changes what `nfsvers` accepts. The owner should decide whether SMB 3.1.1 is in scope for v1.12.

## Summary (5 lines)

1. `nfsDefaultOptions` and the web default are now `vers=4.2,soft,timeo=50,retrans=2`; the spec, API.md and OpenAPI text match.
2. The NFS help text warns that `vers=3` needs `rpc.statd`, which the container does not run; `vers=3` is still accepted when set explicitly.
3. Tests pin the literal default in the API and the exact mount vector in the mount package, and a web test pins the help copy; the e2e storage and library specs pass on a rebuilt UI.
4. Skipped: the real-NAS check (task 81), the SMB `vers=3.1.1` change (G1, recommended above), and the ROADMAP status line.
5. Next task (81) should verify a real NFS v4.2 mount and that a `vers=3` mount fails with the statd message, so the operator help text is backed by a real run.
