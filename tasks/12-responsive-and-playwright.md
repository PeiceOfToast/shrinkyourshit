# Task 12: Responsive tables and Playwright layout tests

## Goal
No page scrolls sideways or leaves unpainted regions at phone, tablet or desktop width, and CI fails when a layout regression brings the bug back.

## Read first
UI review at `docs/ui-review-2026-10-29.md` sections 2 and 8. Depends on task 10.

## Do
- Responsive content:
  - Every table gets an `overflow-x-auto` wrapper. Today only `LibraryDetailPage` has one.
  - Under `sm`, file and node tables render as stacked cards: secondary columns collapse into a label and value pair, primary columns stay visible.
  - Long paths, ffmpeg logs and node names wrap or truncate deliberately instead of forcing width.
- Playwright in `web/e2e`:
  - Config with three projects: 390x844, 768x1024, 1280x800.
  - For each of the five pages: assert `document.documentElement.scrollWidth <= innerWidth` (no horizontal overflow), assert the shell covers the viewport (no unpainted region below or beside it), assert the nav is reachable (drawer under md, rail at md and up).
  - Capture a screenshot per page per viewport as a CI artifact on failure.
  - Serve the built app from the Go server during the run so the test exercises the real binary, not the Vite dev server.
- `.github/workflows/ci.yml`: add a `e2e` job that builds the image, runs the tests, and uploads artifacts on failure. Private to public is moot now, Actions is free on the public repo.
- `make e2e` target, documented in README.

## Out of scope
Visual diff screenshots against baselines, mobile device farm testing, accessibility auditing beyond what task 13 covers.

## Acceptance
- [ ] `npm run e2e` passes at all three viewports for all five pages.
- [ ] A deliberately broken layout (a fixed 900 px wide div added in a scratch branch) fails the overflow assertion, and the failure text names the page and the overflow in px. Recorded in `tasks/hermes-log.md`, then reverted.
- [ ] CI runs the e2e job on every pull request and the job is green on the task branch.
- [ ] At 390 px the file table renders as cards, not a squeezed table. (Playwright assertion on a layout class or a computed width check)
- [ ] `make e2e` documented in README and working on a clean checkout.
