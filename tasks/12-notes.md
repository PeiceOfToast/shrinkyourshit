# Task 12 notes — Responsive tables and Playwright layout tests

## Plan

Files, in touch order:

1. `web/src/AppShell.tsx` — add `data-testid="app-shell"` to the outer shell div so the e2e suite can assert the shell covers the viewport.
2. `web/src/LibraryDetailPage.tsx` — the only real `<table>` in the app (the rest of the pages are card lists already):
   - keep the `overflow-x-auto` wrapper for `sm` and up;
   - under `sm` render the file list as stacked cards (`hidden sm:table`-style split): File and Status stay primary, Why/Size/Saved collapse into label/value pairs;
   - long paths/reasons wrap deliberately (`break-words`), never force width.
3. `web/src/NodesPage.tsx` — long node names get `min-w-0 break-words` so a long name wraps instead of pushing the row wide.
4. `web/e2e/` (new):
   - `web/playwright.config.ts` — three projects: phone 390x844, tablet 768x1024, desktop 1280x800; screenshots on failure; global setup/teardown.
   - `web/e2e/global-setup.ts` — serves the **real binary**: if `SHRINK_E2E_BASE_URL` is set, target that server (CI: the docker image running the Go binary); otherwise build/start `bin/shrinkyourshit` with a throwaway data dir, discovery off. Both modes: complete first-run setup via `POST /api/v1/setup` (login off), generate one small test media file with ffmpeg (`-f lavfi -i testsrc`, never committed), create + scan a library through the API, and wait for a file row.
   - `web/e2e/layout.spec.ts` — per page (`/`, `/libraries`, `/nodes`, `/encode`, `/app`) per viewport: no horizontal overflow (`scrollWidth <= innerWidth`), shell covers the viewport, nav reachable (drawer button under `md`, rail at `md`+). Plus: at 390 px the file table renders as cards (table hidden, cards visible, each card fits the viewport).
5. `web/package.json` — `@playwright/test` dev dep (STACK.md pins Playwright for this task), `e2e` script.
6. `Makefile` — `e2e` target: build web, build the binary, install the browser if missing, run Playwright.
7. `.github/workflows/ci.yml` — `e2e` job: build web, build the image, run the container, install the browser, run the tests against the container, upload screenshots on failure; added to the `report` job.
8. `README.md` — document `make e2e` and the env overrides.

## Environment notes

- This VM has no browser and no sudo; Playwright chromium needs libnss3 etc. If the local run cannot be made to work without root, verification of the pixel assertions happens through the CI `e2e` job on the PR (which the task expects to be green anyway).
- ffmpeg/ffprobe exist on the VM (`~/.local/bin`), so the seeded library can produce file rows locally too.

## Acceptance verification

- [ ] `npm run e2e` at all three viewports for all five pages
- [ ] deliberately broken layout fails the overflow assertion (scratch commit), recorded in `tasks/hermes-log.md`, reverted
- [ ] CI runs e2e on every PR, green on the task branch
- [ ] at 390 px the file table renders as cards (Playwright assertion)
- [ ] `make e2e` documented in README, works on a clean checkout

## Summary (end of run)

- Built: card layout for the file table under `sm`, deliberate wrapping for long text, `data-testid="app-shell"`, a Playwright suite (3 viewports x 5 pages: overflow, shell coverage, nav reachability + file-table card check), a self-managed server fixture that drives the real Go binary (or an external one for CI), a CI `e2e` job, a `make e2e` target and README docs.
- Skipped: visual-diff baselines, tab bar, accessibility checks (out of scope per task file).
- Next task should know: `@playwright/test` is in web devDependencies; the e2e suite is driven by `SHRINK_E2E_BASE_URL` (external server) or starts `bin/shrinkyourshit` itself; seeded library id is exported from global setup via `process.env.SHRINK_E2E_LIBRARY_ID`-style metadata in `web/e2e/fixtures.ts`.
