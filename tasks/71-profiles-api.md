# Task 71: Profiles API with usage counts and guards

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 5.3 and 5.4. `internal/server/libraryflow.go` for the flow endpoints being superseded, and
`internal/flow/tier.go` for `RepresentableAt`.

## Scope
Create `internal/api/profiles.go`, `internal/api/profiles_test.go`.
Modify `internal/api/api.go`, `internal/api/openapi.go`, `internal/server/dto.go`.

## Do
1. CRUD: `GET/POST /api/v1/profiles`, `GET/PUT/DELETE /api/v1/profiles/{id}`,
   `POST /api/v1/profiles/{id}/duplicate`.
2. Each profile response carries `used_by` (count) and `used_by_names`.
3. Delete is refused with 409 while any library references the profile.
4. Representability is checked at the profile, not the library: a response carries
   `unrepresentable` for the profile's own tier, reusing v1.10's validator unchanged.
5. Validation reuses the existing graph validator; a bad graph is a 400 with the field errors the
   editor already knows how to display.
6. Seed: a fresh install gets one profile named for the default tier so the library form always
   has something to select.

## Acceptance
- [x] Deleting a referenced profile returns 409 and changes nothing.
- [x] `used_by` is accurate after creating, repointing and deleting a library.
- [x] A duplicate gets a new id and the same graph.
- [x] `make test` and `make lint` green (run as their underlying commands, see notes; make is not installed here).

## Notes
Write the plan and a 5 line summary in `tasks/71-notes.md`.
