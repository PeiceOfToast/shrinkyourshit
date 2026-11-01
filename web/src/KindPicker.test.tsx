// SPDX-License-Identifier: AGPL-3.0-only

/**
 * KindPicker tests (task 34): plain-language labels, multi-select toggles,
 * and the at-least-one-kind guard.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import KindPicker, { kindLabel } from './KindPicker'

function renderPicker(value: string[], onChange = vi.fn()) {
  render(<KindPicker value={value} onChange={onChange} />)
  return onChange
}

describe('KindPicker', () => {
  it('shows the plain-language labels', () => {
    renderPicker(['video'])
    expect(screen.getByText('Video')).toBeInTheDocument()
    expect(screen.getByText('Audio')).toBeInTheDocument()
    expect(screen.getByText('Photos')).toBeInTheDocument()
    expect(screen.getByText('Subtitles')).toBeInTheDocument()
    expect(screen.getByText('Anything else')).toBeInTheDocument()
  })

  it('marks the selected kinds and toggles a kind on', () => {
    const onChange = renderPicker(['video'])
    expect(
      (screen.getByTestId('kind-video') as HTMLInputElement).checked,
    ).toBe(true)
    expect(
      (screen.getByTestId('kind-image') as HTMLInputElement).checked,
    ).toBe(false)
    fireEvent.click(screen.getByTestId('kind-image'))
    expect(onChange).toHaveBeenCalledWith(['video', 'image'])
  })

  it('removes a kind but never the last one', () => {
    const onChange = renderPicker(['video', 'image'])
    fireEvent.click(screen.getByTestId('kind-image'))
    expect(onChange).toHaveBeenCalledWith(['video'])
  })

  it('never removes the last kind', () => {
    const onChange = renderPicker(['video'])
    fireEvent.click(screen.getByTestId('kind-video'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('labels kinds in plain language', () => {
    expect(kindLabel('image')).toBe('Photos')
    expect(kindLabel('other')).toBe('Anything else')
    expect(kindLabel('unknown')).toBe('unknown')
  })
})
