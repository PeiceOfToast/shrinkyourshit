# Task 51 notes: Setup and Login (spec 5.1, 5.2)

## Plan

1. `SetupWizard.tsx`: visual stepper (`ol`, `aria-current="step"`), step titles as the only h1, restyle the three
   steps with uiControls (`input`, `btnPrimary`, `btnGhost`), Switch for the login choice, `Field` for
   username/password/confirm, Banner for validation and submit errors. `mode-join` becomes a `button`; the server
   list and pairing form sit beside it (a button cannot hold a list or an input), inside the same option group.
2. `SetupPage.tsx`: `main.min-h-dvh`, brand mark plus wordmark as a `div` (no h1), joined panel with one h1 and an
   info Banner.
3. `LoginPage.tsx`: `main.min-h-dvh`, Card (`card` string, `max-w-sm`) with the brand mark, h1 "Log in", subtitle,
   two `Field`s, error Banner above the button (`login-error` kept), `btnPrimary` + `btnBlock` submit.
4. Tests: new files for the acceptance items (see below). Existing test files are untouched.

## Changed assertions

None. No existing assertion in `SetupWizard.test.tsx`, `App.test.tsx` or any other file was edited or removed.
`git diff --stat` shows no change to any existing test file. Every `data-testid` from the old markup survives:
`mode-server`, `mode-join`, `join-servers`, `join-server-*`, `join-token`, `join-confirm`, `join-cancel`,
`login-toggle`, `username`, `password`, `confirm`, `step-errors`, `start-node-now`, `leave-node-stopped`,
`submit-error`, `wizard-back`, `wizard-next`, `wizard-finish`, `joined-panel`, `login-form`, `login-username`,
`login-password`, `login-error`, `login-submit`.

Wording and structure changes that no existing assertion checks (listed for the owner, not test changes):
- Join card: the pairing code label is now a `Field` labelled "Pairing code", optional, with help text "Only if the
  server gave you one." (was "Pairing code (optional, if the server gave you one)").
- Step 3: the summary is a `dl` (Role, Login) instead of one sentence.
- Joined panel body: the sentence is shortened; the Banner carries "Approve this machine on the server's Nodes page."
- Step 3 start-node choice stays two radios (`leave-node-stopped` / `start-node-now`), restyled. The spec's wording
  says "Checkbox", but its own rule is "whatever exists today, restyled", and the existing tests assert radio state.
  I followed the rule. Changing it to a checkbox would change the asserted state and needs an owner call.

## New tests (all pass without ffmpeg, all run in `npm run test`)

- `web/src/SetupWizardLayout.test.tsx` (8 tests): one h1 per step; stepper `aria-current` on the current step and
  "(done)" on finished steps; `mode-join` is a BUTTON, disabled with no server, `aria-pressed` tracks the pick, and
  the server list sits outside it; 44 px classes on inputs and buttons; Back is `invisible` and disabled on step 1;
  step-2 errors are a danger Banner; the login choice is a labelled switch.
- `web/src/LoginPage.test.tsx` (4 tests): one h1 "Log in" and `main.min-h-dvh`; `min-h-11` on the fields and the
  submit button; fields labelled through `Field`; a wrong-password error is a danger Banner above the submit button.
- `web/src/SetupPage.test.tsx` (2 tests): the brand is a div, not a heading, one h1 on step 1, `main.min-h-dvh`;
  the joined panel has one h1 and an info Banner (see the pre-existing issue below for how the test reaches it).

## Verification

- `web`: `npx tsc --noEmit` clean. `npm run lint` clean (`--max-warnings 0`). `npx prettier --check .` clean.
  `npx vitest run`: 61 files, 431 tests passed (was 58 / 416 at task 50; +15 new). `npm run build` clean.
- `go test ./...`: all packages ok. No Go source changed.
- `make build` steps run by hand (make is not installed here): `web/dist` copied to `internal/webui/dist`, then
  `go build -trimpath -o bin/shrinkyourshit ./cmd/shrinkyourshit`. Both ran clean.
- `npx playwright test` NOT RUN LOCALLY. Two blockers: (a) the port 8712 default is held by an old server process
  (the same as the task 50 run), so I used `SHRINK_E2E_PORT=8741`; (b) Chromium cannot launch here because the
  browser needs system libraries that are not installed (`libatk-1.0.so.0`, `libatk-bridge`, `libcups`, `libgbm`,
  `libasound`, `libcairo`, `libpango`, and others), and there is no passwordless sudo to install them. The e2e
  suite does not visit `/setup` or `/login` (global setup completes setup with login off), so this change should not
  move any e2e assertion. CI is the verification path for the Playwright gate; see the PR.
- `make lint` substitutes: `npm run lint` and `prettier` above. `golangci-lint` is not installed here; no Go changed.
- Visual check (screenshots): not done. No browser is available here. Layout is from the spec class strings and
  reads only the tokens already in use on other restyled pages.

## Pre-existing defect (not changed here): the joined panel never shows in the real flow

`SetupPage.tsx` calls `markSetupComplete(status)` on success. The server answers a join with
`setup_complete: true` (`internal/api/pair.go`, around line 219). `RedirectWhenSetupComplete` then sends the user
to `/` before `setJoinedServer` renders the joined panel. So the panel, and this task's h1 and Banner on it, can
only be reached when `setup_complete` is false. The old code had the same behaviour, so this predates task 51.
The test reaches the panel with a mocked `setup_complete: false`, with a comment saying so.

**Owner question:** should a join keep the joined panel (skip `markSetupComplete` for `mode === 'join'`, one line in
`SetupPage.tsx`), or should the user land on `/` as now? The first matches the comment ("Tell the user what happens
instead of navigating"). It changes the join flow, so I did not make it.

## Summary

1. **Built:** the setup wizard (stepper, one h1 per step, join as a button, Switch login choice, `Field` inputs, Banner
   errors, 44 px uiControls), the setup page frame (min-h-dvh, wordmark as a div, joined panel with a Banner), and the
   login page (Card, brand, h1 "Log in", Field inputs, Banner error, `login-error` kept). Three new test files, 15 tests.
2. **Skipped:** the Playwright run and a screenshot check (no browser libraries here). CI is the check. No Go or
   `api.ts` change. No new dependency.
3. **Owner calls:** the joined-panel reachability bug above; keep the start-node radios vs the spec's "Checkbox"; the
   step-2 error banner (spec says "inline"; I used a danger Banner placed inside the step).
4. **Next task (52, copy sweep) should know:** the copy on Setup and Login changed in three places (join pairing
   label, step-3 summary, joined-panel sentence). Check those words against the copy sweep. The guards
   (`guards.tsx` FullPageCard, Loading, RequireReady) still use slate classes and are out of this task's scope.
5. **Not done here:** Playwright and visual checks. Both need CI or a machine with the browser libraries.
