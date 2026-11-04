# Task 46 notes: Flow page

## Plan
1. `FlowCanvas.tsx`: node face on tokens (`rounded-card bg-surface ring-1 ring-line`), remove the step-id span from the face, Badge chips, `fitViewOptions` minZoom 0.85, MiniMap only at 10 or more nodes, handles on tokens.
2. `FlowInspector.tsx`: step id moves here (mono caption), tokens and `uiControls` classes, `btnDangerGhost` for removes, Badge chips.
3. `FlowPalette.tsx`: card surface with "Add a step" heading, `min-h-11` entries, overline group headers, token colours.
4. `FlowCanvasPage.tsx`: `PageHeader` with breadcrumb `Libraries › {name}` and title "Rules", `SegmentedControl` "Rule editor" (Easy and Steps link to the rules page, Advanced is the current route with the "In use" badge), one "Edit flow" (banner copy removed), `Banner`s for the mobile notice, the read-only notice and the easy-replace warning, "Save rules", `useDocumentTitle`, canvas frame filling the viewport.
5. `index.css`: React Flow variables already present from task 36; verify with Playwright.
6. Tests: unit tests for each acceptance item, an e2e test for the canvas background.

Files: `web/src/FlowCanvas.tsx`, `FlowCanvasPage.tsx`, `FlowInspector.tsx`, `FlowPalette.tsx`, their tests, `web/e2e/flow.spec.ts` (new). No Go, no `api.ts`, no `DECISIONS.md`.

## Changed assertions (the rule for this wave)
- `web/src/FlowCanvas.test.tsx:469` (old file line; now `:476`): `expect(getByTestId('flow-source').textContent).toBe('advanced')` changed to `expect(getByTestId('tier-advanced-in-use').textContent).toBe('In use')`. Why: spec 5.9 replaces the source pill with the "In use" tier badge from the Rules header. The intent (the refetched flow reports the new source) is kept, read from the badge.
- `web/src/FlowCanvas.test.tsx:488` and `:496` (now `:497` and `:505`): `getByTestId('flow-back')` changed to `getByTestId('tier-steps')`. Why: spec 3.3 and 5.9 delete the "Back to rules" button. The Steps segment is now a link to the same rules route, so the guard still intercepts the navigation and the leave path still lands on the rules page. The assertions themselves are unchanged.
- No assertion was deleted. Every other existing test id survives, including `flow-edit`, `flow-save`, `flow-done`, `flow-discard`, `flow-summary`, `flow-dirty`, `flow-readonly-banner`, `flow-mobile-notice`, `flow-edit-warning`, `flow-save-reason`, `flow-graph-error` and `flow-inspector-*`. `flow-edit-banner` is gone (spec: one Edit flow); the mobile test still asserts it is absent.

## New tests (each acceptance item has one)
- Shares the Rules PageHeader and SegmentedControl: `FlowCanvas.test.tsx` "shares the Rules header: breadcrumb, Rules title and the tier control" (breadcrumb links, h1 "Rules", `Rule editor` group, Easy and Steps link to rules, Advanced `aria-current`, In use badge, document title).
- One "Edit flow": `FlowCanvas.test.tsx` "has exactly one Edit flow button and no duplicate in the banner".
- Canvas background `rgb(2, 6, 23)`: `web/e2e/flow.spec.ts` (Playwright, computed `background-color` of `.react-flow`, skips with a reason when no library is seeded). Passed on phone, tablet and desktop projects.
- No step-id text on node faces, inspector shows the id: `FlowCanvas.test.tsx` "keeps the step id off the node face and shows it in the inspector".
- fitView `minZoom` 0.85: `FlowCanvasView.test.tsx` "fits the view with a 0.85 minimum zoom" (records the ReactFlow props, the real component still renders).
- Minimap only at 10 or more nodes: `FlowCanvasView.test.tsx` "shows the minimap only from 10 steps" (9 steps no MiniMap, 10 steps MiniMap).
- Mobile notice is a Banner: `FlowCanvas.test.tsx` mobile test now also asserts `role="status"` and the `ring-info/30` tint on `flow-mobile-notice`.
- `e2e/mobile.spec.ts` flow leg: unchanged, passed in the Playwright run (its phone project, which audits 320, 390, 414 and 768 px).

## Deviations (decided by the build agent, owner can override)
1. **The `flow-back` test id moved to the Steps tier link** (see the changed assertions). Spec 5.9 deletes the back button, and the Steps link is now the route back to rules. Easy also links to rules, which is the same route with no id of its own.
2. **The tier links do not open their own tier.** Easy and Steps both go to `/libraries/:id/rules`, which opens on the tier of the saved source (`RulesPage` derives it from `flow_source`), not the one clicked. Making a link open a specific tier needs a query parameter on the Rules page, which is outside this task's touch list. Owner call.
3. **The tier caption** ("Easy: three sliders…") is not on the flow page. Spec 5.9 asks for the same SegmentedControl, not the caption.
4. **Read-only banner copy.** It is the spec sentence for Easy sources. For other sources it reads "This is the flow this library runs. Edit the flow to change it.", because "take over manually" is wrong for a flow that is already manual.
5. **Action labels.** "Save flow" is now "Save rules" (spec 5.9, and 3.6 for the Rules family). "Discard changes" and "Done editing" are `btnGhost`, placed before the one `btnPrimary`, which is last.
6. **The source pill is gone** (see the changed assertions). The Advanced tier's "In use" badge replaces it.
7. **Inspector and palette are not `Card`.** `Card` renders a `div`, `section` or `article` with a title header, and the inspector and palette have their own headers and an `aside`. Both use the card recipe (`card` and `cardPad` from `uiControls`) directly.
8. **Canvas frame** is `card` with `h-[calc(100dvh-16rem)] min-h-[28rem]`. The spec's height formula is used as written, so on a short phone window the frame may run under the browser chrome. Not checked on a real device.
9. **Layout** is a grid: `lg:grid-cols-[14rem_minmax(0,1fr)_20rem]` in edit mode and `lg:grid-cols-[minmax(0,1fr)_20rem]` in read mode. Under `lg` the palette is `order-last`, so the canvas and the inspector come first.
10. **Extras in scope:** `useDocumentTitle(['Rules', name])` (spec 3.3); the error state renders the PageHeader and a "Try again" `onRetry` (spec 3.7); the minimap keeps its `!hidden md:!block` class, so it stays off phones as before.
11. **React Flow variables** were already set in `index.css` by task 36. No CSS change was needed: the Playwright check passes on the existing rules.

## Left for other tasks (not in this task's touch list)
- `easyControls.tsx` and `FlowPreviewPanel.tsx` are untouched by this task.
- `FlowCanvas` keeps `colorMode="dark"`. The controls and background use dark styles and the variables above; no task asked to remove it.
- The `rose-` check (`RulesPage.test.tsx:275`) covers only the rules files. Flow files are now free of `rose-`, `slate-`, and `text-[10px]`, but no test enforces it yet. Task 53 is the place to add that.

## Verification
- `make test` (Go suites and web): exit 0. Web 58 files, 389 tests (task 45 baseline 383, plus 6 new). Go packages all `ok`.
- `make lint` (go vet, golangci-lint, SPDX, eslint `--max-warnings 0`, prettier): exit 0.
- `make build` (web build, copy into the embed, Go binary): exit 0.
- Playwright (`npx playwright test`) against the rebuilt binary, with ffmpeg on PATH: 64 passed, 3 skipped, exit 0. The 3 skips are the file-table tests in `e2e/layout.spec.ts` that the suite runs only at one width (its own skips). The flow background test passed on all three projects.
- The first Playwright run in this task had no ffmpeg on PATH, so 6 skips appeared and the seed had no file row. That run is superseded by the one above.

## Summary
1. **Built:** the Flow page (Advanced tier) on spec 5.9: the Rules `PageHeader` with breadcrumb `Libraries › {name}` and title "Rules", the Rules `SegmentedControl` with the "In use" badge, one "Edit flow", "Save rules", Banners for the notices, the canvas frame at viewport height, node faces on tokens with no step id, the inspector showing the id, the palette as a card, `fitViewOptions` at minZoom 0.85, and the minimap only at 10 or more steps.
2. **Skipped:** nothing in the acceptance list. The tier caption and the tier-specific Rules link (deviations 2 and 3) are not on this page.
3. **Next tasks should know:** `flow-back` and `flow-source` are gone, and the tests now use `tier-steps` and `tier-advanced-in-use`. Easy and Steps links land on the rules page with the saved tier, not the chosen one (deviation 2). `FlowInspector` and `FlowPalette` use the card recipe, not `Card`.
4. **Owner decisions:** deviation 2 (tier links open the saved tier), deviation 4 (banner copy for non-Easy sources), and the canvas height formula on phones (deviation 8).
5. **Known gaps:** no real-device check of the canvas height.
