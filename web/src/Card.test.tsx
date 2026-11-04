// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import Card from './Card'
import { card, cardPad } from './uiControls'

describe('Card', () => {
  it('is the card surface with a title, description and actions', () => {
    render(
      <Card
        title="Working now"
        description="Right now."
        actions={<a href="/nodes">View nodes</a>}
      >
        <p>body</p>
      </Card>,
    )
    expect(
      screen.getByRole('heading', { level: 2, name: 'Working now' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Right now.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'View nodes' })).toBeInTheDocument()
  })

  it('uses the card recipe and the default padding for the body', () => {
    render(
      <Card testId="c">
        <p>body</p>
      </Card>,
    )
    const root = screen.getByTestId('c')
    for (const cls of card.split(' ')) {
      expect(root).toHaveClass(cls)
    }
    expect(root).toHaveClass('flex', 'flex-col')
    expect(screen.getByText('body').parentElement).toHaveClass(
      ...cardPad.split(' '),
    )
  })

  it('drops the body padding for padding none, for a DataList or divided list', () => {
    render(
      <Card padding="none" testId="c">
        <ul>
          <li>row</li>
        </ul>
      </Card>,
    )
    const body = screen.getByText('row').closest('ul')!.parentElement!
    expect(body).not.toHaveClass('p-4')
    expect(body).not.toHaveClass('sm:p-6')
  })

  it('renders a footer with a top border', () => {
    render(
      <Card footer={<button type="button">Save changes</button>}>
        <p>body</p>
      </Card>,
    )
    const button = screen.getByRole('button', { name: 'Save changes' })
    expect(button.parentElement).toHaveClass(
      'border-t',
      'border-line',
      'px-4',
      'py-3',
      'sm:px-6',
    )
  })

  it('renders as a section when asked', () => {
    const { container } = render(
      <Card as="section" title="Rules">
        <p>body</p>
      </Card>,
    )
    expect(container.querySelector('section')).not.toBeNull()
  })
})
