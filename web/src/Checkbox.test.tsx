// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Checkbox tests (task 39). The input is the 44 px target (h-11 w-11) and
 * the visible box is 20 px (h-5 w-5). The test id goes on the input, so
 * click and checked assertions work as they do on a native checkbox.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Checkbox from './Checkbox'

describe('Checkbox', () => {
  it('has a 44 px input and a 20 px visible box', () => {
    const { container } = render(
      <Checkbox
        checked={false}
        onChange={() => {}}
        label="Remove subtitles"
        testId="cb"
      />,
    )
    const input = screen.getByTestId('cb')
    expect(input).toHaveAttribute('type', 'checkbox')
    expect(input).toHaveClass('h-11', 'w-11')
    expect(container.querySelector('span.h-5.w-5')).not.toBeNull()
  })

  it('reports the new state and is labelled by its text', () => {
    const onChange = vi.fn()
    render(
      <Checkbox
        checked={false}
        onChange={onChange}
        label="Watch for new files"
        help="Checks the folder every few minutes."
        testId="cb"
      />,
    )
    fireEvent.click(screen.getByTestId('cb'))
    expect(onChange).toHaveBeenCalledWith(true)
    expect(screen.getByLabelText('Watch for new files')).toBe(
      screen.getByTestId('cb'),
    )
    expect(screen.getByTestId('cb')).toHaveAccessibleDescription(
      'Checks the folder every few minutes.',
    )
  })
})
