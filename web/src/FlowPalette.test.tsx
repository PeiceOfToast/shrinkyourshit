// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The step palette (task 15) and the plugin step (task 32): with no
 * enabled step plugin the "Run a plugin" entry explains how to install
 * one and links to the plugins tab; once one exists the entry adds the
 * step as usual.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import FlowPalette from './FlowPalette'
import type { StepTypeInfo } from './api'

function pluginType(options: string[]): StepTypeInfo {
  return {
    type: 'plugin',
    label: 'Run a plugin',
    description:
      'Run an installed step plugin on the working copy, at this position in the chain. A later condition sees what it did.',
    condition: false,
    params: [
      {
        name: 'plugin',
        label: 'plugin',
        kind: 'plugin',
        options,
        required: true,
      },
      {
        name: 'continue_on_error',
        label: 'continue on error',
        kind: 'toggle',
        help: 'For logging and sidecar style plugins only: a failing step is a warning and the chain continues.',
      },
    ],
  }
}

describe('FlowPalette plugin step (task 32)', () => {
  it('shows the install hint instead of the entry while no step plugin exists', () => {
    const onAdd = vi.fn()
    render(<FlowPalette palette={[pluginType([])]} onAdd={onAdd} />)
    expect(screen.getByTestId('palette-plugin-hint')).toBeTruthy()
    expect(screen.getByTestId('palette-plugin-hint').textContent).toContain(
      'plugins tab',
    )
    expect(screen.queryByTestId('palette-add-plugin')).toBeNull()
  })

  it('offers the entry once a step plugin is enabled', () => {
    const onAdd = vi.fn()
    render(
      <FlowPalette palette={[pluginType(['remove-chapters'])]} onAdd={onAdd} />,
    )
    expect(screen.getByTestId('palette-add-plugin')).toBeTruthy()
    fireEvent.click(screen.getByTestId('palette-add-plugin'))
    expect(onAdd).toHaveBeenCalledWith('plugin')
  })
})
