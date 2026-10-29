// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen must stay plain-language (AGENTS.md rule 14, task 07
 * acceptance): no codec names or encoder terms anywhere in its source.
 * Codec names are allowed only in the Advanced panel and its tests. The
 * sources are loaded raw with Vite's glob so the test truly greps the
 * shipped code, comments included.
 */
import { describe, expect, it } from 'vitest'

const sources = import.meta.glob<string>(
  './{EasyLibraryForm.tsx,easyControls.tsx,easyModel.ts,FolderPickerModal.tsx}',
  { query: '?raw', import: 'default', eager: true },
)

const BANNED = /\b(hevc|h265|h264|av1|crf|ffmpeg)\b/i

describe('easy screen plain-language rule', () => {
  it('never mentions codecs or encoder terms', () => {
    expect(Object.keys(sources)).toHaveLength(4)
    for (const [name, source] of Object.entries(sources)) {
      expect(source, `${name} mentions a codec or encoder term`).not.toMatch(
        BANNED,
      )
    }
  })

  it('uses the required slider labels verbatim', () => {
    const model = sources['./easyModel.ts']
    for (const label of [
      'Smallest files',
      'Smaller',
      'Balanced',
      'Higher quality',
      'Best quality',
      'Keep everything',
      'Keep my languages',
      'Keep the best track',
      'Keep the best track, make it small',
    ]) {
      expect(model, `missing label ${label}`).toContain(label)
    }
  })
})
