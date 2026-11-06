# Task 78: Drop the library rule columns in a forward only migration

Part of the v1.12 wave. Tasks 76 and 77 removed the code and the API that touched the six legacy
library columns. This task drops them and closes the wave. `DECISIONS.md` is read only.

## Read first

`ROADMAP.md` v1.12, `internal/store/migrations.go` and the last three migrations for the house
style, `ARCHITECTURE.md` schema section, `CHANGELOG.md`, and `tasks/76-notes.md` plus
`tasks/77-notes.md`.

## Scope

`internal/store/migrations/0019_*.sql`, `internal/store`, `ARCHITECTURE.md`, `API.md` if it still
mentions the columns, `CHANGELOG.md`, `ROADMAP.md`, `tasks/78-notes.md`.

## Do

1. Add one forward only migration that drops `libraries.flow_json`, `libraries.flow_source`,
   `libraries.target`, `libraries.easy`, `libraries.rule_tier` and `libraries.result_policy`.
   modernc.org/sqlite v1.34.5 carries a SQLite new enough for `ALTER TABLE ... DROP COLUMN`; prove
   it on a copy before writing the file, and if the runtime refuses, rebuild the table with the
   documented table rebuild procedure instead. Either way the migration runs once, in the existing
   transaction, and is not reversible.
2. Prove the migration on copies of a v1.11 database that holds real rows: library count unchanged,
   every library still has a profile, `PRAGMA table_info(libraries)` no longer lists the six
   columns, the server boots with no error and no legacy warning, and one real encode through a
   library still runs.
3. Prove a fresh install on an empty database still seeds the default profile and creates a library.
4. `CHANGELOG.md`: state plainly that after this migration a rollback to v1.10 cannot run a library
   (v1.11 and later are fine, they stopped reading the columns in tasks 76 and 77).
5. `ARCHITECTURE.md`: the libraries table section without the six columns, and a line saying the
   rule lives on the profile. Close the v1.12 section in `ROADMAP.md` with what shipped.
6. `tasks/STATUS.md`: row 78 in progress with the branch name, done once the PR is open, per
   `AGENTS.md`.
7. Run the full suite: `make test`, `make lint`, and `make test-integration` when ffmpeg is present.

## Acceptance

- [x] One migration file drops all six columns; the migration list shows it applied exactly once.
- [x] A v1.11 database copy migrates with the library count unchanged and every library still
      attached to a profile, and `PRAGMA table_info(libraries)` output is quoted in the notes.
- [x] One real encode through a library after the migration is quoted in the notes with the ffmpeg
      command line.
- [x] The `CHANGELOG.md` rollback warning is present.
- [x] `make test`, `make lint` and `make test-integration` green.

## Notes

Plan, migration output, row counts, the encode proof and the 5 line summary go in
`tasks/78-notes.md`.
