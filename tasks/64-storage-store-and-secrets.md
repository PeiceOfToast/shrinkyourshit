# Task 64: Storage table, encryption at rest, store CRUD

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 4.3. `internal/store/migrations/`, `internal/store/crud.go`, `internal/store/interface.go`,
`internal/config/config.go`.

## Scope
Create `internal/store/migrations/0017_storages.sql`, `internal/store/storages.go`,
`internal/store/storages_test.go`, `internal/secret/secret.go`, `internal/secret/secret_test.go`.
Modify `internal/store/migrations.go`, `internal/store/interface.go`, `internal/config/config.go`.

## Do
1. Migration `0017_storages.sql` with the table exactly as spec 4.3 draws it. Forward only.
2. `internal/secret`: AES-GCM encrypt and decrypt with a 32 byte key. Key resolution order:
   `SHRINK_STORAGE_KEY` (base64) if set, else `/config/storage.key`, created on first use with
   mode 0600. Tests cover round trip, wrong key, and a truncated ciphertext.
3. Store methods: list, get by id, get by name, create, update, delete. The password is stored
   through `secret` and never returned by a read path; expose a `HasSecret` bool instead.
4. Name is a slug: lowercase letters, digits and dashes only, unique, and it becomes the
   mountpoint, so validation lives with the store method and rejects a bad slug with a clear
   error.
5. Config: `SHRINK_MANAGE_MOUNTS` (default true) and `SHRINK_STORAGE_KEY`.

## Acceptance
- [x] Migration applies to a database created by v1.10 and is a no-op on a second run.
- [x] A created storage never returns its password through any read method, proven by a test.
- [x] Wrong key or tampered ciphertext fails closed with an error, never a panic.
- [x] `make test` green.

## Notes
Write the plan and a 5 line summary in `tasks/64-notes.md`.
