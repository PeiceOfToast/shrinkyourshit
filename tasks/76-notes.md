# Task 76 notes: remove the legacy library rule code, stop writing the rule columns

## Plan (files, in order)

1. `internal/store/store.go`: drop `TargetJSON`, `EasyJSON`, `FlowJSON`, `FlowSource`, `RuleTier`,
   `ResultPolicy` from `Library`. Fix the struct comments.
2. `internal/store/crud.go`: remove the six columns from `libraryColumns`, the scan and dest lists,
   the INSERT and the UPDATE. Remove the create-time tier default and the result policy check on
   library writes.
3. `internal/store/profiles.go`, `interface.go`: `BackfillProfiles(ctx, profileID)` attaches one
   profile to every unassigned library. Remove the hashing and matching helpers only it used
   (`canonicalJSON`, `sha256Hex`, `findProfileFor`, `freeProfileName`, `unassignedLibraries`).
4. `internal/server/libraryflow.go`: remove `legacyLibraryFlow`, `backfillFlows`,
   `logLegacyColumns`. `backfillProfiles` calls the store with `defaultProfile()`'s id.
5. `internal/server/server.go`: boot no longer calls `backfillFlows` or `logLegacyColumns`.
6. `internal/server/library.go`: `attachProfile` drops the snapshot block (`lib.FlowJSON = ...`
   through `lib.TargetJSON = ...`).
7. Tests: store, server, api, worker tests that named the fields. Docs: `ARCHITECTURE.md`
   (Library columns line, Storage bullet), `API.md` (task 74 section, two sentences).
8. New boot test: `internal/server/legacy_columns_test.go`.

## Grep evidence

Before the change (main at b24dc53), readers and writers of the six columns in `internal/store`
and `internal/server` (non test):

- `internal/store/crud.go`: `libraryColumns`, `scanLibrary`, `libraryDest`, `CreateLibrary`
  (INSERT), `UpdateLibrary` (UPDATE).
- `internal/store/profiles.go`: `BackfillProfiles` read `FlowJSON`, `RuleTier`, `ResultPolicy`.
- `internal/server/libraryflow.go`: `legacyLibraryFlow` (read `FlowJSON`, `TargetJSON`, `EasyJSON`,
  `FlowSource`), `backfillFlows`, `logLegacyColumns`.
- `internal/server/library.go`: `attachProfile` wrote all six.
- `internal/server/dto.go`, `queue.go`, `transfer.go`, `api/worker.go`: already read the profile
  (task 74). No legacy read left there.

After the change, the same grep over non-test code in `internal/store`, `internal/server`,
`internal/api` and `internal/worker`:

```
internal/store/crud.go   libraryColumns: only the profile subqueries (COALESCE(profile_id), graph, tier, result_policy of the profile)
internal/store/crud.go   INSERT: `target` written as the empty literal '' (see decision 1)
```

Nothing else matches `flow_json`, `flow_source`, `easy`, `rule_tier`, or a `Library` field access
to the six names. `internal/server` and `internal/api` mention `libraries.rule_tier` or `legacy`
only in comments, in `libraryflow.go` (`LibraryFlow`'s doc) and the new boot test.

## Decisions taken in this task

1. **`target` is still named in the INSERT, as an empty literal.** The task says drop the six
   columns from the INSERT. `libraries.target` is `NOT NULL` with no default (`0001_init.sql`),
   and the task keeps the schema unchanged (migration 0019 is task 78). Without a value the insert
   fails with `NOT NULL constraint failed: libraries.target`. The insert writes `''`, a constant. It
   is not the rule, no struct field holds it, and nothing reads it. **Owner to confirm.** The
   alternatives are a schema change in this task (against the task) or blocking the task.
2. **The backfill attaches the default profile only, and reads no legacy column.** The task
   asks for this. Why the smaller version is the honest one: every boot since v1.11 runs the
   backfill, so a library row can only be unassigned after a boot that failed part way. For such a
   row the old per-graph matching (hash the old `flow_json`, reuse or create a profile) would
   re-derive a rule the v1.11 boot already failed to assign. The default profile is the rule the
   form pre-selects, and the library sees it on its next save. The old matching code had its own
   test suite (`profiles_test.go`), which is removed with it.
3. **Default profile source.** `defaultProfile()` (first profile in list order, seeded when the
   table is empty) gives the id. `server.Start` already calls `defaultProfile()` at every boot, so
   a fresh install still gets its seed. The backfill runs only when a library is unassigned, so it
   creates no profile on an install with none.

### Behaviour changes to know about

- **A direct v1.10 to v1.12 upgrade (skipping v1.11) now gets the default profile, not its own
  rule.** On v1.11 those libraries got a profile built from their old graph (task 70). Skipping v1.11
  skips that step. This is the cost of the owner's "smaller version". Check the release notes for
  the upgrade path: upgrading through v1.11 keeps every rule.
- **A row the v1.11 boot could not assign** (its legacy graph was not JSON) used to run nothing
  (`ErrNoProfile`). It now runs the default profile. The old graph is not recovered.

## Test changes

| File | Change | Why |
|------|--------|-----|
| `internal/store/profiles_test.go` | Rewritten. The v1.10 hashing test, the broken graph test and the canonical JSON test are gone. New: default profile assignment, idempotent run, unknown profile refused, assigned libraries untouched, legacy columns not written by the backfill (raw read), profile id survives writes | The matching code is gone |
| `internal/store/tier_migration_test.go` | `TestRuleTierMigrationMatchesFlowSource` reads `rule_tier` raw (the migration 0016 check); `TestCreateLibraryDefaultsRuleTier` replaced by `TestLibraryWritesLeaveLegacyColumnsEmpty` (profile round trip; the columns keep their defaults) | The struct has no field for them. The raw read is a test fixture check on the migration, not a reader |
| `internal/store/store_test.go` | Removed the `target` and `result_policy` library asserts; removed `TestLibraryEasyRoundTrip`; removed the library result policy check | Fields gone |
| `internal/server/legacy_columns_test.go` | New: `TestV111DatabaseBootsAndServesLibrariesUnchanged`. A v1.11 state (profile assigned, legacy columns set to values that disagree) plus one unassigned row with a broken legacy graph. Boots the server, asserts no error line and no "legacy" log line, the assigned library's DTO is unchanged (target from the profile, not the legacy h264), the unassigned row gets the default profile, and no profile is created | Acceptance: v1.11 database serves unchanged; boot does not call the removed steps |
| `internal/server/dto_test.go` | Removed the stored easy input assert (`EasyJSON` gone) | Field gone; the profile decides, covered by the rest of the test |
| `internal/server/library_test.go` | Target asserts read the DTO's derived `Target` (profile graph); `mustTarget` helper removed | Proves the DTO target comes from the profile |
| `internal/server/transfer_test.go` | The harness library follows a profile (`targetProfile`) | Field gone; the profile is the rule |
| `internal/api/nodes_test.go` | Fields dropped from the fixture library | Stats test, needs no rule |
| `internal/worker/integration_test.go` | `TestLegacyTargetStillRuns` removed | Its premise (a row with empty flow columns runs from its Target) ended with task 74: such a library runs nothing. The other integration encode tests cover running a library |

## Acceptance

- [x] `store.Library` has no field for the six columns; no query selects or writes them (grep above).
- [x] Boot no longer calls `backfillFlows` or `logLegacyColumns`; `BackfillProfiles` reads no legacy
      column (`TestBackfillDoesNotWriteLegacyColumns`, `TestV111DatabaseBootsAndServesLibrariesUnchanged`).
- [x] A v1.11 database with filled legacy columns opens and serves libraries unchanged
      (`TestV111DatabaseBootsAndServesLibrariesUnchanged`).
- [x] Docs updated: `ARCHITECTURE.md` (Library columns line, Storage bullet), `API.md` (task 74
      rollback sentence, boot warning sentence).
- [x] `make test` and `make lint` green (see verification).

Task 74 open gap 2 (the boot warning fires every boot) is closed: the warning is removed.

## Not done here (left for the next tasks)

- Task 77: the request fields `target`, `easy`, `result_policy`, the `deprecated` list and
  `PUT /libraries/{id}/tier` (still there, still ignored or refused as before).
- Task 78: migration 0019 drops the six columns. Then `TestRuleTierMigrationMatchesFlowSource`
  (raw `rule_tier` read) and `TestLibraryWritesLeaveLegacyColumnsEmpty` (raw reads of
  `flow_json`, `target`, `rule_tier`, `result_policy`) must go, and the INSERT's `target` literal
  goes with the column.
- Task 74 gap 6: the web edit page still resends `target`.
- Pre-existing gofmt differences on `main`: `internal/worker/integration_test.go` (touched here,
  already unformatted before this task), `internal/store/webhooks.go`, `internal/store/webhooks_test.go`,
  `internal/server/cron_test.go`, `harness_test.go`, `node_test.go`, `scanner.go`, `scheduler.go`,
  `webhooks.go`, `runner.go`, `runner_test.go`, `installer_test.go`, `lifecycle_integration_test.go`.
  Not reformatted: outside this task.

## Verification

Commands run on this VM from the repository root, `PATH=/home/mb/.local/go/bin:/home/mb/.local/bin:$PATH`:

- `go vet ./...`: clean.
- `golangci-lint run`: 0 issues.
- `./scripts/check-spdx.sh`: ok.
- Targeted: `go test ./internal/store/ -run 'Backfill|Profile|Library|RuleTier|Migrations'`: ok.
- `go test ./internal/server/ -run TestV111`: ok.
- Full: see Verification, full run below.

Full run, same tree (after the last comment-only edit in `dto.go`):

- `go test ./... -count=1 -timeout 30m`: every package ok, exit 0 (api 543 s, server 506 s, store 231 s, worker 47 s).
- `cd web && npx vitest run` (Node 22 from `/home/mb/.local/node22/bin`): 69 files, 524 tests pass.
- Integration tag: compiled for `./internal/server/`, `./internal/api/`, `./internal/worker/` (`-tags=integration`). The integration tests were not run; ffmpeg was not exercised by this change.

## Summary

1. Built: the library rule columns are no longer read or written. `store.Library` has no field for
   them; `BackfillProfiles` attaches the default profile to unassigned libraries and reads nothing
   legacy; the boot warning and the two legacy boot steps are removed; `attachProfile` no longer
   writes a snapshot; docs say the columns are unread and unwritten until task 78.
2. Skipped: removing the columns and the migration (task 78), the request fields and the tier route
   (task 77), the web resend (task 74 gap 6).
3. Owner to confirm: (a) the INSERT still names `target` with `''` because the column is NOT NULL and
   the schema is unchanged (decision 1); (b) a direct v1.10 to v1.12 upgrade now gets the default
   profile instead of its own rule (behaviour changes above).
4. Next task (77) should know: `PUT /libraries/{id}/tier`, the `deprecated` list and the request fields
   are still there, unchanged by this task.
5. The v1.11 boot test is the place to extend if task 78 adds migration 0019: it should then open a
   database with the migration applied and check the same DTO.
