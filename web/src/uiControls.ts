// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Shared control classes (task 20). `min-h-11` is 44px, the Apple and WCAG
 * touch minimum. Pages import these instead of hand writing button and
 * input classes, so one change keeps every control tappable.
 */
const base =
  'inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-3 text-sm font-medium transition-colors disabled:opacity-50'

/** The default action button. */
export const btn = `${base} bg-slate-800 text-slate-100 hover:bg-slate-700`

/** The one call to action on a screen. */
export const btnPrimary = `${base} bg-emerald-600 text-white hover:bg-emerald-500`

/** A quiet action: no filled background. */
export const btnGhost = `${base} text-slate-300 hover:bg-slate-800 hover:text-white`

/** Text, number, select and textarea fields. Full width by default. */
export const input =
  'min-h-11 w-full rounded-lg border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100'
