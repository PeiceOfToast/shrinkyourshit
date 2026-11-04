// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import {
  encoderLabel,
  formatBytes,
  formatCount,
  formatDateTime,
  formatDuration,
  formatEta,
  formatPercent,
  formatRelative,
  shortHash,
} from './format'

describe('formatBytes', () => {
  it('uses SI units with one decimal below 100 and none above', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(551_000)).toBe('551 KB')
    expect(formatBytes(15_185_350)).toBe('15.2 MB')
    expect(formatBytes(1.2e12)).toBe('1.2 TB')
  })
})

describe('formatCount', () => {
  it('groups thousands in en-US', () => {
    expect(formatCount(43655, 'en-US')).toBe('43,655')
  })
})

describe('formatRelative', () => {
  const now = Date.parse('2026-11-04T12:00:00Z')
  const ago = (ms: number) => new Date(now - ms).toISOString()

  it('reads seconds, minutes, hours and days', () => {
    expect(formatRelative(ago(5_000), now)).toBe('5 s ago')
    expect(formatRelative(ago(4 * 60_000), now)).toBe('4 min ago')
    expect(formatRelative(ago(3 * 3_600_000), now)).toBe('3 h ago')
    expect(formatRelative(ago(2 * 86_400_000), now)).toBe('2 d ago')
  })

  it('reads "never" for missing or unparseable input', () => {
    expect(formatRelative(undefined, now)).toBe('never')
    expect(formatRelative('not a date', now)).toBe('never')
  })

  it('does not go negative when the clock is behind the timestamp', () => {
    expect(formatRelative(new Date(now + 9_000).toISOString(), now)).toBe(
      '0 s ago',
    )
  })
})

describe('shortHash', () => {
  it('keeps the first seven characters', () => {
    expect(shortHash('0123456789abcdef')).toBe('0123456')
    expect(shortHash('abc')).toBe('abc')
  })
})

describe('encoderLabel', () => {
  it('maps encoder ids to plain words', () => {
    expect(encoderLabel('cpu')).toBe('CPU')
    expect(encoderLabel('qsv')).toBe('Intel')
    expect(encoderLabel('vaapi')).toBe('Intel')
    expect(encoderLabel('nvenc')).toBe('NVIDIA')
    expect(encoderLabel('videotoolbox')).toBe('Apple')
  })

  it('passes unknown ids through unchanged', () => {
    expect(encoderLabel('amf')).toBe('amf')
  })
})

describe('formatPercent', () => {
  it('uses a unicode minus for negatives and a space before the sign', () => {
    expect(formatPercent(-10)).toBe('−10 %')
    expect(formatPercent(0)).toBe('0 %')
    expect(formatPercent(9.6)).toBe('10 %')
  })
})

describe('formatDuration and formatEta', () => {
  it('are the same function and format durations', () => {
    expect(formatEta).toBe(formatDuration)
    expect(formatDuration(150)).toBe('2m 30s')
    expect(formatDuration(0)).toBe('')
  })
})

describe('formatDateTime', () => {
  it('is empty for missing or invalid input', () => {
    expect(formatDateTime(undefined)).toBe('')
    expect(formatDateTime('nope')).toBe('')
  })

  it('renders a valid timestamp as a non-empty locale string', () => {
    const out = formatDateTime('2026-11-04T12:00:00Z')
    expect(out).toContain('2026')
  })
})
