// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import EmptyState from './EmptyState'

describe('EmptyState', () => {
  it('renders the title, body and action', () => {
    render(
      <EmptyState
        icon="library"
        title="No libraries yet"
        body="A library is a folder ShrinkYourShit watches."
        action={<a href="/libraries/new">Add library</a>}
      />,
    )
    expect(
      screen.getByRole('heading', { level: 2, name: 'No libraries yet' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText('A library is a folder ShrinkYourShit watches.'),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add library' })).toHaveAttribute(
      'href',
      '/libraries/new',
    )
  })

  it('renders with only a title', () => {
    render(<EmptyState title="Nothing here yet" testId="empty" />)
    const root = screen.getByTestId('empty')
    expect(root).toHaveTextContent('Nothing here yet')
    expect(root.querySelector('svg')).toBeNull()
  })
})
