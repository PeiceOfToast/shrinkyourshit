# Task 22: Honest flow canvas on phones, and close the mobile audit

## Goal
The flow canvas is not usable for editing on a phone. Say so in the UI instead of shipping a desktop editing surface squeezed into 390 px, keep the graph inspectable, and then extend the mobile audit to the library detail and canvas routes so the whole app is covered by the gate.

## Read first
`docs/mobile-portability-2026-10-30.md` (tasks 9 and 10 of that plan), `web/src/FlowCanvasPage.tsx`, `web/e2e/mobile.spec.ts` from task 20. Depends on task 20.

## Do

### 1. Read only below `md`
In the flow canvas page, gate the palette, the node inspector panel and any add or delete controls behind `useMediaQuery('(min-width: 768px)')`. When narrow, render instead:

```tsx
<div
  className="mb-3 rounded-xl bg-slate-900 p-3 text-sm text-slate-300 ring-1 ring-slate-800"
  data-testid="flow-mobile-notice"
>
  Viewing only on a phone. Editing the flow needs a desktop browser.
</div>
```

Pan and pinch zoom stay enabled so the graph can be inspected, and the node count stays visible. Nothing is silently read-write-but-broken: if a control cannot work, it is not rendered.

### 2. Touch sized canvas controls
Any remaining canvas chrome controls (zoom in, zoom out, fit view, reset) must be at least 44 px tall and carry an `aria-label`. They are the most likely regression against the task 20 audit.

### 3. Extend the audit to every route
`web/e2e/mobile.spec.ts`: fetch a library id at runtime from `GET /api/v1/libraries` and add `/libraries/{id}` and `/libraries/{id}/flow` to the audited paths. If the API returns no libraries, skip those two paths rather than failing.

### 4. Document the rules
- Append a `## Mobile pass (2026-10-30)` section to `docs/ui-review-2026-10-29.md` recording what was measured, what changed and what remains open.
- Add to the UI routes section of `ARCHITECTURE.md`:

```
Layout rules: every interactive control is at least 44px tall (`web/src/uiControls.ts`).
Tables above `sm` render as cards below it via `web/src/DataList.tsx`. Stat tiles use
`clamp()` type and scale with the viewport (`web/src/StatCard.tsx`). The rail auto
collapses below `lg` and remembers an explicit override in `localStorage` under `sys.rail`.
The flow canvas is view only below `md`. `web/e2e/mobile.spec.ts` fails CI on any
horizontal overflow or sub 44px control at 320, 390, 414 and 768 px.
```

- Tick nothing in `DECISIONS.md`. It does not change.

## Out of scope
Mobile flow editing with a different interaction model. That is a product decision and is not in this task. No API changes.

## Acceptance
- [ ] Below `md` the canvas page shows `flow-mobile-notice` with the desktop wording, and the palette and inspector are absent from the DOM, proven by a test.
- [ ] Pan and zoom still work below `md`, and the node count is still displayed.
- [ ] Every canvas chrome control is at least 44 px tall and has an `aria-label`.
- [ ] The mobile audit covers `/libraries/{id}` and `/libraries/{id}/flow` at 320, 390, 414 and 768 px, and passes.
- [ ] `docs/ui-review-2026-10-29.md` has the mobile pass section and `ARCHITECTURE.md` has the layout rules block.
- [ ] `DECISIONS.md` is unchanged versus `origin/main`.
- [ ] `make test`, `make lint`, `cd web && npm run build` and `cd web && npx playwright test` all exit 0.
