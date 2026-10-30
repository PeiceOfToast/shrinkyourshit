// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import { btn, btnGhost, btnPrimary, input } from './uiControls'

describe('uiControls', () => {
  it('every control string carries the 44px touch minimum', () => {
    for (const cls of [btn, btnPrimary, btnGhost, input]) {
      expect(cls).toContain('min-h-11')
    }
  })

  it('inputs use the full width on small screens', () => {
    expect(input).toContain('w-full')
  })
})
