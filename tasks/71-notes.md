# Task 71 notes: profiles API with usage counts and guards

## Owner ruling (applied)

Result policy values: keep the set the code already validates (`validateResultPolicy`,
`ResultPolicyValues()`): `replace_if_smaller | replace | keep_both`. Spec 5.1's
`replace | move | copy` is not adopted. Engine unchanged. The API accepts only the stored
set, and `API.md` says so. Spec 5.1 still lists the old set; the owner decided, so the spec is
not edited here.

## Plan (files, in order)

1. `internal/store/profiles.go` and `interface.go`: `UpdateProfile`, `DeleteProfile`, and
   `ErrProfileInUse`. The delete is one statement with `NOT EXISTS (SELECT 1 FROM libraries
   WHERE profile_id = ?)`, so a library cannot be assigned between the check and the delete.
2. `internal/server/dto.go`: `ProfileDTO` and `NewProfileDTO`. `unrepresentable` comes from
   `flow.RepresentableAt` at the profile's own tier, unchanged.
3. `internal/api/profiles.go`: the six handlers. Writes go through `a.st`, the same as
   `integrations.go`.
4. `internal/api/api.go`: six routes behind `requireAdmin`.
5. `internal/api/profiles_test.go`: one test per acceptance item, plus CRUD, validation and the
   seed.
6. Docs: `API.md` (Task 71 section), `docs/openapi.yaml` (paths, schemas, tag). The anti-drift
   test `TestOpenAPIMatchesRouteTable` requires the YAML to list every route.
7. Boot seed: `internal/server/libraryflow.go` `seedDefaultProfile`, called from `server.go`
   after `backfillProfiles`.

## Scope notes (files outside the task's list)

- `internal/store/profiles.go`, `internal/store/interface.go`: the task needs update and delete,
  and the store has neither. Added as small methods; no other store behaviour changed.
- `internal/server/libraryflow.go`, `internal/server/server.go`: the fresh install seed (`Do` item
  6) runs at boot, next to the backfill, because the API never runs at boot. Task 70 set the same
  precedent.
- `docs/openapi.yaml`, `API.md`: AGENTS.md rule 4, and the anti-drift test.
- `internal/api/openapi.go` is unchanged: it only serves the YAML, and the YAML is where routes
  are listed.

## Decisions I made

- **Delete guard.** The 409 check and the delete are one SQL statement. The message names the
  libraries (`profile is used by 2 libraries: A, B`), using the same `profileUsers` read as
  `used_by`.
- **`used_by`** is one `ListLibraries` read per request, shared by every profile in that request.
- **Representability is informational.** A save is not refused when the profile's tier cannot
  express its graph (the v1.10 downgrade rule, spec 5.4). The reply lists the steps.
- **Validation order.** Name, then tier, then policy, then graph (`flowErrors`: `flow.Validate`
  plus the raw-args switch, the same as the library flow endpoint), then name uniqueness.
- **Duplicate name.** `<name> (copy)`, then `(copy 2)`, `(copy 3)`.
- **Seed.** Runs only when no profile exists after the backfill. Named `Default (<default
  tier>)`, carries the default Target's graph, policy `replace_if_smaller` (the library default).

## Open questions and gaps (do not block this task)

1. **Library create still leaves `profile_id` NULL.** `POST /libraries` does not take a profile
   yet, so the used-by test assigns libraries with `SetLibraryProfile` after creating them.
   The acceptance item is met for assigned libraries. Task 73 must set the profile when a library
   is created.
2. **Plugin-step check not applied to profile graphs.** The library flow save checks each plugin
   step against the installed plugins (`LibraryManager.pluginCheck`, private). A profile save
   does not. Task 73 (or the library attach) should run it.
3. **Move-destination check not applied to profile graphs.** `validateMoveDestinations` depends
   on the library root, so it can only run when a library attaches the profile. Same owner: task 73.
4. **Name race.** The name check is a read followed by a write. The schema's UNIQUE constraint
   still holds, but a race would return 500, not 409. Low risk on a single-admin install.
5. **Seed after deleting every profile.** A user who deletes all profiles and restarts gets
   `Default (basic)` back, because the seed is "no profile exists". Intended for "the library form
   always has something to select".
6. **No SSE event on profile writes.** The `events` list in `API.md` has no profile type. Task 72
   should refresh after saves, or an event type is added with its doc change.
7. **Stored graph is not canonical.** API writes store `json.Marshal(flow.Graph)`. The backfill
   hashes canonical JSON, so an API-written profile does not match a backfill hash. The backfill
   only runs for libraries with no profile, so this is harmless today.

## Verification

- `go test ./internal/api -count=1`: ok (382s); after the final edit it also passed in the full run.
- `go test ./... -count=1`: exit 0. Every package ok (internal/api 529s, internal/server 495s,
  internal/store 252s, internal/worker 52s, others).
- `go vet ./...`: clean. `scripts/check-spdx.sh`: exit 0. `golangci-lint run` on api, server,
  store and flow: 0 issues.
- **`make` is not installed on this machine**, so `make test` and `make lint` were not run. The
  commands their bodies run were run instead: `go test ./...` (test-go), and vet, spdx and
  golangci-lint (lint-go and spdx). `lint-web` and `test-web` were not run, because no web file
  changed. Go is at `/home/mb/.local/go/bin`, not on the default PATH.
- The new tests: `TestFreshInstallSeedsOneDefaultProfile`, `TestProfileCRUDAndValidation`,
  `TestDeleteReferencedProfileReturns409AndChangesNothing`,
  `TestUsedByAccurateAfterCreateRepointAndDelete`, `TestDuplicateGetsNewIDAndSameGraph`,
  `TestUnrepresentableIsJudgedAtTheProfilesTier`. All pass.

## Summary

1. Built: profile CRUD (`GET/POST /profiles`, `GET/PUT/DELETE /profiles/{id}`, `POST
   /profiles/{id}/duplicate`) with `used_by`, `used_by_names`, `unrepresentable`; the delete guard
   (409 `profile_in_use`, atomic in the store); name conflicts (409 `name_taken`); graph validation
   (400 `invalid_flow`); the fresh install seed; API.md, openapi.yaml and tests. Owner ruling applied.
2. Skipped: no change to the engine, no library-side wiring (task 73), no profile UI (tasks 72 and
   73), no `move` or `copy` result policies (owner ruling).
3. Out of scope and open: `POST /libraries` still leaves profile_id NULL; plugin-step and
   move-destination checks are not run on profile graphs; both belong to task 73.
4. Scope additions (store methods, boot seed, openapi.yaml, API.md) are listed above; the
   anti-drift test requires the YAML, and the seed needs the boot path.
5. Next task (72) should read `ProfileDTO` and the 409 `profile_in_use` shape, refresh after
   saves (no SSE event exists for profiles), and show `unrepresentable` from the profile reply.
