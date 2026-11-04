# Task 38 notes: layout primitives

## Plan (written before the code)

1. `Icon.tsx`: named paths, `ICONS` exported (AppShell still has its own copy; task 40 moves it).
2. `PageContainer.tsx`: `max-w-6xl`, default `flex flex-col gap-6`.
3. `PageHeader.tsx`, `Card.tsx`, `FormSection.tsx` (`FormSection`, `Field`, `FormActions`), `EmptyState.tsx`, `Banner.tsx`, `Disclosure.tsx`, each with a `*.test.tsx`.
4. Run `make test`, `make lint`, `npm run build`, `npx playwright test` (via `make e2e`).

No existing test assertion was changed. No existing test asserted `max-w-7xl`, so the width change needed no edit.

## Changed assertions

None. Every existing `data-testid` is untouched and no existing file was edited except `PageContainer.tsx`.

## Deviations and decisions

1. **PageHeader skeleton is a `span`, not `Skeleton`.** The spec says `<Skeleton className="h-8 w-48" />` inside the h1. `Skeleton` renders a `div`, and a div is not valid phrasing content inside an h1. The header uses an `aria-hidden` span with the same pulse and `bg-raised` classes. The h1 stays in place with `aria-busy="true"`.
2. **Breadcrumb is always links.** The current page is the h1 and is not repeated (spec 3.3). Items take an optional `testId`, so task 45 can keep `rules-back` style test ids.
3. **Card default `as` is `div`.** The spec example passes `as="section"`, so `section` is an opt-in.
4. **Field wires its control by cloning the single child element.** It adds `aria-describedby` (help id, error id, plus any id the caller already set) and `aria-invalid` when there is an error. `htmlFor` is required, because the label needs it and the ids are derived from it. A `Field` with more than one child does not get the aria wiring.
5. **`FormActions` is exported** from `FormSection.tsx`. Spec 4.1 lists it there, though 3.6 doesn't name it.
6. **Banner role for `warning` is `status`.** The spec only names info, success and danger. A warning is a notice, not an interruption, so it does not announce as an alert.
7. **EmptyState icon is 40 px (`h-10 w-10`) inside the 48 px (`h-12 w-12`) disc.** This follows spec 3.7 literally. If it looks too big on the first page that uses it, the owner can say so.
8. **The Disclosure keyboard acceptance item is verified in two places.** The vitest test checks that a click on the summary toggles `open`. The keyboard step is the browser's own behaviour for `summary`. I checked it in headless Chromium with a throwaway page outside the repo (`/tmp/disclosure-check`, not committed): Tab to the summary, Enter opened it, Space closed it. jsdom does not emulate that key-to-click step, so no committed test covers the keyboard itself.
9. **Interim spacing side effect (for the next tasks).** Pages still pass `space-y-4` or `space-y-6`. With `PageContainer` now a flex column with `gap-6`, those pages get the `space-y` margin on top of the gap. That is about 48 px between blocks instead of 24 px until the page tasks (42 to 51) remove `space-y`. This is inferred from the CSS rules, not measured in a browser.
10. **Left for later:** AppShell still has its own `Icon` and `ICONS` (task 40). The `btnIcon` default and the `Skeleton` colour stay as they are (tasks 39 and 40).

## Verification

- `npx vitest run`: **43 files, 293 tests passed** (up from 262). New tests: Icon 3, PageHeader 5, Card 5, FormSection 8, EmptyState 2, Banner 3, Disclosure 3, PageContainer 2. Every existing page test passes with the new PageContainer.
- `npm run lint` (eslint, `--max-warnings 0`) and `npm run format:check`: pass.
- `npm run build` (`tsc --noEmit` and `vite build`): passes.
- `make e2e` (Playwright, real Go binary, layout suite at phone, tablet and desktop): **61 passed, 3 skipped, exit 0**. The 3 skips are the suite's own per-viewport skips.
- `make test`: **exit 0**. All Go packages pass, and the web suite passes (293 tests).
- `scripts/check-spdx.sh`: passes.
- Disclosure keyboard: checked in headless Chromium with a throwaway page (Enter opened, Space closed). Not committed.

## Summary

1. Built: `Icon.tsx` (the 24 named icons from spec 4.1), `PageHeader.tsx`, `Card.tsx`, `FormSection.tsx` (`FormSection`, `Field`, `FormActions`), `EmptyState.tsx`, `Banner.tsx`, `Disclosure.tsx`, and `PageContainer.tsx` at `max-w-6xl` with a flex-column `gap-6`. Each new primitive has a test file, and all of them pass.
2. Skipped: no page changes. Pages still use the old classes, and the spacing side effect in deviation 9 stays until they change. AppShell keeps its own icons until task 40.
3. Next tasks can import `PageHeader` (with `breadcrumb`, `title`, `subtitle`, `meta`, `actions`), `Card`, `FormSection`, `Field`, `FormActions`, `EmptyState`, `Banner` and `Disclosure`. Pages should drop `space-y-*` when they adopt `PageContainer`'s gap.
4. Owner: decide whether the 40 px icon in the EmptyState disc looks right (deviation 7). Also decide whether the Banner `warning` tone should announce as `alert` (deviation 6).
5. Known gap: the Disclosure keyboard step is verified in a throwaway browser check, not in a committed test (deviation 8). Adding one would need a page that uses `Disclosure`, which no page does yet.
