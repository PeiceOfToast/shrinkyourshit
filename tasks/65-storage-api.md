# Task 65: Storage API with validation and locking

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.4 and 4.6. `internal/api/api.go` route table, `internal/server/handlers.go` error style,
`internal/server/auth.go` for how login state is read.

## Scope
Create `internal/api/storage.go`, `internal/api/storage_test.go`.
Modify `internal/api/api.go` (routes), `internal/api/openapi.go`, `internal/store/dto` or the
server DTO file.

## Do
1. Routes: `GET /api/v1/storages`, `POST /api/v1/storages`, `GET/PUT/DELETE /api/v1/storages/{id}`,
   `POST /api/v1/storages/{id}/test`, `POST /api/v1/storages/{id}/mount`,
   `POST /api/v1/storages/{id}/unmount`, `GET /api/v1/storages/{id}/browse`.
2. Option allowlist and source validation exactly per spec 4.4, each as a field error, never a
   panic and never a silent drop.
3. **Auth gate**: every write endpoint and the browse endpoint return 403 with an actionable
   message when `login_enabled` is false (spec 4.6). Read of the list may stay open so the UI can
   explain why.
4. `storage_allowed_hosts` setting: comma separated host or CIDR list. When non empty, a source
   whose host is not in the list is refused. Empty means allowed, and the response says so.
5. Per storage lock so mount, unmount and test cannot race. Test with two concurrent requests.
6. The response type exposes status fields (`mounted`, `error`, `mountpoint`) that task 66 fills.

## Acceptance
- [x] A shell metacharacter, a newline, an unknown option or a second `-o` is rejected with 400.
- [x] With login disabled, every write endpoint returns 403, proven by a test.
- [x] A disallowed host is refused when `storage_allowed_hosts` is set.
- [x] Two concurrent mount calls on one storage serialise.
- [x] `make test` and `make lint` green.

## Notes
Write the plan and the option allowlist in `tasks/65-notes.md`.
