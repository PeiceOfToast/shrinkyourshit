// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
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
