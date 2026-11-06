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

- `main.go` wiring (see plan item 7).
- Slug rule: `[a-z0-9-]`, 1 to 64 characters, no leading or trailing dash. The spec says only "lowercase
  letters, digits and dashes". Leading and trailing dashes are rejected because a mountpoint of
  `/storage/-x` is not useful. Tell me if you want the looser rule.
- AAD: each ciphertext is bound to its storage id, so an encrypted password copied to another row fails to
  decrypt.
- The key file is created with `O_EXCL` at 0600. An existing key file is not checked for its mode.
- `SHRINK_STORAGE_KEY` is validated when first used, not at startup. A bad key therefore fails the storage
  operations with a clear error and does not stop the server. This matches "one broken storage never
  prevents the server from starting".
