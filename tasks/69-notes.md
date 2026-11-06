# Task 69 notes: storage browser and path picker

## Plan (written before implementation)

Files, in order:

1. Go browse listing: `internal/api/storage_browse.go` (replaces the 501 placeholder that was in `storage.go`), `storage_space_unix.go` and `storage_space_other.go` (statfs free space, build-tagged), `storage_browse_test.go`. `storage_test.go` gains `newStorageHarnessRoot` so the browse tests can point the storage root at a temp dir. `api.go` gains a `storageRoot` field, `WithStorageRoot` option, default `/storage`.
2. Contracts: `API.md` (task 69 section, browse row), `docs/openapi.yaml` (browse path with `path` query, `StorageBrowse` schema, 200 response). `ARCHITECTURE.md` has no storage browse shape, so it is unchanged.
3. Web: `api.ts` (`browseStorage`, `StorageBrowse`), `storageModel.ts` (pure: `isInsideStorage`, `storageContaining`, `storageCrumbs`, `checkTypedPath`, `storageBrowseKey`, `freeSpaceText`), `StorageBrowser.tsx` (dialog, folders only), `EasyLibraryForm.tsx` (storage choice, badge and free space, Choose opens the browser for a share, "Type the path instead" disclosure).
4. Tests: vitest for each of the above, Playwright in `e2e/library.spec.ts`.

## Decisions taken without the owner

- **Typed path rule.** Read literally, "a typed path that resolves outside any mounted storage is refused" would also refuse local folders such as `/media/movies`. I took it literally for the typed field only: a typed path must be inside a mounted share, otherwise it is refused with the reason. Local folders still work through the server folder picker (`Choose`, when "This server" is selected). Owner to confirm.
- **Refusal is UI only.** `POST /libraries` still accepts any path. The spec did not ask for a server check, and the library create path has no notion of storages. Open question below.
- **Free space** comes from `statfs` on the share's mountpoint, returned as `free_bytes` and `total_bytes` on the browse response (null when unknown). Linux and darwin have a probe; other platforms return null. Storages are only mounted on Linux, so this matters for dev builds only.
- **Hidden folders.** Dot folders are not offered, as in the server folder picker. The server still lists them; the UI filters.
- **Symlinks.** A symlink inside a share is listed only when it resolves to a folder inside the share. A symlink that leaves the share is not listed and cannot be browsed (`400 outside_storage`).
- **Login off.** The browser shows the login notice with a link to `/app`. The form hides nothing; it lets the user open Choose, and the browser explains the fix.
- **Not mounted.** The form disables Choose for an unmounted share and says where to mount it. The browser's own not-mounted notice links to `/storage`.
- **Form storage list.** The form reads `GET /storages` (shared key with the Storage page). If that read fails the storage choice is hidden and the server folder picker still works.
- **Path containment.** The check is lexical first (`/storage/nas` must be followed by `/`, so `/storage/nas2` is refused) and then after `EvalSymlinks`.

## Changed assertions (spec section 7 requires each one to be recorded)

- `internal/api/storage_test.go`: no assertion changed. The helper `newStorageHarness` now delegates to `newStorageHarnessRoot(t, mounter, "")`. `TestStorageDefaultMounterAnswersNotImplemented` still expects browse `409 not_mounted` with the default mounter, which is unchanged.
- `web/src/EasyLibraryForm.test.tsx`: the original test is unchanged. A new `describe` block was added with four tests.
- `web/src/storageModel.test.ts`: the import list gained the new helpers, and a new `describe` block was added. No existing assertion changed.
- Browse no longer answers `501 not_implemented`. No test asserted the 501 on a mounted storage, so nothing else changed. OpenAPI and API.md were updated in the same commit.

## Test ids (new)

Form: `easy-storage-select`, `easy-storage-badge`, `easy-storage-space`, `easy-typed-path`, `easy-typed-path-input`, `easy-use-typed-path`, `easy-typed-path-error`.
Browser: `storage-browser` (the dialog), `storage-browser-badge`, `storage-browser-space`, `storage-browser-crumbs`, `storage-browser-up`, `storage-browser-dir-<name>`, `storage-browser-current`, `storage-browser-use`, `storage-browser-cancel`, `storage-browser-close`, `storage-browser-login`, `storage-browser-unmounted`, `storage-browser-error`.

## Verification

- Go: `go vet ./...` clean, `golangci-lint` 0 issues (via `make lint`). `go test ./internal/api/ -run 'TestBrowse|TestStorage|OpenAPI'` green. The browse unreadable test uses a `0000` folder and runs as uid 1000 here; it skips when run as root.
- Web: `npx vitest run` 68 files, 522 tests, green. `npx tsc --noEmit` clean. `npm run lint` and `npm run format:check` clean. `npm run build` green.
- `make test` (go all packages ok incl. internal/api 403 s and internal/server 396 s, web 68 files / 522 tests) and `make lint` (0 issues, eslint, prettier) both green.
- Playwright, full suite on the rebuilt binary (`make build`): 142 passed, 4 skipped, exit 0 (phone, tablet and desktop). `e2e/library.spec.ts` picker tests pass on all three projects.
- Environment: Chromium needs `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib` for libatk, as in task 68. Go is at `/home/mb/.local/go/bin`, Node 22 at `/home/mb/.local/node22/bin`, make at `/home/mb/.local/bin`. The default PATH on this box has none of them.

## Acceptance mapping

1. Playwright picks a folder inside a storage and creates a library with that path: `web/e2e/library.spec.ts`, "picks a folder inside a storage and creates the library with that path". The storage routes are answered by the test, as in task 68, because the e2e runner cannot mount a share.
2. Typed path outside every mounted storage is refused with a message: `web/src/EasyLibraryForm.test.tsx` ("refuses a typed path outside every mounted storage, with the reason"), `web/src/storageModel.test.ts` (refusal cases), and the e2e test "refuses a typed path outside every mounted storage".
3. Unreadable folder is a readable error: `internal/api/storage_browse_test.go` (`TestBrowseUnreadableFolderIsAnError`, returns `403 unreadable`) and `web/src/StorageBrowser.test.tsx` ("shows an unreadable folder as an error, not an empty list").
4. `npm run build` and `npx playwright test` green: see Verification.

## Gaps and open questions for the owner

1. **Typed path rule.** Confirm the literal reading above (typed paths must be inside a mounted share; local folders only through the server picker). If local folders should also be typeable, the rule needs a different trigger.
2. **Server-side refusal.** `POST /libraries` does not check storages. A library created through the API with a path under `/storage/<name>` for an unmounted share is accepted, then fails at scan time. Spec 4.7 says a worker that cannot see a library path must fail the job with an error naming the storage, which is task 66's area, not this one. Proposed: a server check on create and update for paths under `/storage/`. Needs owner decision, so not made here.
3. **Server folder picker.** `GET /fs/browse` still answers `500` with a generic message on an unreadable folder. Only the storage browser has the readable `403 unreadable`. Changing `/fs/browse` was out of scope.
4. **Free space probe.** Only linux and darwin have one; other platforms show no figure. Fine for storages (Linux container), noted for dev builds.

## Summary

1. Built: the storage browse listing (folders only, contained in the mountpoint, symlink-safe, readable errors, free space), the storage browser dialog, the library form's storage choice with mounted badge and free space, the typed path disclosure with refusal, login and not-mounted notices with links, and tests at every layer (Go, vitest, Playwright).
2. Skipped: a server-side check that library paths sit inside mounted storages (open question 2), a readable error for the server folder picker (open question 3), and free space on non-Linux platforms.
3. Next task (70 profiles schema) should know: `GET /storages/{id}/browse` is live and needs login; `EasyLibraryForm` now reads `GET /storages` and the browse of each share's mountpoint; the library form's storage choice is UI only, so a typed or picked path is not re-checked by the server.
4. Environment: the Go toolchain is at `/home/mb/.local/go/bin`, Node 22 at `/home/mb/.local/node22/bin`, make at `/home/mb/.local/bin`, and Chromium needs `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`. These are machine settings, not repo changes.
5. Test ids are listed above. The form has no card view, so none of its ids carry a `-card` suffix.
