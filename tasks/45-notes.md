# Task 45 notes: Rules page (Easy and Steps) plus StepStackEditor and FlowPreviewPanel

## Plan (as executed)
1. `flowParams.tsx`: fields and selects from `uiControls` (`input`, `select`, `btnGhost`), Field-style label stack, toggle token colour, no slate or emerald.
2. `StepStackEditor.tsx`: rows on `bg-canvas`, `btnIcon` (44 px, `h-11 w-11`) on every row action with aria-labels, remove in the danger tint, chips as `Badge`, errors `text-danger`, add row as `select` plus `btn` "Add step".
3. `FlowPreviewPanel.tsx`: Card-surface disclosure titled "What this does" with a chevron (no Show/Hide text), 44 px summary, `list-disc` list. Stays controlled (see deviation 1).
4. `RulesPage.tsx`: `PageHeader` with breadcrumb `Libraries › {name}` (the library crumb carries `rules-back`), title "Rules", the spec subtitle. `SegmentedControl` "Rule editor" with `In use` Badge from `flow.source`, the three-tier caption, Banners for the replace warnings, `FormSection` for Easy, `Card` "Steps, in order" for Steps, `FormActions` with `btnPrimary` "Save rules" on both tiers.
5. Tests: updated the one changed assertion, added tests for each acceptance item and a source-level check for `rose-`.

Files touched: `web/src/RulesPage.tsx`, `StepStackEditor.tsx`, `FlowPreviewPanel.tsx`, `flowParams.tsx`, `RulesPage.test.tsx`, `StepStack.test.tsx`. Nothing else in `web/src` changed, and `api.ts`, Go and `DECISIONS.md` are untouched.

## Changed assertions (the rule for this wave)
- `web/src/StepStack.test.tsx:199`: `expect(row.className).toContain('ring-rose')` changed to `toContain('ring-danger')`. Why: spec 2.10 and 4.3 move row errors to the danger token, and no `rose-` may remain in the rules files. The intent (an error row is visibly marked) is unchanged.

No other existing assertion changed. None was deleted. The "Save easy settings" and "Save steps" labels were not asserted anywhere.

## New tests (each acceptance item has one)
- `RulesPage.test.tsx`: "marks the tier in use with a neutral selection and an In use badge" (SegmentedControl, neutral `bg-raised`, badge from `flow.source`).
- `RulesPage.test.tsx`: "shows the three-tier caption under the selector".
- `RulesPage.test.tsx`: "names the save action 'Save rules' on the steps tier".
- `RulesPage.test.tsx`: "names the save action 'Save rules' and shows the replace banner on the easy tier" (Banner role `status`, `easy-replace-warning` kept). The steps replace banner is covered by the existing "switching back to Steps warns" test.
- `RulesPage.test.tsx`: "gives every step row icon button a 44 px box and an aria-label" (`h-11 w-11`, aria-label, remove is `text-danger`).
- `RulesPage.test.tsx`: "leaves no rose palette classes in the rules files" (reads the four source files).

## Verification
- `make lint`: exit 0 (eslint `--max-warnings 0`, prettier check).
- `make test`: exit 0. Web 57 files, 383 tests (baseline 377, +6 new). Go suites unchanged and green.
- `npm run build`: clean.
- `npx playwright test` (embedded build, `SHRINK_E2E_PORT=8731`, `LD_LIBRARY_PATH=/home/mb/.local/chromium-deps/lib`): 61 passed, 3 skipped (the suite's own per-viewport skips), exit 0.
- `grep -n 'rose-\|slate-\|emerald-'` on the four files: no matches.

## Deviations (decided by the build agent, owner can override)
1. **FlowPreviewPanel is not the native `Disclosure` component.** The spec says "Disclosure titled 'What this does'". The native `details` keeps its body in the DOM when closed and cannot be driven from state. `EasyLibraryForm.test.tsx` (task 41) asserts that the lines are absent until the toggle is clicked, and the panel's preview fetch is lazy on open. So the panel keeps its controlled button with `aria-expanded`, uses the Disclosure's visual recipe (44 px summary, rotating chevron), and keeps every test id.
2. **The Advanced tier still shows the step stack and its save button under the gateway.** That is the behaviour on `main`, and the gateway caption says "Switching to Steps or Easy below". Spec 5.8 does not mention the stack on Advanced, so I kept it rather than silently remove it. Owner call: if the stack should be Steps-only, that is a one-line change in `RulesPage.tsx`.
3. **Remove button colour.** `uiControls.ts` has no danger icon variant and is outside this task's touch list. `StepStackEditor.tsx` derives `removeIcon` from `btnIcon` by swapping `text-fg-muted` for `text-danger`. A proper `btnIconDanger` belongs in `uiControls.ts` in a later task.
4. **Row icon glyphs are kept.** The Icon set has no arrow-up, pencil or x glyphs, so `↑ ↓ ✎ ✕` stay as text inside the 44 px buttons. The drag handle is a non-interactive `span` (h-11, w-6), not a button, because it has no action.
5. **Step rows use `bg-canvas` and `rounded-card`, not a nested `Card`.** `Card.tsx` says never to nest a Card in a Card, and the rows sit inside the "Steps, in order" Card. Spec 4.3 says "Card", so this is a small deviation in surface, not in layout.
6. **Copy changes in the stack.** The add placeholder is "Choose a step…" (was "Add a step…") and the button is "Add step" (was "Add"), to match spec 4.3's "Add step". Test ids are unchanged.
7. **Toggle checkbox in `flowParams.tsx`** is a plain `h-5 w-5 accent-accent` input, not the `Checkbox` component, because ParamField's whole row is one `label` and `Checkbox` would nest a label inside a label. Its 44 px target is therefore the label row, not the input.
8. **Extras beyond the acceptance list, all in scope:** `useDocumentTitle(['Rules', name])` (spec 3.3); the error state renders PageHeader and a "Try again" `onRetry` (spec 3.7); the `In use` Badge carries `tier-{id}-in-use` as a new test id; the `FormSection` "How much to shrink" has no description.
9. **`flow-preview-lines` is now a `ul` (was `ol`)** because spec 5.9 asks for `list-disc`. No test reads the list role.

## Left for other tasks (not in this task's touch list)
- `FlowInspector.tsx` (`rose-400` on two buttons), `FlowCanvas.tsx` (`rose-500/70`, `rose-400`) and `FlowCanvasPage.tsx` (`rose-400`) still use `rose-`. Task 46 (flow page) touches `FlowCanvas`, `FlowCanvasPage` and `FlowInspector`, so that is where they go.
- `easyControls.tsx` (the sliders and `RemoveSubtitlesCheckbox`) keeps its old styles. The sliders now sit inside the FormSection, which caps their width, but the spec 4.3 `StopSlider` rework was not in this task's touch list.
- `FlowPreviewPanel.tsx` also serves `EasyLibraryForm` (New library). Its look changed with this task, and that page's tests pass.

## Summary
1. **Built:** the Rules page on spec 5.8 (PageHeader with breadcrumb, neutral SegmentedControl with In use badge, three-tier caption, Banners for the replace warnings, FormSection for Easy, Card "Steps, in order", "Save rules" on both tiers) and the restyled StepStackEditor, FlowPreviewPanel and flowParams on tokens and uiControls.
2. **Skipped:** the native `Disclosure` for the preview panel (deviation 1); the stop-label rework of `easyControls.tsx`; the `rose-` in the flow files (tasks 46 and 48).
3. **Next tasks should know:** Advanced still shows the stack under the gateway (deviation 2, owner to confirm). `removeIcon` derives from `btnIcon` by string swap (deviation 3), so a `btnIconDanger` in `uiControls.ts` would be cleaner. ErrorBox gets `onRetry` on this page, so task 47 can copy the pattern.
4. **Owner decisions:** deviation 1 (controlled preview panel, not native Disclosure) and deviation 2 (stack on Advanced).
5. **Known gaps:** no Go or API change. The Save label on Easy and Steps is "Save rules" as the spec asks; the server's preview text ("HEVC", "MKV") still appears on the easy screen, flagged in spec 8.4 item 3.
