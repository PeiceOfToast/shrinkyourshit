// SPDX-License-Identifier: AGPL-3.0-only

import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { documentTitle, useDocumentTitle } from './useDocumentTitle'

describe('documentTitle', () => {
  it('joins the parts and appends the app name', () => {
    expect(documentTitle(['Rules', 'Movies'])).toBe(
      'Rules · Movies · ShrinkYourShit',
    )
  })

  it('skips empty and undefined parts', () => {
    expect(documentTitle([undefined, '', 'Dashboard'])).toBe(
      'Dashboard · ShrinkYourShit',
    )
    expect(documentTitle([])).toBe('ShrinkYourShit')
  })
})

describe('useDocumentTitle', () => {
  afterEach(() => {
    document.title = ''
  })

  it('sets document.title and follows changes to the parts', () => {
    const { rerender } = renderHook(
      ({ parts }: { parts: (string | undefined)[] }) => useDocumentTitle(parts),
      { initialProps: { parts: ['Rules', 'Movies'] } },
    )
    expect(document.title).toBe('Rules · Movies · ShrinkYourShit')
    rerender({ parts: ['Edit library'] })
    expect(document.title).toBe('Edit library · ShrinkYourShit')
  })
})
