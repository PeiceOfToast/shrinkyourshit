# Task 20: Mobile touch targets and shared controls

## Goal
Every page is genuinely usable on a phone. Interactive controls are at least 44 px tall at every width, and lists stop being raw desktop tables below `sm`. This is the foundation task: it adds the shared primitives and one Playwright gate that the rest of the mobile work builds on.

## Read first
`docs/mobile-portability-2026-10-30.md` (the measurements and the reasoning), `docs/ui-review-2026-10-29.md` sections 3 and 8. Depends on tasks 12, 18 and 19.

## Measured starting point
At a 390 x 844 viewport on the running build there is **no horizontal overflow on any page** and the nav already collapses to a single `Open menu` button. The defects are interior:

| Page | Controls under 44 px |
|------|---------------------|
| `/nodes` | 8 (Pause all 36, Resume all 36, Add node 36, Edit 28) |
| `/libraries` | 9 (Add folder 36, library name 24, Scan now 32, Edit 32), plus a raw `<table>` |
| `/libraries/:id` | 11 |
| `/encode` | 4 |
| `/app` | 6 |

## Do

### 1. The audit gate, written first
New `web/e2e/mobile.spec.ts`. For widths **320, 390, 414, 768** and every page (`/`, `/libraries`, `/nodes`, `/encode`, `/app`), assert:

- `document.documentElement.scrollWidth - window.innerWidth <= 0`, and the failure message must name the page, the width and the overflow in px.
- No element matching `main button, main a, main input, main select, main textarea` has a rendered height greater than 0 and less than 44. The failure message must list the offending tag, label and height.

Empty strings and hidden elements are excluded. Write this spec before any UI change and record that it fails, then let the rest of the task turn it green.

### 2. Shared control classes
New `web/src/uiControls.ts` exporting `btn`, `btnPrimary`, `btnGhost` and `input` class strings. Every one of them carries `min-h-11` (44 px). `input` also carries `w-full`. Unit test asserts both properties for all four exports. No page may hand write button classes afterwards.

### 3. `DataList` primitive
New `web/src/DataList.tsx` with a generic `Column<T>` shape (`key`, `header`, `cell`, optional `primary`). It renders **both**:

- a `<ul>` of stacked cards, `sm:hidden`, one `<li>` per row with the primary column as a heading and the rest as a `<dl>` of label and value pairs;
- a real `<table>` inside `overflow-x-auto`, `hidden sm:block`, with `<thead>` and one `<tr>` per row.

The table stays in the DOM at every width for assistive tech. `data-testid` is `{testid}-cards` for the list and `{testid}` for the table, plus an optional `rowTestId(row)`.

### 4. Migrate the five surfaces
- **`web/src/NodesPage.tsx`**: Pause all, Resume all, Add node, Edit and Stop now onto `btn` / `btnPrimary` / `btnGhost`. Keep every existing `data-testid` so task 11 and 18 tests keep passing.
- **`web/src/LibrariesPage.tsx`**: replace the hand written `<table>` with `DataList`. Columns: Library (name, a `min-h-11` link to detail), Path, Files, Satisfied, Waiting, Saved. Row actions (Scan now, Edit) use the shared classes.
- **`web/src/LibraryDetailPage.tsx`**: file list onto `DataList`. Columns: Name (primary), Before, After, Saved, Status, and an action opening the existing job history panel as a dialog rather than an expanding row, so phone rows stay short.
- **`web/src/EncodeSettingsPage.tsx`** and **`web/src/AppSettingsPage.tsx`**: every `<select>` and `<input>` gets `input`, Save gets `btnPrimary`, secondary actions get `btn`.

### 5. Component tests per surface
Each migrated page gets a test asserting its controls carry `min-h-11`, and the list pages additionally assert both the card list and the table are present. These are the fast checks; the Playwright spec is the real gate.

## Out of scope
The dashboard stat cards, the nav rail behaviour (task 21), the flow canvas (task 22). Do not change the API, do not change any `data-testid` that exists today, do not restyle anything not named above.

## Acceptance
- [ ] `web/e2e/mobile.spec.ts` exists, covers 320, 390, 414 and 768 px on all five pages, and fails with a message naming the page, width and exact overflow or the offending controls.
- [ ] `web/src/uiControls.ts` exports `btn`, `btnPrimary`, `btnGhost`, `input`, each containing `min-h-11`, with a unit test proving it.
- [ ] `web/src/DataList.tsx` renders cards below `sm` and a wrapped table at `sm` and up, with a test asserting both exist in the DOM.
- [ ] `/nodes`, `/libraries`, `/libraries/:id`, `/encode` and `/app` have zero controls under 44 px tall in the Playwright audit at all four widths.
- [ ] No page has horizontal overflow at any audited width.
- [ ] `/libraries` and `/libraries/:id` render a card list below `sm` instead of a raw table.
- [ ] Every pre existing `data-testid` still resolves; tasks 11, 12, 18 and 19 tests pass unchanged.
- [ ] `make test`, `make lint` and `cd web && npm run build` all exit 0.
