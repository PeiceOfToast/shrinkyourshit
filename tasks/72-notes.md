# Task 72 notes: encode page profiles

## Plan (files, in order)

1. Go, one addition: `GET /api/v1/profiles/{id}?tier=` judges the graph at a tier, like the library
   flow endpoint. Needed so the Moderate tree and Advanced canvas can show their own lock for a
   profile (the profile DTO only judged at its own tier). `internal/api/profiles.go`,
   `profiles_test.go`, `API.md`, `docs/openapi.yaml`. Done.
2. `web/src/api.ts`: profile types and calls (list, get with tier, create, update, delete,
   duplicate).
3. `web/src/flowSubject.tsx`: one hook that gives the Moderate tree and the Advanced canvas either
   a library or a profile (queries, lock, save, the save confirmation dialog, route paths).
4. `FlowTreePage.tsx`, `FlowCanvasPage.tsx`: take `subject="profile"` from a route. Library
   behaviour and tests unchanged (same URLs, same route params).
5. `TierNotices.tsx`: optional `editHref` so the read only reason links to the profile's editor.
6. `ProfilesPage.tsx`: the Profiles section (list, New, Duplicate, Edit), and `ProfileEditorPage`
   (the Basic form plus name, description, result policy, the tier control, the downgrade banner).
   Basic saves through `previewFlow({easy})` then `updateProfile`.
7. `EncodeSettingsPage.tsx`: keeps app settings, renders the Profiles section. `default_rule_tier`
   is the tier a New profile gets.
8. `App.tsx`: routes `encode/profiles/:id`, `encode/profiles/:id/tree`, `encode/profiles/:id/flow`.
9. Tests: `ProfilesPage.test.tsx` (vitest), `e2e/profiles.spec.ts` (Playwright).

## Decisions

- **Editor location.** Selecting a profile opens its editor on its own route under the Encode
  page (`/encode/profiles/:id`), not on a library page. Deep-linkable and each editor keeps its
  full page. Read as "in place" = inside the Encode settings area.
- **Tier of a profile.** The profile's tier is the editor that saves last: Basic saves tier
  `basic`, Tree saves `moderate`, Canvas saves `advanced`. The tier selector picks the editor and
  the profile's tier together, the same as the v1.10 library rule.
- **Save confirmation.** When `used_by > 0`, every save of a profile asks first and names the
  libraries from `used_by_names`. Implemented in the hook, shared by all three editors.
- **Basic form on a profile.** The Easy sliders compile through `POST /flows/preview` into a
  graph, which is then saved to the profile. The Basic editor's lock uses the profile judged at
  `basic`.

## Scope notes

- Go change above is the only backend change. It is an endpoint parameter, documented in
  `API.md` and `openapi.yaml` in the same commit (AGENTS rule 4).
- Library pages keep their behaviour. Task 73 wires the library side; task 74 deprecates the
  library's own graph and tier.

## Open gaps (do not block this task)

- No profile delete button. The spec's Do list does not ask for it and the API refuses while a
  library uses the profile. Left for a later task.
- `POST /libraries` still leaves `profile_id` NULL (task 73).
- No SSE event for profile writes. The page refreshes its own queries after each save (see
  `tasks/71-notes.md` gap 6).
- The e2e test cannot attach a profile to two real libraries because no endpoint assigns one
  before task 73. The two-library confirmation case is therefore answered by route mocks, the
  same approach as `e2e/storage.spec.ts`. The list, create and duplicate flows run against the
  real seeded server.

## Verification

- `go test ./internal/api -count=1`: ok (373 s). `TestGetProfileJudgesAtTheRequestedTier` is new.
- `go vet ./internal/api/`: clean. `golangci-lint run ./internal/api/...`: 0 issues.
  `scripts/check-spdx.sh`: exit 0.
- `make` is not installed on this VM, so the recipes were run by hand: `npm run build` (tsc and
  vite), `npm run lint` (`eslint . --max-warnings 0`), `npm run format:check` (prettier), and the
  Go steps above. Node 22 is at `/home/mb/.local/node22/bin`, Go at `/home/mb/.local/go/bin`.
- `npx vitest run`: 69 files, 531 tests, all pass. The new file is `ProfilesPage.test.tsx` (9 tests).
  `EncodeSettingsPage.test.tsx` changed: its mocks answer `/profiles`, and the section order test
  now expects a fourth heading, "Profiles".
- `npx playwright test` (full suite, after `npm run build`, `internal/webui/dist` copied, and
  `bin/shrinkyourshit` rebuilt): 148 passed, 8 skipped, and 2 failures in `profiles.spec.ts`. Both
  failures were my spec reading the "Profile created." toast with `.first()`. Fixed to match the
  save toast by text; `profiles.spec.ts` re-run alone: 11 passed, 1 skipped (the phone canvas case,
  view only below 768 px by design).

## Acceptance mapping

- Two-library confirmation: `e2e/profiles.spec.ts` "asks first, names both libraries, and saves
  once confirmed". The two libraries are answered by route mocks (see gap 1). The test proves the
  dialog names both, nothing is sent before confirm, and one PUT goes to the profile at tier basic
  when confirmed. It does not prove the server applies the change to both libraries, since no
  real library is attached to the profile in this build.
- Per-editor saves: Basic form, `ProfilesPage.test.tsx` "the Basic form saves ..." and e2e
  "New and Duplicate"; tree, `ProfilesPage.test.tsx` "the tree saves ..." and e2e "the Moderate
  tree saves"; canvas, e2e "the Advanced canvas saves" (desktop and tablet; the canvas has no
  unit test, see gap 4).
- Banner: `ProfilesPage.test.tsx` "names the steps the profile tier cannot express ..." (banner
  text, the step id, the tier that can edit it, the link to the canvas). The lock on the Basic
  form is a second test.
- Commands: see Verification above.

## Decisions taken in this task

1. **One new endpoint parameter.** `GET /profiles/{id}?tier=` judges at a tier. The DTO only
   judged at the profile's own tier, so the Moderate tree and the Basic form could not show their
   own locks. It mirrors `GET /libraries/{id}/flow?tier=`, has a test, and is in `API.md` and
   `openapi.yaml`.
2. **Editors on routes.** `/encode/profiles/:id` (Basic), `/tree` and `/flow`. The tree and the
   canvas are the same components as the library pages, given `subject="profile"` by the route.
   The library routes and their tests are unchanged.
3. **The profile's tier is the editor that saved last.** Basic saves `basic`, the tree `moderate`,
   the canvas `advanced`. The tier control on the profile is the same three way control as v1.10.
4. **Basic form on a profile.** The sliders compile through `POST /flows/preview`, then the
   profile is saved with the result.
5. **Save confirmation.** Any save of a profile with `used_by > 0` asks first and lists
   `used_by_names`. It lives in `useFlowSubject`, so every editor gets it.
6. **Duplicate and New** open the new profile. New takes the saved `default_rule_tier`.

## Open gaps (do not block this task)

1. **Two real libraries.** Nothing attaches a profile to a library until task 73. The e2e test for
   the shared profile therefore uses route mocks, as `e2e/storage.spec.ts` does. Task 73 should
   replace it with a real two-library test.
2. **Sliders do not reload.** A profile does not keep its slider values, so the Basic form always
   opens at the defaults. A banner says so. Saving replaces the profile's steps. Options: store the
   easy input on the profile, or derive the sliders from the graph. Owner decision.
3. **No profile delete button.** The Do list did not ask for one, and the API refuses while a
   library uses the profile.
4. **Canvas has no unit test.** The canvas is covered by e2e only. Its save path is the same
   `useFlowSubject.save` the tree uses, and the tree has a unit test.
5. **Library create still leaves `profile_id` NULL** (task 73), and the plugin-step and move-
   destination checks still do not run on profile graphs (task 73, `tasks/71-notes.md` gaps 1 to 3).
6. **Result policy wording.** The list says "Replace if smaller", "Always replace" and "Keep
   both". The Basic form uses the longer wording from the app settings page. The values are the
   three the API stores (task 71 owner ruling).
7. **Name race** on create and duplicate (`tasks/71-notes.md` gap 4). A duplicate name gets a 409
   that the page shows as an error toast, not a retry.

## Summary

1. Built: the Profiles section on Encode settings (list with tier, used-by count and names, result,
   last edited; New; Duplicate), the profile editor at `/encode/profiles/:id` (Basic form, name,
   description, result policy, tier switch, representability banner and read-only reason), the
   Moderate tree and Advanced canvas bound to a profile through `useFlowSubject`, the save
   confirmation that names the libraries, and `GET /profiles/{id}?tier=`.
2. Skipped: profile delete UI, slider persistence on profiles, library-side wiring (task 73), and a
   real two-library e2e (gap 1). The canvas has no unit test (gap 4).
3. Open and not blocking: gaps 1, 2 and 5 above. Gap 2 needs an owner decision.
4. Scope additions outside the listed files: `internal/api/profiles.go`, `profiles_test.go`,
   `API.md` and `docs/openapi.yaml` (the query parameter), `web/src/TierNotices.tsx` (an optional
   link target and noun), `web/src/ProfileControls.tsx`, `web/src/flowSubject.ts`, and
   `web/src/EncodeSettingsPage.test.tsx` (the mocks and the new heading).
5. Next task (73) should set `profile_id` on library create and edit, replace the route-mocked
   shared-profile e2e with a real one, run the plugin-step and move-destination checks on profile
   graphs, and remove the library Rules route's embedded builder. It should read `useFlowSubject`
   and the `?tier=` parameter.
