# Task 64 notes: storage table, encryption at rest, store CRUD

## Plan

Branch `task/64-storage-store-and-secrets`. Go is at `/home/mb/.local/go/bin` (not on PATH by default).

1. `internal/secret/secret.go` and `secret_test.go`: AES-256-GCM `Box` (Seal/Open with AAD), `ParseKey`
   (base64, 32 bytes), `LoadOrCreateKey` (file mode 0600), and `Source`, which resolves the key lazily on
   first use: `SHRINK_STORAGE_KEY` if set, else the key file. Lazy so a fresh install creates no key until
   a secret is first stored.
2. `internal/store/migrations/0017_storages.sql`: the spec 4.3 table, verbatim. Forward only.
3. `internal/store/storages.go` and `storages_test.go`: `Storage` struct, CRUD on the `Store` interface,
   slug validation (`ValidStorageName`), protocol enum (`nfs | smb`). `Password` is write only
   (`json:"-"`), cleared after a write, never populated by a read. `HasSecret` is the read side.
   One more method, `StorageSecret(ctx, id)`, returns the plaintext for task 66's mount executor. It is
   the only read that decrypts, and the tests need it to prove the fail-closed rules.
4. The secret source reaches the store through a variadic option, `store.WithSecrets(*secret.Source)`,
   so existing `store.New(path)` callers compile unchanged. Without a source, writing a password fails
   closed.
5. `internal/config/config.go`: `SHRINK_MANAGE_MOUNTS` (default true, env only) and `SHRINK_STORAGE_KEY`
   (env only), plus `StorageKeyPath()` = `<data dir>/storage.key`.
6. Docs in the same commit: ARCHITECTURE.md (storages table, protocol enum, secret rules) and the two
   env rows in the STACK.md config reference.
7. Wiring: `cmd/shrinkyourshit/main.go` passes `store.WithSecrets(...)` built from the config. This file is
   not in the task's Scope list. Without it the feature cannot work in a running server, so it is in this
   task, and it is listed under "Deviations" below.

Order of commits: secret, migration + store, config, wiring + docs.

## Dependencies

No new dependency. AES-GCM, base64, crypto/rand and crypto/sha256 are all stdlib.

## Deviations and choices

- Files outside the Scope list, each needed for the feature to work:
  - `cmd/shrinkyourshit/main.go`: passes `store.WithSecrets(secret.NewSource(...))` to `store.New`.
    Without it a running server cannot seal a password.
  - `internal/store/sqlite.go`: `New(path, opts ...Option)` and a `secrets` field. The variadic option
    keeps every existing `store.New(path)` caller compiling.
  - `ARCHITECTURE.md`: the storages entry (rule 5 requires it for an enum and a data shape).
  - `STACK.md`: two rows in the config reference (the env vars are the documented config surface).
- `Store` gained `StorageSecret(ctx, id)`, which is not named in the task. It is the only plaintext read,
  and the fail-closed tests need a decrypt path. ARCHITECTURE says only task 66 may call it. The API
  must not.
- `Storage.Password` is `json:"-"`, and `Storage.String()` redacts it. `%v`, `%+v` and JSON never show it.
- Slug rule: `[a-z0-9-]`, 1 to 64 characters, no leading or trailing dash. The spec says only "lowercase
  letters, digits and dashes". Leading and trailing dashes are rejected because a mountpoint of
  `/storage/-x` is not useful. Tell me if you want the looser rule.
- AAD: each ciphertext is bound to its storage id, so an encrypted password copied to another row fails to
  decrypt.
- The key file is created with `O_EXCL` at 0600. An existing key file is not checked for its mode.
- `SHRINK_STORAGE_KEY` is validated when first used, not at startup. A bad key therefore fails the storage
  operations with a clear error and does not stop the server. This matches "one broken storage never
  prevents the server from starting".

## Results (2026-11-06, branch task/64-storage-store-and-secrets)

- `go test ./...` exit 0 (all packages; store 215 s, server 473 s, api 438 s on this box).
- New tests: `internal/secret` 13 tests, `internal/store/storages_test.go` 13 tests, `internal/config`
  2 tests. All pass.
- `make lint`: `go vet` and `golangci-lint` 0 issues, `check-spdx` ok, `lint-web` ok.
- `make test-web`: 65 files, 474 tests pass. `npm` is not on the default PATH here; I used
  `/tmp/tools/node-v20.18.1-linux-x64/bin`.
- Smoke: built the binary and ran `serve` on a scratch data dir for 8 s. It started, the DB migrated,
  and no `storage.key` was created (lazy, as intended).
- `gofmt -l` lists pre-existing unformatted files that I did not touch. None of my files appear.

## Review fix (2026-11-06)

- dev-review found that a whitespace-only `SHRINK_STORAGE_KEY` was treated as unset, so the source silently
  generated a key file. `Source.Box` now treats any non-empty raw value as set, and `ParseKey` rejects the
  blank value. New test `TestSourceWhitespaceEnvKeyFailsClosed` fails on the old code and passes now.

## Summary

1. Built: `internal/secret` (AES-256-GCM with AAD, lazy key source from `SHRINK_STORAGE_KEY` or
   `<data dir>/storage.key`), migration `0017_storages.sql` (spec 4.3 verbatim), store CRUD with
   slug and protocol validation, `WithSecrets` option, `SHRINK_MANAGE_MOUNTS` and `SHRINK_STORAGE_KEY`.
2. Skipped: no endpoints (task 65), no mount logic (66), no UI. `options`, `source` and credential
   rules are left to task 65, as the spec says.
3. Next task (65) should know: the store does not validate `source` or `options`. `StorageSecret` must
   stay out of the API. `Update` with an empty `Password` keeps the stored secret, and there is no
   "clear password" path yet; add one in 65 if the UI needs it.
4. Owner questions: (a) slug rule: I reject leading and trailing dashes, which the spec does not say.
   Say if you want that looser. (b) An existing key file's mode is not checked; only a newly created
   one is 0600.
5. Open: nothing blocks task 65.
