// SPDX-License-Identifier: AGPL-3.0-only

/**
 * StatCard tests (task 21): the testid renders, label and value are
 * present, the icon is an aria-hidden inline SVG, and both the icon and
 * the value paragraph scale via clamp(). The tone variant is checked
 * separately because it is the only class difference.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StatCard from './StatCard'

describe('StatCard', () => {
  it('renders label, value, an aria hidden icon and a fluid value size', () => {
    render(<StatCard testid="stat-files" label="Files" value="12" icon="M4 6h16" />)
    const card = screen.getByTestId('stat-files')
    expect(card).toHaveTextContent('Files')
    expect(card).toHaveTextContent('12')

    const svg = card.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg?.querySelector('path')?.getAttribute('d')).toBe('M4 6h16')
    // The icon box itself scales with the viewport.
    expect(svg?.getAttribute('class')).toContain('h-[clamp(')
    expect(svg?.getAttribute('class')).toContain('w-[clamp(')

    // The value paragraph (the bold one, not the label) carries the
    // fluid size.
    const value = card.querySelector('p.font-bold')
    expect(value?.className).toContain('text-[clamp(')
    expect(value?.className).toContain('text-slate-100')
    expect(value).toHaveTextContent('12')
  })

  it('tone good colours the value emerald', () => {
    render(
      <StatCard
        testid="stat-saved"
        label="Space saved"
        value="15 GB"
        icon="M4 17l6-6"
        tone="good"
      />,
    )
    const value = screen
      .getByTestId('stat-saved')
      .querySelector('p.font-bold')
    expect(value?.className).toContain('text-emerald-400')
    expect(value?.className).not.toContain('text-slate-100')
  })
})
