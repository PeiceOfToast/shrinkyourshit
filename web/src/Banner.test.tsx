// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import Banner from './Banner'

describe('Banner', () => {
  it('is a status region for info and success', () => {
    const { unmount } = render(<Banner tone="info">Saved here.</Banner>)
    expect(screen.getByRole('status')).toHaveTextContent('Saved here.')
    unmount()
    render(<Banner tone="success">Done.</Banner>)
    expect(screen.getByRole('status')).toHaveTextContent('Done.')
  })

  it('is an alert for danger', () => {
    render(<Banner tone="danger">Something failed.</Banner>)
    expect(screen.getByRole('alert')).toHaveTextContent('Something failed.')
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('uses the tone recipe and renders the title and action', () => {
    render(
      <Banner
        tone="warning"
        title="Encoding paused"
        action={<button type="button">Resume</button>}
        testId="b"
      >
        Nothing is being shrunk right now.
      </Banner>,
    )
    const root = screen.getByTestId('b')
    expect(root).toHaveClass('bg-warning/10', 'ring-warning/30', 'rounded-card')
    expect(screen.getByText('Encoding paused')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument()
  })
})
