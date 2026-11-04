# Task 59: Tier storage, representability validator and endpoints

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec sections 4 item 8 and 6. `internal/server/libraryflow.go`, `internal/server/dto.go`,
`internal/store/migrations/0015_library_last_scan.sql`, `internal/store/migrations.go`,
`internal/store/crud.go`, `internal/store/interface.go`.

## Scope

Create: `internal/store/migrations/0016_library_tier.sql`, `internal/flow/tier.go`,
`internal/flow/tier_test.go`.
Modify: `internal/storemigrations.go`, `internal/store/crud.go`, `internal/store/interface.go`,
`internal/server/libraryflow.go`, `internal/server/dto.go`, `internal/server/api.go`,
`internal/flow/easymode.go` if a helper is needed, `web/src/api.ts` (types only), and the API tests.

## Do

1. Migration `0016_library_tier.sql`: add `rule_tier TEXT NOT NULL DEFAULT ''` to libraries and
   backfill from `flow_source` (`easy` to `basic`, `steps` to `moderate`, `advanced` to
   `advanced`). Follow the existing migration style exactly, including the idempotency guard the
   other migrations use.
2. App setting `default_rule_tier` (default `basic`), read wherever `default_video_codec` is read.
3. `internal/flow/tier.go`: `Tier` type, `ParseTier`, `RepresentableAt(g Graph, tier Tier)
   []FieldError` with the three rule sets from spec section 6, and a test per rule.
4. `PUT /api/v1/libraries/{id}/tier` accepting `{"tier": "..."}`, returning
   `{"tier": "...", "unrepresentable": [{path, message}]}`. A non empty list still returns 200:
   the downgrade is allowed, decision 4.
5. `GET /api/v1/libraries/{id}/flow?tier=basic|moderate|advanced` returns the graph plus the same
   `unrepresentable` list, defaulting to the library's own tier when the parameter is absent.
6. **No path may rewrite a stored graph because the tier changed.** Prove it: an API test that
   stores an advanced graph, downgrades the library to basic, and asserts the stored graph bytes
   are identical.
7. New libraries inherit `default_rule_tier`.

## Out of scope

No UI (60 to 62).

## Acceptance

- [ ] Migration applied to a database created by the current release leaves every library with a
      tier that matches its `flow_source`.
- [ ] `RepresentableAt` has a test per rule, three tiers times the step set.
- [ ] The downgrade test proves the stored graph is byte identical after the tier change.
- [ ] `make test` green and the OpenAPI document regenerated if the project generates one.

## Notes

Write the plan and the migration result in `tasks/59-notes.md`.
