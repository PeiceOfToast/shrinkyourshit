# Task 60 notes: Moderate decision tree editor

Branch: `task/60-moderate-tree-editor` (from main 25cd4e2).

## Plan

1. `web/src/treeModel.ts`: pure model. `buildTree(graph, isCondition)` builds the tree from `edges`
   (a condition owns the subtree of its outgoing edges; an action is a leaf and its continuation returns to
   the level above, which is how the engine gates: a guard applies to the next action only). `flattenTree`
   gives ordered rows with depth. Edit ops (add, remove, move, set param) return a whole graph with the
   chain edges rebuilt. `isChainGraph` says whether the editor can save the graph.
   Test file `treeModel.test.ts`.
2. `web/src/FlowTreePage.tsx`: rows from `treeModel`, indentation capped at 4 levels with a visible
   "nested" marker, drag handle plus up/down buttons plus ArrowUp/ArrowDown on the handle, 44 px targets.
   Fields via the existing `ParamField` from `StepTypes()` params. Palette select from the fetched
   `StepTypes()` list. Server `invalid_flow` field errors mapped onto the owning row with the existing
   `mapErrors`. Save: `PUT /libraries/:id/flow` with `source: 'steps'` (same as the step stack), toast on
   success, server error toast on failure. The page never touches tier and never sends a graph to change tier.
3. Route `libraries/:id/tree` in `App.tsx`, a "Tree editor" link on the library detail page, and a row in
   `ARCHITECTURE.md` UI routes.
4. `FlowTreePage.test.tsx` (vitest, fetch mock) and `web/e2e/tree.spec.ts` (Playwright: seeded library,
   known graph, add condition plus child, save, reload, same tree; 390 px no horizontal overflow).

## Open question for the owner (does not stop this task)

Spec section 2, decision 3 says Moderate is "condition nodes ... with branches". ARCHITECTURE.md ("v1 flows
are a single chain") and `internal/flow/graph.go` (`Validate` rejects a node with more than one outgoing or
incoming edge) say the v1 engine has no branches, and the spec never lifts that rule. Consequences here:

- The acceptance item "a condition with two children" is covered at the model level only
  (`treeModel.test.ts`, a fork graph). The page never creates a fork. A stored graph is always a chain, so
  the page cannot show one. If a fork ever reaches the page, it shows the tree read only with a banner.
- A condition's children are the steps it gates. In the engine, a guard gates only the next action, so every
  condition in a real saved graph has exactly one child (or a nested condition).

Question for the owner: does v1 allow branching (a condition with several gated children)? If yes, the server
validator and the engine change in a later task and the editor add/remove rules must change with them. If no,
the "two children" acceptance item should be restated as a single child plus nested conditions.

## Other choices

- Removing a condition row removes only that row. Its child then runs ungated, which is the engine's meaning
  for a removed guard. The row list shows it at once. No cascade delete.
- Depth is derived from the chain. A new action placed directly after a condition becomes its child. There is
  no separate indent control.
- Editing is available at every width (the rows stack at 390 px).
- Live server validation on each change is not added; errors come from Save as in the step stack.

## Deviations from the task file

- `web/src/api.ts` is unchanged. The helpers the page needs already exist (`getLibraryFlow`, `putLibraryFlow`,
  `getFlowStepTypes`, `getPlugins`), so "helpers only" needed nothing new.
- `LibraryDetailPage.tsx`: one link, `library-tree-editor`, next to "Change rules". No existing assertion changed.
- `App.tsx`: the route `libraries/:id/tree` (the task says `/libraries/:id/tree`, which the router resolves the same).
- `ARCHITECTURE.md`: one row in the UI routes table for the new route (AGENTS rule 4 spirit: docs match routes).
- Added beyond the list: a Revert button with a confirm dialog and the unsaved-changes guard (the same guard as the
  canvas page), and a read-only view for a branching graph (see the open question above).
- The page does not validate on each change (the canvas does). Server field errors appear when Save fails, so the
  client never guesses. The task asked for errors on the owning row, and that is what happens.
- Playwright needs `libatk-1.0.so.0`, which is not in `/home/mb/.local/pwlibs` (it has `debs` and `root`). It is in
  `/home/mb/.local/chromium-deps/lib`, so the runs used `LD_LIBRARY_PATH=/home/mb/.local/pwlibs:/home/mb/.local/chromium-deps/lib`.
  Port 8712 was already in use, so the runs used `SHRINK_E2E_PORT=8791`.
- The tree e2e test overwrites the seeded library's flow with a known graph. That is test state only; the server
  is torn down afterwards.

## Checks

- `npx vitest run` (web, full suite): 64 files, 464 tests, pass. Before this task: 62 files, 444 tests.
  New: `treeModel.test.ts` (11) and `FlowTreePage.test.tsx` (9).
- `npm run build`: pass (`tsc --noEmit` and `vite build`).
- `make test`: pass. Every Go package `ok`; web 64 files and 464 tests.
- `make lint`: exit 0. `go vet` clean, golangci-lint 0 issues, SPDX check, ESLint with `--max-warnings 0`, Prettier.
- `npx playwright test` (all three projects, port 8791): 113 passed, 3 skipped. The 3 skips are the existing
  `file table responsive` cases. The two new `tree.spec.ts` tests pass on phone, tablet and desktop.
- No existing test assertion was changed.

## Acceptance mapping

- `treeModel` tests: flat chain (`flat chain`), a condition with two children (`a condition with two children`,
  model level, see the open question), nested conditions (`nested conditions`).
- Playwright: `adds a condition and its child, saves, reloads and shows the same tree` (`tree.spec.ts`).
- Palette: `offers every StepTypes() entry and adds each one` iterates the fetched palette, not a hard-coded list.
- `npm run build`, `npx playwright test`, `make lint`: all green (see Checks).

## Summary

- Built: `treeModel.ts` (the tree from edges, flattening, chain edits, chain check), `FlowTreePage.tsx` at
  `/libraries/:id/tree` (rows with indent and cap, add, remove, reorder by drag or arrow keys, fields from
  `StepTypes()`, server errors on the owning row, save as `steps` with toasts), and the link from the library page.
- Skipped: server-side tier enforcement and tier switching UI (tasks 61 and 62), as the owner's context said.
- Next task (61) should know: the tree page uses `chainSteps`, `graphOf` and `isChainGraph` from `treeModel.ts`. A
  Moderate save always rebuilds the chain, so it drops any edge that does not sit on the chain. The page shows
  a branching graph read only.
- Open owner question (above): does v1 allow branching? The spec says "with branches", but the engine and the validator
  allow one chain. The "two children" test is model level only. Owner answer needed before 62.
- Also for the PM: this branch leaves STATUS `in_progress` with the branch name, as the task asks. No PR was opened.
