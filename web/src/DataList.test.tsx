// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import DataList, { type Column } from './DataList'

type Row = { id: string; name: string; saved: string }

const columns: Column<Row>[] = [
  { key: 'name', header: 'Library', cell: (r) => r.name, primary: true },
  { key: 'saved', header: 'Saved', cell: (r) => r.saved },
]

describe('DataList', () => {
  it('renders a semantic table for assistive tech and a card list for eyes', () => {
    render(
      <DataList
        rows={[{ id: '1', name: 'Movies', saved: '14 GB' }]}
        columns={columns}
        testid="libs"
      />,
    )
    const table = screen.getByRole('table')
    expect(table).toBeInTheDocument()
    expect(table).toHaveClass('w-full')
    expect(table.parentElement).toHaveClass('overflow-x-auto')
    expect(screen.getByTestId('libs')).toBe(table)
    expect(screen.getByTestId('libs-cards').tagName).toBe('UL')
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
    // Both trees carry the row, so CSS alone decides which is visible.
    expect(screen.getAllByText('Movies')).toHaveLength(2)
    expect(screen.getAllByText('14 GB')).toHaveLength(2)
  })
})

describe('DataList layouts (task 39)', () => {
  type Lib = { id: string; name: string; files: number; saved: string }
  const libs: Lib[] = [{ id: 'a', name: 'Movies', files: 1204, saved: '14 GB' }]
  const cols: Column<Lib>[] = [
    { key: 'name', header: 'Library', cell: (r) => r.name, primary: true },
    { key: 'files', header: 'Files', cell: (r) => r.files, align: 'end' },
    {
      key: 'status',
      header: 'Status',
      cell: () => <span>Up to date</span>,
      cardPlacement: 'badge',
    },
    {
      key: 'actions',
      header: '',
      cell: () => <button type="button">Scan now</button>,
      actions: true,
    },
  ]

  it('switches from cards to table at the cardsBelow breakpoint', () => {
    render(
      <DataList rows={libs} columns={cols} testid="files" cardsBelow="md" />,
    )
    expect(screen.getByTestId('files-cards').className).toContain('md:hidden')
    const table = screen.getByTestId('files')
    expect(table.parentElement?.className).toContain('hidden md:block')
  })

  it('defaults the breakpoint to sm', () => {
    render(<DataList rows={libs} columns={cols} testid="files" />)
    expect(screen.getByTestId('files-cards').className).toContain('sm:hidden')
    expect(screen.getByTestId('files').parentElement?.className).toContain(
      'hidden sm:block',
    )
  })

  it('makes the row a link to rowHref and keeps action buttons above it', () => {
    render(
      <MemoryRouter>
        <DataList
          rows={libs}
          columns={cols}
          testid="files"
          rowHref={(r) => `/libraries/${r.id}`}
        />
      </MemoryRouter>,
    )
    const link = within(screen.getByTestId('files')).getByRole('link', {
      name: 'Movies',
    })
    expect(link).toHaveAttribute('href', '/libraries/a')
    expect(link).toHaveClass('after:absolute', 'after:inset-0')
    const button = within(screen.getByTestId('files')).getByRole('button', {
      name: 'Scan now',
    })
    expect(button.parentElement).toHaveClass('relative', 'z-10')
  })

  it('shows the empty node in place of both layouts when there are no rows', () => {
    render(
      <DataList
        rows={[]}
        columns={cols}
        testid="files"
        empty={<p>No libraries yet</p>}
      />,
    )
    expect(screen.getByText('No libraries yet')).toBeInTheDocument()
    expect(screen.queryByTestId('files')).toBeNull()
    expect(screen.queryByTestId('files-cards')).toBeNull()
  })

  it('right-aligns numeric columns and puts badges top right in cards', () => {
    render(<DataList rows={libs} columns={cols} testid="files" />)
    const numberCell = screen
      .getByTestId('files')
      .querySelectorAll('tbody td')[1]
    expect(numberCell).toHaveClass('text-right', 'tabular-nums')
    const card = screen.getByTestId('files-cards').querySelector('li')
    // The badge sits in the top row, not in the dl.
    expect(card?.querySelector('.flex.items-start')).toHaveTextContent(
      'Up to date',
    )
    expect(card?.querySelector('dl')).not.toHaveTextContent('Up to date')
  })
})
