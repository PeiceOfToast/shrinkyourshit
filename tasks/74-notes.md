# Task 74 notes: result policy on the profile, deprecate library rule columns

## Plan (files, in order)

1. Store: join the profile's `tier` and `result_policy` onto every library read (`Library.ProfileTier`, `Library.ProfileResultPolicy`), next to `ProfileGraph`. `internal/store/store.go`, `crud.go`.
2. Server, reads: `LibraryFlow` takes only the profile graph and returns `ErrNoProfile` without one. The legacy target/flow reader moves to `legacyLibraryFlow`, used only by the boot migration (`backfillFlows`). `EffectivePolicy`, the queue's completion policy, the tier and the DTO read the profile. `internal/server/libraryflow.go`, `queue.go`, `dto.go`.
3. Server, writes: `LibraryParams` loses `TargetJSON`, `EasyJSON`, `ResultPolicy`. Save attaches a profile (explicit `profile_id`, else the default profile; an update without one keeps the current profile). Attach mirrors the profile's graph, tier, policy and compiled target onto the legacy columns, so a rollback to v1.10 reads the same rule. `SetRuleTier` and the legacy tail of `UpdateFlow` are removed. `internal/server/library.go`, `libraryflow.go`.
4. API: library create and update ignore `target`, `easy`, `result_policy` and answer a `deprecated` list. `PUT /libraries/{id}/tier` and `PUT /libraries/{id}/flow` answer `409 follows_profile`. `GET /libraries/{id}/flow` judges at the profile's tier. `internal/api/handlers.go`, `flows.go`.
5. Boot: one warn line when any library row still holds a non empty legacy field (`flow_json`, `flow_source`, `target`, `easy`, `rule_tier`, `result_policy`), naming the count.
6. Tests: update the Go tests that depended on the legacy path. Each changed assertion goes in the table below.
7. Docs: `ARCHITECTURE.md`, `API.md`, `ROADMAP.md` (follow up removal entry), `docs/openapi.yaml` if it describes the library body.
8. Web: `web/src/api.ts` types for the `deprecated` list. Web behaviour unchanged unless a unit test needs it.

## Grep evidence (before the change, main at 9f5483a)

Command: `grep -rn "TargetJSON\|RuleTier\|ResultPolicy\|FlowJSON\|EasyJSON\|FlowSource\|LibraryFlow(\|EffectivePolicy" internal --include=*.go | grep -v _test`

Readers of the legacy columns as truth before this task:
- `internal/server/libraryflow.go` `LibraryFlow`: read `FlowJSON` then `TargetJSON`/`EasyJSON`. `EffectivePolicy`: `lib.ResultPolicy`.
- `internal/server/queue.go` completion: `lib.ResultPolicy`.
- `internal/server/library.go` `SetRuleTier`, `logFlowTargetMismatch`: `RuleTier`, `FlowJSON`, `TargetJSON`.
- `internal/server/dto.go`: `EasyJSON`, `RuleTier`, `ResultPolicy`, `FlowSource`.
- `internal/api/flows.go` `libraryTier`: `RuleTier`.

Migration-only readers (kept, they feed the boot migration and the store backfill): `backfillFlows`, `store/profiles.go` `BackfillProfiles` (`RuleTier`, `ResultPolicy`, graph hash).

The grep result after this task is pasted in the verification section.

## Decisions taken in this task

1. **Legacy request fields are ignored, not translated.** Spec 5.1 and the task file say "accept and ignore with a deprecation note". `target`, `easy` and `result_policy` on create and update are ignored. The reply carries `deprecated: [{field, message}]` for each one sent. An old client that sends no `profile_id` gets the default profile, and the note names it.
2. **`profile_id` is never cleared by a save.** Task 73 said an empty value detaches. Now an empty or absent `profile_id` keeps the library's current profile, and only a library with none gets the default. Otherwise an old client's PUT would silently drop a library back to a legacy flow that no longer runs. Changes the task 73 test `TestProfileOnlyUpdateKeepsTheStoredRule` and the detach case.
3. **Tier and flow saves on a library are refused.** `PUT /libraries/{id}/tier` answers `409 follows_profile` (the tier belongs to the profile). `PUT /libraries/{id}/flow` keeps its 409, now for every library. Validation still runs first, so a bad graph is still `400 invalid_flow`.
4. **Legacy columns mirror the profile on save.** `flow_json`, `flow_source`, `target`, `rule_tier` and `result_policy` are written from the attached profile on every library save, so a rollback to v1.10 sees the same rule as of the last save. Nothing new reads them. A profile edit does not refresh them until the library is saved again (noted, not fixed).
5. **Default profile on create.** The first profile in `ListProfiles` order (the one the form pre-selects). If the table is empty, the default seed is created first.
6. **`flow_source` is derived from the tier** (basic → easy, moderate → steps, advanced → advanced) in the DTO, since a profile has no source of its own.

## Changed assertions

Each row: file, old, new, why.

| File | Old | New | Why |
|------|-----|-----|-----|
| `internal/server/library_test.go` `TestLibraryCreateValidation` | create without target uses the default Target; bad target `mpeg5` refused; bad result policy refused | create without profile follows the default profile; unknown `profile_id` refused (`no such profile`); bad result policy case removed | The target and result policy are no longer manager params (the policy is the profile's, and the profile was validated at create) |
| `internal/server/dto_test.go` `TestLibraryDTOEasy` | stored `EasyJSON` wins; easy-shaped `TargetJSON` describes; hand-edited target gives null | the same three cases on the profile's graph; a stored `EasyJSON` is not read | The DTO's easy is derived from the profile graph (task 74) |
| `internal/server/scanner_test.go` `TestScannerNewTargetQueuesFromCacheOnScan` | writes `TargetJSON`/`FlowJSON` to the row, then scans | attaches an h264 profile by store write (`SetLibraryProfile`), then scans | The rule is the profile's; the store write keeps the test's "scan judges it" intent, with no re-evaluation |
| `internal/server/scanner_test.go` `TestScannerNonMediaKindQueuesKindAgnosticWork` | `UpdateFlow` with rename + move | `Libraries().Update` with a profile carrying that graph | `UpdateFlow` is refused for every library (409) |
| `internal/server/queue_worker_test.go` `claimJobPolicy` | `ResultPolicy: policy` on the library | the policy is the profile's (`targetProfile(..., policy)`) | Completion reads the profile's policy |
| `internal/server/transfer_test.go` `TestOutputExtFollowsTargetContainer` | `UpdateLibrary` with an mp4 `TargetJSON` | `SetLibraryProfile` to an mp4 profile | Output container comes from the profile graph |
| `internal/api/easy_test.go` `TestCreateLibraryWithEasy` | easy input compiled and stored; `easy` round-trips | `TestCreateLibraryIgnoresLegacyEasy`: 201, default profile, target is the default, `easy` named in `deprecated` | The easy input is ignored (spec 5.1, task file) |
| `internal/api/easy_test.go` `TestCreateLibraryEasyValidation` | target+easy 400; easy out of range 400 | `TestCreateLibraryIgnoresInvalidLegacyFields`: 201, `target`, `easy`, `result_policy` (and `profile_id`) named | Legacy fields are ignored, so their validation no longer runs |
| `internal/api/easy_test.go` `TestLibraryEasyRoundTripViaAPI` | easy save persists easy state; target-only save clears it; update with neither rule is 400 | `TestLegacyEasyOnUpdateIsIgnored`: the rule and profile do not change, `easy` named; a save with name and path only is 200 and keeps the profile | Nothing is stored from the legacy fields; an empty `profile_id` never detaches |
| `internal/api/tier_test.go` (whole file) | `PUT /libraries/{id}/tier` 200 with changed tier, downgrade lists unrepresentable, empty list case | `TestPutLibraryTierIsRefused` (409 `follows_profile`, nothing changes); `TestProfileTierDowngradeKeepsGraphByteIdentical` and `TestProfileTierChangeReachesTheLibraryFlow` (the tier moves on the profile); `TestGetFlowTierQuery` on the profile | The tier belongs to the profile (spec 5.1, 5.4) |
| `internal/api/tier_test.go` `TestNewLibraryInheritsDefaultRuleTier` | new library has `default_rule_tier`; setting change does not move it | `TestNewLibraryFollowsFirstProfileTier`: no profile named follows the first profile, its tier is that profile's | The setting seeds the first profile only |
| `internal/api/flow_test.go` (13 saves) | `PUT /libraries/{id}/flow` 200 | `putFlowViaProfile`: the same graph saved on the library's profile | The library flow route is refused (409) for every library |
| `internal/api/flow_test.go` `TestFlowHealthCheckAndMoveRoundTrip` | a move destination inside the library refused on `PUT /flow` (400) | refused on library attach (`POST /libraries` with that profile, 400 `invalid_flow`, `nodes[3].params.destination`) | Move check runs per library at attach (task 73 decision 2) |
| `internal/api/flow_test.go` `TestLegacyLibraryFlow` | a row with no stored flow is compiled from its target and answers 200 | `TestLibraryWithoutProfileHasNoFlow`: no profile answers 409 `no_profile` | Legacy rows are not a rule any more |
| `internal/api/flowplugin_test.go` `TestFlowPluginStepSaveRefusals` | `PUT /flow` with a missing, disabled or wrong-type plugin refused 422 | `attach` helper: `POST /libraries` with that profile refused 422; enabled attach 201 | The plugin check runs on attach; a profile save does not run it (task 73 gap 4, still open) |
| `internal/api/flowplugin_test.go` `TestFlowPluginStepDispatch` | `UpdateFlow` to save the flow | `Libraries().Update` with a profile carrying it (enabled at attach, then disabled) | `UpdateFlow` is refused |
| `internal/api/library_profile_test.go` `TestProfileOnlyUpdateKeepsTheStoredRule` | profile-only save keeps `target`; a save without profile is 400 | `TestProfileOnlyUpdateSwitchesAndKeepsTheProfile`: profile-only save switches; no profile named keeps the current one (200) | An empty `profile_id` no longer detaches (decision 2) |
| `internal/api/library_profile_test.go` `TestLibraryProfileMustExist` | refused library left with empty `profile_id` | refused library keeps its profile (the same id as before) | Every library has a profile now |
| `internal/api/api_v1_test.go` `TestLibraryEndpointsCRUD` | `PUT` with a changed `target` gives the new codec | `PUT` with `profile_id` of an h264 profile gives the new codec (via the DTO's derived target) | The target field is ignored |
| `internal/api/worker_test.go` `TestWorkerSpecCarriesMove` | `UpdateFlow` after create | the move graph is on the profile the library is created with | `UpdateFlow` is refused |
| `internal/api/worker_test.go`, `internal/worker/*_test.go` (helpers) | `TargetJSON`/`ResultPolicy` on `LibraryParams` | `ProfileID: targetProfile(...)` | Those params are gone; the helper stores a profile with the same graph |
| `internal/store/tier_migration_test.go` | `SetLibraryRuleTier` tests | removed; `TestCreateLibraryDefaultsRuleTier` writes the column directly | The store method is removed (no caller) |
| `web/src/api.ts` | `putLibraryTier` and `TierResponse` | removed; `DeprecatedField` added; `LibraryBody` fields marked deprecated | No caller left (task 62's tier control moved to the profile); the endpoint is 409 |

Helper files added: `internal/server/profile_helper_test.go`, `internal/api/profile_helper_test.go`, `internal/worker/profile_helper_test.go` (`targetProfile`, `graphProfile`, `putFlowViaProfile`). Each is a test-only file with the SPDX header.

## Open gaps and questions (do not block this task)

1. **Old clients get the default profile, not their target.** Spec 5.1 and this task say "ignore". A client that sends a custom `target` and no `profile_id` now runs the default profile. The reply says so in `deprecated`, but the rule it asked for does not run. The safer alternative is to create or reuse a profile from the target (the spec 5.2 migration rule). Owner question: keep "ignore" or translate?
2. **The boot warning fires on every boot.** `rule_tier` and `result_policy` are never empty (defaults), and the snapshot keeps `flow_json` filled. So `logLegacyColumns` warns until the v1.12 drop. Owner question: is that the intended signal, or should the warning count only rows where a legacy column differs from the profile?
3. **`target` is in the v1.12 removal list.** The spec's task 74 list names `flow`, `easy`, `rule_tier` and `result_policy`. `target` is added because the API field is deprecated too. Owner to confirm (ROADMAP v1.12).
4. **The snapshot goes stale on a profile edit.** `PUT /profiles/{id}` changes the graph the library runs, but the legacy columns change only when that library is next saved. A rollback to v1.10 in that window runs the older rule. Task 73 gap 1 (profile edits do not re-evaluate files) is still open too.
5. **Profile save still skips the plugin check and the move-destination check.** Both run when a library attaches the profile (task 73 gap 4). The task 74 plugin and move refusal tests therefore assert on the attach path.
6. **The web edit page still resends the legacy rule.** `web/src/LibraryEditPage.tsx` sends `target` (or `easy`) on every save so the old field round-trips. The server ignores it and reports it in `deprecated`. Removing the resend needs four `LibraryEditPage.test.tsx` cases changed; left for a cleanup, since the save is correct as it stands.
7. **No dead `UpdateFlow` body.** `UpdateFlow` keeps validation, then answers 409 for every library. Its callers are the 409 tests and the `flowSubject` library path (task 73 gap 8, no route renders it). Removing it is part of v1.12.
8. **Pre-existing gofmt differences** in `internal/server/scanner.go`, `internal/server/harness_test.go` and `internal/worker/integration_test.go`. They are on `main` at 9f5483a; not touched here (outside scope).

## Verification

Final code (after the last test change); each line is a run on this VM.

- `go vet ./...` clean. `make lint`: exit 0 (go vet, golangci-lint 0 issues, spdx, eslint, prettier).
- `go test ./internal/api/ -timeout 30m`: ok (468 s). The default 10 minute timeout tripped once under CPU load from a parallel integration run, not from a failure; the package takes 8 to 9 minutes alone, the same as before this task (task 73 notes: 547 s).
- `go test ./internal/store/`: ok (242 s). `./internal/server/`: ok (494 s).
- `go test ./internal/worker/`: ok. Other packages (`cmd`, `deploy`, `internal/config`, `discovery`, `encoders`, `flow`, `mount`, `plugin`, `probe`, `secret`, `update`): ok.
- Integration (`go test -tags=integration`, ffmpeg 7.0.2 on this VM): `./internal/server/` ok (586 s), `./internal/api/` ok (640 s), `./internal/worker/` ok on the final code (250 s). Seven worker integration tests failed in the first integration run, because their helper still built the rule through `UpdateFlow`; the helper now creates the library with a profile carrying the graph, and all seven pass.
- Web: `npm run lint` and `prettier --check .` clean; `npx vitest run`: 69 files, 524 tests pass.
- `make e2e` (web build, binary, Playwright): 156 passed, 5 skipped, exit 0.
- `make test`: see the line below.

Grep, after the change, of the legacy column names in non-test Go outside `internal/store`:

```
internal/server/libraryflow.go  legacyLibraryFlow (boot migration only), backfillFlows (migration), logLegacyColumns (boot warning)
internal/server/library.go      attachProfile: snapshot writes (FlowJSON, FlowSource, RuleTier, ResultPolicy, EasyJSON, TargetJSON)
internal/server/dto.go          compat fields only: RuleTier from ProfileTier, ResultPolicy from EffectiveResultPolicy
internal/api/*                  none (the library rule is read through server.LibraryFlow, server.LibraryTier, server.EffectiveResultPolicy)
internal/worker/*               none
```

## Summary

1. Built: the library's rule is its profile's graph, tier and result policy (store join, `LibraryFlow`, `EffectiveResultPolicy`, the queue's completion policy, the job spec, the DTO). Library saves ignore `target`, `easy` and `result_policy` and report them in `deprecated`. A save with no `profile_id` takes the default profile on create and keeps the current one on update. `PUT /libraries/{id}/tier` answers 409 `follows_profile`; `PUT /libraries/{id}/flow` answers 409 for every library. The legacy columns are a rollback snapshot, and a boot warning names them. Docs: `API.md` task 74, `ARCHITECTURE.md`, `docs/openapi.yaml`, `ROADMAP.md` v1.12 entry.
2. Skipped: removing the legacy columns and fields (v1.12, per the task), the web resend cleanup (gap 6), translating old clients' targets into profiles (gap 1, owner question), the plugin and move checks on profile save (gap 5, task 73 gap 4).
3. Open owner questions: gap 1 (ignore versus translate old clients' targets), gap 2 (the boot warning fires every boot), gap 3 (`target` in the removal list).
4. Next task (75, verify wave) should cover: a library's profile switch changes what runs on real hardware (the encoder-level behaviour), the rollback snapshot after a profile edit (gap 4), and whether an old client's `target` should be translated (gap 1).
5. Scope additions outside the listed files: `internal/server/server.go` (boot order and the warning), `internal/server/transfer.go` and `internal/api/worker.go` (the last legacy readers of the job path), `internal/store/tier_migration_test.go` (the removed store method's test), `internal/server/profile_helper_test.go`, `internal/api/profile_helper_test.go`, `internal/worker/profile_helper_test.go`, `web/src/api.ts`, `API.md`, `ARCHITECTURE.md`, `ROADMAP.md`, `docs/openapi.yaml`.
