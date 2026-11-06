# Task 78 notes: drop the library rule columns (migration 0019)

Run: claude/haiku, branch task/78-drop-columns, from main fd97b95 (task 77 merged).

## Plan (files, in order)

1. `internal/store/migrations/0019_drop_library_rule_columns.sql`: six `ALTER TABLE libraries DROP COLUMN`
   statements. Proved first on copies (below), no table rebuild needed.
2. `internal/store/crud.go`: the library INSERT no longer names `target` (the empty literal from task 76
   goes with the column).
3. Tests that read or write the legacy columns raw:
   - `internal/store/migration_0019_test.go` (new): `migrateTo` helper (applies migrations up to a name,
     recording them in `schema_migrations`), plus the 0019 proofs.
   - `internal/store/profiles_test.go`: the v1.11 builder now stops at 0018, so the legacy columns exist
     when the rows are inserted. Assertions that read dropped columns are replaced.
   - `internal/store/tier_migration_test.go`: the 0016 test stops at 0016 and reads `rule_tier` raw
     before 0019 can drop it. The round-trip test no longer reads legacy columns.
   - `internal/server/legacy_columns_test.go`: the UPDATE of legacy columns is removed (the columns no
     longer exist). The boot assertions stay.
   - Unchanged on purpose: `storages_test.go` and `node_lifecycle_test.go` build databases at schema
     versions 0016 and 0005, where `target` and the legacy columns still exist.
4. Comments that named the dropped columns as live: `internal/flow/tier.go`, `internal/server/libraryflow.go`.
5. Docs: `ARCHITECTURE.md` (libraries line, Storage bullet), `API.md` (task 74 notes, new Task 78 section),
   `CHANGELOG.md` (upgrade warning), `ROADMAP.md` (v1.12 section closed).
6. `tasks/STATUS.md`: row 78 in progress, then done with the PR.

## Migration

```
ALTER TABLE libraries DROP COLUMN flow_json;
ALTER TABLE libraries DROP COLUMN flow_source;
ALTER TABLE libraries DROP COLUMN target;
ALTER TABLE libraries DROP COLUMN easy;
ALTER TABLE libraries DROP COLUMN rule_tier;
ALTER TABLE libraries DROP COLUMN result_policy;
```

The runtime is `modernc.org/sqlite` v1.34.5. `DROP COLUMN` worked on every copy below, so the
table-rebuild fallback in the task file was not needed. Each column is unindexed and unreferenced by any
view, trigger, foreign key or CHECK. The runner applies the file once, inside its transaction, and
`schema_migrations` records it.

## Proof 1: a real v1.11 database, migrated by the new binary

Baseline: the v1.11 code, built from `eb559e3` (main before task 76, schema through 0018). Tags on the
repo stop at v0.1.2, so this commit is the closest thing to a v1.11 release binary.

1. Setup with `login_enabled: false`, which seeded `Default (basic)`. One library `Films` at
   `/tmp/proof/lib-films`, following the default profile. The node was left stopped. No job ran on v1.11.
2. The v1.11 database, read with python sqlite3 (read only), before migration:

   ```
   libraries cols: ['id','name','path','target','watch','schedule','result_policy','created_at',
     'updated_at','easy','flow_json','flow_source','last_scan_at','file_kinds','last_scan','rule_tier','profile_id']
   libraries rows: [('48cfcc44-…','Films','/tmp/proof/lib-films','8ad7c4c7-…','easy','basic','replace_if_smaller')]
   migrations tail: ['0016_library_tier.sql','0017_storages.sql','0018_profiles.sql']
   ```

3. The copy `copy1` was booted with the new binary (`shrinkyourshit-v112`, built from this branch). After boot:

   ```
   libraries cols: ['id','name','path','watch','schedule','created_at','updated_at','last_scan_at',
     'file_kinds','last_scan','profile_id']
   libraries count: 1  (before: 1)
   0019 rows: [('0019_drop_library_rule_columns.sql', '2026-11-06 20:53:33 …')]   (exactly one)
   migration count: 19
   ```

4. `GET /api/v1/libraries` and `GET /api/v1/profiles` on the copy: byte identical to the v1.11 responses
   (`cmp` said identical). Every library keeps its profile (`profile_id 8ad7c4c7-…`).
5. Boot log: no migration error and no legacy warning. There is one `level=ERROR` line, "network storage
   needs CAP_SYS_ADMIN …". That is the mount-capability check from the storage work (tasks 79 and 80),
   which this change does not touch. The v1.11 baseline binary does not print it, because it predates
   that code.
6. Restart with a second boot (the copy was started and stopped several times): no re-run, the row count
   stays at one, and no error.

## Proof 2: one real encode through a library after the migration

Node started through `POST /api/v1/nodes/{id}/start`. Encoder: cpu (no hardware on this box).
Files: `/tmp/proof/lib-films/clip-h264.mkv` (150,389 bytes, h264 testsrc with sine, from ffmpeg lavfi).
`second-h264.mkv` was added and rescanned, `POST /api/v1/libraries/{id}/scan`.

Results (the job log download is at `/tmp/proof/job-log.txt`, outside the repo):

| File | Job status | Before | After | Codec after |
|------|-----------|--------|-------|-------------|
| clip-h264.mkv | done | 150,389 | 79,130 | hevc (ffprobe) |
| second-h264.mkv | done | 150,389 | 79,130 | hevc (ffprobe) |

Job log line: `target: mkv container, hevc video, keep_all audio`, `result policy: applied, output at …`, `encoder chosen: cpu`.

The ffmpeg command line for the second file, captured by a logging wrapper set as `SHRINK_FFMPEG_PATH`
(the job log does not print argv):

```
ffmpeg -hide_banner -nostdin -progress pipe:1 -nostats -y -i /tmp/proof/copy1/scratch/job-e950c5ff-b02d-42d2-a3aa-93f044ae5702/source.mkv -map 0:V:0 -map 0:a:0 -c:v libx265 -crf 24 -preset medium -c:a:0 copy /tmp/proof/copy1/scratch/job-e950c5ff-b02d-42d2-a3aa-93f044ae5702/output.mkv
```

The output replaced the original, as the library's `replace_if_smaller` policy says. The job log does not
print argv; the line above came from the wrapper.

## Proof 3: fresh install on an empty database

New binary, empty data dir, `POST /api/v1/setup`. Result: `GET /api/v1/profiles` lists one profile,
`Default (basic)`, tier basic. `POST /api/v1/libraries` `Fresh` got `profile_id` set to that default
profile. The schema has no six columns, and `schema_migrations` has 0019 once.
(Also covered by `TestMigration0019FreshInstall`.)

## Upgrade-path note (carried from task 76, still open)

A direct v1.10 to v1.12 upgrade (skipping v1.11) gives unassigned libraries the default profile, not
the rule they had under v1.10. Upgrading through v1.11 keeps each rule. The owner decided this in task 76
(smaller version). Not changed here. Release notes should say to step through v1.11.

## Verification

- `go test ./internal/store/ -count=1`: ok (172 s, the full package).
- `go test ./internal/store/ -run 'Migration0019|RuleTier|LibraryWrites|Backfill' -v`: all PASS.
- `go test ./internal/server/ -run V111 -count=1`: ok.
- `make test`, `make lint`, `make test-integration`: see the run results at the end of this file.

## Acceptance

- [x] One migration file drops all six columns; applied exactly once (`schema_migrations`: one 0019 row).
- [x] v1.11 database copy migrates: library count 1 before and after, every library still on its profile,
      PRAGMA output quoted above.
- [x] One real encode through a library after the migration, ffmpeg command line quoted above.
- [x] `CHANGELOG.md` rollback warning present ("Upgrade warning: rolling back to v1.10").
- [x] `make test`, `make lint`, `make test-integration` green (numbers under supervisor verification).

## Decisions taken in this task

1. Used `DROP COLUMN`, not a table rebuild. SQLite in the pinned driver accepts it on the real schema.
2. The legacy-column raw reads in tests stay only where they run before 0019 (the 0016 test, the
   0005/0015 builders). Post-0019 assertions use the store API only.
3. The server boot test lost its raw UPDATE of legacy columns. It can no longer write a column that is
   gone. The migration proofs live in the store package, which can build a real 0018 database.

## Summary

1. Built: migration 0019 drops the six legacy library columns; the insert names no `target`; the v1.11
   copy migrates and encodes through a library; docs and the rollback warning say so.
2. Skipped: a real NAS or hardware encode (CPU only here); the owner's own lab database (proof uses a
   copy of a database built by the v1.11 code).
3. Open for the owner: the direct v1.10 to v1.12 upgrade gives the default profile (task 76 notes).
4. Next: the v1.12 queue is empty after this task. Task 81 (real hardware verification of mounts) stays
   blocked on the owner.
5. Pre-existing: the built-in node prints a CAP_SYS_ADMIN error on boot here (mount feature, tasks 79/80).

## Supervisor verification (2026-11-06, run by the PM, not the agent)

The two agent runs for this task both ended while their own suite was still running (turn limit on the
wait), so the PM ran the three suites itself on the VM after stopping the leftover processes:

```
make test            EXIT=0   go: api 636s, server 588s, store 327s, worker 65s, all other packages ok
                               web: 69 files / 524 tests passed
make lint            EXIT=0   golangci-lint 0 issues, SPDX ok, eslint ok, prettier ok
make test-integration EXIT=0  go -tags=integration: api 883s, server 836s, store 465s, worker 394s
```

Housekeeping in the same commit: `make test-go` now passes `-timeout 30m`, matching the existing
`test-integration` target. With the default 10-minute deadline, `internal/api` and `internal/server`
hit `panic: test timed out after 10m0s` on this 6-core VM even with no other load (636s and 588s in the
green run above), so the deadline, not the code, was the failure. No test was skipped or weakened.
