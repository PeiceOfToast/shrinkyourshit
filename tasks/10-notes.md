# Task 10 notes: UI shell, navigation and dashboard

## Plan

Branch `task/10-ui-shell-navigation` from `main`. Commits, in order:

1. **Background + container**: `web/src/index.css` paints `html, body`
   (`@apply bg-slate-950`, plus `color-scheme: dark`); App sets the same
   classes on `documentElement`/`body` at mount so the vitest assertion from
   the acceptance ("class present on documentElement or body") is a real
   runtime behaviour, not a file grep. New `web/src/PageContainer.tsx`
   (`mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8`).
2. **Shell rewrite**: `web/src/AppShell.tsx` — new `useMediaQuery` hook
   (`(min-width: 768px)`), drawer under `md` (top bar: product name, menu
   button, live job indicator; closes on route change and Escape; overlay;
   logout moves into the drawer footer), left rail at `md`+ collapsible to
   icons, `min-h-dvh` on the shell, 44 px min touch targets on nav links.
   Five nav entries: Dash, Library, Nodes, Encode settings, App settings.
3. **Settings split**: `SettingsPage.tsx` → `EncodeSettingsPage.tsx`
   (library defaults: languages, default video codec, default result
   policy) + `AppSettingsPage.tsx` (concurrency, scratch dir, login).
   `settingsModel.ts` unchanged (both pages reuse `toForm`/`buildUpdate`).
   `App.tsx`: routes `/encode`, `/app`, `/settings` → `<Navigate to="/app">`.
   EasyLibraryForm drops its `max-w-2xl` (the page container owns width).
4. **Dashboard read-only**: remove `EasyLibraryForm` + "Add folder" button;
   empty state is one line linking `/libraries/new`; nodes query moves to
   the page level (shared by warnings + activity list); new
   `dashboardWarnings.ts` pure module — warnings: every node
   stopped/disabled while files are queued and nothing is running; approved
   node with no/fresh heartbeat for >3 min (heartbeat interval is 30 s,
   server sweep 120 s); failed jobs in the last 24 h. Each names the cause
   and links to the fix (`/nodes`, `/libraries`).
5. **PageContainer adoption** on all pages (Dashboard, Libraries, Nodes,
   LibraryDetail, LibraryEdit, NewLibrary, both settings pages); per-page
   `max-w-*` removed.
6. **Tests**: `AppShell.test.tsx` (mocked narrow/wide `matchMedia`: drawer
   opens/closes on Escape and on route change, five entries, 44 px targets,
   no fixed rail in narrow, rail in wide, job indicator); `App.test.tsx`
   (five entries, `/settings`→`/app`, each route renders its page, bg class
   on html+body, empty dashboard has no form/create button);
   `DashboardPage.test.tsx` (empty state link, no form/create, warnings);
   `dashboardWarnings.test.ts` for the pure logic. Then lint/format/tests.

## Decisions taken (within the docs' framing)

- **matchMedia-driven shell** (`useMediaQuery`, `(min-width: 768px)`): the
  task's acceptance asks for "an AppShell test with a mocked narrow
  matchMedia", which implies the shell consults `matchMedia` rather than
  pure CSS visibility. jsdom default (no matchMedia) falls back to desktop
  so existing tests keep seeing the rail nav.
- **Warnings module is pure** (`dashboardWarnings.ts`) so the three
  conditions are unit-tested without React; timestamps are injected.
- **Result policy default moves to `/encode`** (task text: "library
  defaults (codec, quality, container, audio and subtitle policy, result
  policy) go to /encode"), even though the old page had it under "Server".
  `/app` keeps concurrency, scratch dir and login. No new settings keys —
  API.md task 10 says the endpoints stay the same, so quality/container/
  audio defaults (which live per-library, not in settings) are not added.
- **Job indicator** counts `running` + `claimed` jobs under query key
  `['jobs', 'indicator']`; the shell's SSE invalidation of the `['jobs']`
  prefix keeps it live. Hidden when the count is 0.
- **Heartbeat staleness threshold: 3 minutes** — worker heartbeats every
  30 s, the server requeues at 120 s; 3 min is safely past both and avoids
  flapping on a slow LAN.
- **Failed-jobs warning links to `/libraries`** (retry lives on the library
  detail page; the dashboard cannot know which library failed).
- **No localStorage persistence for the rail collapse** — resets per load;
  persistence would be bookkeeping beyond the task.

## Acceptance → test mapping

1. Background on html/body → `App.test.tsx` (class on `documentElement`
   and `body` after App renders; CSS rule in `index.css` paints without JS).
2. 390 px content column + reachable drawer → `AppShell.test.tsx` narrow
   matchMedia (no fixed 224 px rail, drawer opens with all five entries,
   full-width fluid column). True pixel check is task 12's Playwright job.
3. Five nav entries + routes render → `App.test.tsx`.
4. `/settings` redirect → `App.test.tsx`.
5. Dashboard: no form, no create button, empty-state link →
   `DashboardPage.test.tsx` + `App.test.tsx`.
6. Warning surface (all nodes stopped/disabled → links `/nodes`) →
   `DashboardPage.test.tsx` + `dashboardWarnings.test.ts`.
7. `make lint`, `make test`, `npm run build` → run locally.
8. Manual 390/1280 check → recorded in `tasks/hermes-log.md`; this VM has
   no browser binary, so the check is done with the mocked-matchMedia
   structural tests at both viewports (no browser on the VM — Playwright
   arrives in task 12).

## Out of scope (recorded, not built)

- Table/card responsive layouts (task 12), toasts/dialogs (task 13), node
  start/stop (task 11), encoding behaviour changes (task file).
- Mobile bottom tab bar (review's option, drawer chosen per task).
- Breadcrumbs on detail pages (review section 5 detail; not in task Do list).
