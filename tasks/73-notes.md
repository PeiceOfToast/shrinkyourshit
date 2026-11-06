# Task 73 notes: library pages point at a profile

## Plan (files, in order)

1. Store: `Library.ProfileGraph`, a read-only join of the attached profile's graph, in every library query. `internal/store/store.go`, `crud.go`.
2. Server: `LibraryFlow` follows the profile graph when one is attached. Create and update take `profile_id`, check it (exists, plugin steps installed, move destinations outside the library folder), and refuse a flow save while a profile is attached. `internal/server/libraryflow.go`, `library.go`, `dto.go`.
3. API: body field `profile_id`, DTO field `profile_id`, the error mapping for library saves, `409 follows_profile`. `internal/api/handlers.go`, `flows.go`. Tests in `library_profile_test.go`. Docs in `API.md` and `docs/openapi.yaml`.
4. Web: `ProfileChoice.tsx` (dropdown and plain-language summary), `EasyLibraryForm.tsx` (sliders out, profile in), `LibraryEditPage.tsx` (rules summary replaced by the profile choice), `LibraryDetailPage.tsx` (inherited profile name, link, warning when none, the profile's own tier warning), `LibraryRulesRedirect.tsx` (old rule routes go to the profile editor), `App.tsx` routes, `dashboardWarnings.ts` and `DashboardPage.tsx` (one warning per library whose profile tier cannot show a step).
5. Tests: unit tests for each changed page, the redirect, the warning; Playwright `e2e/library.spec.ts`.

## Why the Go side is in this task

The task lists web files only, but two acceptance items cannot hold without the server:

- "Switching a library from one profile to another changes what the encoder does" needs the library to run the profile's graph. Before this task the library ran its own `flow_json`. `LibraryFlow` now prefers the attached profile's graph, so the scanner, re-evaluation and the job builder all follow it with no change elsewhere. Owner decision 3 (spec section 2) says the library follows the profile's flow.
- Creating a library by choosing a profile needs `profile_id` on create. Task 72 notes say that was left for this task.

Both are additive. A library without a profile still runs its own flow, so the old API keeps working.

## Decisions taken in this task

1. **Profile wins over the legacy flow.** `LibraryFlow` returns the profile graph when `ProfileGraph` is set. `flow_json`, `target` and `rule_tier` stay in place and are ignored for running, as spec 5.1 says.
2. **Move check at attach, not at profile save.** A profile is not bound to a folder, so `validateMoveDestinations` runs when a library attaches the profile. The plugin check runs at the same point (`checkProfile`).
3. **Profile-only saves.** On update, `target` or `easy` is optional when `profile_id` is sent. The stored target is kept, so the legacy columns never change under a profile save.
4. **Flow save refused while a profile is attached.** `PUT /libraries/{id}/flow` returns `409 follows_profile`. Without this, a saved flow would be stored and silently never run.
5. **Old rule routes redirect.** `/libraries/:id/rules`, `/flow` and `/tree` go to `/encode/profiles/:profileId` (with the same suffix), or to the edit page when the library has no profile. `RulesPage.tsx` is deleted, since it was the embedded builder the acceptance forbids.
6. **Default profile in the form.** The library form pre-selects the first profile in the list, which is the boot seed's profile. The user can change it.
7. **Profile name on the detail page** comes from `GET /profiles/{id}`, keyed `['profile', id]`, the same key the profile editor invalidates.
8. **Plain-language summary** is `POST /flows/preview` on the profile graph, the same server rendering as before. The detail page keys its summary on the effective graph, so a profile edit shows without a reload.

## Changed assertions

Each row: file, old, new, why.

| File | Old | New | Why |
|------|-----|-----|-----|
| `web/src/EasyLibraryForm.test.tsx` | Start posts `{path, easy:{quality:3, remove_subtitles:true, audio:2}, watch}` | Start posts `{path, profile_id:'prof-basic', watch}` | The form has no sliders |
| `web/src/EasyLibraryForm.test.tsx` | "lists one plain language line per generated step in 'what this does'" | removed; replaced by "shows the chosen profile in plain language and posts that profile" | No "what this does" panel any more; the summary is the profile's |
| `web/src/EasyLibraryForm.test.tsx` | "updates the preview lines when the sliders move" | removed | No sliders |
| `web/src/EasyLibraryForm.test.tsx` | error test renders with `initial={{path}}`, expects `invalid easy audio` | path set through a typed path inside a mounted share, expects `invalid profile` | The `initial` prop is gone |
| `web/src/EasyLibraryForm.test.tsx` | every mock omits `/profiles` | every mock includes `/profiles` | Start needs a profile |
| `web/src/NewLibraryPage.test.tsx` | heading `How much to shrink` | heading `Encode profile` | Section renamed |
| `web/src/NewLibraryPage.test.tsx` | "drops the explanation once a folder is chosen" with no profile mock | profile mock added | Start needs a profile |
| `web/src/LibraryDetailPage.test.tsx` | header `library-detail-rules` text `Rules` | `Profile` | Link now opens the profile |
| `web/src/LibraryDetailPage.test.tsx` | heading `Rules · Easy`; `library-change-rules` href `/libraries/lib-1/rules` | heading `Encode profile`; the profile name `Small files`; href `/encode/profiles/prof-1` | The summary is the inherited profile |
| `web/src/LibraryEditPage.test.tsx` | section heading `Rules` | `Encode profile` | Section renamed |
| `web/src/LibraryEditPage.test.tsx` | "summarises the easy rules and links to the Rules page" | "shows the chosen profile and links to its editor" | The rules summary is replaced |
| `web/src/LibraryEditPage.test.tsx` | describe "rules summary (task 47)": Steps, Advanced and Easy summary text | describe "encode profile (task 73)": switching the profile changes the summary and the saved `profile_id` | The old summary strings came from `rulesSummary()`, which is removed |
| `web/src/LibraryEditPage.test.tsx` | saved body has no `profile_id` | saved body includes `profile_id: 'prof-basic'` | The save now carries the profile |
| `web/src/routeTitles.test.tsx` | `/libraries/lib-1/rules` and `/flow` title `Rules · Movies` | `Edit library · Movies` | Those routes redirect; the test library has no profile |
| `web/src/StepStack.test.tsx` | describe "RulesPage step stack save" (2 tests) and fixtures | removed | Tests the deleted `RulesPage`. `StepStackEditor` tests stay |
| `web/e2e/library.spec.ts` | "tier switch (task 62)": downgrade via `library-tier-basic`, locked Basic editor on `/rules`, PUT `/libraries/{id}/flow`, upgrade back | "a library whose profile tier cannot show a step says so on its page" | The library has no tier control and no flow it owns. The downgrade banner now comes from the profile. The upgrade-back round trip is not re-covered end to end; the tier saves are covered by `profiles.spec.ts`, and the banner by `ProfilesPage.test.tsx` |
| `web/src/RulesPage.test.tsx` | whole file | deleted | Page deleted |
| `web/e2e/tree.spec.ts` (both tests) | seed `PUT /libraries/{id}/flow`, open `/libraries/{id}/tree`, expect `Tree saved.` | each test makes its own moderate profile by `POST /profiles`, opens `/encode/profiles/{id}/tree`, expects `Profile saved.`, deletes the profile at the end | `PUT /libraries/{id}/flow` now answers 409 `follows_profile` (seeded library has a profile). The profile's tree saves with the profile's toast |
| `web/e2e/flow.spec.ts` (raw args test) | seed `PUT /libraries/{id}/flow`, open `/libraries/{id}/flow`, expect `Flow saved.` | own advanced profile by `POST /profiles`, open `/encode/profiles/{id}/flow`, expect `Profile saved.`, delete at the end | Same 409, and the profile's canvas toast. The canvas-background test still uses the library route, which now redirects |
| `web/e2e/mobile.spec.ts` (audit) | `/libraries/{id}/rules` ready on testid `rules` | `/encode/profiles/{profileId}` ready on `profile-page`, settled on `tier-selector` (profile id read from `GET /libraries/{id}`) | The rules editor is the profile's. The `/flow` audit step is unchanged and goes through the redirect |
| `web/e2e/global-setup.ts` (not an assertion) | seeded library created with `target` only | created with `target` and `profile_id` = the boot seed profile | The seeded library follows its profile, as a library from the form does. Without it the seeded library has no profile, so the flow, tree and mobile audits could not reach a profile editor |

## Open gaps (not blocking this task)

1. **Profile edits do not re-evaluate files.** `PUT /profiles/{id}` changes the graph that libraries run, but it does not queue or satisfy files now. The new behaviour applies on the next scan, watch event or library save. Spec 5.3 says "changes behaviour for every library using it"; "at once" may need a re-evaluate on profile save. Owner question.
2. **The library's own result policy still runs.** `EffectivePolicy` reads `libraries.result_policy`, not the profile's. Moving the policy is task 74.
3. **Tier on the library still drives the flow API.** `GET /libraries/{id}/flow` judges at the library's own `rule_tier`, which no UI shows now. Task 74 deprecates it.
4. **Profile saves do not run the plugin-step check.** It runs when a library attaches the profile. A profile can therefore hold a plugin step that is not installed until a library uses it. The move-destination check has the same timing and is covered by a test.
5. **A `target` sent with a profile is stored and ignored.** The UI always resends the stored legacy target, which is harmless. An API client that sends a different `target` to a profile library gets no error and sees no change in what runs.
6. **Dashboard does not warn about libraries with no profile.** The library detail page does.
7. **`StepStackEditor` has no consumer outside its tests.** `RulesPage` was its only user. The component and its tests are left for a cleanup task.
8. **`FlowTreePage` and `FlowCanvasPage` still have the `library` subject code.** No route renders it now. It can go with task 74.
9. **Plugin-step attach check has no test.** The harness has no plugin service (`pluginCheck` is nil). The code path is the one save already uses.
10. **A library created through the API with no `profile_id` stays unattached.** The boot backfill (task 70) runs only at boot, so such a library runs its own stored flow until someone attaches a profile (`PUT /libraries/{id}` with `profile_id`). The form always sends one. The e2e seed now does too. Owner question: should a create without `profile_id` take the default profile?

## Verification

- `go build ./...`, `go vet ./...`: clean. `scripts/check-spdx.sh`: exit 0. `golangci-lint run` on `internal/store`, `internal/server`, `internal/api`: 0 issues.
- `go test ./internal/store ./internal/server ./internal/api -count=1`: all ok (store 245 s, server 479 s, api 512 s). This run began before `library_profile_test.go` existed and before the `openapi.yaml` edit. Both were checked separately: the five new tests pass (`go test ./internal/api -run ...`, 18 s); `go test ./internal/api -run 'OpenAPI|DocRoutes'` is ok.
- Web: `npx tsc --noEmit` clean. `npm run lint` (`eslint . --max-warnings 0`) clean. `npx prettier --check .` clean. `npx vitest run`: 69 files, 524 tests pass. Before this task (task 72 notes): 69 files, 531 tests. `RulesPage.test.tsx` and two `StepStack.test.tsx` cases are gone; the redirect and dashboard tests are new.
- `npm run build`: ok. `internal/webui/dist` copied and `bin/shrinkyourshit` rebuilt by hand (no `make` on this VM).
- `npx playwright test e2e/library.spec.ts`: 12 passed (3 viewports × 4 tests).
- `npx playwright test` (full, after the last UI change, 1.7 min): **153 passed, 8 skipped, 0 failed**. The skips are the phone-only canvas and raw-args cases, and the file-table cases that need ffmpeg in the seed. The first full run had 15 failures in flow, tree and mobile. They came from the unattached seed library and from the old library routes. The second run also caught a 24 px "Encode settings" link on the form (fixed, `linkTouch`).

## Acceptance mapping

- Playwright creates a library by choosing a profile and a folder, and the detail page shows the profile name: `e2e/library.spec.ts` "creates a library by choosing a profile and a folder, then switching the profile changes its summary". The folder is picked through the server's folder picker, walked from `/` to the seeded library's folder.
- Switching a library's profile changes the summary: same test, the plain lines change after the save. The encoder-level change is task 75's job. The Go test `TestLibraryRunsItsProfileGraph` shows the library's effective flow changing when it switches and when the profile is edited.
- No library page renders an embedded encode builder: `RulesPage` deleted; `/libraries/:id/rules`, `/flow`, `/tree` redirect; the form and edit page have no sliders (`LibraryEditPage.test` asserts the slider test ids are absent; `EasyLibraryForm.test` covers the form).
- `npm run build` and `npx playwright test` green: see Verification.

## Summary

1. Built: the profile picker and plain-language summary on the library form and edit page; the inherited profile name, its own tier warning and a link to its editor on the detail page; a dashboard warning per library whose profile tier cannot show a step; the old rule routes redirect to the profile editor. Server: a library runs its profile's graph, `profile_id` on create and update, attach-time checks, `409 follows_profile` on flow saves. Docs and Go tests for each.
2. Skipped: result policy and tier on the library (task 74), profile re-evaluation on save (gap 1, owner question), the plugin check on profile save (gap 4), removing dead library-subject code (gaps 7 and 8).
3. Open owner question: should a profile save re-evaluate the libraries that use it (gap 1)? Not blocking.
4. Scope additions outside the listed files (also `ARCHITECTURE.md`, for the `profile_id` field and the profile record, as AGENTS rule 5 asks; `web/e2e/global-setup.ts`, `tree.spec.ts`, `flow.spec.ts`, `mobile.spec.ts`): `internal/store` (join column), `internal/server` (`libraryflow.go`, `library.go`, `dto.go`), `internal/api` (`handlers.go`, `flows.go`, the new test file), `API.md`, `docs/openapi.yaml`, `web/src/ProfileChoice.tsx`, `web/src/LibraryRulesRedirect.tsx` (+ test), `web/src/queries.ts` (`useProfiles`), `web/src/dashboardWarnings.ts` (+ tests), `web/src/DashboardPage.tsx`, `web/src/api.ts`, `web/src/App.tsx`. Each is needed for an acceptance item or for the redirect.
5. Next task (74) should move `result_policy` and the tier onto the profile in the running path (gaps 2 and 3), deprecate `libraries.flow`, `easy` and `rule_tier`, and remove the `library` subject code (gap 8). It should read `LibraryFlow` and `checkProfile` first.
