// SPDX-License-Identifier: AGPL-3.0-only

/**
 * StatusChip tests (task 39): a zero count renders nothing (spec 2.1, hide
 * zero), a satisfied file reads "Shrunk" only when bytes were saved, and a
 * working file carries the pulsing dot.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StatusChip from './StatusChip'

describe('StatusChip', () => {
  it('renders null for a zero count', () => {
    const { container } = render(
      <StatusChip status="done" count={0} testId="chip" />,
    )
    expect(container).toBeEmptyDOMElement()
    expect(screen.queryByTestId('chip')).toBeNull()
  })

  it('reads satisfied as Shrunk when bytes were saved, and Nothing to do otherwise', () => {
    const { rerender } = render(
      <StatusChip
        status="satisfied"
        count={2}
        bytesSaved={1024}
        testId="chip"
      />,
    )
    expect(screen.getByTestId('chip')).toHaveTextContent('2 Shrunk')
    expect(screen.getByTestId('chip')).toHaveClass('bg-success/10')

    rerender(
      <StatusChip status="satisfied" count={2} bytesSaved={0} testId="chip" />,
    )
    expect(screen.getByTestId('chip')).toHaveTextContent('2 Nothing to do')
    expect(screen.getByTestId('chip')).toHaveClass('bg-raised')
  })

  it('gives a processing count a pulsing dot', () => {
    const { container } = render(
      <StatusChip status="processing" count={1} testId="chip" />,
    )
    expect(screen.getByTestId('chip')).toHaveTextContent('1 Working')
    expect(container.querySelector('.animate-pulse')).not.toBeNull()
  })
})
