# ShrinkYourShit UI redesign specification

Date: 2026-11-04
Scope: `web/` only. No Go, no API, no `DECISIONS.md` changes.
Inputs:
- the 22 screenshots in `/home/mb/ui-shots/` (11 pages, 1440x900 and 390x844) and `measures.json`;
- the code in `web/src` at `2f2b365`;
- `docs/ui-review-2026-10-29.md` and `docs/mobile-portability-2026-10-30.md`.

Already fixed, so not covered here: no horizontal overflow at either width, the rail is hidden on mobile, and html/body/shell all paint slate-950 (`rgb(2,6,23)` in `measures.json`).

How to read this: sections 2 to 4 define the system, section 5 applies it page by page, and section 7 splits the work into tasks. A build agent takes one task from section 7 and uses sections 2 to 6 as the reference. Where this document gives a class string, use that exact string.

---

## 1. Verdict

The app works. It looks unfinished because each task built its own visual language and nothing pulled them back together. In the code there are 51 `rounded-2xl` and 73 `rounded-lg` uses and four h1 styles (`text-2xl font-bold` x23, `text-xl font-bold` x4, `text-2xl font-bold tracking-tight`, `text-3xl ... text-center`). There are also six h2 styles, two input styles, five primary-button sizes, three checkbox sizes and two "danger red" families (`red-*` and `rose-*`). Specifics, worst first:

1. **The colour green means everything, so it means nothing.** `bg-emerald-600` fills the primary button ("Add folder", "Save changes"). The same fill marks the *selected tab* on Integrations (`IntegrationsPage.tsx:149`, "Servers"), the *selected tier* on Rules ("Easy", `RulesPage.tsx:199,214`) and the "on" switch. The status chip "1 Nothing to do" (`StatusChip.tsx:13`) and the "Saved" column are green too. On `desktop-integrations.png` the selected "Servers" tab and the "Add server" button are the same green block 130 px apart, and you cannot tell which one is the action. On `desktop-library-rules.png` the selected "Easy" segment is a bigger green slab than the "Save easy settings" button. White text on emerald-600 is also **3.77:1**, below the 4.5:1 AA minimum for 14 px text.
2. **Three controls do the same job, and none of them explains itself.** `desktop-nodes.png` has a "Pause all encoding" native checkbox in the top bar (amber, checked), "Pause all" (disabled) and "Resume all" buttons in the page header, and a 80x44 white-knob "OFF" switch on the node card. They are three different widgets with three different polarities: checked means paused, a button means do it, on means running. Meanwhile `desktop-dashboard.png` shows the checkbox ticked and says "Nothing is running right now." without a word about *why* (encoding is paused).
3. **The desktop top bar has nothing in it.** At 1440 px the 45 px bar shows only `v0.1.2-44-g2f2b365` at the left and the pause checkbox at the right (every desktop shot). It costs a full row and makes every page look like a dev build. The git-describe version is the first thing the eye lands on after the brand.
4. **Identifiers instead of things.** Dashboard "Recent history" rows read `job a35a5e4b · shrunk` (`DashboardPage.tsx:323`), although `Job.file_name` is right there in the API. The flow canvas prints step type ids (`probe`, `if_codec_...`, `transcode_vid...`) in 10 px grey next to every node title (`FlowCanvas.tsx:77`). The Edit page's "Advanced" panel offers `keep_all`, `keep_preferred`, `keep_best`, `compact` as option *labels* (`LibraryEditPage.tsx:331-334, 378-380`). App settings shows a 40-character commit hash and an ISO timestamp `2026-11-04T01:09:10Z`.
5. **Forms stretch across the full 1150 px.** On `desktop-library-new.png`, `-edit`, `-rules` and `desktop-encode.png` the sliders, selects and the "Files processed at once" number field run edge to edge. A 1-character number input 1100 px wide, and slider stop labels 270 px apart, make the form look like a placeholder. On mobile the same five stop labels wrap to three lines each (`mobile-library-rules.png`: "Keep the best track, make it small").
6. **Zero counts and dead controls everywhere.** Libraries rows show `0 Waiting`, `0 Done`, `0 Failed` chips at 50 % opacity (about 2:1 contrast), and on desktop they wrap to two lines (`desktop-libraries.png`). Library detail shows `0 New 0 Waiting 0 Working 0 Done 0 Failed 0 Missing`, a disabled-looking "Retry failed (0)", and a full pager ("Per page 50 · Page 1 of 1 · ← Previous · Next →") for **one** file. Nodes shows a disabled "Pause all". Each one is honest, but together they bury the one number that matters.
7. **The file status contradicts itself.** `desktop-library-detail.png`: `sample-1080p-51.mkv`, status **"Nothing to do"** (green), before 31.6 MB, after 17.0 MB, saved 14.6 MB. The file *was* shrunk. The rescan marked it `satisfied` because it is now HEVC. The dashboard repeats this: "Files 2 / Nothing to do 2" beside "15.2 MB saved".
8. **The three-tier story is broken in the UI** (DECISIONS item 6). Rules shows Easy | Steps | Advanced as a segmented control, but "Advanced" is a link that leaves for `/flow`. That page has a different header ('Flow “Movies”', an "easy" pill, a "Back to rules" text button) and two "Edit flow" buttons (header and banner). On top of that, the library **Edit** page carries its *own* copy of the easy sliders and a fourth rule editor, also called "Advanced", that edits raw `Target` fields. "Advanced" therefore means two different things, one click apart.
9. **Every nested page invents its own way back.** Edit and Rules use an underlined "Back to files" link at the far right; Flow uses a borderless "Back to rules" button; Library detail has none. None of them shows where you are (Libraries › Movies › Rules). Titles quote the library name as decoration: `Edit “Movies”`, `Rules “Movies”`, `Flow “Movies”`.
10. **Checkboxes, switches and inputs come in three sizes each.**
    - Checkboxes: the 44 px solid-green checkbox next to "Check for updates automatically" (`AboutCard.tsx:148`, `desktop-app.png`), the 16 px checkboxes on Edit, and the unstyled grey "Remove subtitles" box that sits vertically centred on a two-line label (`easyControls.tsx:107`, `mobile-library-rules.png`).
    - Inputs: Edit uses filled `bg-slate-800` with no border (`LibraryEditPage.tsx:157`), while Encode and App use bordered `bg-slate-900` (`uiControls.ts:20`).
    - Primary buttons: "Save settings" (`text-sm font-medium`) is visibly smaller than "Save changes" (`font-semibold px-6 py-2.5`, base size).
11. **The flow canvas is a grey hole.** React Flow's `colorMode="dark"` paints `#141414`, a neutral grey that clashes with the blue-slate app. The minimap is an empty black rectangle, the "React Flow" attribution sits in a 44 px grey block, and node chips render at about 6 px after `fitView` (`desktop-library-flow.png`).
12. **Text that is too dim to read.** Help text, slider stop labels, table headers and paths use `text-slate-500` on slate-900 (**3.75:1**) or on slate-950 (**4.24:1**), both below AA, often at 12 px: "Smallest files", "Two-letter codes…", "PATH", `/media/Movies`.
13. **Smaller cuts:**
    - The mobile header squeezes "ShrinkYourShit" and "Pause all encoding" into one 390 px row.
    - App settings' container-update `<pre>` is clipped mid-sentence on mobile (`mobile-app.png`: "so it cannot").
    - Encode selects truncate to "(re" and "sma" on mobile.
    - "Save settings" floats *between* the Login and About cards on App settings, so its scope is unclear.
    - The Encode help text says "Two-letter codes" next to the example `eng, deu`, which are three-letter codes.
    - Node "Revoke" fires with no confirmation (`NodesPage.tsx:542`) while the milder "Stop now" has one.
    - The nav says "Dash" and "Library" while the pages say "Dashboard" and "Libraries".
    - The Integrations nav icon is a download tray.
    - Setup renders two h1s ("ShrinkYourShit" plus the step title), and its "Join an existing server" choice is a clickable `div`, not a button (`SetupWizard.tsx:111`).

What is good and stays: the dark slate base, the plain-language copy on the easy screen, `DataList`'s table-to-card idea, the native `<dialog>` wrapper, the toast system, the 44 px touch rule and its Playwright audit, and the inline-SVG icon approach.

---

## 2. Design system

### 2.1 Principles that decide ties

1. **One accent, one job.** Emerald fill is only for the single primary action on a screen and for "on" state of a switch. Selection (tabs, segments, nav) is neutral: a lighter surface plus fg text.
2. **Status colour is semantic, never decorative.** Green = something good happened (shrunk, running). Neutral = nothing to report. Amber = needs a look. Red = failed or destructive. Sky = in progress.
3. **Hide zero.** A count of 0 is not rendered as a chip, button label or tile unless it is the only state (for example "0 waiting" inside a sentence).
4. **Names, not ids.** File names, library names, node names. Ids and hashes appear only in mono, in a "Technical details" disclosure or in the inspector.

### 2.2 Colour tokens

Tokens are CSS variables holding RGB channels, so Tailwind's `/<alpha>` modifiers work. They live in `web/src/index.css` under `:root` and are mapped in `web/tailwind.config.js` (exact config in 2.9). The values are Tailwind's own palette, so nothing shifts by accident.

| Token (CSS var) | Tailwind class root | Value | Source palette | Use |
|---|---|---|---|---|
| `--c-canvas` | `canvas` | `2 6 23` | slate-950 | page background, input fill |
| `--c-surface` | `surface` | `15 23 42` | slate-900 | cards, rail, mobile top bar, dialogs |
| `--c-raised` | `raised` | `30 41 59` | slate-800 | secondary buttons, selected nav/tab/segment, inset rows, hover on surface |
| `--c-raised-hover` | `raised-hover` | `51 65 85` | slate-700 | hover on raised |
| `--c-line` | `line` | `30 41 59` | slate-800 | card ring, dividers (decorative only) |
| `--c-line-strong` | `line-strong` | `100 116 139` | slate-500 | borders of inputs, selects, checkboxes, unchecked switch track (needs 3:1) |
| `--c-fg` | `fg` | `241 245 249` | slate-100 | primary text |
| `--c-fg-muted` | `fg-muted` | `148 163 184` | slate-400 | secondary text, help, labels of read-only data, table headers, placeholders |
| `--c-fg-subtle` | `fg-subtle` | `100 116 139` | slate-500 | **disabled text and decorative glyphs only**; never for content |
| `--c-accent` | `accent` | `16 185 129` | emerald-500 | primary button fill, switch on, slider fill, progress bar |
| `--c-accent-hover` | `accent-hover` | `52 211 153` | emerald-400 | primary hover |
| `--c-accent-fg` | `accent-fg` | `2 6 23` | slate-950 | text on accent fill |
| `--c-accent-text` | `accent-text` | `52 211 153` | emerald-400 | links, "saved" numbers |
| `--c-focus` | `focus` | `125 211 252` | sky-300 | focus ring, everywhere |
| `--c-success` | `success` | `52 211 153` | emerald-400 | success text, and tint base at /10 and /30 |
| `--c-info` | `info` | `125 211 252` | sky-300 | in-progress text and tint |
| `--c-warning` | `warning` | `252 211 77` | amber-300 | warning text and tint |
| `--c-danger` | `danger` | `248 113 113` | red-400 | danger text and tint |
| `--c-danger-solid` | `danger-solid` | `220 38 38` | red-600 | destructive confirm button fill (white text) |
| `--c-danger-solid-hover` | `danger-solid-hover` | `185 28 28` | red-700 | its hover (darker, keeps contrast) |

Semantic tint recipe (badges, banners, ErrorBox): `bg-{tone}/10 text-{tone} ring-1 ring-inset ring-{tone}/30`, where tone is one of `success`, `info`, `warning`, `danger`. Neutral: `bg-raised text-fg-muted ring-1 ring-inset ring-line`.

**Retired** (lint them out in task 53): `rose-*` (use `danger`), `bg-emerald-600` (use `accent`), `text-slate-500` on content, `opacity-50` as a "zero" signal, and every raw `slate-*`, `emerald-*`, `red-*`, `amber-*` or `sky-*` class in page components. Raw palette names may remain only in `index.css`.

Contrast, computed with the WCAG formula, on `surface` (#0f172a) unless noted:

| Pair | Ratio | Verdict |
|---|---|---|
| fg on surface | 16.9 | AAA |
| fg-muted (slate-400) on surface | 6.97 | AA |
| fg-muted on raised (slate-800) | 5.71 | AA |
| fg-subtle (slate-500) on surface (today's help text) | 3.75 | **fail**, so it is retired for content |
| slate-500 on canvas (today's help text) | 4.24 | **fail** |
| white on emerald-600 (today's primary) | 3.77 | **fail** |
| accent-fg (slate-950) on accent (emerald-500) | 7.96 | AAA |
| accent-text (emerald-400) on surface | 9.3 | AAA |
| danger (red-400) on surface | 6.46 | AA |
| white on danger-solid (red-600) | 4.83 | AA |
| white on danger-solid-hover (red-700) | 6.47 | AA |
| line-strong (slate-500) border vs canvas input fill | 4.24 | passes the 3:1 non-text rule |
| slate-700 border vs surface (today's input border) | 1.72 | **fails** the 3:1 non-text rule |

### 2.3 Typography

- **Font:** Inter Variable, self-hosted from `@fontsource-variable/inter` (the justification is in section 8). The screenshots render in the Linux fallback (DejaVu Sans), whose wide letterforms are a large part of the "dev build" look. The stack is `"Inter Variable", ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`. Import only the latin subset: `import '@fontsource-variable/inter/wght.css'` in `main.tsx`, about 48 KB woff2.
- **Numbers:** every number column, stat and size uses `tabular-nums`.

| Role | Classes | Size/line | Replaces |
|---|---|---|---|
| Page title (h1) | `text-title text-fg` | 24/32, 600, -0.01em | `text-2xl font-bold`, `text-xl font-bold`, `text-3xl …` |
| Section title (h2 in a card) | `text-heading text-fg` | 16/24, 600 | `font-semibold`, `text-lg font-semibold`, `text-lg font-bold`, `text-sm font-semibold …` |
| Sub-section (h3) | `text-body font-semibold text-fg` | 14/20, 600 | `text-lg font-semibold` h3s in Integrations and Plugins |
| Body | `text-body text-fg` | 14/20 | `text-sm` |
| Secondary | `text-body text-fg-muted` | 14/20 | `text-sm text-slate-400` |
| Field label | `text-body font-medium text-fg` | 14/20, 500 | `text-sm font-medium text-slate-300`, `text-sm text-slate-300` |
| Help / caption | `text-caption text-fg-muted` | 12/16 | `text-xs text-slate-500` |
| Overline (table head, stat label) | `text-caption font-medium uppercase tracking-wider text-fg-muted` | 12/16 | `text-xs uppercase tracking-wide text-slate-500` |
| Stat value | `text-[clamp(1.5rem,3.2vw,1.875rem)] leading-tight font-semibold tabular-nums` | 24 to 30 | `text-[clamp(18px,5.5vw,24px)] font-bold` |
| Mono (paths, ids, commands) | `font-mono text-caption` (12) or `font-mono text-body` (14) | | ad hoc `text-xs` paths |
| Brand wordmark | `text-base font-semibold tracking-tight` | 16 | `text-lg font-bold tracking-tight` |

The **minimum size is 12 px.** `text-[10px]` and `text-[11px]` (5 uses: `FlowCanvas.tsx`, `FlowPalette.tsx`, `Switch.tsx`, `FlowCanvasPage.tsx`) are removed. `font-bold` is not used anywhere outside the brand.

### 2.4 Spacing

The Tailwind 4 px grid stays, restricted to these steps: `1` (4), `2` (8), `3` (12), `4` (16), `6` (24), `8` (32), `12` (48). Any `5`, `7`, `10`, `py-1.5`, `py-2.5` or `px-6 py-3` on buttons goes.

| Context | Rule |
|---|---|
| Page container gutters | `px-4 sm:px-6 lg:px-8` (unchanged) |
| Main vertical padding | `py-6 lg:py-8` (was `py-4 sm:py-6 lg:py-8`) |
| Page header to first section | `gap-6` |
| Between sections/cards | `gap-6` (every page; `space-y-4` pages move to 6) |
| Card padding | `p-4 sm:p-6` |
| Inside a card: header to body | `gap-4` |
| Between form fields | `gap-4`; between field groups `gap-6` |
| Label to control | `gap-2` |
| Control to help text | `mt-2` |
| Inline button group | `gap-2` |
| Table cell | `px-4 py-3` |
| List rows inside a card | `divide-y divide-line` with rows `py-3`; no per-row tinted boxes (replaces `rounded-lg bg-slate-800/50 px-3 py-2` history rows) |

### 2.5 Radii

| Token class | px | Use |
|---|---|---|
| `rounded-md` | 6 | kbd, inline code, mono ids |
| `rounded-control` (= `rounded-lg`) | 8 | buttons, inputs, selects, list rows, segments, nav items |
| `rounded-card` (= `rounded-xl`) | 12 | cards, banners, table wrappers, error boxes, canvas frame, flow nodes |
| `rounded-overlay` (= `rounded-2xl`) | 16 | dialogs, drawer edge, toasts |
| `rounded-full` | | badges, switch, progress bars, dots |

Cards move from 16 to 12 px. At 1150 px wide, 16 px corners with 24 px padding read as bubbly.

### 2.6 Elevation

Dark UI elevation is lightness plus a 1 px ring. Shadows are only for things that float.

| Level | Recipe | Used by |
|---|---|---|
| 0 Canvas | `bg-canvas` | page |
| 1 Surface | `bg-surface ring-1 ring-line` | cards, rail, table wrapper |
| 2 Raised | `bg-raised` (no ring) | secondary buttons, selected nav/segments, inset rows, code blocks use `bg-canvas` (sunken) |
| 3 Overlay | `bg-surface ring-1 ring-line-strong/40 shadow-overlay` | dialogs, drawer, toasts |

`shadow-overlay` = `0 16px 48px -12px rgb(0 0 0 / 0.7)`. `shadow-xl` on the Setup, Login and Joined cards goes; they are level 1.

### 2.7 Motion

| Token | Value | Use |
|---|---|---|
| `duration-fast` | 120 ms | colour and background on hover/press |
| `duration-base` | 200 ms | switch knob, disclosure chevron, toast in |
| `duration-slow` | 300 ms | drawer slide, progress-bar width |
| `ease-standard` | `cubic-bezier(0.2, 0, 0, 1)` | all of the above |

Every interactive element gets `transition-colors duration-fast ease-standard`. Under `prefers-reduced-motion: reduce` a global rule in `index.css` sets durations to 0.01 ms and stops `animate-pulse` (the running-dot and skeletons become static). Exact CSS is in 2.9.

### 2.8 Focus

One ring: `:focus-visible { outline: 2px solid rgb(var(--c-focus)); outline-offset: 2px; }` (the existing rule, recoloured from emerald-400 to sky-300 so it never reads as "selected"). Delete the per-component override in `Switch.tsx` (`focus-visible:outline-sky-400 …`). For inputs, use `focus-visible:border-focus` plus the global outline.

### 2.9 Exact config and CSS

`web/tailwind.config.js`:

```js
// SPDX-License-Identifier: AGPL-3.0-only
import defaultTheme from 'tailwindcss/defaultTheme'

const c = (name) => `rgb(var(--c-${name}) / <alpha-value>)`

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        canvas: c('canvas'),
        surface: c('surface'),
        raised: { DEFAULT: c('raised'), hover: c('raised-hover') },
        line: { DEFAULT: c('line'), strong: c('line-strong') },
        fg: { DEFAULT: c('fg'), muted: c('fg-muted'), subtle: c('fg-subtle') },
        accent: {
          DEFAULT: c('accent'),
          hover: c('accent-hover'),
          fg: c('accent-fg'),
          text: c('accent-text'),
        },
        focus: c('focus'),
        success: c('success'),
        info: c('info'),
        warning: c('warning'),
        danger: {
          DEFAULT: c('danger'),
          solid: c('danger-solid'),
          'solid-hover': c('danger-solid-hover'),
        },
      },
      fontFamily: {
        sans: ['"Inter Variable"', ...defaultTheme.fontFamily.sans],
      },
      fontSize: {
        caption: ['0.75rem', { lineHeight: '1rem' }],
        body: ['0.875rem', { lineHeight: '1.25rem' }],
        heading: ['1rem', { lineHeight: '1.5rem', fontWeight: '600' }],
        title: ['1.5rem', { lineHeight: '2rem', letterSpacing: '-0.01em', fontWeight: '600' }],
      },
      borderRadius: { control: '0.5rem', card: '0.75rem', overlay: '1rem' },
      boxShadow: { overlay: '0 16px 48px -12px rgb(0 0 0 / 0.7)' },
      transitionDuration: { fast: '120ms', base: '200ms', slow: '300ms' },
      transitionTimingFunction: { standard: 'cubic-bezier(0.2, 0, 0, 1)' },
      zIndex: { rail: '30', topbar: '40', toast: '70' },
    },
  },
  plugins: [],
}
```

Additions to `web/src/index.css`, above the existing rules. Keep the existing html/body, dialog and React Flow 44 px rules, but recolour them to tokens:

```css
:root {
  --c-canvas: 2 6 23;
  --c-surface: 15 23 42;
  --c-raised: 30 41 59;
  --c-raised-hover: 51 65 85;
  --c-line: 30 41 59;
  --c-line-strong: 100 116 139;
  --c-fg: 241 245 249;
  --c-fg-muted: 148 163 184;
  --c-fg-subtle: 100 116 139;
  --c-accent: 16 185 129;
  --c-accent-hover: 52 211 153;
  --c-accent-fg: 2 6 23;
  --c-accent-text: 52 211 153;
  --c-focus: 125 211 252;
  --c-success: 52 211 153;
  --c-info: 125 211 252;
  --c-warning: 252 211 77;
  --c-danger: 248 113 113;
  --c-danger-solid: 220 38 38;
  --c-danger-solid-hover: 185 28 28;
}

html, body { @apply bg-canvas text-fg font-sans antialiased; }
:focus-visible { outline: 2px solid rgb(var(--c-focus)); outline-offset: 2px; }
dialog::backdrop { background: rgb(var(--c-canvas) / 0.7); }

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}

/* React Flow on the app palette. Variable names are from @xyflow/react 12;
   verify them against node_modules/@xyflow/react/dist/style.css. */
main .react-flow {
  --xy-background-color: rgb(var(--c-canvas));
  --xy-background-pattern-color: rgb(var(--c-raised));
  --xy-edge-stroke: rgb(var(--c-line-strong));
  --xy-edge-stroke-selected: rgb(var(--c-accent));
  --xy-minimap-background-color: rgb(var(--c-surface));
  --xy-minimap-mask-background-color: rgb(var(--c-canvas) / 0.7);
  --xy-controls-button-background-color: rgb(var(--c-surface));
  --xy-controls-button-background-color-hover: rgb(var(--c-raised));
  --xy-controls-button-color: rgb(var(--c-fg));
  --xy-controls-button-border-color: rgb(var(--c-line));
  --xy-attribution-background-color: transparent;
}
main .react-flow__attribution a { color: rgb(var(--c-fg-subtle)); }
```

`App.tsx` mirrors `bg-slate-950` onto html/body at runtime, and a test asserts that class. Keep the mirrored class as `bg-slate-950`: it is the same colour as `bg-canvas`, and keeping it avoids touching that test. Note this exception in a code comment.

### 2.10 Control class strings (`web/src/uiControls.ts`)

This stays the single source for control classes. Every string keeps `min-h-11` (the existing test asserts it). The final contents:

```ts
const focus = 'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-canvas'
const base = `inline-flex min-h-11 items-center justify-center gap-2 rounded-control px-4 text-body font-medium transition-colors duration-fast ease-standard disabled:cursor-not-allowed disabled:opacity-60 ${focus}`

export const btnPrimary = `${base} bg-accent text-accent-fg hover:bg-accent-hover`
export const btn = `${base} bg-raised text-fg hover:bg-raised-hover`          // secondary
export const btnGhost = `${base} text-fg-muted hover:bg-raised hover:text-fg`
export const btnDanger = `${base} bg-danger-solid text-white hover:bg-danger-solid-hover`
export const btnDangerGhost = `${base} text-danger hover:bg-danger/10`
export const btnIcon = `${focus} inline-flex h-11 w-11 items-center justify-center rounded-control text-fg-muted transition-colors duration-fast hover:bg-raised hover:text-fg`
export const btnBlock = 'w-full'                                              // modifier

export const input = `min-h-11 w-full rounded-control border border-line-strong bg-canvas px-3 text-body text-fg placeholder:text-fg-muted transition-colors duration-fast focus-visible:border-focus disabled:opacity-60`
export const select = `${input} pr-9`                                        // room for the native chevron
export const textarea = `${input} py-2 leading-5`
export const link = 'text-accent-text underline-offset-4 hover:underline'
export const linkTouch = `${link} inline-flex min-h-11 items-center`         // links inside <main> on mobile

export const card = 'rounded-card bg-surface ring-1 ring-line'
export const cardPad = 'p-4 sm:p-6'
```

Wherever a page uses `${btnGhost} !text-red-300 hover:!bg-red-950/40` (`NodesPage.tsx`, 4 places), replace it with `btnDangerGhost`. No `!important` overrides remain.

---

## 3. Layout system

### 3.1 App shell

One component, `AppShell.tsx`, with three states. Breakpoints are Tailwind's: `md` = 768 and `lg` = 1024.

**Desktop, `lg` and up: labelled rail, no top bar.**

```
┌────────────┬──────────────────────────────────────────────┐
│ ◆ ShrinkYourShit │  (main scrolls; rail is sticky)            │
│            │  Libraries › Movies                           │
│ ⌂ Dashboard│  Movies                         [Rules] [Scan]│
│ ▤ Libraries│  /media/Movies                                │
│ ▣ Nodes    │                                               │
│ ≡ Encode   │                                               │
│ ⇄ Integr.  │                                               │
│ ⚙ App      │                                               │
│            │                                               │
│ ┌────────┐ │                                               │
│ │Encoding│ │  ← EncodingStatus (rail variant)              │
│ │● 1 run │ │                                               │
│ │[switch]│ │                                               │
│ └────────┘ │                                               │
│ admin  ⎋   │  ← user + logout icon (only if login enabled) │
│ v0.1.2 « │  ← version caption (links /app) + collapse toggle │
└────────────┴──────────────────────────────────────────────┘
```

- Rail: `aside` with `sticky top-0 h-dvh w-56 shrink-0 flex flex-col bg-surface border-r border-line z-rail`. It stays `w-56` (labels) and `w-16` (icons), because `AppShell.test.tsx` asserts both.
- The **desktop `<header>` is deleted.** Its contents move:
  - `JobIndicator` and `PauseAllSwitch` merge into `EncodingStatus` (3.1.1) in the rail footer;
  - `LogoutButton` becomes a row in the rail footer;
  - the version moves to a caption in the rail footer (keep `data-testid="version"`).
- Nav item: `flex min-h-11 items-center gap-3 rounded-control px-3 text-body font-medium text-fg-muted hover:bg-raised hover:text-fg`. Active: `bg-raised text-fg` plus a 2 px accent bar, drawn as `relative before:absolute before:left-0 before:top-2 before:bottom-2 before:w-0.5 before:rounded-full before:bg-accent`, and `aria-current="page"` (NavLink already sets it).
- Nav labels: **Dashboard, Libraries, Nodes, Encode settings, Integrations, App settings**. They must match the h1s.
- Brand: a 20 px inline-SVG mark (a square with an inward arrow, drawn in `Icon.tsx`) plus the wordmark. The collapsed rail shows the mark only, never the letters "SYS".

**Tablet, `md` to `lg` (768 to 1023): icon rail.**
- `w-16`, icons centred, `title` and `aria-label` per item.
- The `EncodingStatus` rail variant collapses to a 44 px icon button: a pause glyph when paused, or a pulsing dot plus count when running. It links to `/nodes`.
- The `sys.rail` override behaviour is unchanged.

**Phone, under `md`: top bar plus drawer.** Not bottom tabs: there are six destinations, and bottom tabs hold five. The drawer is already built and tested.
- Top bar: `sticky top-0 z-topbar flex h-14 items-center gap-2 border-b border-line bg-surface/95 backdrop-blur px-2`.
  - Contents: `[menu btnIcon] [brand mark + wordmark, truncate] [ml-auto EncodingStatus compact pill]`.
  - The compact pill is read-only (`Paused` warning tone, `2 running` info tone, or nothing when idle and not paused). It links to `/nodes`. The 44 px hit area comes from `min-h-11` on the link; the visible pill is smaller.
  - "Pause all encoding" as a label in the top bar is gone. That fixes the squeeze in every `mobile-*.png`.
- Drawer (`ModalDialog`): `h-dvh w-72 max-w-[85vw] rounded-r-overlay bg-surface shadow-overlay`. Contents: brand plus close, nav, then at the bottom `EncodingStatus` (full variant with switch), logout, version.

#### 3.1.1 `EncodingStatus`: one control for "is it working"

This replaces `PauseAllSwitch` (AppShell), `JobIndicator` (AppShell) and the Nodes page's "Pause all" / "Resume all" header buttons. Data comes from the same queries (`['nodes']`, `['jobs','indicator']`) and the same mutations (`pauseAllNodes`, `resumeAllNodes`).

Polarity is positive: **the switch reads "Encoding", on = running.** It matches the per-node switch, which is on = running.

| State | Derived from | Pill / text | Tone |
|---|---|---|---|
| Running, n active | any node `desired_state==='running'`, n>0 running/claimed jobs | `{n} running` (keep this exact text: `AppShell.test.tsx:249`) | info + pulsing dot |
| Running, idle | any node running, 0 active jobs | `Idle` | neutral |
| Paused | every node `desired_state!=='running'` | `Paused` | warning |
| No nodes | `items.length===0` | `No nodes` | neutral, switch disabled |

Variants: `rail` (label, pill, Switch), `icon` (tablet rail), `compact` (mobile top bar, pill only) and `panel` (Dashboard and Nodes, a full-width card row with a sentence; see 5.3 and 5.10).

Test ids:
- `job-indicator` stays on the pill when n>0;
- `pause-all-switch` stays on the wrapper;
- `nodes-pause-all` and `nodes-resume-all` stay on the Nodes page `panel` variant's action buttons (`NodesPage.test.tsx:763-777` assert them). In the panel, both buttons stay rendered, because the test expects `nodes-pause-all` to be disabled after pausing. The applicable one is `btn`; the inapplicable one is `btnGhost` and disabled. That is the current behaviour, restyled.

### 3.2 PageContainer: one width rule

`PageContainer` is `mx-auto w-full max-w-6xl px-4 sm:px-6 lg:px-8` on **every** route, including Flow. The change is from `max-w-7xl` to `max-w-6xl` (1152 px). Content width is then 1088 px at 1440, and comfortable from 1024 up.

Pages never add `max-w-*`. Readability inside the container comes from patterns, not widths:
- forms use `FormSection` (3.6), which caps the field column at `max-w-2xl`;
- prose uses `max-w-prose`, applied by `PageHeader` to subtitles and by `EmptyState` to its text.

Inner layout is a single `flex flex-col gap-6`. `PageContainer` gets that as its default class (pages stop passing `space-y-4` or `space-y-6`).

### 3.3 Page header: `PageHeader`

```tsx
<PageHeader
  breadcrumb={[{ label: 'Libraries', to: '/libraries' }, { label: 'Movies', to: `/libraries/${id}` }]}
  title="Rules"
  subtitle="How this library shrinks files."       // optional, max-w-prose
  meta={<Badge tone="neutral">Easy in use</Badge>}   // optional, inline after title
  actions={<>…secondary…, …one primary last…</>}     // optional
/>
```

Markup and classes:
- Wrapper: `flex flex-col gap-2`.
- Breadcrumb: `nav[aria-label="Breadcrumb"] > ol.flex.flex-wrap.items-center.gap-1.text-caption.text-fg-muted`. Each link is `linkTouch` minus colour (`text-fg-muted hover:text-fg`) and keeps the 44 px rule. The separator is a `›` glyph, `aria-hidden`. The current page is not repeated in the breadcrumb; it is the h1.
- Title row: `flex flex-wrap items-start justify-between gap-x-4 gap-y-3`.
  - Left: `div.min-w-0 > h1.text-title.text-fg.break-words` plus the meta inline (`flex items-center gap-3`).
  - Right: `div.flex.flex-wrap.gap-2` with the actions.
  - The title row has `min-h-11`, so headers with and without buttons sit at the same y. Today the h1 is at 92 px on Dashboard and 98 px on Libraries.
- Subtitle: `p.text-body.text-fg-muted.max-w-prose`.
- Under `sm`, actions wrap under the title, left-aligned.

Rules:
- At most three actions. Exactly zero or one is `btnPrimary`, and it is always last.
- Destructive actions never sit in the header; they live in a "Danger zone" card at the bottom of the relevant page, or in a row action.
- "Back to …" links are deleted everywhere. The breadcrumb replaces them. Keep their test ids by putting `data-testid="rules-back"` and similar on the matching breadcrumb link, and check the tests for `toHaveTextContent('Back to files')` before relying on this. If a test asserts the text, update it in the same commit and list it in the notes.
- Titles no longer quote the library name. `Edit “Movies”` becomes title "Edit library" with breadcrumb `Libraries › Movies`. `Rules “Movies”` becomes "Rules", and `Flow “Movies”` becomes "Rules" too (see 5.8). The page's `<title>` becomes `Rules · Movies · ShrinkYourShit` via a `useDocumentTitle` hook.
- Loading: `PageHeader` renders immediately. When the title depends on data, use `title={lib?.name}` and PageHeader shows `<Skeleton className="h-8 w-48" />` while undefined. The h1 never changes words after load (today Edit shows "Edit library" and then `Edit “Movies”`).

### 3.4 Section and card: `Card`

```tsx
<Card title="Working now" description="…" actions={<Link…>View nodes</Link>} padding="default|none" footer={…} as="section">
```

- Wrapper: `${card}` plus `flex flex-col`. With `padding="default"` the body is `${cardPad}`. `none` is for cards that hold a `DataList` or a divided list.
- Header (when title is present): `flex items-start justify-between gap-4`, h2 `text-heading`, description `mt-1 text-body text-fg-muted`. The header shares the body padding and is followed by `gap-4`.
- Footer: `border-t border-line px-4 py-3 sm:px-6 flex flex-wrap items-center gap-2`. Used for form actions.
- Never nest a Card in a Card. Inside a card, sub-groups are divided with `border-t border-line pt-4`.

It replaces the 14 copies of `rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800`, the 4 copies of `… p-4 …`, and the `bg-slate-800/40 p-6` and `bg-slate-950/60` variants.

### 3.5 Table to card: `DataList`

Keep the component and its column API. Changes:

1. New prop `cardsBelow?: 'sm' | 'md' | 'lg'` (default `'sm'`, so existing tests keep passing). The file list uses `'md'`: eight columns do not fit 640 to 767.
2. Table: wrapper `${card} overflow-x-auto`; `thead` row `bg-surface`; `th` is overline plus `px-4 py-3`; `tbody` is `divide-y divide-line`; `td` is `px-4 py-3 text-body align-middle`. Numeric columns get a new `align: 'end'` column option, which applies `text-right tabular-nums`. The actions column is always last and right-aligned (`w-px whitespace-nowrap`).
3. Row hover `hover:bg-raised/40` only when the column set has a `rowHref` (new optional prop). With `rowHref`, the primary cell renders as a `Link` with `after:absolute after:inset-0` and the row gets `relative`, so the whole row is clickable while action buttons (`relative z-10`) keep working.
4. Cards: `li` is `${card} p-4`.
   - Top row: `flex items-start justify-between gap-3`, with the primary cell (`text-body font-semibold`) on the left and an optional `cardBadge` (a column with `cardPlacement: 'badge'`) on the right.
   - Then the `dl` as `mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-body`, with `dt` `text-fg-muted` and `dd` `text-right text-fg tabular-nums`. Labels have **no colons** (today's file card has "Size:" and "Encoder:" but "Status" without one).
   - Actions: a column with `cardPlacement: 'footer'` renders in `mt-3 flex flex-wrap gap-2 border-t border-line pt-3`.
5. Empty: `DataList` takes `empty?: ReactNode` and renders it in place of both layouts when `rows.length===0`.

### 3.6 Forms: `FormSection` and `Field`

The settings pattern, used by New library, Edit library, Rules (Easy), Encode settings, App settings and the dialogs' bodies (dialogs use the single-column variant).

```tsx
<FormSection title="Server" description="How this machine runs the work.">
  <Field label="Files processed at once" help="…" error={…} htmlFor="concurrency">
    <input id="concurrency" className={`${input} w-32`} … />
  </Field>
</FormSection>
```

- `FormSection` at `lg`: `grid grid-cols-[minmax(0,18rem)_minmax(0,1fr)] gap-x-8`. The left column has the h2 `text-heading` and the description `text-body text-fg-muted`. The right column is a `Card` whose body is `flex flex-col gap-4 max-w-2xl`. Under `lg` it stacks: title and description above the card.
- `Field`: `flex flex-col gap-2`.
  - `label.text-body.font-medium.text-fg`, with an optional `(optional)` suffix in `text-fg-muted`.
  - Then the control.
  - Then `p.text-caption.text-fg-muted` help (id `${id}-help`).
  - Then `p.text-caption.text-danger` error (id `${id}-error`, `role="alert"`).
  - The control gets `aria-describedby` for both and `aria-invalid` when there is an error.
- Width by data type, inside the 2xl column:

  | Type | Width |
  |---|---|
  | number | `w-32` |
  | short select | `max-w-xs` |
  | long select | `w-full` |
  | path | `w-full font-mono` |
  | text | `w-full` |

- Choice rows (checkbox or switch plus label plus help): `flex items-start gap-3`. The control is aligned to the first line of the label (`mt-0.5`), not centred on two lines.
- Form actions: the parent page puts **one** action bar after the last FormSection. It is right-column aligned at `lg` (`lg:pl-[calc(18rem+2rem)]`) and has `flex gap-2`: `btnPrimary` "Save changes", then `btnGhost` "Cancel" or "Reset" when the form is dirty. On mobile the action bar is `sticky bottom-0 -mx-4 border-t border-line bg-canvas/95 px-4 py-3 backdrop-blur` for forms taller than one screen (Edit, Rules, New).
- Save buttons are named **"Save changes"** everywhere, except creation ("Add library") and Rules ("Save rules").

### 3.7 Empty, loading, error: one way each

**Empty: `EmptyState`.**

```tsx
<EmptyState icon="library" title="No libraries yet"
  body="A library is a folder ShrinkYourShit watches. Add one and it starts looking for files to shrink."
  action={<Link className={btnPrimary} to="/libraries/new">Add library</Link>} />
```

- Layout: `flex flex-col items-center gap-3 py-12 text-center`, with a 40 px muted icon in a `h-12 w-12 rounded-full bg-raised` disc, the `text-heading` title and `text-body text-fg-muted max-w-prose` body, then the action.
- Inside a Card (when the section has other chrome) or standalone in place of the section.
- Copy formula: title "No {plural} yet"; body is one sentence saying what the thing does and what adding one causes; action "Add {singular}". Where nothing can be added (history, deliveries, logs), the title is "Nothing here yet" or "No {plural} yet", the body says when it will fill up, and there is no action.

**Loading: skeletons that match the final shape.**
- PageHeader always renders; it is never replaced.
- Lists use `SkeletonRows` inside the same Card/DataList wrapper, with the row count equal to the page size, capped at 5.
- Stat grids use `SkeletonStatCards`. Cards use a `SkeletonCard` of the final height.
- No spinners and no "Loading…" text. Buttons in flight keep their width and show "Saving…" text, as today.
- Skeleton colour `bg-raised`; pulse disabled under reduced motion.

**Error: `ErrorBox`, restyled with one addition.**
- Classes: `rounded-card bg-danger/10 p-4 ring-1 ring-inset ring-danger/30`, with an alert icon, title `text-body font-semibold text-fg`, and a plain reason `text-body text-fg-muted`.
- Then the technical line in `text-caption font-mono text-fg-muted`: `HTTP 500 · internal_error`. Keep the `error-status` and `error-code` spans.
- Then the `details` disclosure "Technical details".
- **New `onRetry?: () => void`.** When provided, a `btn` "Try again" renders. Every page-level query error passes `() => query.refetch()`.
- Page-level errors render under the PageHeader, in place of the content. Mutation errors render where the mutation's button is (for example above a form's action bar) and also toast, as today.

**Banners (`Banner`)** are for page-level notices that are not errors, such as dashboard warnings, "Viewing only on a phone", "Saving here replaces the manual steps…", the update-available box and the "Encoding paused" sentence.
- Classes: `rounded-card p-4 ring-1 ring-inset bg-{tone}/10 ring-{tone}/30 flex items-start gap-3`, with an icon, `text-body text-fg` body, and an optional action on the right (`btn`, or for warnings `btn` with `text-warning`).
- It replaces 5 different amber styles: `bg-amber-950/50 ring-amber-800/60`, `bg-amber-500/10 text-amber-300` (x4), `bg-amber-800/60` buttons, plus the sky update box.

---

## 4. Component inventory

Legend: **Keep** (no visual change), **Restyle** (same API, new classes on tokens), **Merge** (folds into another), **Replace** (new component takes over), **New**.

### 4.1 Primitives and shared pieces

| Component | Verdict | Reason | Target API / notes |
|---|---|---|---|
| `uiControls.ts` | Restyle and extend | It is the right idea but only 12 of about 40 UI files use it; it lacks danger, icon and select variants | Exact contents in 2.10 |
| `PageContainer.tsx` | Restyle | Width and gap rule | `max-w-6xl`, default `flex flex-col gap-6`; API unchanged |
| `PageHeader.tsx` | **New** | Five ad hoc header layouts and three back-link styles | 3.3 |
| `Card.tsx` | **New** | 20+ hand-written card containers | 3.4 |
| `FormSection.tsx` (exports `FormSection`, `Field`, `FormActions`) | **New** | Forms stretch 1100 px; three label styles | 3.6 |
| `EmptyState.tsx` | **New** | Empty states are one grey line ("No servers configured yet.") | 3.7 |
| `Banner.tsx` | **New** | Five warning and notice styles | `{ tone: 'info'\|'success'\|'warning'\|'danger', icon?, title?, children, action? , testId? }` |
| `Icon.tsx` | **New** (extract) | `Icon` and `ICONS` live inside AppShell; StatCard takes raw paths | `<Icon name="library" className? />` with named paths: dash, library, nodes, encode, integrations (new: two arrows ⇄, replacing the tray), app, menu, close, collapse, expand, check, alert, info, clock, pause, play, chevron-right, chevron-down, copy, folder, file, external, logout, brand. No icon package. |
| `Badge.tsx` | **New** (generalises `StatusChip`) | Node pills, encoder chips, the "easy" pill, status chips, level chips and flow chips each have their own classes | `{ tone: 'neutral'\|'info'\|'success'\|'warning'\|'danger', dot?: boolean, pulse?: boolean, children, testId? }`. Classes: `inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-caption font-medium` plus the tint recipe |
| `StatusChip.tsx` | Restyle, built on Badge | Keep the file and API (`status`, `count`, `testId`), since tests import it | Tone map in 5.0. **Renders nothing when `count===0`** (see the test note in 5.4). Label via `statusLabel` |
| `Switch.tsx` | Restyle | 80x44 white knob with "OFF" text inside looks foreign | Button stays `h-11` (hit area) with `inline-flex items-center`. The visual track is an inner span `h-6 w-11 rounded-full` (`bg-accent` on, `bg-raised ring-1 ring-inset ring-line-strong` off), knob `h-5 w-5 rounded-full bg-fg` with `translate-x-5`. The ON/OFF text is removed (state is in `aria-checked`, and the visible label sits beside it). Add an optional `visibleLabel?: string` that renders a `<span>` and drops `aria-label` in favour of `aria-labelledby`. |
| `Checkbox.tsx` | **New** | Three checkbox sizes; the 44 px green box on App settings | `{ checked, onChange, label, help?, testId, disabled? }`. Markup: `label.flex.items-start.gap-3.min-h-11.py-2.cursor-pointer` > `span.relative.mt-0.5.h-5.w-5.shrink-0` > `input[type=checkbox].peer.absolute.-inset-3.h-11.w-11.cursor-pointer.appearance-none.opacity-0` plus `span` box `h-5 w-5 rounded-md border border-line-strong bg-canvas peer-checked:bg-accent peer-checked:border-accent peer-focus-visible:ring-2 peer-focus-visible:ring-focus` plus a check `Icon` shown on `peer-checked`. The input is 44x44 for the touch audit and the box is 20 px. The `data-testid` goes on the `input`, so tests that `click` or `toBeChecked` keep working. |
| `Tabs.tsx` | **New** | Integrations tabs are styled as primary buttons | `{ tabs: {id,label,testId}[], value, onChange, label }`. `role="tablist"`, arrow and Home/End keys, `aria-selected`, roving tabindex. Style: `border-b border-line flex gap-1`; tab `min-h-11 px-3 text-body font-medium text-fg-muted hover:text-fg`; selected `text-fg` plus `shadow-[inset_0_-2px_0] shadow-accent`. |
| `SegmentedControl.tsx` | **New** | The rules tier selector fills the selected tier with primary green | `{ options: {id,label,testId,to?,badge?}[], value, onChange?, label }`. Container `inline-flex rounded-control bg-canvas p-1 ring-1 ring-line`; segment `min-h-11 rounded-md px-4 text-body font-medium text-fg-muted`; selected `bg-raised text-fg`. An option with `to` renders as a `Link` with `aria-current="page"`. Options render as buttons with `aria-pressed` (the current semantics, which tests rely on). |
| `Disclosure.tsx` | **New** | "What this does … Show" and "▶ Advanced" are two disclosure patterns | Native `details`/`summary`. The summary is `flex min-h-11 items-center justify-between gap-3 px-4 sm:px-6 text-body font-semibold`, with a chevron icon that rotates 90° on `[open]` (`duration-base`). Used inside a Card with `padding="none"`. |
| `CopyButton.tsx` | **New** (extract from NodesPage) | Copy appears on Nodes, the webhook secret, JobLog and container instructions | `{ text, label, testId? }`; `btn`, swaps to "Copied" for 2 s |
| `CodeBlock.tsx` | **New** | `pre` blocks clip on mobile (`mobile-app.png`) | `{ children: string, copy?: boolean, testId? }`. Classes `relative rounded-control bg-canvas ring-1 ring-line p-3 font-mono text-caption text-fg whitespace-pre-wrap break-words`. CopyButton at top-right when `copy`. |
| `format.ts` | **New** | Number and date formatting is scattered (`formatBytes` in easyModel, `timeAgo` in NodesPage, ISO dates raw) | `formatBytes` (moved; easyModel re-exports it so imports and tests keep working), `formatCount(n)` (`Intl.NumberFormat` grouping), `formatPercent`, `formatDateTime(iso)` (`Intl.DateTimeFormat(undefined,{dateStyle:'medium',timeStyle:'short'})`), `formatRelative(iso, now)` (moved from NodesPage `timeAgo`, outputs "23 s ago", "4 min ago", "3 h ago", "2 d ago"), `formatDuration` (existing `formatEta`), `shortHash(s)` (first 7) |
| `useDocumentTitle.ts` | **New** | Every tab says the same thing | `useDocumentTitle(parts: (string\|undefined)[])` joins with " · " and appends "ShrinkYourShit" |
| `skeleton.tsx` | Restyle | Token colours; `SkeletonCard` uses the card recipe | API unchanged |
| `ErrorBox.tsx` | Restyle and extend | 3.7 | Add `onRetry?` |
| `dialog.tsx` (`ModalDialog`) | Restyle | Token overlay recipe | The default className becomes `m-auto w-[calc(100%-2rem)] max-w-md rounded-overlay bg-surface p-0 text-fg ring-1 ring-line-strong/40 shadow-overlay`. Add a `size?: 'sm'\|'md'\|'lg'` prop (`max-w-md`/`max-w-xl`/`max-w-3xl`) and export `DialogHeader` (title plus close `btnIcon`, `px-6 pt-6`), `DialogBody` (`px-6 py-4 max-h-[70dvh] overflow-y-auto`) and `DialogFooter` (`px-6 pb-6 flex flex-wrap justify-end gap-2`). |
| `ConfirmDialog.tsx` | Restyle | Its buttons are hand-written; the confirm sits left, cancel right | Built on DialogHeader/Body/Footer. The footer order is **Cancel (btnGhost), then confirm** (`btnDanger` when `danger`, else `btnPrimary`), right-aligned; on mobile both are `w-full`, confirm first. Test ids unchanged. |
| `toast.tsx` | Restyle | Uses raw emerald/red; dismiss is 20 px | Recipe: overlay surface plus a 3 px left border in tone colour (`border-l-[3px] border-success` or `border-danger`), icon, text `text-body text-fg`, dismiss `btnIcon`. Position: top-right on `sm+`, **bottom** on mobile (`inset-x-4 bottom-4`), so it does not cover the top bar. Durations unchanged. |
| `StatCard.tsx` | Restyle | Uses raw slate and font-bold; the icon is a raw path | Props add `sub?: ReactNode` (caption under the value) and `tone?: 'default'\|'good'\|'warning'\|'danger'`. Keep the `clamp(` classes the test asserts. The value colour for `good` stays `text-emerald-400` **or** the test is updated to `text-accent-text`. Prefer updating `StatCard.test.tsx:34,49` to assert the tokens (`text-fg`, `text-accent-text`) and list that in the notes. The `icon` prop takes an `Icon` name. |
| `DataList.tsx` | Restyle and extend | 3.5 | Adds `cardsBelow`, `rowHref`, `empty`, column `align` and `cardPlacement` |
| `useMediaQuery.ts`, `useUnsavedGuard.ts`, `guards.tsx`, `events.ts`, `queries.ts`, `toastContext.ts`, `main.tsx` | Keep | Not visual | `main.tsx` gains the font import |
| `*Model.ts`, `kinds.ts`, `api.ts` | Keep | Not visual; API contract frozen | `easyModel.ts` re-exports `formatBytes` and `formatEta` from `format.ts` |

### 4.2 Shell

| Component | Verdict | Reason | Target |
|---|---|---|---|
| `AppShell.tsx` | Restyle (structural) | 3.1 | Desktop header removed; rail sticky; rail footer; drawer contents |
| `AppShell` › `PauseAllSwitch` | **Merge** into `EncodingStatus.tsx` | Three controls for one job | 3.1.1 |
| `AppShell` › `JobIndicator` | **Merge** into `EncodingStatus.tsx` | Same | Keep `job-indicator` and its text |
| `AppShell` › `LogoutButton` | Restyle | A full-width grey block in the drawer | Rail footer row: user name (`text-body text-fg-muted truncate`) plus a `btnIcon` logout with `aria-label="Log out"`. Keep `data-testid="logout"`. |
| `AppShell` › `Icon`/`ICONS` | **Move** to `Icon.tsx` | Reuse | Import from `Icon.tsx` |
| `EncodingStatus.tsx` | **New** | 3.1.1 | Variants: rail, icon, compact, panel |

### 4.3 Feature components

| Component | Verdict | Reason | Target |
|---|---|---|---|
| `EasyLibraryForm.tsx` | Restyle | Its own button classes; no FormSection; disabled "Start" looks broken | FormSection "Folder" (folder field), "Rules" (sliders), "Watching" (Checkbox). Primary "Add library", disabled until a folder is picked, with help "Pick a folder first." under the action bar while disabled. |
| `easyControls.tsx` | Restyle | Stop labels wrap three lines on mobile and run 1100 px wide on desktop; the checkbox is unstyled | `StopSlider`: range input styled through a `.range` class in `index.css` (track `h-1.5 rounded-full bg-raised`, filled part via `background: linear-gradient(...)` driven by a `--pct` CSS var, thumb 20 px `bg-fg ring-4 ring-accent/40`, keyboard unchanged). Stop buttons `min-h-11 text-caption`. **Under `sm`, only the first, the selected and the last stop label render** (the others stay in the DOM as `sr-only` so their test ids remain clickable in tests). The hint line is `text-body`, with the selected label in `font-semibold text-fg`. `RemoveSubtitlesCheckbox` uses `Checkbox`. |
| `KindPicker.tsx` | Restyle | 3-column grid with a dangling empty cell | `grid gap-2 sm:grid-cols-2`. Each option uses the `Checkbox` markup inside a `rounded-control ring-1 ring-line px-3` tile; checked tile `ring-accent/60 bg-accent/5`. |
| `FolderPickerModal.tsx` | Restyle | Its own button classes | ModalDialog `size="lg"`. Header "Choose a folder"; body has the breadcrumb path (mono) and a directory list (`divide-y divide-line`, rows `min-h-11` with a folder icon); footer has the current path (mono, middle-truncated as today), then Cancel and "Use this folder" (`btnPrimary`). |
| `FileHistory.tsx` | Restyle | Card recipe, Badge for job status, format.ts | Rendered in ModalDialog `size="lg"`. Each attempt is a row: Badge, encoder, before → after, `formatDateTime`, Log button. |
| `JobLogDialog.tsx` | Restyle | Level chips become Badge; filter chips become SegmentedControl | `size="lg"`. Log body is a `CodeBlock`-style region with `max-h-[60dvh] overflow-auto`; rows `grid grid-cols-[auto_auto_1fr] gap-x-3 font-mono text-caption`. |
| `NodeJobLine.tsx` | Restyle | Progress bar colours; keep `sm:flex-row` (asserted) | Progress `h-1.5 rounded-full bg-raised` with fill `bg-accent transition-[width] duration-slow`. Meta in `text-caption text-fg-muted tabular-nums`. |
| `StepStackEditor.tsx` | Restyle | rose errors; tiny `px-2 py-1` up/down/remove buttons (not 44 px) | Each step row is a Card (`padding="none"`) with a drag handle `btnIcon`, the title, the condition `text-caption text-fg-muted`, Badges for params, and on the right `btnIcon` up, down and remove (remove uses `text-danger`). Errors use `text-danger text-caption`. "Add step" is a `btn` with a select of step types beside it. |
| `flowParams.tsx` | Restyle | Uses the raw `bg-slate-800` input | `Field` plus `input`/`select` from uiControls |
| `FlowCanvas.tsx` | Restyle | Grey `#141414` canvas, 10 px ids, empty minimap | Node face: `rounded-card bg-surface ring-1 ring-line px-4 py-3 min-w-[15rem] max-w-[17rem]`; selected `ring-2 ring-accent`; error `ring-2 ring-danger`. **Remove the step-id span from the node face** (`FlowCanvas.tsx:77`; it moves to the inspector). Chips use `Badge tone="neutral"` (12 px). Handles `!bg-line-strong !border-surface`. `fitViewOptions={{ padding: 0.2, minZoom: 0.85, maxZoom: 1 }}`, so text never renders below about 10 px. MiniMap only when `nodes.length >= 10`, with `nodeColor="rgb(var(--c-raised))"` (check no test asserts the minimap; none did at `2f2b365`). |
| `FlowInspector.tsx` | Restyle | rose-400 danger buttons; raw inputs | Card with header "Step" and a `text-caption font-mono` type id (the id lives here now), `Field`s, `btnDangerGhost` for "Remove step" and "Remove connection". Empty state: "Select a step to see its settings." |
| `FlowPalette.tsx` | Restyle | 10 px text | Card "Add a step"; entries are `min-h-11` buttons with title plus caption description; group headers are overline |
| `FlowPreviewPanel.tsx` | Restyle | Its own disclosure ("Show/Hide" text) | `Disclosure` titled "What this does"; list `list-disc pl-5 marker:text-fg-subtle space-y-1 text-body` |
| `AboutCard.tsx` | Restyle | ISO date, full hash, 44 px checkbox, clipped `pre` | Becomes a FormSection "About and updates". `dl` rows: Version (mono), Commit (`shortHash`, mono, `title`=full), Built (`formatDateTime`), Install type. "Check for updates automatically" uses `Checkbox`; "Check now" is `btn`; ContainerInstructions uses `CodeBlock copy`; the update-available box uses `Banner tone="info"`; Update channel select `max-w-xs`. |
| `NodesPage` › `NodeRow` | Restyle (structural) | Two pills ("Ready" plus "Stopped"); Revoke has no confirm | See 5.10 |
| `NodesPage` › `AddNodeDialog`, `EditNodeDialog` | Restyle | Dialog sections | DialogHeader/Body/Footer, `Field`, `CodeBlock copy` for the join command |
| `NodesPage` › `QSVFeatureStrip` | Restyle | Badges | Badge row |
| `IntegrationsPage` › `ServersTab`, `WebhooksTab`, `DeliveriesDialog` | Restyle | Tabs as buttons; one-line empty states | See 5.12 |
| `PluginsPage` (`PluginsTab`, `PluginSettingsForm`) | Restyle | Same as above | See 5.12 |
| `SetupWizard.tsx` | Restyle (structural) | Text-only step list, `div` as button, its own buttons | See 5.1 |
| `LoginPage.tsx`, `SetupPage.tsx` | Restyle | Its own inputs and buttons, `min-h-full` | See 5.1 and 5.2 |

---

## 5. Page by page

There are 13 screens: the 11 photographed pages plus Setup and Login. Each page lists **First** (what the user must see first), **Layout**, **Actions** and **Changes from today**.

### 5.0 Shared mappings used below

File status (via `StatusChip` and Badge):

| status | Label | Tone |
|---|---|---|
| new | New | neutral |
| satisfied, `bytes_saved > 0` | **Shrunk** (derived in the UI, no API change) | success |
| satisfied, `bytes_saved == 0` | Nothing to do | neutral |
| queued | Waiting | neutral |
| processing | Working | info, `dot pulse` |
| done | Done | success |
| failed | Failed | danger |
| missing | Missing | warning |
| cancelled (jobs) | Cancelled | neutral |

Node lifecycle: running = success with dot; stopped = neutral; draining = warning; error = danger. Approval: pending = warning "Waiting for approval"; revoked = danger "Revoked"; approved is **not shown** (it is the normal state).

### 5.1 Setup (`/setup`, outside the shell)

- **First:** the question for the current step, and where you are in three steps.
- **Layout:** `main.min-h-dvh.grid.place-items-center.bg-canvas.p-4`. Column `w-full max-w-lg flex flex-col gap-6`: brand mark plus wordmark (a `div`, **not an h1**, `text-base font-semibold`, centred), then a level 1 Card (`p-6 sm:p-8`).
- **Stepper:** `ol` with three items, each a 24 px numbered circle and a label: done = `bg-accent text-accent-fg` with a check icon; current = `ring-2 ring-accent text-fg`; upcoming = `ring-1 ring-line-strong text-fg-muted`. Connectors are `h-px flex-1 bg-line`. Current item `aria-current="step"`.
- **Step 1:** two option cards as real `button`s with `aria-pressed`. The "Join an existing server" `div` becomes a `button`, disabled when not joinable, keeping `data-testid="mode-join"` and the click behaviour. Selected option: `ring-2 ring-accent bg-raised`. The server list (join) is a `divide-y` list of `min-h-11` buttons.
- **Step 2:** Choice "Require a login" (Switch with visible label), then `Field`s for username and password with `input` and inline validation errors from `validateAccountStep`.
- **Step 3:** summary `dl` plus the "Start the built-in node now" Checkbox (whatever exists today, restyled).
- **Footer:** `flex justify-between`. "Back" is `btnGhost`, hidden (not `opacity-0`) on step 1 via `invisible`, to keep the layout. "Next"/"Finish" is `btnPrimary`.
- **Joined panel:** the same Card. Title "Joining {server}" is the page's only h1, with a `Banner tone="info"` "Approve this machine on the server's Nodes page."
- **Changes:** one h1 per page (the step title); `min-h-full` becomes `min-h-dvh`; buttons and inputs come from uiControls (they are 40 px today); the stepper is visual instead of "1. Server 2. Account 3. Finish" text.

### 5.2 Login (`/login`, outside the shell)

- **First:** two fields and one button.
- **Layout:** the same centring as Setup. Card `max-w-sm`, with the brand mark, h1 "Log in" (`text-title`; the brand is not the h1), and a subtitle "to ShrinkYourShit".
- `Field` username (`autoComplete="username"`) and password, both `input` (`min-h-11`; they are 40 px today). The error is `Banner tone="danger"` above the button, `role="alert"`, keeping `login-error`.
- The button is `btnPrimary w-full`, "Log in" or "Logging in…".
- **Changes:** inputs and button sizes; one h1 with a real title; the Banner error replaces bare red text.

### 5.3 Dashboard (`/`)

- **First:** is it working, and how much has it saved.
- **Layout** (`lg` = 12-column grid inside the container, `gap-6`):
  1. `PageHeader title="Dashboard"`, with no actions (the page stays read-only, per the review).
  2. `EncodingStatus variant="panel"`, full width. One sentence plus one action:
     - Paused with work waiting: `Banner warning` "Encoding is paused. {n} files are waiting." Action: "Resume encoding" (`btn`).
     - Paused with nothing waiting: `Banner neutral` "Encoding is paused. Nothing is waiting right now." Action "Resume encoding".
     - Running: "Shrinking {k} files · {n} waiting" in `info`.
     - Idle: "All caught up." in `success`, with no action.
     
     This replaces the silent paused state of today. The existing `Warnings` list renders immediately below it as Banners, keeping the `warning-{id}` test ids.
  3. Stats row: `grid grid-cols-2 gap-4 lg:grid-cols-4`, using `StatCard` with `sub`:

     | testid | Label | Value | sub |
     |---|---|---|---|
     | `stat-space-saved` | Space saved | `formatBytes(bytes_saved)`, tone good | "from {Σ node.stats.files_done} files" (derived from `/nodes`, no API change) |
     | `stat-files` | Files watched | `formatCount(files)` | — |
     | `stat-satisfied` | Already small | `formatCount(satisfied)` | "nothing to do" |
     | `stat-queued` | Waiting | `formatCount(queued)` | "{failed} failed" in `text-danger` when failed > 0 |

  4. `lg:grid-cols-12`: Card "Working now" (`lg:col-span-8`) and Card "Nodes" (`lg:col-span-4`).
     - Working now: one `NodeJobLine` per active job (it already shows file name, percent, speed and ETA). Empty: "Nothing is being shrunk right now." plus the reason when paused ("Encoding is paused."), keeping `dashboard-idle`.
     - Nodes: a `divide-y` list with name, the lifecycle Badge and the last check-in (`formatRelative`). The header action is the link "Manage" → `/nodes` (`linkTouch`).
  5. Card "Recent activity" (`padding="none"`), full width, a `divide-y` list of up to 10 rows (`history-job-{id}` kept):
     - Row: `grid grid-cols-[auto_1fr_auto] items-center gap-3 px-4 py-3 sm:px-6`, with a status icon (check in success, or alert in danger), then **`job.file_name`** (`text-body text-fg truncate`, with `title` = `file_path`) over a caption of `formatRelative(created_at)` and the library name, then on the right `6.0 MB → 5.4 MB` (`tabular-nums text-fg-muted`) above "−10 %" (`text-caption text-accent-text`). Failed rows show the error in `text-caption text-danger truncate`.
     - On mobile the right column stacks under the name, so the sizes never wrap mid-number.
- **Empty dashboard** (no libraries): `EmptyState` titled "Add your first library", body "Pick a folder of videos. ShrinkYourShit makes smaller copies and swaps them in only when they are smaller and verified.", action `btnPrimary` "Add library" → `/libraries/new`. Keep the `empty-dashboard` test id.
- **Changes:** job ids become file names; the silent pause is explained; stat labels; the history row style (tinted boxes become a divided list); sizes no longer wrap mid-value on mobile.
- Test updates: `DashboardPage.test.tsx` assertions on "job a35a5e4b"-style text, if any, and the stat labels "Files" and "Nothing to do".

### 5.4 Libraries (`/libraries`)

- **First:** each library's name, how much it saved, and whether anything needs attention.
- **Layout:** `PageHeader title="Libraries"` with the action `btnPrimary` "Add library" (was "Add folder").
- `DataList` with `rowHref` → `/libraries/{id}`:

  | Column | Table | Card |
  |---|---|---|
  | Library (primary) | name `font-semibold` over path `font-mono text-caption text-fg-muted truncate` | top-left |
  | Status | **only the non-zero chips**, in a `flex flex-wrap gap-1` that never wraps when fewer than 3 chips. When every file is satisfied or done: one success Badge, "Up to date". | `cardPlacement:'badge'` (top-right shows the most urgent chip only: failed > working > waiting > up to date) |
  | Files | `formatCount`, `align:'end'` | dl row |
  | Saved | `formatBytes`, `text-accent-text`, `align:'end'` | dl row (value only, without the word "saved"; today the card shows "14.6 MB saved" under the label "Saved") |
  | Looking for | kinds as words ("Video") | dl row |
  | Actions | `btn` "Scan now" and `btnGhost` "Edit". **"Remove" leaves the row** for a "Danger zone" card on the library's Edit page (5.7). No overflow menu: there is no menu primitive and one is not worth building for two actions. | footer |

- **Empty:** EmptyState (copy in 3.7), keeping the existing empty test id.
- **Changes:** zero chips gone (the wrapping `0 Failed` on desktop disappears); the path becomes mono under the name, not its own grey column; rows are clickable; there are two row actions instead of three; "Add folder" becomes "Add library".
- **Test updates** (list them in the notes):
  - `LibrariesPage.test.tsx:116-121` assert `0 Done` and `0 Failed` chips; change them to `queryByTestId(...)` being null.
  - The remove-library flow tests move to `LibraryEditPage.test.tsx`, keeping the ConfirmDialog test ids (`confirm-dialog`, and so on).
  - If moving Remove is judged too large for the Libraries task, leave Remove in the row as `btnDangerGhost` and do the move in task 47. Say which in the notes.

### 5.5 New library (`/libraries/new`)

- **First:** pick the folder.
- **Layout:** `PageHeader` with breadcrumb `Libraries`, title "Add a library", subtitle "Pick a folder of videos. ShrinkYourShit makes copies that need less space and swaps them in when they are smaller." (the existing copy).
- **FormSection "Folder"** ("The folder to watch. Subfolders are included."): a read-only path display (`input` styling, `font-mono`, placeholder "No folder chosen yet") with an inline `btn` "Choose…" in a `flex gap-2` row. The picker opens `FolderPickerModal`.
- **FormSection "How much to shrink"** ("You can change this later in Rules."): QualitySlider, AudioSlider and the RemoveSubtitles Checkbox, then the `FlowPreviewPanel` Disclosure ("What this does").
- **FormSection "Watching"**: the Checkbox "Watch this folder for new files", with help "New files are picked up automatically."
- **Action bar:** `btnPrimary` "Add library" (was "Start"), plus `btnGhost` "Cancel" → `/libraries`. Sticky on mobile.
- **Changes:** the form column is capped at 672 px; the field is labelled; the action says what it does; the disabled state explains itself.

### 5.6 Library detail (`/libraries/:id`)

- **First:** how much this library saved, what is happening to it, and the file list.
- **Layout:**
  1. `PageHeader`:
     - breadcrumb `Libraries`;
     - title = library name;
     - meta = a Badge for watch state ("Watching" neutral; omitted if not watched);
     - subtitle = path (`font-mono`);
     - actions: `btn` "Rules", `btn` "Edit" and `btnPrimary` "Scan now". Scan moves here from the Libraries row so the detail page has its primary action; keep the existing scan test id.
  2. Summary Card, which replaces the chip soup:
     - Top row `grid grid-cols-2 sm:grid-cols-4 gap-4` of inline stats (no card per stat): **Saved** (`text-accent-text`, the large value), **Files**, **Waiting**, **Failed** (danger when > 0). These come from the same per-library stats.
     - Then a caption "Last scan {formatDateTime}".
     - Then `border-t`, and the rules summary: an h3 "Rules · Easy" (the tier in use) with the plain-language step list (the existing "What this library does" lines, as `list-disc` `text-body`), ending with the link "Change rules" → Rules.
     - Keep the chip test ids by rendering `StatusChip`s for the non-zero statuses in a row under the stats (zero hidden).
  3. File list toolbar: `flex flex-wrap items-end gap-3`, labels above the controls (not inline):
     - Search (`input`, `flex-1 min-w-[12rem]`, search icon);
     - Status (`select max-w-[10rem]`);
     - Sort (`select`), with the direction as a `btnIcon` showing an up or down arrow (`aria-label` "Sort ascending" or "Sort descending");
     - "Retry failed ({n})" `btn`, **rendered only when failed > 0**.
  4. `DataList cardsBelow="md"` with columns:
     - File (primary; name, plus a mono caption of the relative path when it differs);
     - Status (StatusChip label using the 5.0 mapping, so "Shrunk" appears);
     - Before and After (`align:end`);
     - Saved (`text-accent-text`, `align:end`, with the ratio as a caption "1.9× smaller");
     - Encoder ("CPU", "Intel", "NVIDIA": map the `cpu`, `qsv`, `vaapi`, `nvenc` ids to words in `format.ts` `encoderLabel`);
     - Why (only when non-empty, otherwise the column is hidden on cards);
     - Actions `btnGhost` "Log" plus "History".
  5. Pager `flex items-center justify-between`, with "{from}–{to} of {total}" on the left and Previous/Next `btn` on the right. Per-page select in a `Field`. **The whole pager is hidden when `total <= perPage`.**
- **Changes:** six chips with zeros become a four-number summary; the "Shrunk" status; Scan is reachable on this page; the pager and Retry only appear when meaningful; filter labels sit above their controls.

### 5.7 Edit library (`/libraries/:id/edit`)

**Target state, needing owner sign-off.** The full change is task 47. The tier logic is untouched; only where it is edited moves.

- **First:** name and folder.
- **Layout:**
  - `PageHeader` with breadcrumb `Libraries › {name}`, title "Edit library", and no actions.
  - **FormSection "Library"**: Name, Folder (path display plus "Change…"), and the "Watch for new files" Checkbox.
  - **FormSection "File types"**: the KindPicker, with the help line "Files of other types stay untouched."
  - **FormSection "Rules"**: a read-only summary ("Easy · Balanced · keep every audio track") plus `btn` "Change rules" → `/libraries/:id/rules`.
  - Action bar "Save changes" / "Cancel".
  - **FormSection "Danger zone"**: description "Removing a library forgets its history. Files on disk are not touched." and `btnDangerGhost` "Remove library", which opens the existing ConfirmDialog.
- **Why:** today the Edit page duplicates the easy sliders *and* adds a fourth rule editor ("Advanced": raw `Target` with `keep_all`-style option labels). That contradicts DECISIONS item 6 ("all three tiers on one engine … lower tiers generate the same flow format") as a UI story. Two "Advanced" buttons now mean two different things. Every capability of the Target panel (codec, re-encode-same-codec, quality, container, audio mode, languages, subtitle mode) is a Steps-tier parameter.
- **Interim** (task 41, before sign-off): keep both panels but restyle them. Put the sliders in a FormSection "How much to shrink". Rename the panel from "Advanced" to "**Exact settings**" (a Disclosure) with the description "Replaces the sliders with exact values for this library." Give options human labels: `keep_all` → "Keep every track", `keep_preferred` → "Keep my languages", `keep_best` → "Keep the best track", `compact` → "Keep the best track, make it small", `remove` → "Remove all"; codec labels come from `CODEC_CHOICES`; container "MKV"/"MP4". Values are unchanged, so the API is unchanged.

### 5.8 Rules (`/libraries/:id/rules`), Easy and Steps tiers

- **First:** which tier this library uses, and its controls.
- **Layout:**
  1. `PageHeader` with breadcrumb `Libraries › {name}`, title "Rules", and subtitle "Pick one way to describe what happens to each file. Easy is enough for most libraries."
  2. `SegmentedControl` (label "Rule editor") with options Easy, Steps and Advanced. Advanced has `to=/libraries/:id/flow`.
     - The option matching `flow.source` gets a small `Badge tone="success"` "In use" inside the segment (as the `badge` option).
     - The selected segment is neutral (`bg-raised`), **not green**.
     - Test ids `tier-easy`, `tier-steps` and `tier-advanced` and the `aria-pressed`/`aria-current` semantics are kept.
  3. Under it, a caption (`text-caption text-fg-muted`) explaining the three tiers in one line: "Easy: three sliders. Steps: an ordered list. Advanced: a visual graph. All three produce the same kind of rules." This puts the three-tier story on screen.
  4. Replace warnings ("Saving here replaces…") become `Banner tone="warning"`, keeping their test ids.
  5. **Easy:** FormSection "How much to shrink" with the sliders and the subtitle Checkbox, then the "What this does" Disclosure. Action bar: `btnPrimary` "Save rules" (was "Save easy settings").
  6. **Steps:** the `StepStackEditor` restyled (4.3) inside a Card titled "Steps, in order", with the description "Each file goes through these steps from top to bottom." Action bar "Save rules".
  7. **Advanced (gateway):** when the library's source is advanced and the user is on this route, show a Card with one sentence and `btnPrimary` "Open the flow editor" (keep `advanced-open-canvas`). The "graph structure will be lost" caption becomes `Banner warning`.
- **Changes:** the selected tier is no longer primary green; "In use" is shown; the save label; the sliders are capped in width; the back link becomes a breadcrumb.

### 5.9 Flow (`/libraries/:id/flow`), Advanced tier

- **First:** the graph, and whether you can edit it.
- **Layout:**
  1. The same `PageHeader` as Rules (breadcrumb `Libraries › {name}`, title "Rules"), so Advanced reads as the third tier of one page, not a different place.
     - Meta: "{n} steps · {m} connections" caption.
     - Actions in read mode: `btnPrimary` "Edit flow" (**one** instance; the duplicate in the banner is removed; keep the header one's test id and drop the banner's after checking the tests).
     - Actions in edit mode: `btnGhost` "Cancel", `btn` "Validate" (if present today), `btnPrimary` "Save rules".
     - The unsaved marker is a `Badge tone="warning"` "Unsaved changes", keeping `flow-dirty`.
  2. The same `SegmentedControl` as Rules, with Advanced selected.
  3. Read-only notice: `Banner tone="info"` "This is the flow this library runs. The rules come from the Easy settings. Edit the flow to take over manually." On mobile, the existing `flow-mobile-notice` copy appears as a `Banner info`.
  4. The editor at `lg`: `grid grid-cols-[14rem_minmax(0,1fr)_20rem] gap-4`, with the palette (edit only), the canvas, and the inspector.
     - The canvas frame is `${card} overflow-hidden h-[calc(100dvh-16rem)] min-h-[28rem]`, which fills the viewport instead of today's fixed `68vh` with a gap below.
     - Under `lg` the columns stack: canvas first, then the inspector.
  5. The canvas is themed with tokens (2.9 CSS); node faces as in 4.3; the minimap only at 10 or more nodes; the attribution has a transparent background.
- **Changes:** no grey hole; no 6 px text; one Edit button; it shares the Rules header and tier control; the canvas fills the height.

### 5.10 Nodes (`/nodes`)

- **First:** is each machine working, and what is it working on.
- **Layout:**
  1. `PageHeader title="Nodes"`, subtitle "Machines that do the shrinking. This server is one of them.", action `btnPrimary` "Add node".
  2. `EncodingStatus variant="panel"`: the sentence ("Encoding is paused on all nodes.") plus Pause all / Resume all (`nodes-pause-all`, `nodes-resume-all` kept, with the existing disabled logic).
  3. Pending approvals first: Cards with a `Banner warning`-tinted header "Waiting for approval", plus "Approve" (`btnPrimary`) and "Reject" (`btnDangerGhost`, existing confirm).
  4. Node cards, `grid gap-4 lg:grid-cols-2`. One Card per node:
     - **Header row:** name (`text-heading`, `break-words`), then **one** lifecycle Badge (Running, Stopped, Draining or Error). "Ready" is dropped; approval shows only when not approved. On the right, the Switch with the visible label "Encoding" (keep `node-switch-{id}` and the aria-label).
     - **Current job:** NodeJobLine if any; otherwise the caption "Not working on anything."
     - **Facts `dl`**, `grid grid-cols-2 sm:grid-cols-4 gap-4`:
       - "Files done" (`formatCount`);
       - "Saved" (`formatBytes`, accent);
       - "Last check-in" (`formatRelative`);
       - "Parallel jobs", as the stepper: `btnIcon` −, the value `w-8 text-center tabular-nums`, `btnIcon` +, with `aria-label`s "Fewer parallel jobs" and "More parallel jobs".
     - **Hardware**: Badge per encoder ("CPU", "Intel Quick Sync"…), and `linux/amd64` as a caption. The FFmpeg version moves into a `Disclosure` "Technical details": the version is the first token only (`7.1.4-Jellyfin`) and the full banner is in `CodeBlock`.
     - **Footer:** `btnGhost` "Edit", `btnDangerGhost` "Stop now" (when running), `btnDangerGhost` "Revoke", which **now opens ConfirmDialog**: "Revoke {name}? It stops taking work and must be paired again to come back. Jobs it holds go back to the queue." (Confirm "Revoke node".)
  5. Empty (no other nodes): `EmptyState` titled "No other machines yet", body "Add a node to share the work with another computer.", action "Add node".
- **Changes:** two pills become one; three pause controls become one pattern; Revoke is confirmed; the ffmpeg copyright banner leaves the card face; the stepper looks enabled; the switch is restyled and labelled.
- **Test updates:** `node-status-{id}` for approved nodes. If a test asserts "Ready", keep the element but `sr-only` it ("Approved"), or update the test and say so in the notes. The Revoke tests must click confirm (new step).

### 5.11 Encode settings (`/encode`) and App settings (`/app`)

**Encode settings** (`/encode`):
- **First:** what new libraries get by default.
- `PageHeader title="Encode settings"` with the subtitle "Defaults for new libraries. Each library can change them in its Rules."
- **FormSection "Library defaults"** (moved *first*, because it applies to everyone):
  - "Keep my languages" (`input max-w-xs`), with the help **"Three-letter language codes, comma separated (for example eng, deu)."** (copy bug fixed);
  - "Video format for new libraries" (`select`, full width at the 2xl cap, so it no longer truncates on mobile);
  - "What happens to the original file" (`select`).
- **FormSection "Intel Quick Sync extras"** with the description "Only used on machines where the hardware check found Quick Sync. Off changes nothing." Two Switch rows with visible labels and help. When no node reports QSV (check `capabilities` from `/nodes`), add `Banner neutral` "None of your nodes has Intel Quick Sync, so these do nothing today."
- Action bar: "Save changes".

**App settings** (`/app`):
- **First:** the server's own settings; the version is secondary.
- `PageHeader title="App settings"`.
- **FormSection "Server"**:
  - "Files processed at once" (`input w-32`, number);
  - "Scratch folder" (`input font-mono`, placeholder "Default scratch folder", help "Where copies are made before they replace the original. Leave empty to use the default.").
  - Build agent: confirm in `internal/` what `worker_concurrency` governs relative to the per-node "Parallel jobs". If it is the built-in node's default, the help text says "For this machine's built-in node. Other nodes set their own on the Nodes page." Do not change the label without confirming.
- **The action bar sits directly under the "Server" FormSection** ("Save changes"), because it only saves Server. Today it floats between Login and About.
- **FormSection "Login"**: a status sentence, then `btn` "Turn login on" or "Turn login off" (existing flows and dialogs unchanged). It acts immediately and has no Save button of its own, as today.
- **FormSection "About and updates"**: `AboutCard` restyled (4.3).
- **Changes:** sections are labelled at the left at `lg`; field widths; the Save scope is unambiguous; dates, hash and update commands are readable and copyable; the code block wraps on mobile.

### 5.12 Integrations (`/integrations`)

- **First:** which tab, and what is configured in it.
- **Layout:**
  1. `PageHeader title="Integrations"`, subtitle "Tell other apps when files change, and add new steps." (one line for the whole page; the per-tab explanation moves into each tab's Card description).
  2. `Tabs` with Servers, Webhooks and Plugins, under the header, full width (not in the header's action slot). The `?tab=` deep link, test ids and `role="tab"`/`aria-selected` are kept.
  3. Each tab is one Card (`padding="none"`):
     - Card header: title ("Media servers", "Webhooks", "Plugins"), the description (today's paragraph, for example "After a file is shrunk, your media server or download client can be told to rescan."), and the action `btnPrimary` "Add server", "Add webhook" or "Install plugin". The primary is in the card header, so it never sits next to a green tab again.
     - Body: a `divide-y` list of configured items. Each row has the name (`text-body font-semibold`), the kind and URL (`font-mono text-caption text-fg-muted truncate`), a Badge for state ("Connected" success, "Not connected" danger, "Disabled" neutral), and the row actions (`btnGhost` "Test", "Edit", "Deliveries", and `btnDangerGhost` "Remove").
     - Empty: `EmptyState`, for example title "No media servers yet", body "Add Plex, Jellyfin or Emby and it rescans by itself after a file is shrunk.", action "Add server".
  4. Dialogs (add/edit server, webhook, plugin settings, deliveries, the "Save this secret now" panel) use DialogHeader/Body/Footer and `Field`. The secret uses `CodeBlock copy` plus `Banner warning` "You will not see this again."
- **Changes:** tabs no longer look like buttons; one green element per view; real empty states; the copy sits where it applies.

### 5.13 Mobile specifics (applies to all of the above)

- The top bar is 56 px (brand plus status pill) and the drawer holds the switch. The pause label never shares the row with the brand.
- PageHeader actions wrap under the title; the primary stays last (rightmost, or last in reading order).
- Forms with a sticky action bar get `pb-24` on `main` so the last field is not hidden.
- Slider labels show only the first, selected and last stops.
- No `pre` without `whitespace-pre-wrap`.
- Long paths use `truncate` with `title` (single line) or `break-all` (detail subtitle).

---

## 6. Accessibility and polish

| Area | Rule | How it is enforced |
|---|---|---|
| Text contrast | 4.5:1 minimum for all text; 3:1 for control boundaries and focus. Content uses `fg` or `fg-muted` only; `fg-subtle` only for disabled or decorative text. | Token table in 2.2. Task 53 adds an ESLint `no-restricted-syntax` rule that rejects `text-slate-500`, `rose-`, `emerald-600`, `opacity-50` and `text-[1`… in `className` literals (a regex on JSXAttribute string values). No new dependency. |
| Primary button | slate-950 on emerald-500 (7.96:1) | `btnPrimary` |
| Focus | Visible 2 px sky-300 ring, offset 2, on every focusable element. Never `outline-none` without a replacement. | `index.css` global rule; uiControls `focus` |
| Keyboard order | DOM order is visual order. Shell: skip link "Skip to content" (first focusable, `sr-only focus:not-sr-only`, targets `main#content`), then rail nav, then rail footer controls, then main. Page: breadcrumb, header actions, then content. Dialogs: focus the first field (or Cancel in ConfirmDialog); Escape closes; focus returns (already in ModalDialog). Tabs use arrow keys; SegmentedControl uses Tab between segments (buttons). The flow editor keeps its menu alternatives for every drag. | Unit tests in each task |
| Touch targets | 44x44 for every interactive element in `main`, the shell and dialogs: stop-label buttons, breadcrumb links, step-row icon buttons, the toast dismiss and pager buttons included. The Checkbox uses the 44 px transparent input. | `e2e/mobile.spec.ts` audit, extended in task 53 to `/libraries/new`, `/libraries/:id/edit`, `/libraries/:id/rules`, `/integrations` and an open dialog |
| Reduced motion | Global media rule (2.9); the pulse dot becomes static; the drawer appears without sliding | CSS |
| Headings | Exactly one h1 per route (Setup and Login included); h2 for Cards and FormSections; h3 inside. No skipped levels. | `layout.spec.ts` already reads h1; add `expect(h1 count).toBe(1)` |
| Landmarks | `nav[aria-label=Main]`, `nav[aria-label=Breadcrumb]`, `main#content`, `aside` rail | — |
| Live regions | The toast region stays `aria-live=polite`. The `EncodingStatus` pill is `role="status"`. Progress bars keep `role="progressbar"` with `aria-valuenow`. | — |
| Forms | Every control has a `<label>` or `aria-labelledby`; help and errors use `aria-describedby`; errors use `aria-invalid` | `Field` |
| Numbers | Bytes in SI units (1000-based, matches today's `formatBytes`): one decimal below 100, none above ("15.2 MB", "551 KB", "1.2 TB"). Counts grouped with `Intl.NumberFormat` ("43,655"). Percent saved as "−10 %"; ratio "1.9× smaller". Durations "2 min 30 s". Relative times "23 s ago". Absolute times `Intl.DateTimeFormat` medium date plus short time. Ids `shortHash`, mono. `tabular-nums` in tables and stats. | `format.ts` with unit tests |
| Empty copy | "No {plural} yet" titles; one sentence on what it does; "Add {singular}" action; never "No X configured yet." as a bare grey line | `EmptyState` |
| Naming | One noun per thing: **library** (not folder) in chrome and buttons; "node"; "rules"; "Shrunk" for a file that got smaller. Nav labels equal h1s. Buttons say the action ("Add library", "Save rules", "Revoke node"), never "OK" or "Start". | Review checklist in each task |
| Plain language | The easy screens (New library, Rules › Easy, Dashboard) show no codec, container or FFmpeg terms (AGENTS rule 14). "HEVC" and "MKV" in "What this does" are produced by the server's preview text, so they stay; flag it in notes for the owner. Encode settings and Steps may name codecs. | Existing `easyPlainLanguage.test.ts` |

---

## 7. Implementation plan

Numbering continues from `tasks/STATUS.md` (35 is the last done). Each task is one agent run: branch `task/NN-…`, `make test`, `make lint`, `npm run build`, and `npx playwright test` green.

Two rules for every task:
- **Test updates:** a task may change an existing assertion only where this spec changes the asserted text or class, and must list each changed assertion (file:line, old → new, why) in `tasks/NN-notes.md`. No assertion is deleted without a replacement that checks the same intent.
- **Test ids:** every existing `data-testid` survives unless the task says otherwise.

### Phase A: tokens and config (no component rewrites)

**36: Design tokens, font, motion.**
- Touches: `web/src/index.css`, `web/tailwind.config.js`, `web/package.json` and `package-lock.json` (add `@fontsource-variable/inter`), `web/src/main.tsx` (font import), `STACK.md` (add a "Fonts" row: Inter Variable via @fontsource, self-hosted, OFL-1.1), `tasks/36-notes.md` (dependency justification, copied from section 8).
- Acceptance:
  - [ ] All variables from 2.9 exist and the Tailwind config matches it.
  - [ ] New `src/tokens.test.ts` imports `tailwind.config.js` and asserts that the colour keys `canvas, surface, raised, line, fg, accent, focus, success, info, warning, danger`, the font sizes `caption, body, heading, title` and the durations `fast, base, slow` exist.
  - [ ] The focus ring is sky-300; the reduced-motion rule is present; React Flow variables are set (verify the names against the installed CSS).
  - [ ] `npm run build` passes; the bundle includes one Inter woff2 (latin).
  - [ ] No component file changed apart from `main.tsx`.

**37: Control classes and formatting helpers.**
- Touches: `web/src/uiControls.ts`, `uiControls.test.ts`, new `web/src/format.ts` and `format.test.ts`, `easyModel.ts` (re-export `formatBytes` and `formatEta`), new `useDocumentTitle.ts` and test.
- Acceptance:
  - [ ] Exports exactly as in 2.10. The test asserts every control string contains `min-h-11` and contains none of `slate-`, `emerald-`, `red-`, `rose-`, `!`.
  - [ ] `format.test.ts` covers bytes (0, 551 000, 15 185 350, 1.2e12), counts (43655 → "43,655" with `en-US` forced), relative times (5 s, 4 min, 3 h, 2 d), `shortHash`, and `encoderLabel` (cpu, qsv, vaapi, nvenc, videotoolbox, unknown passthrough).
  - [ ] Existing `easyModel.test.ts` passes unchanged.

### Phase B: primitives

**38: Layout primitives.**
- Touches: new `Icon.tsx`, `PageHeader.tsx`, `Card.tsx`, `FormSection.tsx`, `EmptyState.tsx`, `Banner.tsx`, `Disclosure.tsx`, each with `*.test.tsx`; `PageContainer.tsx` (max-w-6xl and the default gap).
- Acceptance:
  - [ ] PageHeader renders one h1, breadcrumb `nav[aria-label=Breadcrumb]` with links, a skeleton when `title` is undefined, and actions in order.
  - [ ] Field wires `aria-describedby` and `aria-invalid`.
  - [ ] EmptyState renders title, body and action.
  - [ ] Banner has `role="status"` for info and success and `role="alert"` for danger.
  - [ ] Disclosure toggles via the keyboard.
  - [ ] Every page still renders (the existing page tests pass with the new PageContainer).

**39: Control primitives.**
- Touches: new `Badge.tsx`, `Checkbox.tsx`, `Tabs.tsx`, `SegmentedControl.tsx`, `CopyButton.tsx`, `CodeBlock.tsx` with tests; restyle `Switch.tsx`, `StatusChip.tsx` (on Badge, hiding zero, adding the "Shrunk" derivation via a new optional `bytesSaved` prop), `StatCard.tsx` (`sub`, `tone`, Icon names), `skeleton.tsx`, `ErrorBox.tsx` (`onRetry`), `dialog.tsx` (size plus Header/Body/Footer), `ConfirmDialog.tsx`, `toast.tsx`, `DataList.tsx` (`cardsBelow`, `rowHref`, `empty`, `align`, `cardPlacement`).
- Acceptance:
  - [ ] The Checkbox input's bounding box is 44x44 (asserted through its classes `h-11 w-11`) and the visible box is 20 px.
  - [ ] Tabs arrow-key navigation works.
  - [ ] SegmentedControl keeps `aria-pressed`.
  - [ ] Switch has no "On"/"Off" text node and keeps `role=switch` and `aria-checked`.
  - [ ] StatusChip renders null for 0.
  - [ ] DataList `cardsBelow="md"` emits `md:hidden` and `hidden md:block`.
  - [ ] ConfirmDialog order is Cancel then confirm.
  - [ ] Toast sits at the bottom under `sm`.
  - [ ] Test updates: `StatCard.test.tsx:34,49` (to the tokens); any test asserting a zero StatusChip is noted, and fixed in the page task that changes it.

### Phase C: shell

**40: App shell and EncodingStatus.**
- Touches: `AppShell.tsx`, new `EncodingStatus.tsx` and test, `AppShell.test.tsx`, `e2e/layout.spec.ts` (nav labels), `e2e/mobile.spec.ts` (only if a selector referenced the header).
- Acceptance:
  - [ ] No `<header>` at `md+`.
  - [ ] The rail is sticky and keeps `w-56`/`w-16` and the `sys.rail` behaviour.
  - [ ] The rail footer holds EncodingStatus, logout (`data-testid=logout`) and version (`data-testid=version`).
  - [ ] Nav labels are Dashboard, Libraries, Nodes, Encode settings, Integrations, App settings.
  - [ ] `job-indicator` shows "1 running" for one running job.
  - [ ] The switch shows on = running and calls pauseAll/resumeAll.
  - [ ] The mobile top bar is 56 px with the menu, brand and compact pill, without the label "Pause all encoding".
  - [ ] The drawer contains nav, the EncodingStatus switch and logout.
  - [ ] A skip link exists.
  - [ ] Test updates: `AppShell.test.tsx:174-175, 220, 287-288` and `e2e/layout.spec.ts:25-26` ('Dash' → 'Dashboard', 'Library' → 'Libraries').

### Phase D: pages (each restyles one page onto the primitives)

**41: Library forms (New and Edit, interim).**
- Touches: `NewLibraryPage.tsx`, `EasyLibraryForm.tsx`, `easyControls.tsx`, `KindPicker.tsx`, `FolderPickerModal.tsx`, `LibraryEditPage.tsx` (5.7 *interim*), `index.css` (`.range`), and their tests.
- Acceptance:
  - [ ] FormSection layout; the field column is capped at `max-w-2xl`.
  - [ ] "Add library" button; Cancel.
  - [ ] Slider stop labels: under `sm` only three are visible, while all test ids remain.
  - [ ] Checkboxes use `Checkbox`.
  - [ ] The Edit page's Advanced panel becomes "Exact settings" with human option labels (values unchanged; `LibraryEditPage.test.tsx` asserts values, not labels; check).
  - [ ] Sticky action bar on mobile.
  - [ ] The mobile audit passes on both routes (added to `mobile.spec.ts` here).

**42: Dashboard.**
- Touches: `DashboardPage.tsx`, `DashboardPage.test.tsx`.
- Acceptance:
  - [ ] EncodingStatus panel with the four sentences in 5.3 (a unit test per state).
  - [ ] Stats with the labels and subs in 5.3, keeping the test ids.
  - [ ] Recent activity shows `file_name`, never `job {id}`.
  - [ ] The empty dashboard uses EmptyState with the "Add library" link.
  - [ ] Working now and Nodes split 8/4 at `lg`.

**43: Libraries list.**
- Touches: `LibrariesPage.tsx`, its test.
- Acceptance:
  - [ ] PageHeader with "Add library".
  - [ ] Rows link to detail.
  - [ ] Only non-zero chips; "Up to date" when everything is satisfied or done.
  - [ ] Path in mono under the name.
  - [ ] The card has no "saved" suffix.
  - [ ] EmptyState.
  - [ ] Test update: `LibrariesPage.test.tsx:116-121`.

**44: Library detail plus FileHistory and JobLogDialog.**
- Touches: `LibraryDetailPage.tsx`, `FileHistory.tsx`, `JobLogDialog.tsx`, `NodeJobLine.tsx` (colours only), and tests.
- Acceptance:
  - [ ] Header actions are Rules, Edit and Scan now (primary).
  - [ ] Summary Card with four numbers and the rules summary.
  - [ ] Labels above toolbar controls.
  - [ ] Retry hidden at 0.
  - [ ] Pager hidden when `total <= perPage`.
  - [ ] Status "Shrunk" for satisfied with savings.
  - [ ] Encoder words.
  - [ ] `cardsBelow="md"`.
  - [ ] The dialogs use DialogHeader/Body/Footer.

**45: Rules page (Easy and Steps) plus StepStackEditor and FlowPreviewPanel.**
- Touches: `RulesPage.tsx`, `StepStackEditor.tsx`, `FlowPreviewPanel.tsx`, `flowParams.tsx`, and tests.
- Acceptance:
  - [ ] SegmentedControl with a neutral selected state and an "In use" badge from `flow.source`.
  - [ ] The three-tier caption.
  - [ ] Banners for the replace warnings (test ids kept).
  - [ ] "Save rules".
  - [ ] Step rows use 44 px icon buttons with aria-labels.
  - [ ] No `rose-` remaining.

**46: Flow page.**
- Touches: `FlowCanvasPage.tsx`, `FlowCanvas.tsx`, `FlowInspector.tsx`, `FlowPalette.tsx`, `index.css` (React Flow vars if not done in 36), and tests.
- Acceptance:
  - [ ] Shares the Rules PageHeader and SegmentedControl.
  - [ ] One "Edit flow".
  - [ ] Canvas background equals canvas `rgb(2,6,23)` (Playwright: computed `background-color` of `.react-flow`).
  - [ ] No step-id text on node faces (FlowCanvas test: `canvas-node-probe` does not contain "probe" as a separate element); the inspector shows the id.
  - [ ] fitView `minZoom` 0.85.
  - [ ] Minimap only at 10 or more nodes.
  - [ ] The mobile notice is a Banner.
  - [ ] `e2e/mobile.spec.ts` flow leg still green.

**47: Edit page becomes properties only (needs owner OK, see 8.4).**
- Touches: `LibraryEditPage.tsx`, `LibraryEditPage.test.tsx`, `LibrariesPage.tsx` (Remove leaves the row, if not done in 43), `RulesPage.test.tsx`.
- Acceptance:
  - [ ] The Edit page has FormSections Library, File types, Rules (summary plus link) and Danger zone.
  - [ ] The easy sliders and the Exact settings panel are gone from Edit.
  - [ ] Their tests move to `RulesPage.test.tsx` or are replaced by Steps-tier tests proving the same settings are reachable.
  - [ ] Remove library works from the Danger zone with the same ConfirmDialog test ids.

**48: Nodes.**
- Touches: `NodesPage.tsx`, `NodesPage.test.tsx`.
- Acceptance:
  - [ ] One lifecycle Badge for approved nodes.
  - [ ] The switch has the visible label "Encoding".
  - [ ] The EncodingStatus panel holds `nodes-pause-all` and `nodes-resume-all` (existing tests green).
  - [ ] Revoke opens ConfirmDialog (new test).
  - [ ] FFmpeg details inside a Disclosure.
  - [ ] Facts `dl` using `format.ts`.
  - [ ] EmptyState.
  - [ ] Dialogs restyled; the join command in `CodeBlock copy`.

**49: Encode and App settings plus AboutCard.**
- Touches: `EncodeSettingsPage.tsx`, `AppSettingsPage.tsx`, `AboutCard.tsx`, and tests.
- Acceptance:
  - [ ] FormSections in the order of 5.11.
  - [ ] The language help says "Three-letter".
  - [ ] The App Save sits directly under Server.
  - [ ] Number input `w-32`.
  - [ ] The About commit is `shortHash` with a `title`; the date via `formatDateTime`.
  - [ ] Container instructions in `CodeBlock` with `whitespace-pre-wrap` (Playwright at 390: `pre` scrollWidth ≤ clientWidth).
  - [ ] The update toggle is a `Checkbox`.
  - [ ] The QSV "none of your nodes" Banner when no node reports QSV.

**50: Integrations and Plugins.**
- Touches: `IntegrationsPage.tsx`, `PluginsPage.tsx`, and tests.
- Acceptance:
  - [ ] Tabs component under the header (test ids and `?tab=` kept).
  - [ ] Primary action in the Card header.
  - [ ] EmptyStates for all three tabs.
  - [ ] Rows with state Badges.
  - [ ] The secret panel uses CodeBlock plus a warning Banner.
  - [ ] Dialogs restyled.

**51: Setup and Login.**
- Touches: `SetupPage.tsx`, `SetupWizard.tsx`, `LoginPage.tsx`, and tests.
- Acceptance:
  - [ ] One h1 per screen.
  - [ ] Visual stepper with `aria-current="step"`.
  - [ ] "Join an existing server" is a `button` (test id kept).
  - [ ] Inputs and buttons from uiControls (44 px).
  - [ ] `min-h-dvh`.
  - [ ] Login error as a Banner (`login-error` kept).

### Phase E: enforcement

**52: Copy sweep.**
- Touches: any page with leftover copy from 6 (Naming, Empty copy) and `useDocumentTitle` calls on every route.
- Acceptance:
  - [ ] A grep for `Add folder`, `job {`, `Back to files` and `Back to rules` in `web/src/*.tsx` (non-test) returns nothing.
  - [ ] Every route sets the document title.

**53: Guardrail lint and audit.**
- Touches: `web/eslint.config.js` (a `no-restricted-syntax` regex rule on `className` literals for the retired classes in 2.2), `web/e2e/mobile.spec.ts` (add the new routes plus a single-h1 check), `web/e2e/layout.spec.ts`, `ARCHITECTURE.md` (the UI routes section: a one-paragraph pointer to this spec and the token rule).
- Acceptance:
  - [ ] `npm run lint` fails on a sample `text-slate-500` (prove it in a throwaway commit, then revert).
  - [ ] The audit covers `/`, `/libraries`, `/libraries/new`, `/libraries/:id`, `/libraries/:id/edit`, `/libraries/:id/rules`, `/libraries/:id/flow`, `/nodes`, `/encode`, `/app` and `/integrations` at 320, 390, 414 and 768.
  - [ ] Zero raw palette classes remain outside `index.css` and the `App.tsx` mirror.

Order rationale:
- Tokens (36) and classes (37) change nothing structural but make every later diff small.
- Primitives (38, 39) come before the shell, because the shell uses Icon, Switch and Badge.
- The shell (40) comes before pages, because every page screenshot changes with it.
- Pages go from highest traffic (dashboard, libraries) to least, except 41 first: it fixes the most visible form problem and exercises FormSection early.
- 47 waits for the owner.
- 52 and 53 lock it in.

---

## 8. Guardrails

### 8.1 Do not touch
- **No Go changes, no API changes.** Everything here is computed from fields the UI already receives:
  - "Shrunk" from `status` plus `bytes_saved`;
  - "from N files" from the `/nodes` stats;
  - encoder words from `last_job_encoder`;
  - QSV presence from node `capabilities`.
  
  If a task finds it needs a new field, it stops and records the question in its notes (AGENTS rule 4 still applies, but this redesign should not need it).
- `DECISIONS.md` is read-only. `api.ts` request and response shapes are frozen. `ARCHITECTURE.md` changes only in task 53, and only the UI routes paragraph.

### 8.2 Tests
- Existing Vitest and Playwright suites stay green after every task.
- Assertions change only where this spec changes the asserted output, and each change is listed in the task notes (see section 7).
- Keep `data-testid`s. Where an element moves (Remove library, the pause buttons), its test id moves with it.
- The mobile audit (`e2e/mobile.spec.ts`) only grows.
- New primitives get unit tests that run without ffmpeg (rule 11).

### 8.3 The three-tier rule story (DECISIONS item 6, B3)
- The UI presents exactly three rule editors, Easy, Steps and Advanced, on one Rules page (5.8 and 5.9 share a header and the tier control). Each shows "In use" from `flow.source`.
- Lower tiers still save through their existing endpoints and generate the same flow format. Nothing in this redesign changes what any tier saves.
- The interim Edit page "Exact settings" panel (5.7, task 41) is a relabel, not a fourth tier. Task 47 removes it from Edit once the owner agrees.
- "Replace" warnings when switching tiers stay, as Banners.

### 8.4 Owner decisions this spec needs
1. **Edit page loses the rule controls (task 47).** It removes a second path to the same settings and moves its tests. Recommended: yes, because it is what DECISIONS item 6 describes. Until then, task 41's interim relabel ships.
2. **Remove library moves from the Libraries row to the Edit page's Danger zone (tasks 43 and 47).** Recommended: yes. Destructive actions should not sit next to "Scan now" on every row.
3. **Server preview text** ("Encode the video as HEVC… MKV file") appears on the easy screens. It comes from the server, so changing it is a Go change and out of scope. Flag only.

### 8.5 New dependency (AGENTS rule 3)
**`@fontsource-variable/inter`** is the only addition.
- **Why:** the app renders in whatever sans-serif the OS falls back to. On the Linux and NAS browsers this audience uses, that is DejaVu Sans (visible in every screenshot), whose wide metrics make the whole UI look unfinished. A pinned font makes layout and line breaks the same on every client.
- **Why this package:** it self-hosts the font inside the Vite bundle, so it is embedded in the Go binary and works offline. There is no CDN call, consistent with DECISIONS item 10's "no central hosted dependency" spirit.
- **License:** OFL-1.1, compatible with distributing alongside AGPL-3.0.
- **Cost:** about 48 KB woff2 (latin subset, variable weight).
- **No duplicate:** nothing on the stack provides fonts.
- Rejected alternatives: a Google Fonts link (an external request from a self-hosted home-lab tool, and broken offline); a system-font-only stack (it does not fix Linux).

Nothing else is added: no icon library (inline SVG through `Icon.tsx`), no component library (STACK: "No component library"), no class-merge utility (class strings are composed by hand, as today), and no axe dependency (the lint rule plus the existing Playwright audit cover the measurable parts).
