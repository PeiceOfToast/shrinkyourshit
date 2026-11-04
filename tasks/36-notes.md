# Task 36 notes: design tokens, font, motion

## Plan (files, in order)

1. `web/package.json`, `web/package-lock.json`: add `@fontsource-variable/inter` (dependency justification below).
2. `web/src/index.css`: spec 2.9 `:root` tokens (20 variables), latin-only `@font-face`, html/body on tokens, sky-300 focus ring, dialog backdrop on tokens, reduced-motion rule, React Flow variables on `main .react-flow`. Existing React Flow 44px and attribution rules kept.
3. `web/tailwind.config.js`: spec 2.9 config verbatim (colours, `fontFamily.sans`, `fontSize`, radii, shadow, durations, easing, z-index).
4. `web/tailwind.config.d.ts` (new): type declaration so `src/tokens.test.ts` can import the JS config under strict tsc.
5. `web/src/tokens.test.ts` (new): config and stylesheet assertions.
6. `STACK.md`: "Fonts" row under Frontend.
7. Verify: `npm run test`, `make test`, `make lint`, `make build`, `npm run build` (one Inter woff2), `npx playwright test`.

## Dependency justification (copied from spec 8.5, AGENTS rule 3)

`@fontsource-variable/inter` is the only addition.
- **Why:** the app renders in whatever sans-serif the OS falls back to. On the Linux and NAS browsers this audience uses, that is DejaVu Sans, whose wide metrics make the whole UI look unfinished. A pinned font makes layout and line breaks the same on every client.
- **Why this package:** it self-hosts the font inside the Vite bundle, so it is embedded in the Go binary and works offline. There is no CDN call.
- **License:** OFL-1.1, compatible with distributing alongside AGPL-3.0.
- **Cost:** about 48 KB woff2 (latin subset, variable weight).
- **No duplicate:** nothing on the stack provides fonts.
- Rejected: a Google Fonts link (external request, broken offline); a system-font-only stack (does not fix Linux).

Installed version: `^5.3.0` (licence confirmed as OFL-1.1 with `npm view`).

## Deviations from the task file (read these)

1. **Font import is in `index.css`, not `main.tsx`.** The spec's import line, `import '@fontsource-variable/inter/wght.css'`, declares seven subset faces, and the build emits seven woff2 files. The acceptance item says the bundle includes one Inter woff2 (latin). The package has no latin-only CSS entry point, so `index.css` declares the single latin `@font-face` itself, pointing at `@fontsource-variable/inter/files/inter-latin-wght-normal.woff2`, with the latin `unicode-range` copied from the package. `main.tsx` is unchanged. **Owner: confirm this is acceptable, or say if you want `wght.css` and the seven files.**
2. **`tailwind.config.d.ts` added.** Not in the task's scope list. Without it, `tokens.test.ts` cannot import the JS config under `tsc --noEmit` (TS7016). It is a three-line declaration file.
3. **Stylesheets are read from disk in the test.** Vitest blanks every `.css` import, including `?raw` (tried `?raw` on both files, and `css.include` in the Vitest config; both return an empty string). The test reads `src/index.css` and `node_modules/@xyflow/react/dist/style.css` with `node:fs`, loaded through a variable specifier so tsc needs no Node typings. Paths are relative to `web/`, where `make test-web` and `npm run test` run.
4. **The mirrored `bg-slate-950` class.** The spec says to keep it and note the exception in a code comment. The note is in `index.css` beside the html/body rule, because `App.tsx` is a component file and the scope says no component changes.
5. **Colour count is 20, not 21.** The spec's 2.2 table has 20 `--c-*` variables. The test asserts 20.
6. **PM decision:** Deviation 1 accepted. The `index.css` latin `@font-face` is the required form, because the acceptance item mandates exactly one Inter woff2 (latin). The `main.tsx` import line in the task scope is superseded by the acceptance item.

## Changed assertions

None. The existing 245 Vitest tests pass unchanged, and no existing assertion needed its text changed (no component file changed, and no `text-*` class used by a component changed meaning).

## Verification

- `npm run test`: 33 files, 245 tests passed. That total includes the 11 new tests in `tokens.test.ts`, so the existing suite contributes 234, all unchanged.

- `make test`: passes (Go and web).
- `make lint`: passes (lint-go, eslint with max-warnings 0, prettier check).
- `make build`: passes.
- `npm run build`: passes. The bundle contains exactly one font file, `inter-latin-wght-normal-*.woff2` (48.26 kB), and `index-*.css` references `Inter Variable`.
- `npx playwright test`: 61 passed, 3 skipped (the suite's own per-viewport skips, the same as before this change), exit 0.

  Environment note: the sandbox has no sudo and the Playwright Chromium needs ATK, CUPS, ALSA, GBM and others that are not installed. The libraries were extracted from Ubuntu debs with `apt-get download` and `dpkg -x` into `/tmp/pwlibs` (outside the repo) and put on `LD_LIBRARY_PATH`. The browser itself is unchanged.
- `scripts/check-spdx.sh`: passes.

## Focus ring and motion checks

- The focus rule is `outline: 2px solid rgb(var(--c-focus))`, and `--c-focus` is 125 211 252 (sky-300). Tested.
- The reduced-motion block is present. Tested.
- All 11 React Flow variables from 2.9 are set. Each name is also checked against the installed `@xyflow/react/dist/style.css`. Tested.

## Summary

1. Built: spec 2.9 tokens, Tailwind config, latin Inter `@font-face`, sky-300 focus, reduced-motion rule, React Flow variables, dialog backdrop on tokens, and `tokens.test.ts` (11 tests). `@fontsource-variable/inter` added, and a Fonts row in STACK.md.
2. Skipped: `main.tsx` (font import placed in `index.css`, see deviation 1). No component files changed, and no screen was restyled (that is tasks 37 to 53).
3. Next task (37) can use the token classes (`text-body`, `bg-raised`, `ring-focus`, `border-line-strong`, and so on). They exist now, and the Tailwind colour map is checked by `tokens.test.ts`.
4. Playwright needed system libraries from debs in `/tmp`; a clean CI runner does not need this.
5. The owner should confirm deviation 1 (font import location) before merge.
