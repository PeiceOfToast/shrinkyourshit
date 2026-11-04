// SPDX-License-Identifier: AGPL-3.0-only

/**
 * SegmentedControl tests (task 39): button options keep aria-pressed, and
 * an option with `to` is a link with aria-current on the selected one.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import SegmentedControl from './SegmentedControl'

describe('SegmentedControl', () => {
  it('keeps aria-pressed on button options and reports the click', () => {
    const onChange = vi.fn()
    render(
      <SegmentedControl
        label="File filter"
        value="all"
        onChange={onChange}
        options={[
          { id: 'all', label: 'All', testId: 'seg-all' },
          { id: 'failed', label: 'Failed', testId: 'seg-failed' },
        ]}
      />,
    )
    expect(screen.getByTestId('seg-all')).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByTestId('seg-failed')).toHaveAttribute(
      'aria-pressed',
      'false',
    )
    fireEvent.click(screen.getByTestId('seg-failed'))
    expect(onChange).toHaveBeenCalledWith('failed')
  })

  it('renders options with a route as links, with aria-current on the current one', () => {
    render(
      <MemoryRouter>
        <SegmentedControl
          label="Rules tier"
          value="easy"
          options={[
            {
              id: 'easy',
              label: 'Easy',
              to: '/libraries/1/rules',
              testId: 'tier-easy',
            },
            {
              id: 'steps',
              label: 'Steps',
              to: '/libraries/1/steps',
              testId: 'tier-steps',
            },
          ]}
        />
      </MemoryRouter>,
    )
    const easy = screen.getByTestId('tier-easy')
    expect(easy.tagName).toBe('A')
    expect(easy).toHaveAttribute('href', '/libraries/1/rules')
    expect(easy).toHaveAttribute('aria-current', 'page')
    expect(screen.getByTestId('tier-steps')).not.toHaveAttribute('aria-current')
  })
})
