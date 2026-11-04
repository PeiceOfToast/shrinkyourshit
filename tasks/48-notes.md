# Task 48 notes: Nodes

Spec: `docs/design/ui-redesign-2026-11-04.md` 5.10, with 2 to 4 as the reference (3.1.1, 3.3, 3.4, 3.6, 3.7, 4.1).

## Plan

1. `EncodingStatus.tsx`: add an optional `controls` flag to the `panel` variant. With it, the panel's action slot holds
   `nodes-pause-all` and `nodes-resume-all` (the same mutations and toasts as the shell switch; the applicable one is
   `btn`, the other `btnGhost` and disabled). Off by default, so the Dashboard panel is unchanged. This file is outside
   the task's listed scope, but acceptance item 3 requires the panel to hold the buttons.
2. `NodesPage.tsx`: PageHeader with "Add node"; EncodingStatus panel (controls) above the list; one list with pending
   cards first; each node is a Card (header: name, one lifecycle Badge, "Encoding" plus Switch; body: current job, facts
   dl, hardware, Technical details disclosure; footer: Edit, Stop now, Revoke). Revoke opens ConfirmDialog. Dialogs use
   DialogHeader/Body/Footer, Field, Checkbox and CodeBlock copy.
3. `NodesPage.test.tsx`: add the Revoke confirm step; update only the assertions the spec changes (listed below).

## Scope exception: EncodingStatus.tsx

Acceptance item 3 says the EncodingStatus panel holds `nodes-pause-all` and `nodes-resume-all`, which is not in
the task's file list (`NodesPage.tsx`, `NodesPage.test.tsx`). I added one optional prop, `controls` (default false),
to the `panel` variant only. With it the action slot shows Pause all and Resume all (the applicable one `btn`, the
other `btnGhost`, disabled) instead of the single "Resume encoding" action. The Dashboard panel is unchanged and
`EncodingStatus.test.tsx` passes untouched. Pause and resume use the same mutations as before, so the toasts now
come from EncodingStatus (see the changed assertion for "Resumed 2 nodes.").

## Changed assertions

Format: original `web/src/NodesPage.test.tsx` line in `HEAD`, old to new, why.

- :112 `getByText('Ready')` to a toast check (`attic-box approved.`). Why: "Ready" is dropped (spec 5.10). The old
  assertion was vacuous: the mocked list still returns the pending row, so the match was worker-a's "Ready" pill. The
  toast is what the approve call really produces.
- :180-182 and :185 `copy-docker`, `copy-shell`, `copy-powershell` to `add-node-docker-copy`, `add-node-shell-copy`,
  `add-node-powershell-copy`. Why: the join commands are CodeBlocks with copy (spec 4.1, 5.10); CodeBlock names its copy
  button `<testId>-copy`.
- :315 (Revoke test) gains a confirm step: the test now checks the ConfirmDialog text and clicks
  `node-revoke-confirm-node-1` before it expects the POST. Why: Revoke opens ConfirmDialog (spec 5.10).
- :654-655 `'2 files'`, `'1.5 GB saved'` to `'Files done'`, `'2'`, `'Saved'`, `'1.5 GB'`. Why: the footer is now the
  facts `dl` with "Files done" and "Saved" (spec 5.10).
- :677-678 `'0 files'`, `'0 B saved'` to `'Files done'`, `'0 B'`. Same reason.
- :712-713 `'vaapi'`, `'videotoolbox'` to `'Intel'`, `'Apple'`. Why: encoder Badges show plain-language names through
  the shared `encoderLabel` (spec 5.10 "'CPU', 'Intel Quick Sync'…"). The mapping in `format.ts` says `qsv` and `vaapi`
  are "Intel", so the spec's example is not used verbatim. The test still shows that the page renders the
  reported encoders without a hardcoded list.
- :789 `'Resumed 2 nodes.'` to `'Encoding resumed on 2 nodes.'`. Why: the toast now comes from the shared
  EncodingStatus mutation (see the scope exception).

No assertion was deleted. New tests: one lifecycle Badge and the visible "Encoding" label; Revoke asks first and
cancel keeps the node; FFmpeg details are a closed `details` with the short version. Existing EmptyState
(`nodes-empty`), CodeBlock copy, Stop now, and per-node switch tests are unchanged in intent.

## Judgement calls (not in the spec)

- Stop and Revoke confirm dialogs close when the mutation succeeds. Before, the stop dialog stayed open after confirm.
- The edit dialog's save button reads "Save changes" (spec 3.6 naming rule), not "Save".
- Revoked nodes show a danger Badge "Revoked" with no buttons (spec 5.0). Pending nodes keep "Waiting for approval"
  as a warning Badge, not a Banner, which is a simpler form of the spec's "Banner-tinted header".
- The per-node running-job count moved into the body caption ("Working on N files.") so there is only one Badge.
- Pending and approved nodes share one `ul` with pending first, so the list order test still holds.
- `node-status-{id}` is on the approval Badge (pending and revoked). `node-lifecycle-{id}` is on the lifecycle Badge.

## Verification

- `npm run test` (web): 58 files, 399 tests passed (was 396; 3 new). `make test` exit 0.
- `make lint` exit 0 (`go vet`, `golangci-lint` via `make`, `npm run lint`, `format:check`). `npx tsc --noEmit` clean.
- `make build` exit 0, then Playwright `npx playwright test` with `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib
  SHRINK_E2E_PORT=8733`: 64 passed, 3 skipped (the file-table skips, as in tasks 45 to 47).
- No Go changes, no `api.ts` changes, no new dependency.
- Not done: a visual check in a real browser at 1440 and 390 px. The e2e suite covers layout and touch targets only.

## Summary

1. **Built:** the Nodes page on spec 5.10. Header with "Add node". EncodingStatus panel with Pause all and Resume all
   (`controls`). One list with pending first. Each node is a Card with one lifecycle Badge, a labelled Encoding
   switch, facts `dl` (format.ts), hardware Badges, a Technical details disclosure, and footer Edit, Stop now and
   Revoke. Revoke is confirmed. Dialogs use DialogHeader/Body/Footer, Field, Checkbox, and CodeBlock copy.
2. **Skipped:** nothing in the acceptance list. No visual check in a browser beyond Playwright.
3. **Next tasks should know:** `EncodingStatus` panel now takes `controls`; Nodes-only. Task 53's lint rule should
   allow `opacity-60` (used by the stepper's disabled state). The changed-assertion list above is the place to look
   if a Nodes test breaks. The "Ready" assertion was vacuous before; do not reintroduce it.
4. **Owner decisions:** none needed for this task. The scope exception on EncodingStatus is the one thing to confirm.
5. **Known gaps:** the Parallel jobs stepper and the QSV strip are restyled but not visually checked on a phone; the
   e2e mobile audit covers touch targets only.
