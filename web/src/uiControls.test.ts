// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import * as controls from './uiControls'
import { btn, btnGhost, btnPrimary, input } from './uiControls'

/** Raw palette names and `!important` are banned: colours come from tokens. */
const FORBIDDEN = ['slate-', 'emerald-', 'red-', 'rose-', '!']

/**
 * Controls that are touch targets. `link`, `card`, `cardPad` and `btnBlock` are
 * not. `btnIcon` is checked separately: the 2.10 string sizes it with `h-11 w-11`
 * (44 px) rather than `min-h-11`, so it is held to that instead.
 */
const TOUCH_TARGETS = [
  btnPrimary,
  btn,
  btnGhost,
  controls.btnDanger,
  controls.btnDangerGhost,
  input,
  controls.select,
  controls.textarea,
  controls.linkTouch,
]

describe('uiControls', () => {
  it('every control string carries the 44px touch minimum', () => {
    for (const cls of [btn, btnPrimary, btnGhost, input]) {
      expect(cls).toContain('min-h-11')
    }
    for (const cls of TOUCH_TARGETS) {
      expect(cls).toContain('min-h-11')
    }
    expect(controls.btnIcon).toContain('h-11 w-11')
  })

  it('inputs use the full width on small screens', () => {
    expect(input).toContain('w-full')
  })

  it('exports exactly the 2.10 names', () => {
    expect(Object.keys(controls).sort()).toEqual(
      [
        'btn',
        'btnBlock',
        'btnDanger',
        'btnDangerGhost',
        'btnGhost',
        'btnIcon',
        'btnPrimary',
        'card',
        'cardPad',
        'input',
        'link',
        'linkTouch',
        'select',
        'textarea',
      ].sort(),
    )
  })

  it('uses design tokens only: no raw palette names, no !important', () => {
    for (const [name, cls] of Object.entries(controls)) {
      for (const bad of FORBIDDEN) {
        expect(cls, `${name} contains ${bad}`).not.toContain(bad)
      }
    }
  })
})
