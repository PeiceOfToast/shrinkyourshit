# Task 10: UI shell, navigation and dashboard

## Goal
The app is usable on a phone, has five clear pages, and the dashboard is read only. Today the shell is a fixed 224 px sidebar with no breakpoints, so on a 390 px phone the content column is 166 px, and the dashboard contains the add library form.

## Read first
UI review at `docs/ui-review-2026-10-29.md` sections 2, 4 and 5. ARCHITECTURE.md (repo layout, UI routes). Depends on task 09.

## Do
- `web/src/index.css`: paint the page background on `html, body` (`bg-slate-950`) so no region outside the shell renders white. The shell div alone painting the background is the cause of the white band on short pages.
- `web/src/AppShell.tsx`: responsive rewrite.
  - Under `md`: a top bar with the product name, a menu button that opens the nav as a slide over drawer, and a live job indicator. Nav closes on route change and on Escape.
  - `md` and up: the current left rail, collapsible to icons.
  - Use `min-h-dvh` instead of `min-h-full` for the shell so mobile browser chrome does not clip it.
  - Nav links get at least 44 px touch targets.
- New `web/src/PageContainer.tsx`: one shared container, `mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8`. Every page uses it, and per page `max-w-*` guessing is removed.
- Nav and routes, five pages:
  - Dash `/`
  - Library `/libraries`, `/libraries/new`, `/libraries/:id`, `/libraries/:id/edit`
  - Nodes `/nodes`
  - Encode settings `/encode`
  - App settings `/app`
- Split `SettingsPage.tsx` along its existing two headings: library defaults (codec, quality, container, audio and subtitle policy, result policy) go to `/encode`; server, auth and paths go to `/app`. Keep `/settings` as a redirect to `/app` so existing links and tests keep working.
- `DashboardPage.tsx`: read only.
  - Remove the embedded `EasyLibraryForm` and the Add folder button.
  - Empty state becomes one line plus a link to `/libraries/new`.
  - Keep stats, live jobs, node activity, recent history.
  - Add a warning surface driven by real state: queue not moving because every node is stopped or disabled, node heartbeat lost, failed jobs in the last 24h. Wording must name the cause and link to the fix.
- Vitest: nav renders five entries, `/settings` redirects to `/app`, dashboard renders no form and no create button.

## Out of scope
Table and card responsive layouts (task 12), toasts and dialogs (task 13), node start and stop controls (task 11), any change to encoding behaviour.

## Acceptance
- [ ] `html` and `body` carry the background colour; on a 390 px viewport no region of the page renders white below or beside the shell. (`App.test.tsx` asserts the background class is present on `document.documentElement` or body, plus the Playwright check in task 12)
- [ ] At 390 px the main content column is at least 340 px wide and the nav is reachable by opening the drawer. (`AppShell` test with a mocked narrow matchMedia, or Playwright at 390 in task 12)
- [ ] Five nav entries exist with the labels Dash, Library, Nodes, Encode settings, App settings, and each route renders its page. (`App.test.tsx`)
- [ ] `/settings` returns a redirect to `/app`. (`App.test.tsx`)
- [ ] The dashboard renders no form element and no create button; the empty state links to `/libraries/new`. (`DashboardPage.test.tsx`)
- [ ] The dashboard warning surface renders when every node is stopped or disabled and links to `/nodes`. (`DashboardPage.test.tsx`)
- [ ] `make lint` and `make test` pass, and `npm run build` succeeds with no TypeScript errors.
- [ ] Manual check at 390 px and 1280 px recorded in `tasks/hermes-log.md`.
