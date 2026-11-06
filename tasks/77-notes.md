# Task 77 notes: remove the deprecated library API surface

Run: claude/haiku, branch task/77-remove-api-surface, from main 6dd6761 (task 76 merged).

## Plan (files, in order)

1. `internal/api/handlers.go`: drop `target`, `easy`, `result_policy` from `libraryBody`; drop
   `deprecatedField`, `libraryReply`, `newLibraryReply`; create and update reply `server.NewLibraryDTO`.
2. `internal/api/api.go`: remove the `PUT /api/v1/libraries/{id}/tier` route.
3. `internal/api/flows.go`: remove `putTierBody` and `putLibraryTier`.
4. Tests (`internal/api`): drop the tier-route test and its helper in `tier_test.go`; replace
   `easy_test.go` with `removed_fields_test.go` (git mv); fix fixtures in `api_v1_test.go`,
   `filekinds_test.go`, `integration_test.go`, and the stats fixture in `easy_test.go`.
5. `docs/openapi.yaml`: remove the `/libraries/{id}/tier` path and `LibraryTier*` schemas,
   `deprecated` on the Library reply, and `target`, `easy`, `result_policy` on `LibraryParams`.
6. `web/src/api.ts`: remove `DeprecatedField`, `deprecated?` on `Library`, and the three legacy
   fields on `LibraryBody`. Reads of `Library.target|easy|result_policy` stay.
7. `web/src/LibraryEditPage.tsx`: stop resending the stored rule; body is name, path, watch,
   file_kinds, profile_id. Update its tests.
8. `API.md`: new "Task 77 (v1.12)" section; remove the tier row from the task 14 table; mark the
   task 74 deprecated-list and tier-route lines as removed.
9. `ARCHITECTURE.md`: no field names there need changing beyond the legacy-columns note (those
   are the schema, changed in task 78).

## Decoder decision

Request bodies go through `decode[T]` in `internal/api/handlers.go`: a plain `json.Decoder`, no
`DisallowUnknownFields`. Every API body in the package works this way, so an unknown field is
silently ignored. The removed fields are therefore ignored exactly like any other unknown field:
the save succeeds, the library keeps its profile, and the reply has no `deprecated` key. The only
`DisallowUnknownFields` uses are in `internal/flow/graph.go` (stored graph documents) and
`internal/plugin/runner.go` (plugin output), not request bodies, so no new convention is needed.

## Grep evidence (web client, after the change)

- `grep -rn "/tier\|deprecated\|DeprecatedField" web/src web/e2e`: only `tierModel` import paths
  (a local module, not the route). No hits for the route or the removed field.
- No library save in `web/src` or `web/e2e` sends `target`, `easy` or `result_policy`
  (`LibraryEditPage.tsx` and `e2e/library.spec.ts` `attachProfile` fixed in this task).
- Go: `putLibraryTier`, `putTierBody`, `deprecatedField`, `libraryReply`, `newLibraryReply` are gone.

## Verification (VM, Go at /home/mb/.local/go/bin; no make)

- `go test ./internal/api/ -count=1 -timeout 30m`: ok (370 s), full package.
- Targeted run also covers the OpenAPI route-table test in both directions.
- `go vet ./...`: clean. `golangci-lint run`: 0 issues. `scripts/check-spdx.sh`: ok.
- `web`: `npm run lint` clean, `npm run format:check` clean, `tsc --noEmit` clean,
  `npx vitest run`: 69 files, 524 tests passed.
- `make test` and `make lint` were not run (no make on this VM); the steps above are the same commands.
- Not run: `go test ./...` for internal/server and internal/store (not touched by this change),
  and Playwright e2e (the `attachProfile` helper edit is not exercised by the unit suite).

## Decisions

- Removed request fields are silently ignored (see the decoder decision above).
- The tier route returns `404 not_found` from the `/api/` catch-all.
- The save reply no longer has `deprecated`; the read fields stay.
- `easy_test.go` kept its name: it still holds the stats and jobs tests.

## Summary

1. Built: `POST` and `PUT /api/v1/libraries` no longer accept or report `target`, `easy`,
   `result_policy`. The `deprecated` reply list and `PUT /libraries/{id}/tier` are removed. The web
   client, OpenAPI and API.md match.
2. Skipped: the schema and the six columns (task 78); Playwright e2e run; `make` itself.
3. Open for the owner: none new. Task 78 is still the next item.
4. Task 78 should know: no Go or web code reads or sends the removed fields now, so the migration
   changes only the schema. The `libraries` columns `target`, `easy`, `result_policy` are still
   present until 0019.
5. Pre-existing and untouched: `gofmt -l` lists `internal/api/installer_test.go`.
