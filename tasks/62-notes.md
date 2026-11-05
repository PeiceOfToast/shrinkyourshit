# Task 62 notes: tier switch, downgrade banner and read only rendering

## Plan

1. `web/src/api.ts`: `getLibraryFlow(id, tier?)` sends `?tier=`; `putLibraryTier(id, tier)`; `allow_raw_args`
   on `Settings` and `SettingsUpdate`.
2. `web/src/tierModel.ts` (new): tier labels and the lock rule (a list of unrepresentable steps locks an editor).
3. `web/src/LibraryDetailPage.tsx`: three way tier control (Basic, Moderate, Advanced) in the Rules block. It calls
   the tier endpoint, toasts, and shows the warning banner from `flow.unrepresentable` (judged at the library's own
   tier, so the banner survives a reload). The banner names each step and the tier that can edit it, and says the
   flow was not modified.
4. Read only editors: the lower tier editor locks when its tier cannot represent a step.
   - Basic (the Easy tab of `RulesPage.tsx`): sliders in a disabled fieldset, Save disabled, reason and a link to
     the Advanced editor.
   - Moderate (`FlowTreePage.tsx`): rows, add, reorder and Save disabled, reason and link.
   - Steps tab of `RulesPage.tsx` locks at moderate for the same reason (a save would drop a raw args step).
   The Advanced canvas is never locked.
5. `web/src/EncodeSettingsPage.tsx`: a new "Rule tiers" section with `default_rule_tier` (select, closed list
   validated before submit) and the `allow_raw_args` switch.
6. `web/e2e/library.spec.ts` (new): advanced graph with a `raw_args` step, downgrade to Basic, banner and locked
   Basic editor, upgrade to Advanced, canvas shows the step, API graph unchanged.

## Built

- `web/src/api.ts`: `getLibraryFlow(id, tier?)` (sends `?tier=`), `putLibraryTier`, `TierResponse`,
  `allow_raw_args` on `Settings` and `SettingsUpdate`.
- `web/src/tierModel.ts` (new): tier labels, `stepOf` (the step id from the server message, empty for
  a graph-level error), `editableIn` (the lowest tier named in the message), `isLocked`.
- `web/src/TierNotices.tsx` (new): `UnrepresentableBanner` (names each step and the tier that can edit
  it, says nothing was changed, links to the visual editor) and `ReadOnlyReason` (the lock notice on an
  editor, with a link to the visual editor).
- `web/src/LibraryDetailPage.tsx`: Basic / Moderate / Advanced `SegmentedControl` (`library-tier-*`) in the
  Rules block. A change calls `putLibraryTier`, toasts, and invalidates. The banner comes from the flow
  judged at the library's own tier, so it survives a reload.
- `web/src/RulesPage.tsx`: the Easy tab (Basic) is read only when the basic tier cannot express a step:
  the sliders sit in a disabled fieldset, Save is disabled, and `ReadOnlyReason` shows. The Steps tab does
  the same at moderate. Both locks load the flow with `?tier=`.
- `web/src/FlowTreePage.tsx`: the Moderate tree is read only when the moderate tier cannot express a step.
  Editing and Save are gated by `editable`, which now includes the lock.
- `web/src/EncodeSettingsPage.tsx`: new "Rule tiers" section with `default_rule_tier` (select; Save is off
  while the value is not one of the three tiers, with the message under the field) and the "Allow raw
  ffmpeg arguments" switch (saves at once, toast from the settings mutation). `QSVSwitchRow` is renamed
  `SwitchRow` with an `ariaLabel` prop, so the new switch is not labelled "on Intel Quick Sync".
- `web/src/settingsModel.ts`: `default_rule_tier` in the form state and the PUT body.
- `web/e2e/library.spec.ts` (new): the round trip.
- Unit tests: `web/src/tierModel.test.ts` (5 tests).

## Scope deviations and changed assertions

- `RulesPage.tsx` and `FlowTreePage.tsx` are changed although the task's Scope list does not name them. The
  acceptance item "the Basic form is disabled with a visible reason" and the spec's "lower tier editor renders
  read only" cannot be met without those pages.
- `web/src/settingsModel.ts` gains `default_rule_tier` so the Encode settings Save sends it.
- No Go code changed. Server enforcement on `PUT /libraries/{id}/flow` is NOT added (see open question).
- Changed assertion: `web/src/EncodeSettingsPage.test.tsx` line 180, the section-order check. Old:
  `['Library defaults', 'Intel Quick Sync extras']`. New: `['Library defaults', 'Rule tiers', 'Intel Quick Sync
  extras']`. Why: the new "Rule tiers" section (Do item 4) is a level 2 heading the test pins.
- `web/e2e/library.spec.ts` was created; the task named it, so it is not a changed assertion.

## Judgment calls

- "Validation feedback before submit" on the default tier: the select only offers the three tiers, so the
  check guards a stale or edited form. The message appears under the field and disables Save. No
  cross-field rule was invented (for example, "Advanced needs raw args on") because the spec does not state one.
- The Steps tab on `RulesPage.tsx` also locks at moderate. The task does not name it, but without the lock a
  Steps save would silently drop a raw_args step, which the spec forbids.
- The Advanced canvas is never locked (the spec's advanced tier can express every step).
- The banner names the step and its lowest editable tier from the server's message. The message is parsed by
  a regex (`step "<id>"`, `the <tier> tier can edit it`); the graph-level errors are shown as the server's sentence.

## Open question for the owner (does not block the UI acceptance)

`tasks/59-notes.md` and the PM log say the tier is enforced on `PUT /libraries/{id}/flow` "with task 62". Task 62's
Scope lists no Go file, and enforcing it would refuse Steps saves from libraries on the Basic tier (59 notes).
This task makes the UI refuse to save a locked editor. A direct API `PUT` still succeeds and can drop a step. The
owner should say whether the server gate belongs here (a Go change outside this task's scope) or in a later task.

## Checks

- `npm run build` (tsc and vite): pass.
- `make test`: exit 0. Go: every package ok (`internal/api` 305 s, `internal/server` 338 s). Web: 65 files, 474 tests.
- `make lint`: exit 0 (go vet, golangci-lint, spdx, eslint `--max-warnings 0`, prettier check).
- `npx playwright test` (all projects, after `make build`): 118 passed, 4 skipped. The 4 skips are the 3
  pre-existing `file table responsive` cases and the task 61 raw_args test on phone (editing is desktop only).
- `web/e2e/library.spec.ts`: passes on phone, tablet and desktop.

## Covered by tests and by reading only

- Covered by Playwright: the downgrade banner, the Basic lock (disabled fieldset, Save off, reason), the upgrade,
  the canvas step, the stored `raw_args` args, and the stored graph after the round trip.
- Read only, not run in a browser: the Moderate tree lock in `FlowTreePage.tsx`. No e2e case downgrades to
  moderate. The lock uses the same `isLocked` helper as the tested Basic lock.
- `default_rule_tier` select and the `allow_raw_args` switch: checked by the typecheck and the existing
  Encode settings test, not by a new test of their own.

## Summary

1. Built: the Basic / Moderate / Advanced tier control on the library page, the downgrade banner naming each
   step and its editable tier, read only Basic and Moderate editors with Save off and a reason, the Encode
   settings "Rule tiers" section (`default_rule_tier`, `allow_raw_args`), toasts on every mutation, and the
   round trip Playwright test.
2. Skipped: server enforcement of the tier on `PUT /libraries/{id}/flow` (see the open question). No Go change.
   No new e2e case for the Moderate lock.
3. Next task (63) should know: `?tier=` on the flow read and `PUT /libraries/{id}/tier` drive every lock. The
   e2e library spec leaves the seeded library on the Advanced tier with a `raw_args` step, so later specs see
   that graph. The Easy and tree locks come from `isLocked` in `tierModel.ts`.
4. Branch `task/62-tier-switch-and-banner`, STATUS left `in_progress` with the branch name, no PR opened.
5. Open owner question: does the server gate `PUT /libraries/{id}/flow` by tier now, or in a later task?
