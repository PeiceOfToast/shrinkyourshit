// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from 'vitest'
import {
  AUDIO_STOPS,
  QUALITY_STOPS,
  audioHint,
  audioLabel,
  formatBytes,
  formatEta,
  parseEasy,
  qualityHint,
  qualityLabel,
} from './easyModel'

describe('easy screen labels', () => {
  it('names the five quality stops exactly as specified', () => {
    expect(QUALITY_STOPS.map((s) => s.label)).toEqual([
      'Smallest files',
      'Smaller',
      'Balanced',
      'Higher quality',
      'Best quality',
    ])
    expect(qualityLabel(3)).toBe('Balanced')
    expect(qualityLabel(5)).toBe('Best quality')
    expect(qualityHint(1)).toContain('most space')
  })

  it('names the four audio stops exactly as specified', () => {
    expect(AUDIO_STOPS.map((s) => s.label)).toEqual([
      'Keep everything',
      'Keep my languages',
      'Keep the best track',
      'Keep the best track, make it small',
    ])
    expect(audioLabel(2)).toBe('Keep my languages')
    expect(audioLabel(4)).toBe('Keep the best track, make it small')
    expect(audioHint(4)).toContain('smaller size')
  })
})

describe('parseEasy', () => {
  it('uses balanced defaults when the library has no easy state', () => {
    expect(parseEasy(null)).toEqual({
      quality: 3,
      remove_subtitles: false,
      audio: 1,
    })
  })

  it('keeps stored values and clamps out-of-range ones', () => {
    expect(parseEasy({ quality: 5, remove_subtitles: true, audio: 4 })).toEqual(
      {
        quality: 5,
        remove_subtitles: true,
        audio: 4,
      },
    )
    expect(parseEasy({ quality: 9, remove_subtitles: true, audio: 7 })).toEqual(
      {
        quality: 3,
        remove_subtitles: true,
        audio: 1,
      },
    )
  })
})

describe('formatting helpers', () => {
  it('formats byte counts for humans', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(999)).toBe('999 B')
    expect(formatBytes(1500)).toBe('1.5 KB')
    expect(formatBytes(3_400_000_000)).toBe('3.4 GB')
  })

  it('formats ETAs for humans', () => {
    expect(formatEta(0)).toBe('')
    expect(formatEta(45)).toBe('45s')
    expect(formatEta(150)).toBe('2m 30s')
    expect(formatEta(3600)).toBe('1h 0m')
  })
})
