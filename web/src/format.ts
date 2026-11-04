// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Number, date and label formatting shared by every screen (v1.9 UI redesign,
 * spec section 4.1 and section 6). Pages import from here instead of writing
 * their own `timeAgo` or `Intl` calls, so one rule decides how a value reads.
 */

/** Minus sign used for negative percentages (U+2212, not a hyphen). */
const MINUS = '−'

/** "1.5 GB" style byte counts, SI units (1000-based). */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let value = bytes
  let unit = 0
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000
    unit++
  }
  const digits = value >= 100 || unit === 0 ? 0 : 1
  return `${value.toFixed(digits)} ${units[unit]}`
}

/** "2m 30s" style duration for the UI. Empty for zero or invalid input. */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

/** Same as {@link formatDuration}; the name the easy screens already import. */
export const formatEta = formatDuration

/** Thousands-grouped counts: 43655 becomes "43,655". */
export function formatCount(n: number, locale?: string): string {
  return new Intl.NumberFormat(locale).format(n)
}

/**
 * Whole-number percentage with a unicode minus for negatives: -10 becomes
 * "−10 %", 0 becomes "0 %". Positive values carry no sign.
 */
export function formatPercent(percent: number): string {
  if (!Number.isFinite(percent)) return '0 %'
  const rounded = Math.round(percent)
  const sign = rounded < 0 ? MINUS : ''
  return `${sign}${Math.abs(rounded)} %`
}

/** Absolute date and short time, in the viewer's locale. Empty if unparseable. */
export function formatDateTime(iso: string | undefined): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(t))
}

/**
 * "23 s ago", "4 min ago", "3 h ago", "2 d ago". `now` is injectable so tests
 * do not depend on the clock. Missing or unparseable input reads "never".
 */
export function formatRelative(
  iso: string | undefined,
  now: number = Date.now(),
): string {
  if (!iso) return 'never'
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return 'never'
  const s = Math.max(0, Math.floor((now - then) / 1000))
  if (s < 60) return `${s} s ago`
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  return `${Math.floor(s / 86400)} d ago`
}

/** First seven characters of a hash or id, for display. The full value goes in `title`. */
export function shortHash(s: string): string {
  return s.slice(0, 7)
}

/** Plain-language names for encoder ids. Unknown ids pass through unchanged. */
const ENCODER_LABELS: Record<string, string> = {
  cpu: 'CPU',
  qsv: 'Intel',
  vaapi: 'Intel',
  nvenc: 'NVIDIA',
  videotoolbox: 'Apple',
}

export function encoderLabel(id: string): string {
  return ENCODER_LABELS[id] ?? id
}
