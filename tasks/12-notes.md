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

- [x] `npm run e2e` at all three viewports for all five pages (local: 48 passed, 3 conditional skips; CI run 40969686304 green, files seeded)
- [x] deliberately broken layout fails the overflow assertion (scratch commit 4cfaedb), recorded in `tasks/hermes-log.md`, reverted (3901189)
- [x] CI runs e2e on every PR, green on the task branch (run 40969686304, e2e job 2m04s)
- [x] at 390 px the file table renders as cards (Playwright phone test + vitest structural test)
- [x] `make e2e` documented in README, works on a clean checkout (fresh npm ci, fresh browser download, fresh binary build)

## Environment notes (fill-in at end)

- The VM has no browser system libs and no sudo; chromium runs here with `LD_LIBRARY_PATH=/tmp/opencode/pwlibs/lib` (debs from `apt-get download` + `dpkg -x`, no root). Any normal machine uses `npx playwright install-deps chromium`; README says so.
- `go test -tags=integration ./internal/worker` flaked once on TestNodeDrainFinishesCurrentJob under full parallel load; passes 3/3 in isolation and on a full rerun — pre-existing task-11 harness timing, not touched by this task.
- The suite's first local run caught a real 99px horizontal overflow on `/encode` at 390px (`<select>` sizes to its longest option); fixed with `w-full` on full-width form controls (also applied to the other pages' forms).

## Summary (end of run)

- Built: card layout for the file table under `sm` (label/value pairs for secondary columns), deliberate wrapping for long paths/reasons/node names, `w-full` on form controls, `data-testid="app-shell"`, the Playwright suite (3 viewports x 5 pages: overflow, shell coverage, nav reachability + file-table card checks), a server fixture that drives the real Go binary (or an external one for CI), a CI `e2e` job, `make e2e`, README docs. PR #13, CI green including e2e.
- Skipped: visual-diff baselines, tab bar, accessibility checks (out of scope per task file); no job-log UI exists yet so "ffmpeg logs" needed no new handling beyond the wrapping rules.
- Next task should know: `@playwright/test` is in web devDependencies; `npm run e2e` needs a built binary (`make e2e` does everything); the suite is driven by `SHRINK_E2E_BASE_URL` (external server) or starts `bin/shrinkyourshit` itself; the seeded library id is in `process.env.SHRINK_E2E_LIBRARY_ID` (set by `web/e2e/global-setup.ts`); vitest excludes `e2e/**`.
