// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Switch tests (task 39): role=switch with aria-checked, no On/Off text in
 * the knob, and an optional visible label that becomes the accessible name.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Switch from './Switch'

describe('Switch', () => {
  it('keeps role=switch and aria-checked and has no On or Off text', () => {
    const onChange = vi.fn()
    render(
      <Switch
        checked={true}
        onChange={onChange}
        label="Pause all"
        testId="sw"
      />,
    )
    const sw = screen.getByTestId('sw')
    expect(sw).toHaveAttribute('role', 'switch')
    expect(sw).toHaveAttribute('aria-checked', 'true')
    expect(sw).toHaveAccessibleName('Pause all')
    expect(screen.queryByText(/^(On|Off)$/)).toBeNull()
    fireEvent.click(sw)
    expect(onChange).toHaveBeenCalledWith(false)
  })

  it('names itself from the visible label when one is given', () => {
    render(
      <Switch
        checked={false}
        onChange={() => {}}
        label="ignored"
        visibleLabel="Check for updates"
        testId="sw"
      />,
    )
    const sw = screen.getByTestId('sw')
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(sw).not.toHaveAttribute('aria-label')
    expect(sw).toHaveAccessibleName('Check for updates')
    expect(screen.queryByText(/^(On|Off)$/)).toBeNull()
  })
})
