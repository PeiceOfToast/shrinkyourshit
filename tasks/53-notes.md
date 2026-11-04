# Task 53 notes: guardrail lint and audit

## Plan (as run)

1. `web/eslint.config.js`: `no-restricted-syntax` on JSX `className` values. Three patterns:
   raw palette names (`slate|emerald|red|amber|sky|rose-\w`), `opacity-50`, and text below 12px
   (`text-[Npx]`, N < 12). Each pattern matches every literal or template part inside a
   `className` attribute, so ternaries and template literals are covered.
2. Probe the rule on a scratch `.tsx` (deleted after the run), then the throwaway commit
   proof (see below).
3. `web/src/guards.tsx`: move the route-guard card off raw palette classes. It was the only
   non-test source file still using them (the spec's zero-raw-palette acceptance).
4. `web/e2e/mobile.spec.ts`: add `/integrations` and `/libraries/:id/rules`, and an exact
   one-h1 check in `auditPage`, which runs at every width.
5. `web/e2e/layout.spec.ts`: an exactly-one-h1 test per route in `layoutSuite`, plus
   `/libraries/new` and `/integrations` in `PAGES`.
6. `ARCHITECTURE.md`: one paragraph in the UI routes section pointing to the spec and the
   token rule. Nothing else in that file changed.

## Changed assertions

None. No existing assertion was edited or deleted.

- `web/src/App.test.tsx:192-195` still asserts `bg-slate-950` on `html` and `body`. That mirror
  is kept on purpose (spec 2.9), so the assertion stands.
- No test asserted the classes changed in `guards.tsx`. Its `data-testid`s are unchanged
  (`guard-error`).
- `web/src/Card.tsx`: a doc comment that quoted the old class string was reworded. It is not
  an assertion.

## Choices I made (owner can redirect)

- **Lint scope.** The rule runs on every `.ts` and `.tsx` file, tests included. No test uses a
  retired class in a `className`, so nothing was excluded.
- **"text-[1…" read as text below 12px.** The regex is `text-[Npx]` for N < 12, so
  `text-[length:var(--x)]` and `text-[13px]` pass. Spec 2.3 bans `text-[10px]` and `text-[11px]`,
  and that is what it catches.
- **`guards.tsx` card.** `rounded-2xl` became `rounded-card`, `shadow-xl` was dropped (spec 2.6,
  level 1), and palette names became tokens. `p-10` stays: spec 2.4 retires `10`, but that is a
  spacing pass, not a palette one, so I left it. Owner call.
- **Single-h1 check.** Added to both specs. Setup and Login are outside the shell and not in
  the audited list, so they are not checked here.

## Raw palette sweep (acceptance 3)

`grep -rnE '\b(slate|emerald|red|amber|sky|rose)-[0-9]{2,3}\b' web/src web/e2e` now returns only:

- `web/src/App.tsx:41-42`: the `bg-slate-950` body mirror (spec 2.9, kept on purpose).
- `web/src/App.test.tsx:192-195`: its assertion on that mirror.
- `web/src/tokens.test.ts:118`: a test title that names `sky-300`, not a class.

`web/src/index.css` also holds palette names, as spec 2.2 allows.

## Verification

- `make test`: exit 0. Go suite and web Vitest (444 tests, 62 files) pass.
- `make lint`: exit 0. `go vet`, `golangci-lint`, SPDX, `npm run lint --max-warnings 0`, Prettier check.
- `npm run build`: exit 0 (`tsc --noEmit` and `vite build`).
- `make e2e` (Playwright): 107 passed, 3 skipped, exit 0. The mobile audit ran at 320, 390,
  414 and 768 px across `/`, `/libraries`, `/libraries/new`, the library detail, edit, rules
  and flow routes, `/nodes`, `/encode`, `/app` and `/integrations`. Each width passed the
  overflow, 44 px and single-h1 checks.
- Lint throwaway proof: see the commit log on the branch (a `text-slate-500` commit, a failing
  `npm run lint`, then `git revert`).

### Environment caveats

- Chromium needed `libatk-1.0.so.0` and related libraries, which are not installed system-wide
  and need root. I fetched the Ubuntu noble `.deb` packages with `apt-get download`, unpacked
  them with `dpkg -x` into `~/.local/pwlibs`, and ran Playwright with `LD_LIBRARY_PATH` pointing
  there. Nothing system-wide changed.
- Port 8712 was already in use by two `bin/shrinkyourshit serve` processes started at 06:16
  (not by this run). I ran the e2e with `SHRINK_E2E_PORT=8723` and left those processes alone.

## Owner items (not blocking)

1. `guards.tsx` keeps `p-10`, which spec 2.4 retires. Spacing sweep, not this task.
2. Palette sweep covers `web/src` and `web/e2e`. The `bg-slate-950` mirror is the one
   sanctioned exception.

## Summary

1. **Built:** the `no-restricted-syntax` guardrail for retired classes in JSX `className`
   values; `guards.tsx` moved to tokens; `/integrations` and `/libraries/:id/rules` added to the
   mobile audit; an exactly-one-h1 check at every audited route and width; the ARCHITECTURE.md
   pointer paragraph.
2. **Skipped:** no changes to Go or to `api.ts`, and no new dependencies. No existing assertion
   changed. The `p-10` spacing in `guards.tsx` was left alone (owner item 1).
3. **Owner calls:** the `p-10` spacing; the lint rule's reading of "text-[1…" as sub-12px.
4. **Next task should know:** the lint rule is enforced on every `className` in `web/src`
   and `web/e2e`. Test files count too. Add a token to `index.css` and `tailwind.config.js`
   before writing a new colour. The `bg-slate-950` body mirror in `App.tsx` is the one
   permitted raw palette name outside `index.css`.
5. **Not done here:** the CI run on the PR (no `gh` in this run; the supervisor opens it).
