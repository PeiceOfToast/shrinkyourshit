# Task 21: Fluid dashboard and auto collapsing rail

## Goal
The dashboard scales with the screen instead of being a desktop grid shrunk down: stat tiles get icons and `clamp()` type, and the navigation rail collapses itself below `lg` while remembering a deliberate override. This is the visible half of "it should adapt to whatever screen is there".

## Read first
`docs/mobile-portability-2026-10-30.md` (tasks 7 and 8 of that plan), `web/src/AppShell.tsx`, `web/src/useMediaQuery.ts`. Depends on task 20, because `StatCard` and the audits live on those primitives.

## Do

### 1. `StatCard` component
New `web/src/StatCard.tsx`, props `testid`, `label`, `value`, `icon` (an SVG path `d` string), optional `tone` of `default` or `good`.

- The icon is inline SVG, no icon package, drawn at `h-[clamp(16px,4vw,22px)] w-[clamp(16px,4vw,22px)]`, `aria-hidden="true"`, `stroke="currentColor"`, `fill="none"`, `strokeWidth={1.8}`, `text-slate-500`.
- The label is `text-[clamp(10px,2.6vw,12px)] uppercase tracking-wide text-slate-500`.
- The value is `text-[clamp(18px,5.5vw,24px)] font-bold`, `text-emerald-400` when `tone` is `good`, otherwise `text-slate-100`.
- Container: `rounded-2xl bg-slate-900 p-3 ring-1 ring-slate-800 sm:p-4`.

Test asserts the testid renders, the label and value are present, the `svg` has `aria-hidden="true"`, and the value paragraph class contains `text-[clamp(`.

### 2. Dashboard onto `StatCard`
`web/src/DashboardPage.tsx`: replace the four hand written stat blocks (roughly lines 245 to 285 today) with a grid `grid grid-cols-2 gap-3 sm:grid-cols-4 sm:gap-4` of four `StatCard`s. Testids and order unchanged: `stat-space-saved`, `stat-files`, `stat-satisfied`, `stat-queued`. Keep the labels that exist today: Space saved, Files, Nothing to do, Waiting. Space saved keeps `tone="good"`.

Suggested icon paths (24x24, stroke only):
- space saved: `M4 17l6-6 4 4 6-6`
- files: `M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z`
- nothing to do: `M20 6 9 17l-5-5`
- waiting: `M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z`

### 3. Rail that collapses itself
`web/src/AppShell.tsx`: the rail currently collapses only when a human clicks. Change it to:

- `const wide = useMediaQuery('(min-width: 1024px)')`
- state `override` of `'auto' | 'expanded' | 'collapsed'`, initialised from `localStorage.getItem('sys.rail')`, falling back to `'auto'` when absent or unrecognised.
- `const showLabels = override === 'auto' ? wide : override === 'expanded'`
- The toggle writes the opposite choice to `setOverride` and to `localStorage` under the key `sys.rail`.
- Toggle button accesses `aria-label={showLabels ? 'Collapse navigation' : 'Show labels'}` and `aria-expanded={showLabels}`.

Behaviour to preserve exactly: under `md` the rail is replaced by the `Open menu` button and the drawer, with its existing Escape and backdrop close. Do not regress the drawer.

### 4. Tests
- `web/src/StatCard.test.tsx` as described.
- `web/src/AppShell.test.tsx`: at 900 px (md but below lg) the labels are hidden by default; clicking the toggle shows them and writes `sys.rail=expanded`; a second render with `localStorage.setItem('sys.rail', 'collapsed')` at 1280 px stays collapsed. Existing AppShell tests must keep passing; the media query stub they already use still applies.

## Out of scope
Per user theming, a settings toggle for the rail, mobile editing of the flow canvas (task 22), any new dashboard data.

## Acceptance
- [ ] `web/src/StatCard.tsx` exists with an `aria-hidden` inline SVG icon and `clamp()` sizing, proven by `StatCard.test.tsx`.
- [ ] The dashboard renders four `StatCard`s in a 2 column grid below `sm` and 4 columns at `sm` and up, with the testids `stat-space-saved`, `stat-files`, `stat-satisfied` and `stat-queued` unchanged.
- [ ] Below `lg` the rail shows icons only without any click, and at `lg` and up it shows labels.
- [ ] An explicit toggle persists across reloads via `localStorage` key `sys.rail`, and is proven by an AppShell test.
- [ ] The toggle exposes `aria-expanded` and a label that matches its action.
- [ ] Under `md` the `Open menu` drawer still opens, closes on Escape and closes on backdrop click.
- [ ] The mobile audit from task 20 still passes at 320, 390, 414 and 768 px, including the dashboard and the rail toggle.
- [ ] `make test`, `make lint`, `cd web && npm run build` exit 0.
