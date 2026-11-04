// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Easy-screen controls (task 41): under `sm` only the first, the selected and
 * the last stop label show, every stop keeps its test id, the slider reports
 * its position through --pct, and the subtitles box is the shared Checkbox.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'

function Quality({ start = 3 }: { start?: number }) {
  const [v, setV] = useState(start)
  return <QualitySlider value={v} onChange={setV} />
}

describe('stop slider (task 41)', () => {
  it('shows only the first, selected and last stop under sm', () => {
    render(<Quality start={3} />)
    const shown = (n: number) =>
      screen.getByTestId(`quality-stop-${n}`).className
    expect(shown(1)).toContain('inline-flex')
    expect(shown(1)).not.toContain('hidden')
    expect(shown(5)).not.toContain('hidden')
    // Selected (3) is always visible; the others hide below sm.
    expect(shown(3)).not.toContain('hidden')
    expect(shown(2)).toContain('hidden sm:inline-flex')
    expect(shown(4)).toContain('hidden sm:inline-flex')
  })

  it('keeps every stop in the DOM with its test id and still clickable', () => {
    render(<Quality start={3} />)
    for (const n of [1, 2, 3, 4, 5]) {
      expect(screen.getByTestId(`quality-stop-${n}`)).toBeInTheDocument()
    }
    fireEvent.click(screen.getByTestId('quality-stop-4'))
    expect(screen.getByTestId('quality-hint')).toHaveTextContent(
      'Higher quality',
    )
  })

  it('drives the filled track through --pct from the value', () => {
    render(<Quality start={3} />)
    // Quality runs 1..5, so 3 sits at 50 %.
    expect(
      screen.getByTestId('quality-slider').style.getPropertyValue('--pct'),
    ).toBe('50%')
    fireEvent.change(screen.getByTestId('quality-slider'), {
      target: { value: '5' },
    })
    expect(
      screen.getByTestId('quality-slider').style.getPropertyValue('--pct'),
    ).toBe('100%')
  })

  it('uses the range class so the 44 px input carries the drawn track', () => {
    render(<AudioSlider value={1} onChange={() => {}} />)
    expect(screen.getByTestId('audio-slider').className).toContain('range')
  })
})

describe('RemoveSubtitlesCheckbox (task 41)', () => {
  it('is the shared Checkbox: a 44 px label around a 20 px box, test id on the input', () => {
    render(<RemoveSubtitlesCheckbox checked={false} onChange={() => {}} />)
    const input = screen.getByTestId('subtitles-checkbox')
    expect(input).toHaveAttribute('type', 'checkbox')
    expect(input.closest('label')!.className).toContain('min-h-11')
    expect(input.className).toContain('peer')
    expect(screen.getByTestId('subtitles')).toContainElement(input)
  })
})
