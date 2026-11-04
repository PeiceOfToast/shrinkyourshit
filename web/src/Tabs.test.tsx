// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Tabs tests (task 39): a tablist with one tab stop, aria-selected, and the
 * arrow, Home and End keys moving selection and focus together.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import Tabs from './Tabs'

const tabs = [
  { id: 'nodes', label: 'Nodes', testId: 'tab-nodes-t' },
  { id: 'hooks', label: 'Webhooks', testId: 'tab-hooks-t' },
  { id: 'servers', label: 'Servers', testId: 'tab-servers-t' },
]

function Harness() {
  const [value, setValue] = useState('nodes')
  return (
    <>
      <Tabs
        tabs={tabs}
        value={value}
        onChange={setValue}
        label="Integrations"
      />
      <p data-testid="panel">{value}</p>
    </>
  )
}

describe('Tabs', () => {
  it('marks the selected tab and gives it the only tab stop', () => {
    render(<Harness />)
    expect(screen.getByRole('tablist', { name: 'Integrations' })).toBeTruthy()
    const nodes = screen.getByTestId('tab-nodes-t')
    expect(nodes).toHaveAttribute('aria-selected', 'true')
    expect(nodes).toHaveAttribute('tabindex', '0')
    expect(screen.getByTestId('tab-hooks-t')).toHaveAttribute('tabindex', '-1')
  })

  it('moves selection and focus with the arrow keys, Home and End', () => {
    render(<Harness />)
    const nodes = screen.getByTestId('tab-nodes-t')
    nodes.focus()

    fireEvent.keyDown(nodes, { key: 'ArrowRight' })
    expect(screen.getByTestId('panel')).toHaveTextContent('hooks')
    expect(screen.getByTestId('tab-hooks-t')).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(document.activeElement).toBe(screen.getByTestId('tab-hooks-t'))

    fireEvent.keyDown(screen.getByTestId('tab-hooks-t'), { key: 'End' })
    expect(screen.getByTestId('panel')).toHaveTextContent('servers')

    fireEvent.keyDown(screen.getByTestId('tab-servers-t'), {
      key: 'ArrowRight',
    })
    expect(screen.getByTestId('panel')).toHaveTextContent('nodes')

    fireEvent.keyDown(screen.getByTestId('tab-nodes-t'), { key: 'ArrowLeft' })
    expect(screen.getByTestId('panel')).toHaveTextContent('servers')

    fireEvent.keyDown(screen.getByTestId('tab-servers-t'), { key: 'Home' })
    expect(screen.getByTestId('panel')).toHaveTextContent('nodes')
  })
})
