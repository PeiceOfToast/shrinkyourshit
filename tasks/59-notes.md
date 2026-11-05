# Task 59 notes: tier storage, representability validator and endpoints

Branch: `task/59-tier-storage-and-representability` (from main bb8b1d3).

## Plan (as executed)

1. `internal/flow/tier.go`: `Tier`, `TierValues`, `ParseTier`, `RepresentableAt(g, tier)`.
   Basic = what `CompileEasyMode` regenerates; moderate = all but raw_args and plugin; advanced = all.
2. `internal/flow/tier_test.go`: the step set crossed with the three tiers, plus the basic graph rules.
3. Migration `0016_library_tier.sql`, `store.Library.RuleTier`, CRUD columns, `SetLibraryRuleTier`
   (the only tier write path; it never touches `flow_json`).
4. Setting `default_rule_tier` (default basic) in `server/settings.go`, the settings PUT body in `api/auth.go`.
5. New libraries inherit the default in `LibraryManager.Create`; a library save keeps the stored tier.
6. Endpoints: `PUT /api/v1/libraries/{id}/tier`, `GET /api/v1/libraries/{id}/flow?tier=`.
7. Tests: store migration and persistence, API downgrade byte-identity, tier endpoint, settings inheritance.
8. Docs: `API.md`, `ARCHITECTURE.md`, `docs/openapi.yaml` (hand-maintained; no generator in this repo),
   `web/src/api.ts` (types only, optional fields so existing fixtures stay valid).

## Migration result

`0016_library_tier.sql` adds `rule_tier TEXT NOT NULL DEFAULT ''` and backfills it from `flow_source`:
`easy` to `basic`, `steps` to `moderate`, `advanced` to `advanced`. A row with an empty `flow_source`
(not yet backfilled at startup) follows `LibraryFlow`'s inference: `easy` input and no stored flow is
`basic`, anything else is `advanced`.

`TestRuleTierMigrationMatchesFlowSource` builds a database with migrations 0001 to 0015 applied and six
legacy rows (easy, steps, advanced, legacy easy, legacy target-only, legacy flow without source), opens it
with the new release, and asserts each tier. All six pass.

## Decisions I made (not stated in the task or spec)

- **Basic graph rules.** Beyond the per-step rule, basic needs one `transcode_video` and an `if_codec_not`
  guard on the same codec. The easy screen always writes both, so a graph without them is not regenerable.
  Reported at path `graph`.
- **health_check, skip_if_smaller, rename, move** are not representable in basic. The spec's basic list
  does not include them and `CompileEasyMode` never writes them. Moderate expresses them.
- **Message names the lowest tier** that can edit the step (moderate, or advanced for raw_args and plugin).
  Task 62's banner can use it.
- **Path** of each entry is `nodes[i].type`, the convention `RawArgsRefusals` already uses.
- **Store default.** `CreateLibrary` stores an empty `RuleTier` as `basic`. The server sets the tier
  from the setting before calling it, so the default is only a fallback for direct store use.
- **`RepresentableAt` is not a gate.** `PUT /libraries/{id}/flow` still accepts any valid graph whatever the
  library's tier (see the open question below).

## Open question for the owner (not blocking this task)

Spec section 2, decision 4 says "only a tier that can express a step may edit it". This task only adds the
data, the check and the read/downgrade endpoints. Enforcing it on `PUT /libraries/{id}/flow` would refuse
step-stack saves on new libraries, which default to basic, and break the current step-stack UI until task 62.
I left the PUT unchanged. Should the enforcement land with the tier editors (60 to 62), or does the owner
want it here? Tasks 60 to 62 should confirm which.

## Deviations from the task file

- The task lists `internal/storemigrations.go` to modify. The real file is `internal/store/migrations.go`,
  and it needed no change: the embed glob picks up `0016_library_tier.sql` automatically.
- `internal/server/libraryflow.go` and `internal/server/dto.go` were listed. Only `dto.go` (`rule_tier` in
  the library DTO) changed. The tier logic lives in `library.go` next to `Create` and `build`, which is
  where the default and the save rule belong.
- `internal/api/handlers.go` was not changed; the settings PUT body is in `internal/api/auth.go`, where
  `default_video_codec` is read.
- `internal/api/flows.go` got a `gofmt` pass, which also reformatted a pre-existing struct literal
  (`previewResponse`). No behaviour change.
- No existing test assertion was changed.
- `gofmt` reformatted the existing alignment in the struct literals I touched (`dto.go`, `settings.go`,
  `store.go`) because my added fields changed the column widths. No behaviour change.
- OpenAPI: no generator exists (`docs/openapi.yaml` is hand-maintained and embedded). I edited it by
  hand. `TestOpenAPI*` in `internal/api` passes the route anti-drift check.

## Checks

- `go test ./internal/flow ./internal/store ./internal/api` new tests: pass (see the run below).
- `make test` full run, `make lint` (golangci-lint 0 issues, `go vet` clean), web `tsc --noEmit`, `npm run lint`,
  `npm run format:check`, `npm run test` (62 files, 444 tests): pass.
- Playwright e2e not run: no UI or page change in this task.

## Summary

- Built: rule tier storage (migration 0016, backfilled from flow_source), `flow.RepresentableAt` for the
  three tiers, `PUT /libraries/{id}/tier`, `GET /libraries/{id}/flow?tier=`, `default_rule_tier` setting
  inherited by new libraries, and the byte-identical downgrade proof.
- Skipped: enforcement of the tier on `PUT /libraries/{id}/flow` (see open question); all UI (tasks 60 to 62).
- Next task (60) should know: the library DTO carries `rule_tier`, `RepresentableAt` gives the list the
  Moderate editor needs, and `PUT /libraries/{id}/tier` changes only the tier column, so the editors
  must not call a graph save just to change tier.
- Next task should also know: basic regeneration requires the if_codec_not guard and one encode, so a
  basic editor that drops the guard produces a graph that reads back as unrepresentable.
- Next task should confirm the enforcement question above before building the editors.
