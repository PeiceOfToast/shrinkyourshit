// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Shared control classes. Spec: docs/design/ui-redesign-2026-11-04.md 2.10.
 * `min-h-11` is 44px, the Apple and WCAG touch minimum: every string keeps
 * it, so pages cannot ship a control that is too small to tap. Colours come
 * from the design tokens only (index.css `:root`, tailwind.config.js), never
 * raw palette names, and nothing here uses `!important`.
 */
const focus =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-canvas'
const base = `inline-flex min-h-11 items-center justify-center gap-2 rounded-control px-4 text-body font-medium transition-colors duration-fast ease-standard disabled:cursor-not-allowed disabled:opacity-60 ${focus}`

/** The one call to action on a screen. */
export const btnPrimary = `${base} bg-accent text-accent-fg hover:bg-accent-hover`
/** Secondary action: a filled surface, not the accent. */
export const btn = `${base} bg-raised text-fg hover:bg-raised-hover`
/** A quiet action: no filled background. */
export const btnGhost = `${base} text-fg-muted hover:bg-raised hover:text-fg`
/** A destructive confirm, such as "Revoke node" in a dialog. */
export const btnDanger = `${base} bg-danger-solid text-white hover:bg-danger-solid-hover`
/** A destructive quiet action, such as a row's "Remove". */
export const btnDangerGhost = `${base} text-danger hover:bg-danger/10`
/** Square 44 px icon button. */
export const btnIcon = `${focus} inline-flex h-11 w-11 items-center justify-center rounded-control text-fg-muted transition-colors duration-fast hover:bg-raised hover:text-fg`
/** Modifier: stretch a button to the full width of its container. */
export const btnBlock = 'w-full'

/** Text, number, select and textarea fields. */
export const input = `min-h-11 w-full rounded-control border border-line-strong bg-canvas px-3 text-body text-fg placeholder:text-fg-muted transition-colors duration-fast focus-visible:border-focus disabled:opacity-60`
/** A select: room on the right for the native chevron. */
export const select = `${input} pr-9`
/** A multi-line field. */
export const textarea = `${input} py-2 leading-5`

/** Inline text link. */
export const link = 'text-accent-text underline-offset-4 hover:underline'
/** A link inside `main` on mobile, with the 44 px target height. */
export const linkTouch = `${link} inline-flex min-h-11 items-center`

/** A surface card with a hairline ring instead of a border. */
export const card = 'rounded-card bg-surface ring-1 ring-line'
/** Card padding: tighter on phones, roomier from 640 px up. */
export const cardPad = 'p-4 sm:p-6'
