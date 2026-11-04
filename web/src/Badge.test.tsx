// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Badge tests (task 39): the neutral tone is the raised surface, a semantic
 * tone uses the tint recipe, and the dot is aria-hidden and pulses only when
 * asked.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import Badge from './Badge'

describe('Badge', () => {
  it('renders its children with the neutral surface by default', () => {
    render(<Badge testId="b">Easy in use</Badge>)
    const el = screen.getByTestId('b')
    expect(el).toHaveTextContent('Easy in use')
    expect(el).toHaveClass('rounded-full', 'text-caption', 'bg-raised')
  })

  it('uses the tint recipe for a semantic tone', () => {
    render(
      <Badge tone="success" testId="b">
        Done
      </Badge>,
    )
    expect(screen.getByTestId('b')).toHaveClass(
      'bg-success/10',
      'text-success',
      'ring-success/30',
    )
  })

  it('adds an aria-hidden dot, pulsing only when asked', () => {
    const { container, rerender } = render(
      <Badge tone="info" dot pulse testId="b">
        Working
      </Badge>,
    )
    const dot = container.querySelector('span[aria-hidden="true"]')
    expect(dot).toHaveClass('bg-info', 'animate-pulse')

    rerender(
      <Badge tone="info" dot testId="b">
        Working
      </Badge>,
    )
    expect(container.querySelector('span[aria-hidden="true"]')).not.toHaveClass(
      'animate-pulse',
    )
  })
})
