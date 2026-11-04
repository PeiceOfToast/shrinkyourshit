// SPDX-License-Identifier: AGPL-3.0-only

import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import Disclosure from './Disclosure'

describe('Disclosure', () => {
  it('is a native details with a focusable summary and 44px height', () => {
    const { container } = render(
      <Disclosure summary="What this does">
        <p>It shrinks files.</p>
      </Disclosure>,
    )
    const details = container.querySelector('details')!
    const summary = container.querySelector('summary')!
    expect(details).not.toHaveAttribute('open')
    expect(summary).toHaveClass('min-h-11')
    expect(summary.tabIndex).toBe(0)
    expect(summary).toHaveTextContent('What this does')
  })

  it('toggles open and closed when the summary is activated', () => {
    const { container } = render(
      <Disclosure summary="Advanced">
        <p>Hidden detail.</p>
      </Disclosure>,
    )
    const details = container.querySelector('details')!
    const summary = container.querySelector('summary')!
    fireEvent.click(summary)
    expect(details.open).toBe(true)
    fireEvent.click(summary)
    expect(details.open).toBe(false)
  })

  it('starts open when defaultOpen is set', () => {
    const { container } = render(
      <Disclosure summary="Advanced" defaultOpen testId="d">
        <p>Shown.</p>
      </Disclosure>,
    )
    expect(container.querySelector('details')!.open).toBe(true)
    expect(screen.getByTestId('d')).toBe(container.querySelector('details'))
  })
})
