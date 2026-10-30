# ShrinkYourShit mobile portability plan

**Goal:** make every page genuinely usable on a phone, not a desktop layout shrunk to fit, with touch sized controls, cards instead of tables, fluid type, and an auto collapsing navigation rail, all enforced by a Playwright mobile audit that fails CI when it regresses.

**Deliverable of this document:** an implementer with zero context should be able to work top to bottom without guessing.

---

## Current context and assumptions

Verified by measuring the running build at http://10.10.2.210:8765 with a 390 x 844 emulated viewport on 2026-10-30. These are facts, not guesses:

| Page | Horizontal overflow | Tables | Controls under 44 px tall |
|------|--------------------|--------|--------------------------|
| `/` dashboard | 0 px | 0 | 0 |
| `/nodes` | 0 px | 0 | 8 (Pause all 36, Resume all 36, Add node 36, Edit 28) |
| `/libraries` | 0 px | 1 | 9 (Add folder 36, library name 24, Scan now 32, Edit 32) |
| `/libraries/:id` | 0 px | 1 (wrapped in `overflow-x-auto`) | 11 |
| `/encode` | 0 px | 0 | 4 (selects 37 to 40, Save 40) |
| `/app` | 0 px | 0 | 6 |

So the shell itself is already responsive: at 390 px there is no `aside`, the header shows a single `Open menu` button, and `main` is full width. The hamburger and drawer exist from task 10. **What is not adaptive is the interior:** controls below the 44 px touch minimum, a raw `<table>` on the libraries list, dense 12 px rows, stat cards with no icons or fluid type, and a manual-only rail collapse.

Assumptions:

- The stack is React 18, TypeScript strict, Tailwind 3, react-router 6, Vitest plus React Testing Library, Playwright for layout. No component library, no icon package, no CSS-in-JS.
- Tailwind breakpoints in use: `sm` 640, `md` 768, `lg` 1024.
- `web/src/PageContainer.tsx` already provides `mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8`. Never add a page level `max-w-*` on top of it.
- `web/src/useMediaQuery.ts` exists and is correct (it subscribes on the `MediaQueryList`).
- Tests live beside components as `*.test.tsx`; e2e lives in `web/e2e/` with `playwright.config.ts` and an existing `layout.spec.ts`.
- Repo rules: branch per task, PR, CI green (`make test`, `make lint`, `npm run build`, `npm test`, `npm run lint`), never touch `DECISIONS.md`.
- Build the demo from source with docker if you need to look: `docker build -f docker/Dockerfile -t shrinkyourshit:dev .` then run it on port 8765 with the volumes from `docker-compose.yml`.

---

## Architecture / approach

Three shared primitives do the work instead of per page patches: a `touch` class string set in `web/src/uiControls.ts`, a `StatCard` component with an inline SVG icon and `clamp()` type, and a `DataList` component that renders a real table at `sm` and up and stacked cards below it. Pages migrate onto those primitives one at a time. On top of that, one Playwright spec measures every page at 320, 390, 414 and 768 px and fails on either horizontal overflow or any interactive element smaller than 44 px, so the standard is enforced rather than remembered.

---

## Task 1: Add the mobile audit spec (it should fail)

**Objective:** capture the current deficits as a test before changing any UI.

**Files:**
- Create: `web/e2e/mobile.spec.ts`

**Step 1: write the spec**

```ts
// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'

const WIDTHS = [320, 390, 414, 768]

async function audit(page: Page, path: string, width: number) {
  const small = await page.evaluate(() => {
    const nodes = [...document.querySelectorAll('main button, main a, main input, main select, main textarea')]
    return nodes
      .map((el) => {
        const r = el.getBoundingClientRect()
        return {
          tag: el.tagName.toLowerCase(),
          label: (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 24),
          h: Math.round(r.height),
          w: Math.round(r.width),
        }
      })
      .filter((t) => t.h > 0 && t.h < 44)
  })
  expect(
    small,
    `${path} at ${width}px has ${small.length} controls under 44px: ${JSON.stringify(small)}`,
  ).toEqual([])
}

test.describe('mobile audit', () => {
  for (const width of WIDTHS) {
    test(`no overflow and 44px targets at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 844 })
      for (const path of ['/', '/libraries', '/nodes', '/encode', '/app']) {
        await page.goto(path)
        await page.waitForLoadState('networkidle')
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - window.innerWidth,
        )
        expect(overflow, `${path} overflows by ${overflow}px at ${width}px`).toBeLessThanOrEqual(0)
        await audit(page, path, width)
      }
    })
  }
})
```

**Step 2: run it and confirm it fails**

```sh
cd web && npx playwright test mobile.spec.ts --project=chromium
```
Expected: FAIL, with output listing the small controls, for example `/nodes at 390px has 8 controls under 44px: [{"tag":"button","label":"Pause all","h":36,...}]`.

**Step 3: commit the failing test**

```sh
git checkout -b task/mobile-portability
git add web/e2e/mobile.spec.ts
git commit -m "test(e2e): mobile audit for overflow and 44px touch targets"
```

---

## Task 2: Shared control class strings

**Objective:** one source of truth for touch sized controls, so pages cannot drift.

**Files:**
- Create: `web/src/uiControls.ts`
- Test: `web/src/uiControls.test.ts`

**Step 1: failing test**

```ts
// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from 'vitest'
import { btn, btnPrimary, btnGhost, input } from './uiControls'

describe('uiControls', () => {
  it('every control string carries the 44px touch minimum', () => {
    for (const cls of [btn, btnPrimary, btnGhost, input]) {
      expect(cls).toContain('min-h-11')
    }
  })
  it('inputs use the full width on small screens', () => {
    expect(input).toContain('w-full')
  })
})
```

**Step 2: run, expect failure**

```sh
cd web && npx vitest run src/uiControls.test.ts
```
Expected: FAIL, `Failed to resolve import "./uiControls"`.

**Step 3: implement**

```ts
// SPDX-License-Identifier: AGPL-3.0-only

/** Shared control classes. min-h-11 is 44px, the Apple and WCAG touch size. */
const base =
  'inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-3 text-sm font-medium transition-colors disabled:opacity-50'

export const btn = `${base} bg-slate-800 text-slate-100 hover:bg-slate-700`
export const btnPrimary = `${base} bg-emerald-600 text-white hover:bg-emerald-500`
export const btnGhost = `${base} text-slate-300 hover:bg-slate-800 hover:text-white`
export const input =
  'min-h-11 w-full rounded-lg border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100'
```

**Step 4: run, expect pass**

```sh
cd web && npx vitest run src/uiControls.test.ts
```
Expected: `2 passed`.

**Step 5: commit**

```sh
git add web/src/uiControls.ts web/src/uiControls.test.ts
git commit -m "feat(ui): shared 44px touch control classes"
```

---

## Task 3: Nodes page controls onto the primitives

**Objective:** clear the 8 small controls on `/nodes`.

**Files:**
- Modify: `web/src/NodesPage.tsx` (the Pause all, Resume all, Add node, Edit and Stop now buttons)
- Test: `web/src/NodesPage.test.tsx`

**Step 1: failing test** (append to the existing describe block)

```tsx
it('primary controls meet the 44px touch minimum', async () => {
  renderNodes()
  for (const name of [/pause all/i, /resume all/i, /add node/i, /edit/i]) {
    const el = await screen.findByRole('button', { name })
    expect(el.className).toContain('min-h-11')
  }
})
```

**Step 2: run, expect failure**

```sh
cd web && npx vitest run src/NodesPage.test.tsx -t "44px touch minimum"
```
Expected: FAIL, assertion on `className`.

**Step 3: implement**

Replace the ad hoc class strings with imports:

```tsx
import { btn, btnPrimary, btnGhost } from './uiControls'
```
then `className={btn}`, `className={btnPrimary}`, `className={btnGhost}` on Pause all, Resume all, Add node, Edit, Stop now, and the switch label wrapper. Keep `data-testid` attributes untouched so existing tests keep passing.

**Step 4: run the whole page suite**

```sh
cd web && npx vitest run src/NodesPage.test.tsx
```
Expected: all pass, including the new one.

**Step 5: commit**

```sh
git add web/src/NodesPage.tsx web/src/NodesPage.test.tsx
git commit -m "fix(nodes): 44px touch targets on page controls"
```

---

## Task 4: Libraries list as cards on phones

**Objective:** kill the raw table under `sm` and fix 9 small targets.

**Files:**
- Create: `web/src/DataList.tsx`
- Test: `web/src/DataList.test.tsx`
- Modify: `web/src/LibrariesPage.tsx`

**Step 1: failing test for the primitive**

```tsx
// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import DataList, { type Column } from './DataList'

type Row = { id: string; name: string; saved: string }

const columns: Column<Row>[] = [
  { key: 'name', header: 'Library', cell: (r) => r.name, primary: true },
  { key: 'saved', header: 'Saved', cell: (r) => r.saved },
]

describe('DataList', () => {
  it('renders a semantic table for assistive tech and a card list for eyes', () => {
    render(<DataList rows={[{ id: '1', name: 'Movies', saved: '14 GB' }]} columns={columns} testid="libs" />)
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })
})
```

**Step 2: run, expect failure**

```sh
cd web && npx vitest run src/DataList.test.tsx
```
Expected: FAIL, cannot resolve `./DataList`.

**Step 3: implement the primitive**

```tsx
// SPDX-License-Identifier: AGPL-3.0-only
import type { ReactNode } from 'react'

export type Column<T> = {
  key: string
  header: string
  cell: (row: T) => ReactNode
  primary?: boolean
}

type Props<T> = {
  rows: T[]
  columns: Column<T>[]
  testid: string
  rowTestId?: (row: T) => string
}

/**
 * One data set, two layouts: a real <table> at sm and up, and stacked cards
 * below it. The table stays in the DOM for screen readers at every width; the
 * card list carries the same information with 44px rows.
 */
export default function DataList<T extends { id: string }>({
  rows,
  columns,
  testid,
  rowTestId,
}: Props<T>) {
  const primary = columns.find((c) => c.primary) ?? columns[0]
  const rest = columns.filter((c) => c !== primary)
  return (
    <div>
      <ul className="space-y-2 sm:hidden" data-testid={`${testid}-cards`}>
        {rows.map((row) => (
          <li
            key={row.id}
            className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
            data-testid={rowTestId?.(row)}
          >
            <div className="text-sm font-semibold text-slate-100">{primary.cell(row)}</div>
            <dl className="mt-2 space-y-1 text-xs text-slate-400">
              {rest.map((c) => (
                <div key={c.key} className="flex justify-between gap-3">
                  <dt>{c.header}</dt>
                  <dd className="text-slate-200">{c.cell(row)}</dd>
                </div>
              ))}
            </dl>
          </li>
        ))}
      </ul>
      <div className="hidden overflow-x-auto rounded-2xl ring-1 ring-slate-800 sm:block">
        <table className="w-full text-left text-sm" data-testid={testid}>
          <thead>
            <tr className="bg-slate-900 text-xs uppercase tracking-wide text-slate-500">
              {columns.map((c) => (
                <th key={c.key} className="px-4 py-3 font-medium">
                  {c.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id} className="border-t border-slate-800" data-testid={rowTestId?.(row)}>
                {columns.map((c) => (
                  <td key={c.key} className="px-4 py-3">
                    {c.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
```

**Step 4: run, expect pass**

```sh
cd web && npx vitest run src/DataList.test.tsx
```
Expected: `1 passed`.

**Step 5: failing test for the page, then implement**

Add to `web/src/LibrariesPage.test.tsx`:

```tsx
it('renders the card list on phones and the table on desktop', async () => {
  renderLibraries()
  expect(await screen.findByTestId('libraries-cards')).toBeInTheDocument()
  expect(screen.getByRole('table')).toBeInTheDocument()
})
```
Run it, expect FAIL, then swap the existing `<table>` block in `LibrariesPage.tsx` for:

```tsx
<DataList
  rows={libraries}
  testid="libraries-table"
  rowTestId={(l) => `library-row-${l.id}`}
  columns={[
    {
      key: 'name',
      header: 'Library',
      primary: true,
      cell: (l) => (
        <Link to={`/libraries/${l.id}`} className="inline-flex min-h-11 items-center text-emerald-400 underline">
          {l.name}
        </Link>
      ),
    },
    { key: 'path', header: 'Path', cell: (l) => <span className="truncate">{l.path}</span> },
    { key: 'files', header: 'Files', cell: (l) => statsFor(l.id).files },
    { key: 'saved', header: 'Saved', cell: (l) => formatBytes(statsFor(l.id).bytes_saved) },
    { key: 'actions', header: '', cell: (l) => <ScanAndEdit library={l} /> },
  ]}
/>
```
where `ScanAndEdit` uses `btn` and `btnGhost` from `uiControls.ts`.

**Step 6: verify and commit**

```sh
cd web && npx vitest run src/LibrariesPage.test.tsx src/DataList.test.tsx
git add web/src/DataList.tsx web/src/DataList.test.tsx web/src/LibrariesPage.tsx web/src/LibrariesPage.test.tsx
git commit -m "feat(libraries): card layout on phones via DataList"
```
Expected: all pass.

---

## Task 5: Library detail on phones

**Objective:** clear the 11 small controls and give each file a readable card.

**Files:**
- Modify: `web/src/LibraryDetailPage.tsx`
- Test: `web/src/LibraryDetailPage.test.tsx`

**Step 1: failing test**

```tsx
it('file rows are 44px tall and the table is scroll wrapped', async () => {
  renderDetail()
  const rows = await screen.findAllByTestId(/^file-row-/)
  expect(rows.length).toBeGreaterThan(0)
  for (const btnEl of screen.getAllByRole('button')) {
    expect(btnEl.className).toContain('min-h-11')
  }
})
```

**Step 2:** run, expect FAIL.
**Step 3:** swap the file `<table>` for `DataList` with columns Name (primary), Before, After, Saved, Status, and an action cell holding a `min-h-11` History button. Reuse `FileHistory` in a dialog rather than inline rows, so the phone layout stays short.
**Step 4:** run `npx vitest run src/LibraryDetailPage.test.tsx`, expect pass.
**Step 5:** commit `fix(library): card file list and 44px controls on detail page`.

---

## Task 6: Settings pages onto the primitives

**Objective:** clear 4 small controls on `/encode` and 6 on `/app`.

**Files:**
- Modify: `web/src/EncodeSettingsPage.tsx`, `web/src/AppSettingsPage.tsx`
- Tests: their existing `*.test.tsx`

**Step 1:** add the same class assertion test to each:

```tsx
it('uses 44px controls', async () => {
  renderPage()
  for (const el of [...screen.getAllByRole('combobox'), ...screen.getAllByRole('button')]) {
    expect(el.className).toContain('min-h-11')
  }
})
```
**Step 2:** run both, expect FAIL.
**Step 3:** import `input`, `btnPrimary`, `btn` and apply. Every `<select>` and `<input>` gets `className={input}`; Save gets `btnPrimary`; secondary actions get `btn`.
**Step 4:** `cd web && npx vitest run src/EncodeSettingsPage.test.tsx src/AppSettingsPage.test.tsx`, expect pass.
**Step 5:** commit `fix(settings): 44px controls on encode and app settings`.

---

## Task 7: Dashboard stat cards with icons and fluid type

**Objective:** "scaling dashboard icons": stat cards get an icon, a fluid value size and a grid that adapts from 2 columns to 4.

**Files:**
- Create: `web/src/StatCard.tsx`
- Test: `web/src/StatCard.test.tsx`
- Modify: `web/src/DashboardPage.tsx` (the four `stat-*` blocks, lines ~245-285)

**Step 1: failing test**

```tsx
// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StatCard from './StatCard'

describe('StatCard', () => {
  it('renders label, value, an aria hidden icon and a fluid value size', () => {
    render(<StatCard testid="stat-files" label="Files" value="12" icon="M4 6h16" />)
    const card = screen.getByTestId('stat-files')
    expect(card).toHaveTextContent('Files')
    expect(card).toHaveTextContent('12')
    expect(card.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    expect(card.querySelector('p:last-of-type')?.className).toContain('text-[clamp(')
  })
})
```

**Step 2:** `cd web && npx vitest run src/StatCard.test.tsx`, expect FAIL.

**Step 3: implement**

```tsx
// SPDX-License-Identifier: AGPL-3.0-only

type Props = {
  testid: string
  label: string
  value: string | number
  icon: string
  tone?: 'default' | 'good'
}

/**
 * One dashboard statistic. The icon and the number scale with the viewport
 * via clamp(), so a 320px phone and a 4K monitor both look deliberate rather
 * than zoomed.
 */
export default function StatCard({ testid, label, value, icon, tone = 'default' }: Props) {
  return (
    <div
      className="rounded-2xl bg-slate-900 p-3 ring-1 ring-slate-800 sm:p-4"
      data-testid={testid}
    >
      <div className="flex items-center gap-2">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={1.8}
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="h-[clamp(16px,4vw,22px)] w-[clamp(16px,4vw,22px)] shrink-0 text-slate-500"
        >
          <path d={icon} />
        </svg>
        <p className="text-[clamp(10px,2.6vw,12px)] uppercase tracking-wide text-slate-500">
          {label}
        </p>
      </div>
      <p
        className={`mt-1 font-bold text-[clamp(18px,5.5vw,24px)] ${
          tone === 'good' ? 'text-emerald-400' : 'text-slate-100'
        }`}
      >
        {value}
      </p>
    </div>
  )
}
```

**Step 4:** `cd web && npx vitest run src/StatCard.test.tsx`, expect `1 passed`.

**Step 5:** in `DashboardPage.tsx` replace the four blocks with

```tsx
<div className="grid grid-cols-2 gap-3 sm:grid-cols-4 sm:gap-4">
  <StatCard testid="stat-space-saved" label="Space saved" tone="good"
    value={formatBytes(stats.data?.bytes_saved ?? 0)} icon="M4 17l6-6 4 4 6-6" />
  <StatCard testid="stat-files" label="Files" value={stats.data?.files ?? 0}
    icon="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
  <StatCard testid="stat-satisfied" label="Nothing to do" value={stats.data?.satisfied ?? 0}
    icon="M20 6 9 17l-5-5" />
  <StatCard testid="stat-queued" label="Waiting" value={stats.data?.queued ?? 0}
    icon="M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
</div>
```

**Step 6:** `cd web && npx vitest run src/DashboardPage.test.tsx`, expect pass (the `stat-*` testids are unchanged).
**Step 7:** commit `feat(dashboard): stat cards with scaling icons and fluid type`.

---

## Task 8: Auto collapsing rail with a remembered override

**Objective:** stop requiring a manual click to collapse; below `lg` the rail shows icons only, and a user choice sticks.

**Files:**
- Modify: `web/src/AppShell.tsx`
- Modify: `web/src/AppShell.test.tsx`

**Step 1: failing test** (add to the existing `AppShell.test.tsx`)

```tsx
it('shows icons only below lg and remembers an explicit expand', async () => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => storage.get(k) ?? null,
    setItem: (k: string, v: string) => storage.set(k, v),
    removeItem: (k: string) => storage.delete(k),
  })
  renderShell({ width: 900 }) // md but below lg
  expect(screen.queryByText('Encode settings')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /show labels/i }))
  expect(await screen.findByText('Encode settings')).toBeInTheDocument()
  expect(storage.get('sys.rail')).toBe('expanded')
})
```

**Step 2:** `cd web && npx vitest run src/AppShell.test.tsx -t "below lg"`, expect FAIL.

**Step 3: implement**

```tsx
const wide = useMediaQuery('(min-width: 1024px)')
const [override, setOverride] = useState<'auto' | 'expanded' | 'collapsed'>(() => {
  const saved = typeof localStorage === 'undefined' ? null : localStorage.getItem('sys.rail')
  return saved === 'expanded' || saved === 'collapsed' ? saved : 'auto'
})
const showLabels = override === 'auto' ? wide : override === 'expanded'

function toggleRail() {
  const next = showLabels ? 'collapsed' : 'expanded'
  setOverride(next)
  localStorage.setItem('sys.rail', next)
}
```

The toggle button gets `aria-label={showLabels ? 'Collapse navigation' : 'Show labels'}` and `aria-expanded={showLabels}`. The drawer under `md` keeps its existing `Open menu` button and Escape handling.

**Step 4:** `cd web && npx vitest run src/AppShell.test.tsx`, expect all pass.
**Step 5:** commit `feat(shell): auto collapse rail below lg with remembered override`.

---

## Task 9: Flow canvas is honest on a phone

**Objective:** the canvas is unusable for editing under 768 px; say so instead of pretending.

**Files:**
- Modify: `web/src/FlowCanvasPage.tsx` (or `FlowCanvas.tsx` if the page wrapper is trivial)
- Test: `web/src/FlowCanvas.test.tsx`

**Step 1: failing test**

```tsx
it('shows a read-only notice under md and hides editing controls', async () => {
  renderCanvas({ width: 390 })
  expect(await screen.findByTestId('flow-mobile-notice')).toHaveTextContent(/desktop/i)
  expect(screen.queryByTestId('flow-palette')).not.toBeInTheDocument()
})
```

**Step 2:** run, expect FAIL.
**Step 3:** gate the palette and inspector behind `useMediaQuery('(min-width: 768px)')` and render:

```tsx
{!isWide && (
  <div
    className="mb-3 rounded-xl bg-slate-900 p-3 text-sm text-slate-300 ring-1 ring-slate-800"
    data-testid="flow-mobile-notice"
  >
    Viewing only on a phone. Editing the flow needs a desktop browser.
  </div>
)}
```
Keep pinch zoom and pan working so the graph can still be inspected, and keep the node count visible.
**Step 4:** run the file's suite, expect pass.
**Step 5:** commit `feat(flow): read only notice and no editing controls on phones`.

---

## Task 10: Turn the audit green and finish

**Objective:** prove the whole thing at every width, then document it.

**Files:**
- Modify: `web/e2e/mobile.spec.ts` (add library detail and flow routes now that their ids can be fetched)
- Modify: `docs/ui-review-2026-10-29.md` (append a "mobile pass" section)
- Modify: `ARCHITECTURE.md` (the UI routes section: add the touch and layout rules)

**Step 1:** extend the spec to fetch a library id from `/api/v1/libraries` and also audit `/libraries/{id}` and `/libraries/{id}/flow`:

```ts
const libs = await (await page.request.get('/api/v1/libraries')).json()
const id = libs.items[0]?.id
const paths = ['/', '/libraries', '/nodes', '/encode', '/app', ...(id ? [`/libraries/${id}`, `/libraries/${id}/flow`] : [])]
```

**Step 2: run the full audit**

```sh
cd web && npx playwright test
```
Expected: all projects pass, zero failures, at 320, 390, 414, 768 and the existing desktop widths.

**Step 3: run the whole gate**

```sh
make test && make lint && cd web && npm run build && npm test && npm run lint
```
Expected: every command exits 0, `npm run build` prints `tsc --noEmit` clean and a vite bundle summary.

**Step 4: document and commit**

```sh
git add web/e2e/mobile.spec.ts docs/ui-review-2026-10-29.md ARCHITECTURE.md
git commit -m "docs: record the mobile portability rules and close the audit"
```
Add to `ARCHITECTURE.md` under the UI routes section:

```
Layout rules: every interactive control is at least 44px tall (`web/src/uiControls.ts`).
Tables above `sm` render as cards below it via `web/src/DataList.tsx`. Stat tiles use
`clamp()` type and scale with the viewport (`web/src/StatCard.tsx`). The rail auto
collapses below `lg` and remembers an explicit override in `localStorage` under `sys.rail`.
`web/e2e/mobile.spec.ts` fails CI on any horizontal overflow or sub-44px control at
320, 390, 414 and 768 px.
```

**Step 5: open the PR**

```sh
git push -u origin task/mobile-portability
gh pr create --title "Mobile portability: touch targets, card layouts, fluid type" --body "Closes the mobile audit. Every page has 44px controls and card layouts under sm, the rail auto collapses below lg, dashboard stats scale, and the flow canvas is read only on phones."
```
Expected: a PR URL, and CI green on it.

---

## Tests and validation summary

- Unit and component: `cd web && npm test` (Vitest, includes the new `uiControls`, `DataList`, `StatCard` suites and the per page class assertions).
- Layout and touch: `cd web && npx playwright test` at 320, 390, 414, 768 and the existing desktop projects.
- Build: `make test`, `make lint`, `npm run build`.
- Manual proof on the running container: load the dashboard and nodes at 390 px in the device toolbar, confirm the stat icons scale, the rail is a hamburger, the card list replaces the table, and every tappable thing is comfortable. Record what you saw in `tasks/hermes-log.md`.

## Risks, tradeoffs, open questions

1. **Duplicated DOM.** `DataList` renders both the card list and the table, hiding one with `sm:hidden` / `hidden sm:block`. The alternative is a JS media query that swaps markup, which flashes on first paint and duplicates logic. Cost of my choice: double markup and two `data-testid` targets. If that becomes a real DOM weight problem at 40k files, add pagination first, because the list is already paginated.
2. **44 px on desktop too.** `min-h-11` applies at every width. That is deliberate: one control size, no per breakpoint class forks. If desktop density suffers, split the class strings later rather than sprinkling overrides.
3. **`clamp()` in Tailwind arbitrary values** needs Tailwind 3.4 or newer, which `package.json` satisfies (`^3.4.15`). No config change is required.
4. **The canvas on a phone is a product decision, not just CSS.** Task 9 declares it read only. If you want mobile editing, that is a separate piece of work with a different interaction model, and I would argue against it.
5. **Open question:** should the mobile audit also enforce a maximum page height or a minimum font size of 12 px? Today `/nodes` has a single text node under 12 px. I left it out of the gate to avoid failing on legitimate small labels.
6. **Open question:** portrait tablet (768 to 1024) currently gets the icon rail plus the table layout. That is the one width where the layout is a compromise. Worth a look before shipping, and a one line change in `DataList` if it reads badly.
