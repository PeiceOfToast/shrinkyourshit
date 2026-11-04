// SPDX-License-Identifier: AGPL-3.0-only

/**
 * StatCard tests (task 21, updated in task 39): the testid renders, label
 * and value are present, the icon is an aria-hidden inline SVG drawn from a
 * named Icon, and both the icon and the value paragraph scale via clamp().
 * The tone variant is checked separately because it is the only colour
 * difference. Task 39 adds the `sub` caption.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StatCard from './StatCard'
import { ICONS } from './Icon'

describe('StatCard', () => {
  it('renders label, value, an aria hidden icon and a fluid value size', () => {
    render(
      <StatCard testid="stat-files" label="Files" value="12" icon="library" />,
    )
    const card = screen.getByTestId('stat-files')
    expect(card).toHaveTextContent('Files')
    expect(card).toHaveTextContent('12')

    const svg = card.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg?.querySelector('path')?.getAttribute('d')).toBe(ICONS.library)
    // The icon box itself scales with the viewport.
    expect(svg?.getAttribute('class')).toContain('h-[clamp(')
    expect(svg?.getAttribute('class')).toContain('w-[clamp(')

    // The value paragraph (the semibold one, not the label) carries the
    // fluid size.
    const value = card.querySelector('p.font-semibold')
    expect(value?.className).toContain('text-[clamp(')
    expect(value?.className).toContain('text-fg')
    expect(value).toHaveTextContent('12')
  })

  it('tone good colours the value with the accent text token', () => {
    render(
      <StatCard
        testid="stat-saved"
        label="Space saved"
        value="15 GB"
        icon="encode"
        tone="good"
      />,
    )
    const value = screen
      .getByTestId('stat-saved')
      .querySelector('p.font-semibold')
    expect(value?.className).toContain('text-accent-text')
    expect(value?.className).not.toContain('text-fg')
  })

  it('tone warning and danger colour the value with their tokens', () => {
    const { rerender } = render(
      <StatCard
        testid="stat-w"
        label="Missing"
        value="2"
        icon="alert"
        tone="warning"
      />,
    )
    expect(
      screen.getByTestId('stat-w').querySelector('p.font-semibold')?.className,
    ).toContain('text-warning')
    rerender(
      <StatCard
        testid="stat-w"
        label="Failed"
        value="1"
        icon="alert"
        tone="danger"
      />,
    )
    expect(
      screen.getByTestId('stat-w').querySelector('p.font-semibold')?.className,
    ).toContain('text-danger')
  })

  it('renders the sub caption under the value when given', () => {
    render(
      <StatCard
        testid="stat-sub"
        label="Files"
        value="12"
        icon="library"
        sub="from 3 nodes"
      />,
    )
    expect(screen.getByTestId('stat-sub')).toHaveTextContent('from 3 nodes')
  })
})
