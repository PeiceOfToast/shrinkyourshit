# Task 70 notes: profiles table and the migration off libraries

## Plan (files, in order)

1. `internal/store/migrations/0018_profiles.sql`: `profiles` table (spec 5.1) and
   `ALTER TABLE libraries ADD COLUMN profile_id`. Forward only. Nothing dropped.
2. `internal/store/profiles.go`: `Profile` type, Create/Get/List, `SetLibraryProfile`, and
   `BackfillProfiles` (sha256 of canonical graph JSON, dedupe, seed for graphless rows).
3. `internal/store/interface.go`: the profile methods and `BackfillProfiles` on `Store`.
4. `internal/store/crud.go`: `profile_id` in the library column list, insert, update, scan
   (read as `COALESCE(profile_id, '')`), `Library.ProfileID`.
5. `internal/server/server.go` + `internal/server/libraryflow.go`: one boot step after
   `backfillFlows` that calls `BackfillProfiles` and logs one line per library. Scope note below.
6. `internal/store/profiles_test.go`: v1.10-shaped DB migration test, identical graphs share a
   profile, different graphs get two, every library gets a non null id, second run is a no-op,
   graphless library gets the seed profile.

## Decisions I made (spec gaps)

- **Where the backfill runs.** SQL cannot sha256 the graph, so it is a Go step. It runs at
  server boot, after `backfillFlows`, as the existing boot step does. This is the one place
  outside the listed scope (`server.go`, `libraryflow.go`). Store stays media-agnostic: it
  does not import `internal/flow`; the server passes the seed graph in.
- **Graphless libraries.** `backfillFlows` runs first and compiles a flow for every library
  it can. A library still without a stored graph (compile failed) gets a profile seeded from
  `flow.Default()` compiled through `TargetToFlow`, and the log line says so.
- **Dedupe key.** Spec says "same hash". I match on hash AND `tier` AND `result_policy`,
  because carrying those values onto a shared profile would otherwise change one library's
  tier or policy silently. Two libraries with the same graph but different tiers get two
  profiles.
- **Hash input.** The stored graph is re-encoded through a generic decode with `UseNumber`
  and re-marshalled (sorted keys, no whitespace) before hashing and storing. Profiles store
  that canonical text.
- **Profile names.** Library name. On a UNIQUE clash, the next free `name (2)`, `name (3)`…
- **Profile id on library write.** `UpdateLibrary` writes `profile_id` too (empty writes NULL).
  `SetLibraryProfile` is the dedicated write path, like `SetLibraryRuleTier`.

## Open question for the owner (does not block this task)

- **Result policy enum mismatch.** Spec 5.1 says profile `result_policy` is
  `replace | move | copy`. The code (`validateResultPolicy`, `ResultPolicyValues()`) uses
  `replace_if_smaller | replace | keep_both`. This task copies the library's existing value
  verbatim and adds no CHECK constraint, so the migration works either way. Task 71 (API
  validation) needs the owner to pick one set. Which should it be?

## Results

Row counts from `TestProfilesMigrationBackfillsV110Database` (v1.10 shape, 7 libraries):

| Library | Graph | Tier | Policy | Profile |
|---|---|---|---|---|
| Films | graph A | basic | replace | Films (created) |
| Films copy | graph A, reformatted | basic | replace | Films (shared) |
| shows | graph B | moderate | replace | shows (created) |
| Films moderate | graph A | moderate | replace | Films moderate (created) |
| Films keep | graph A | basic | keep_both | Films keep (created) |
| Bare | none | basic | replace_if_smaller | Bare (seeded, created) |
| shows (second) | graph C | advanced | replace | shows (2) (created, name suffixed) |

6 profiles for 7 libraries. A second run creates 0 profiles and assigns 0 libraries.

Verification:
- `go test ./... -count=1` exit 0 (all packages ok; internal/store 267.8s).
- `golangci-lint run` on internal/store and internal/server: 0 issues.
- `scripts/check-spdx.sh` exit 0.
- Web tests were not run: no web files changed.
- The server boot step (`backfillProfiles`) has no direct test. It is a thin loop over
  `BackfillProfiles` plus logging, and the store tests cover the logic.

## Summary

1. Built: `profiles` table and `libraries.profile_id` (migration 0018, forward only); profile CRUD,
   `SetLibraryProfile`, and `BackfillProfiles` (sha256 of canonical graph, dedupe on hash+tier+policy,
   seed for graphless rows, one transaction, per-library errors); boot step after `backfillFlows`.
2. Skipped: nothing from scope. Dropping `flow`, `easy`, `rule_tier`, `result_policy` is task 74.
3. Out of scope, noted for later: libraries created after boot keep a NULL profile_id until the next
   restart. Task 71 or 73 must assign one on create. The result policy enum conflict above is open.
4. Scope note: `internal/server/server.go` and `internal/server/libraryflow.go` changed, which the
   task did not list. Needed because the backfill needs the flow compiler and the logger.
5. Next task (71) should read the store `Profile` type and `BackfillProfiles` first, and settle the
   result policy values before validating them in the API.
