# Task 70: Profiles table and the migration off libraries

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 5.1 and 5.2, and the v1.10 tier work in `internal/flow/tier.go` and
`internal/server/libraryflow.go`.

## Scope
Create `internal/store/migrations/0018_profiles.sql`, `internal/store/profiles.go`,
`internal/store/profiles_test.go`.
Modify `internal/store/migrations.go`, `internal/store/interface.go`, `internal/store/crud.go`
(library read to include profile_id).

## Do
1. `0018_profiles.sql`: the profiles table per spec 5.1 plus `ALTER TABLE libraries ADD COLUMN
   profile_id`. Forward only.
2. Backfill in the migration or in a one time boot step, whichever the existing pattern supports
   best: for each library with a stored graph, hash the canonical graph JSON with sha256, reuse an
   existing profile with the same hash, otherwise create one named after the library. Carry
   `rule_tier` to `tier` and `result_policy` to the profile. Set `profile_id`.
3. A library with no graph gets a seeded default profile and a log line naming the library.
4. Do not drop `libraries.flow`, `easy`, `rule_tier` or `result_policy` in this task.
5. A test builds a v1.10 shaped database, runs the migration, and asserts profile count, hashes
   and every library's `profile_id`.

## Acceptance
- [ ] Two libraries with identical graphs end up sharing one profile, proven by a test.
- [ ] Two libraries with different graphs get two profiles.
- [ ] Every library ends with a non null `profile_id`.
- [ ] Migration is idempotent and does not lose data on a second run.
- [ ] `make test` green.

## Notes
Write the plan and the resulting row counts in `tasks/70-notes.md`.
