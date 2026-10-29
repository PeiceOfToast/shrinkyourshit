// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Easy-screen model (task 07): the five quality stops and four audio stops
 * with their plain-language labels and one-line hints. Codec names and
 * encoder terms are deliberately absent from the easy screen (AGENTS.md
 * rule 14); they only appear in the library editor's Advanced panel.
 */
import type { EasyInput, Library } from './api'

export interface Stop {
  value: number
  label: string
  hint: string
}

/** Quality slider stops, 1 saves the most space, 5 the least. */
export const QUALITY_STOPS: Stop[] = [
  {
    value: 1,
    label: 'Smallest files',
    hint: 'Saves the most space. Quality visibly drops, but it still plays fine.',
  },
  {
    value: 2,
    label: 'Smaller',
    hint: 'Big space savings with only a small quality trade-off.',
  },
  {
    value: 3,
    label: 'Balanced',
    hint: 'Recommended. Good savings with hardly any visible difference.',
  },
  {
    value: 4,
    label: 'Higher quality',
    hint: 'Smaller savings while keeping the picture very close to the original.',
  },
  {
    value: 5,
    label: 'Best quality',
    hint: 'Gentle shrinking. The result looks like the original.',
  },
]

/** Audio slider stops (ARCHITECTURE.md easy-mode mapping). */
export const AUDIO_STOPS: Stop[] = [
  {
    value: 1,
    label: 'Keep everything',
    hint: 'Every audio track stays exactly as it is.',
  },
  {
    value: 2,
    label: 'Keep my languages',
    hint: 'Keep only tracks in your preferred languages (set in Settings).',
  },
  {
    value: 3,
    label: 'Keep the best track',
    hint: 'Keep one track: your favourite language with the most channels.',
  },
  {
    value: 4,
    label: 'Keep the best track, make it small',
    hint: 'Keep one track and convert it to a smaller size.',
  },
]

function stopLabel(stops: Stop[], value: number, fallback: string): string {
  return stops.find((s) => s.value === value)?.label ?? fallback
}

function stopHint(stops: Stop[], value: number, fallback: string): string {
  return stops.find((s) => s.value === value)?.hint ?? fallback
}

export function qualityLabel(value: number): string {
  return stopLabel(QUALITY_STOPS, value, 'Balanced')
}

export function qualityHint(value: number): string {
  return stopHint(QUALITY_STOPS, value, QUALITY_STOPS[2].hint)
}

export function audioLabel(value: number): string {
  return stopLabel(AUDIO_STOPS, value, 'Keep everything')
}

export function audioHint(value: number): string {
  return stopHint(AUDIO_STOPS, value, AUDIO_STOPS[0].hint)
}

/** The easy state of a library, or null when it has none. */
export function easyOf(lib: Library): EasyInput | null {
  return lib.easy
}

/** The easy-screen form state (task 07). */
export interface EasyFormValue {
  path: string
  quality: number
  remove_subtitles: boolean
  audio: number
  watch: boolean
}

/** Defaults for a new library: balanced quality, watch on. */
export const initialEasyValue: EasyFormValue = {
  path: '',
  quality: 3,
  remove_subtitles: false,
  audio: 1,
  watch: true,
}

/** The easy object this form state compiles to (what the API receives). */
export function toEasyInput(v: EasyFormValue): EasyInput {
  return {
    quality: v.quality,
    remove_subtitles: v.remove_subtitles,
    audio: v.audio,
  }
}

/** Splits an absolute path into clickable breadcrumb segments. */
export function crumbs(path: string): Array<{ name: string; path: string }> {
  const parts = path.split('/').filter(Boolean)
  const out: Array<{ name: string; path: string }> = [
    { name: 'Root', path: '/' },
  ]
  let acc = ''
  for (const part of parts) {
    acc += `/${part}`
    out.push({ name: part, path: acc })
  }
  return out
}

/** Parses a stored easy object back into slider state, with defaults. */
export function parseEasy(raw: EasyInput | null): EasyInput {
  if (!raw) {
    return { quality: 3, remove_subtitles: false, audio: 1 }
  }
  return {
    quality: raw.quality >= 1 && raw.quality <= 5 ? raw.quality : 3,
    remove_subtitles: Boolean(raw.remove_subtitles),
    audio: raw.audio >= 1 && raw.audio <= 4 ? raw.audio : 1,
  }
}

export const STATUS_LABELS: Record<string, string> = {
  new: 'New',
  satisfied: 'Nothing to do',
  queued: 'Waiting',
  processing: 'Working',
  done: 'Done',
  failed: 'Failed',
  missing: 'Missing',
}

export function statusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status
}

/** "1.5 GB" style byte counts for the UI. */
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

/** "2m 30s" style duration for the UI. */
export function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}
