# Task 65 notes: storage API

## Plan (written before implementation)

Files, in order:
1. `internal/store/store.go`: add `SettingStorageAllowedHosts = "storage_allowed_hosts"`.
2. `internal/api/storage.go` (new): DTO, request body, pure validators (option allowlist, source,
   credentials, allowed-host policy), the per-storage keyed lock, the `StorageMounter` seam that
   task 66 fills, and the handlers.
3. `internal/api/storage_test.go` (new): validator table tests, HTTP tests for the gate, the 400s,
   host policy, lock serialisation, CRUD, and a password that never comes back.
4. `internal/api/api.go`: routes, `storageLocks` and `mounter` fields, `WithStorageMounter` option.
5. `docs/openapi.yaml`: every new path and schema (the anti-drift test diffs routes against it).
6. `API.md`, `ARCHITECTURE.md`: task 65 endpoint section and the status and host policy notes.
7. Tick the checklist, update `tasks/STATUS.md`, write the summary.

## Decisions I took (the owner may redirect)

- **Mount seam.** The mount executor is task 66. Task 65 defines `api.StorageMounter` (Status, Mount,
  Unmount, Test) and `api.WithStorageMounter`. The default answers mount, unmount and test with
  `501 not_implemented` and reports every storage as not mounted. Task 66 supplies a real one.
- **Login gate.** Writes and browse return `403 login_required` when `login_enabled` is false, before the
  body is read. Reads (list and get) use `requireAdmin`, so they are open only while login is off, as
  for every other admin read.
- **Field errors.** A 400 has `{error, code: "invalid_request", errors: [{path, message}]}`, the same
  path/message shape as flow errors.
- **Options.** The allowlist is the spec 4.4 list. Each option has a value rule: numbers for `vers`,
  `nfsvers`, `timeo`, `retrans`, `port`, `actimeo`, `uid`, `gid`; octal for `file_mode` and `dir_mode`;
  alphanumerics for `iocharset` and `sec`. Flags take no value. A second `-o`, a line break, a shell
  character, a duplicate option, `soft` with `hard`, `ro` with `rw`, and `rw` on a read-only storage are
  all refused. An empty NFS options string gets the spec default `vers=3,soft,timeo=50,retrans=2`.
- **Source.** NFS is `host:/path` (no spaces, no `..`). SMB is `//host/share`, where the share may end in
  `$` for hidden shares.
- **Immutable fields.** `name` (it is the mountpoint, and libraries hold `/storage/<name>` paths) and
  `protocol` cannot change on PUT; the API refuses a change with a field error. This avoids a
  protocol switch that would leave a sealed SMB password on an NFS row, which the store cannot clear.
- **Credentials.** `username`, `domain` and `password` apply to SMB only; NFS refuses them. None of
  the three may contain a line break, because the credentials file is line based (task 66).
- **Allowed hosts.** `storage_allowed_hosts` is a comma separated list of hostnames, IP literals and
  CIDRs. Empty means unrestricted, and the list response says so in `host_policy`. An invalid entry
  fails closed: every source is refused and the message names the entry. Hostnames match exactly, and
  CIDRs match literal IPs only. There is no DNS lookup. The check runs on create, on PUT, and again on
  mount and test, so a host removed from the list cannot be mounted by an existing row.
- **Locks.** One mutex per storage id. Mount, unmount and test take it. PUT and DELETE take it too, so
  an edit cannot change the source or the password while a mount is in progress.
- **Delete** answers `409 conflict` while the mounter reports the storage as mounted.
- **Browse.** The route exists with the login gate. While the storage is not mounted it answers
  `409 not_mounted`. A mounted storage answers `501`, because the listing is task 69.
- **Test response.** `{ok, error?}`, always 200 after the storage is found, the same convention as the
  integration test. Task 66 can add timings.

## Open questions for the owner

1. `storage_allowed_hosts` has no writer yet. `GET` and `PUT /settings` do not carry it, so an operator
   can set it only in the database until settings expose it. That needs a decision on whether task 65
   or the task 68 App settings work adds it. I did not widen `settings.go` because it is outside the
   task's listed scope.
2. Protocol and name are immutable. If you want a rename or a protocol change, the store needs a
   clear-password path first.

## Gate results (2026-11-06, branch task/65-storage-api)

- `go test ./...` exit 0 (every package; api 515 s, server 517 s, store 243 s on this box).
- Storage tests: `go test ./internal/api/ -run 'Storage|OpenAPI'` passes (17 storage tests, plus the OpenAPI drift test).
- Mutation check: with the lock removed from `mountStorage`, `TestStorageConcurrentMountsSerialise` fails
  (two calls in flight). With the lock restored it passes.
- `go vet ./...` clean. `golangci-lint` v2.15.0 (the version CI pins) `run ./...`: 0 issues after one fix
  (a literal U+202E in a test, now `\u202e`). `scripts/check-spdx.sh` ok.
- `make test-web` (`npm run test`): 65 files, 474 tests pass. `make lint-web`: eslint and prettier pass.
- `gofmt -l` lists `installer_test.go` and `nodes_test.go`, which I did not touch. My files are clean.
- Not run: `make test-integration` (ffmpeg paths; this change does not touch them).

## Summary

1. Built: `internal/api/storage.go` (nine routes, the validators for options, source, credentials and the
   host allowlist, the keyed per-storage lock, the `StorageMounter` seam, and the login gate). `storage_test.go`
   (17 tests). OpenAPI paths and schemas, `API.md` task 65 section, `ARCHITECTURE.md` notes, and the
   `storage_allowed_hosts` setting key constant in the store.
2. Skipped: the mount executor (task 66, the default answers 501), the browse listing (task 69), and any write path
   for `storage_allowed_hosts` (no settings change, see open question 1).
3. Task 66 should know: implement `api.StorageMounter` and pass it with `api.WithStorageMounter`. Mount, unmount
   and test run under the storage lock, and the API has already re-checked the host allowlist before calling.
   `Status` fills `mounted` and `error`. The executor reads the password with `store.StorageSecret`; the API never
   does. A failed mount's message is returned to the client as is, so keep credentials and passwords out of it.
   The API does not unmount on delete; a delete is refused while `Status` says mounted.
4. Owner questions: (1) `storage_allowed_hosts` has no writer yet, since the settings endpoints do not carry it.
   Should it join `GET/PUT /settings` in this wave, or with the task 68 App settings work? (2) `name` and
   `protocol` are immutable after create, and no clear-password path exists. Confirm both are wanted. (3) An
   empty NFS `options` gets the soft default, so an operator cannot save an empty options string on NFS.
5. Open: nothing blocks task 66, once the owner answers question 1 or accepts the gap.
