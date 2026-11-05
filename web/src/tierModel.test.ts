// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import { editableIn, isLocked, stepOf, tierLabel } from './tierModel'

const basicError = {
  path: 'nodes[2].type',
  message:
    'step "raw_args" (raw_args) is not representable in the basic tier; the advanced tier can edit it',
}

describe('tierModel', () => {
  it('names the step and the lowest tier that can edit it from the server message', () => {
    expect(stepOf(basicError)).toBe('raw_args')
    expect(editableIn(basicError)).toBe('advanced')
  })

  it('is empty for a graph-level error that names no step', () => {
    const graph = {
      path: 'graph',
      message: 'the basic tier skips files already in the target codec',
    }
    expect(stepOf(graph)).toBe('')
    expect(editableIn(graph)).toBe('')
  })

  it('locks an editor only when the list is not empty', () => {
    expect(isLocked(undefined)).toBe(false)
    expect(isLocked([])).toBe(false)
    expect(isLocked([basicError])).toBe(true)
  })

  it('labels each tier in plain words', () => {
    expect(tierLabel('basic')).toBe('Basic')
    expect(tierLabel('moderate')).toBe('Moderate')
    expect(tierLabel('advanced')).toBe('Advanced')
  })
})
