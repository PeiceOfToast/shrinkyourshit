// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import PageHeader from './PageHeader'

function renderHeader(props: Parameters<typeof PageHeader>[0]) {
  return render(
    <MemoryRouter>
      <PageHeader {...props} />
    </MemoryRouter>,
  )
}

describe('PageHeader', () => {
  it('renders exactly one h1 with the title', () => {
    renderHeader({ title: 'Rules' })
    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
    expect(headings[0]).toHaveTextContent('Rules')
  })

  it('renders a breadcrumb nav of links, without the current page', () => {
    renderHeader({
      title: 'Rules',
      breadcrumb: [
        { label: 'Libraries', to: '/libraries', testId: 'crumb-libraries' },
        { label: 'Movies', to: '/libraries/1' },
      ],
    })
    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(nav.tagName).toBe('NAV')
    const links = within(nav).getAllByRole('link')
    expect(links.map((a) => a.textContent)).toEqual(['Libraries', 'Movies'])
    expect(links[0]).toHaveAttribute('href', '/libraries')
    expect(links[0]).toHaveAttribute('data-testid', 'crumb-libraries')
    expect(within(nav).queryByText('Rules')).toBeNull()
  })

  it('shows a skeleton in the h1 while the title is undefined, and keeps the h1', () => {
    renderHeader({ title: undefined })
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByTestId('page-header-skeleton')).toBeInTheDocument()
  })

  it('renders actions in the order given', () => {
    renderHeader({
      title: 'Libraries',
      actions: (
        <>
          <button type="button">First</button>
          <button type="button">Second</button>
          <a href="/libraries/new">Last</a>
        </>
      ),
    })
    const actions = screen.getByRole('button', { name: 'First' }).parentElement
    expect(actions).not.toBeNull()
    const order = Array.from(actions!.children).map((el) => el.textContent)
    expect(order).toEqual(['First', 'Second', 'Last'])
  })

  it('renders the subtitle and meta', () => {
    renderHeader({
      title: 'Movies',
      subtitle: 'How this library shrinks files.',
      meta: <span>Easy in use</span>,
    })
    expect(
      screen.getByText('How this library shrinks files.'),
    ).toBeInTheDocument()
    expect(screen.getByText('Easy in use')).toBeInTheDocument()
  })
})
