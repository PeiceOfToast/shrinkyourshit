# Task 15 notes — visual flow editor

## Plan (files, in order)

1. `web/package.json` — add `@xyflow/react` (STACK.md pins React Flow 12 for
   this task; no other new dependency).
2. `web/src/test/setup.ts` — jsdom lacks `ResizeObserver`, which
   @xyflow/react needs; add a minimal stub so component tests run.
3. `web/src/flowCanvasModel.ts` — canvas-side graph edits (pure functions,
   unit tested): `layoutChain` (positions for the linear chain),
   `mapNodeErrors` (server `nodes[<array-index>]` paths → step ids),
   `applyConnect` (single-chain connect that replaces the replaced
   out/in-edge), `removeSteps`, `removeEdge`, `addStep` (palette defaults).
4. `web/src/flowParams.tsx` — extract the parameter-field renderer from
   StepStackEditor so the canvas inspector and the stack share one
   implementation (same testids, no behavior change).
5. `web/src/FlowCanvas.tsx` — the React Flow wrapper: custom step node
   (label, condition line, params chips, server error), vertical chain
   layout, connect/delete by drag plus keyboard (Delete key), select →
   inspector, minimap/controls/background, dark color mode. Fully
   controlled by the graph object the API returns.
6. `web/src/FlowPalette.tsx` — server-driven palette (click/keyboard to
   add; drag is never the only path).
7. `web/src/FlowInspector.tsx` — selected step's parameters rendered from
   the schema, plus menu alternatives for every drag action (connect to…,
   disconnect next, remove step/connection).
8. `web/src/FlowCanvasPage.tsx` — route page: loads library/flow/palette,
   read-only first with an explicit "Edit flow" switch, graph state, dirty
   tracking, debounced `POST /flows/validate` on every change with errors
   rendered on the nodes, Save disabled while invalid/uncertain with the
   reason shown, `PUT /flow` with `source=advanced`, unsaved-changes guard
   (beforeunload + capture-phase link interception, because `useBlocker`
   needs a data router and this app uses plain routers).
9. `web/src/RulesPage.tsx` — the Advanced tier button now opens
   `/libraries/:id/flow`; switching back to Steps from an advanced library
   warns that the manual graph structure will be lost; the old "coming
   later" note becomes a gateway panel. `web/src/App.tsx` — the new route.
10. Tests: `flowCanvasModel.test.ts`, `FlowCanvas.test.tsx`,
    `RulesPage.test.tsx`, Go `TestFlowCanvasSave` (canvas-shaped save →
    GET returns `source=advanced`, round trip).
11. Docs: API.md already covers everything (task 15 = no new endpoints);
    ARCHITECTURE.md already lists the route. No doc changes needed.

## Design decisions (recorded here because they fill gaps in the docs)

- Read-only is a product phase, not a source rule: the canvas always opens
  read-only and an explicit "Edit flow" button unlocks editing for any
  source. This implements "read only first, editing second" and keeps an
  easy-mode library's generated flow safe until the user asks. Saving from
  the canvas always writes `source=advanced`.
- Node positions are presentation only (the FlowGraph schema has none); a
  vertical chain layout is computed on load and preserved across edits.
- The client makes structural edits (connect replaces the chain edge it
  displaces) but never decides validity: every change re-validates on the
  server and errors render verbatim on the nodes.
- Unsaved guard: `useBlocker` requires a data router; the app uses plain
  `BrowserRouter`/`MemoryRouter`, so the guard combines `beforeunload`
  (reload/close) with a capture-phase click interceptor on in-app anchors
  plus a confirm dialog. Known limitation: browser back/forward inside the
  SPA is not interceptable without a data router (recorded for the owner).
- Palette add places an orphan node deliberately: the server flags it
  ("not connected"), which is the teaching moment the task asks for.

## Summary

- Built the visual flow editor (tier 3) at `/libraries/:id/flow` on
  `@xyflow/react` 12: read-only first with an explicit "Edit flow" unlock,
  a server-driven palette, an inspector rendering the parameter schema,
  drag plus menu/keyboard paths for every action (connect, disconnect,
  remove), automatic chain layout on first open, minimap and controls.
- Validation stays server-owned: every change debounces
  `POST /flows/validate`, field errors render on the nodes verbatim
  (mapped by the graph's array order), Save is disabled with the reason
  while invalid or unchecked, and saving writes the whole graph with
  `source=advanced` (Go `TestFlowCanvasSave` round trip).
- The rules page's Advanced tier now opens the canvas; switching back to
  Steps or Easy warns that the manual graph structure will be lost. The
  flow route is lazy-loaded (main bundle 520 → 319 kB; canvas 65 kB gzip).
- Skipped: undo beyond discard-to-loaded-graph, edge reconnection by drag
  (edges are deleted and re-added instead), and adding the canvas to the
  Playwright layout suite (it needs a seeded library; the page reuses the
  shared PageContainer grid so overflow risk is low).
- Next task should know: `useUnsavedGuard` cannot intercept browser
  back/forward inside the SPA (no data router); if a data router lands
  later, swap it for `useBlocker`. Node positions are never persisted —
  re-layout happens per session. React Flow selection is fully controlled
  here (`selectNodesOnDrag=false`, selection flows through
  onNodesChange/onEdgesChange), which is what keeps it testable in jsdom.
