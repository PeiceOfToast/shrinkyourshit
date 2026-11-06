# Task 76: Remove the legacy library rule code and stop writing the rule columns

Part of the v1.12 wave, drop the library rule columns. It is the follow up to task 74 (v1.11), which
made the encode profile the only source of a library's rules and kept six library columns as a
rollback snapshot. `ROADMAP.md` section v1.12 carries the whole wave scope. `DECISIONS.md` is read
only.

## Read first

`ROADMAP.md` v1.12, the `API.md` task 74 section, the libraries section of `ARCHITECTURE.md`, and
`tasks/74-notes.md`. Then grep, before changing anything, for every reader and writer of
`flow_json`, `flow_source`, `target`, `easy`, `rule_tier` and `result_policy` in `internal/store`
and `internal/server`.

Current, not legacy: the setting keys `default_result_policy` and `default_rule_tier`, and
`profiles.tier` / `profiles.result_policy`. Leave those alone.

## Scope

`internal/store/crud.go`, `internal/store/store.go`, `internal/store/profiles.go`,
`internal/server/libraryflow.go`, `internal/server/library.go`, `internal/server/server.go`, the
tests that touch those fields, and the doc sentences that describe the snapshot as still written.

## Do

1. Remove `legacyLibraryFlow` and the helpers only it uses (`internal/server/libraryflow.go`), the
   boot call `backfillFlows` and the task 74 boot warning `logLegacyColumns` (`internal/server/server.go`).
2. `BackfillProfiles` keeps one job only: attach a profile to a library row whose `profile_id` is
   NULL, using the default profile (create the default profile first if there is none). It must
   read none of the six columns. A library row can only be unassigned after a failed boot, so
   record in the notes why the smaller version is the honest one.
3. Stop writing the six columns: drop them from `libraryColumns`, from the INSERT and the UPDATE in
   `internal/store/crud.go`, and drop the snapshot block in the profile attach code in
   `internal/server/library.go` (`lib.FlowJSON = ...` through `lib.TargetJSON = ...`).
4. Remove the six fields from `store.Library` and fix every use, so the compiler proves nothing
   reads them. `rules by grep` is not enough here; the struct must be clean.
5. The library DTO fields `target`, `easy`, `flow`, `flow_source`, `rule_tier`, `result_policy` and
   `profile_id` stay: since task 74 they are derived from the profile, not read from the columns.
   The same holds for the job spec `target`, `flow`, `flow_source` and `result_policy`. Prove one of
   them in a test so a later reader cannot "clean up" the response by mistake.
6. Schema unchanged in this task. Do not drop columns, do not remove API request fields, do not
   touch `PUT /api/v1/libraries/{id}/tier`. Those are tasks 77 and 78.
7. Update the sentences in `ARCHITECTURE.md` and `API.md` that say the six columns are written on
   each save. Say the columns are still in the schema, unread and unwritten, until task 78 drops
   them.
8. Tests: a database created with the v1.11 schema and rows that still hold legacy values opens and
   serves libraries unchanged (no boot error, no legacy warning), and a library create and update
   round trips with the profile only.

## Acceptance

- [ ] `store.Library` has no field for the six columns, and no query selects or writes them (grep
      output in the notes).
- [ ] Boot no longer calls `backfillFlows` or `logLegacyColumns`, and `BackfillProfiles` reads no
      legacy column (test with a v1.11 shaped database).
- [ ] A v1.11 database with filled legacy columns opens and serves libraries unchanged (test).
- [ ] Docs updated.
- [ ] `make test` and `make lint` green.

## Notes

Plan, grep evidence, the backfill decision and the 5 line summary go in `tasks/76-notes.md`.
