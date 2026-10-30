# Task 21 notes: Fluid dashboard and auto collapsing rail

## Plan (files, in order)

1. `web/src/StatCard.tsx` + `web/src/StatCard.test.tsx` — new primitive: inline SVG icon
   (`aria-hidden`, `clamp()` box), `clamp()` label and value, `tone="good"` variant.
   Commit 1.
2. `web/src/DashboardPage.tsx` — replace the four hand written stat blocks with a
   `grid grid-cols-2 gap-3 sm:grid-cols-4 sm:gap-4` of four `StatCard`s; testids and
   labels unchanged. Add a grid/order assertion to `DashboardPage.test.tsx`. Commit 2.
3. `web/src/AppShell.tsx` — rail collapse becomes: `wide = useMediaQuery('(min-width: 1024px)')`,
   `override` state (`'auto' | 'expanded' | 'collapsed'`) seeded from `localStorage['sys.rail']`
   (unrecognised → `'auto'`), `showLabels = override === 'auto' ? wide : override === 'expanded'`,
   toggle writes the opposite to state and `localStorage`. Aria: `aria-expanded={showLabels}`,
   label `Collapse navigation` / `Show labels`. Drawer under `md` untouched. New
   `stubViewport` matchMedia helper in `AppShell.test.tsx` plus three rail-override tests.
   One-line doc sync in `ARCHITECTURE.md` (shell rules) for the `sys.rail` key. Commit 3.
4. `web/e2e/mobile.spec.ts` — after the page loop, if the `md` rail is present (768 px),
   assert it starts icon only, the toggle shows labels, and the page still has no
   horizontal overflow expanded. Commit 4.

## Verification

`cd web && npx vitest run`, `npm run build`, `npm run lint`, `make test`, `make lint`,
`make e2e` (mobile audit at 320/390/414/768 + layout suite) — acceptance ticks follow
the test results.

## Notes

- `sys.rail` is the first `localStorage` key in the app; documented in ARCHITECTURE.md
  shell rules so later tasks reuse the `sys.` prefix deliberately.
- The existing AppShell "wide" test stubs every media query true, so `wide` is also
  true there and the pre-task-21 behavior of that test is unchanged.
- jsdom gives a real `localStorage`; the rail tests clear it in `beforeEach`.
- Environment: chromium could not launch locally (`libatk-1.0.so.0` et al. missing, no
  sudo). Worked around by `apt-get download` of the 9 library packages into
  `/tmp/opencode/pwlibs`, `dpkg-deb -x` into a local prefix, and running the suite with
  `LD_LIBRARY_PATH` pointing there. No system state changed. CI installs deps properly.
- Pre-existing, unrelated failure: `internal/worker TestNodeDrainFinishesCurrentJob`
  (integration tag) times out at `lifecycle_integration_test.go:109` on this machine.
  Verified identical failure on plain `main` with this branch's changes stashed — not
  caused by task 21 (web-only change). Flagging for the owner; not fixed here.

## Summary (written at the end)

- Built: `StatCard` primitive (icon + fluid clamp type, tone variants), dashboard on a
  2→4 column grid of four StatCards, rail that auto collapses below `lg` with a
  `sys.rail` override that persists, plus a rail-toggle leg in the mobile audit.
- Tests added: StatCard (2), dashboard grid/order (1), AppShell rail override (3),
  e2e rail toggle (runs at 768 px only, no-op below). All suites green: web 180 unit,
  Playwright 61 passed / 3 designed skips, `make test` + `make lint` + build exit 0.
- Skipped (out of scope): theming, a settings toggle for the rail, flow canvas mobile
  editing (task 22), new dashboard data.
- For the next task: the rail labels are hidden at tablet width by design; use
  `override`/`showLabels` in AppShell, not a new boolean. `sys.` is the localStorage
  prefix convention.
- Watch item: the worker drain integration test fails on main too (see Notes); task 22
  should not assume it is caused by UI work, but someone should root-cause it.
