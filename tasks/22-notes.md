# Task 22 notes: flow canvas view only below md, audit closed

## Plan

Branch `task/22-flow-canvas-mobile` from main.

1. `web/src/FlowCanvasPage.tsx` — `useMediaQuery('(min-width: 768px)')` gates the
   palette column, the inspector column and every edit control (Edit flow header
   button, readonly-banner CTA, Save, Discard, Done). Below `md` the page renders
   `flow-mobile-notice` (exact wording from the task) and keeps the header (title,
   `flow-summary` node count, source chip, Back to rules link) and the canvas with
   pan/pinch zoom. Editing state becomes `requested && wide` so a resize mid-edit
   degrades to view-only; the unsaved guard stays armed. Page buttons move onto the
   shared `uiControls` classes (min-h-11) so the extended audit passes at every width.
2. `web/src/index.css` — React Flow defaults: `.react-flow__controls-button` 26px →
   44px (border-box, larger icon), and the `.react-flow__attribution` link (a real
   `a` inside `main`) gets a 44px tap target, else the audit would fail on it.
   React Flow 12.12 already sets `aria-label` Zoom In / Zoom Out / Fit View.
3. `web/src/FlowCanvas.test.tsx` — a stubbed matchMedia (all queries false):
   notice present with the desktop wording; palette, `flow-inspector-empty`,
   edit/save/banner controls absent; canvas + nodes + `flow-summary` count present;
   zoom controls keep their accessible names. Existing tests are unaffected
   (jsdom has no matchMedia → wide).
4. `web/e2e/mobile.spec.ts` — fetch the library id at runtime from
   `GET /api/v1/libraries`; audit `/libraries/{id}` and `/libraries/{id}/flow` at
   320/390/414/768; skip both when no library exists. On the flow route also prove
   zoom works below md (Zoom In click changes the `.react-flow__viewport`
   transform) and the step count is visible.
5. `docs/ui-review-2026-10-29.md` — append `## Mobile pass (2026-10-30)`
   (measured / changed / open). `ARCHITECTURE.md` — add the layout-rules block to
   the UI routes section. `DECISIONS.md` untouched.

Then: `make test`, `make lint`, `cd web && npm run build`, `cd web && npx
playwright test`; tick acceptance; STATUS done; PR.

## Summary

1. Built: the flow canvas page is view only below `md` — palette, inspector and
   every edit control gone, `flow-mobile-notice` rendered, canvas with pan/pinch
   zoom and the step count kept; editing is `requested && wide` with the guard
   armed. Canvas chrome (React Flow zoom buttons, attribution link) sized to the
   44px minimum in `index.css`, scoped under `main` because the route is lazy.
2. Built: the mobile audit now fetches the library id from the API at runtime and
   audits `/libraries/{id}` and `/libraries/{id}/flow` at 320/390/414/768, with a
   Zoom In click proving the viewport transform changes on a phone.
3. Found and fixed a pre-existing blank-page bug: the server marshals parameterless
   step types with `"params": null` and the client mapped over it, unmounting the
   whole app in a real browser (jsdom fixture used `[]`, so unit tests never saw
   it). Normalized in `getFlowStepTypes`; regression test in `api.test.ts`.
4. Skipped: mobile flow editing (declared out of scope); no API changes.
5. Next task should know: Playwright needs Chromium's system libs in this env
   (used `LD_LIBRARY_PATH=/tmp/opencode/chromium-deps/usr/lib/x86_64-linux-gnu`
   with `apt-get download`-extracted debs — CI installs them normally). The
   integration suite can flake `TestIntegrationRevokeRequeuesForBuiltinWorker`
   under load; it passed on re-run.
