// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen's controls (task 07, restyled in task 41): a five-stop
 * quality slider, a four-stop audio slider and the remove-subtitles
 * checkbox. Plain language only — no technical terms on this screen.
 */
import type { CSSProperties } from 'react'
import Checkbox from './Checkbox'
import { AUDIO_STOPS, QUALITY_STOPS, type Stop } from './easyModel'

/**
 * A read-only folder path drawn like an input (task 41, spec 5.5 and 5.7).
 * The text colour (`text-fg`, or `text-fg-muted` for the placeholder) is
 * added by the caller, so the two states never carry both colours.
 */
export const pathField =
  'flex min-h-11 min-w-0 flex-1 items-center truncate rounded-control border border-line-strong bg-canvas px-3 font-mono text-body'

function StopSlider({
  stops,
  value,
  onChange,
  testid,
  label,
}: {
  stops: Stop[]
  value: number
  onChange: (value: number) => void
  testid: string
  label: string
}) {
  const current = stops.find((s) => s.value === value) ?? stops[2]
  const first = stops[0].value
  const last = stops[stops.length - 1].value
  const pct = last === first ? 0 : ((value - first) / (last - first)) * 100
  return (
    <div className="grid gap-2" data-testid={testid}>
      <span className="text-body font-medium text-fg">{label}</span>
      <input
        type="range"
        min={first}
        max={last}
        step={1}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="range"
        style={{ '--pct': `${pct}%` } as CSSProperties}
        aria-label={label}
        data-testid={`${testid}-slider`}
      />
      <div className="flex justify-between gap-1">
        {stops.map((s, i) => {
          const selected = s.value === value
          const edge = i === 0 || i === stops.length - 1
          // Under sm only the first, the selected and the last stop render.
          // The others keep their test ids but take no space: a 1 px sr-only
          // button would fail the 44 px touch audit, so they are display:none.
          const shown =
            edge || selected ? 'inline-flex' : 'hidden sm:inline-flex'
          return (
            <button
              key={s.value}
              type="button"
              onClick={() => onChange(s.value)}
              className={`min-h-11 items-center rounded-md px-1 text-caption transition-colors duration-fast ease-standard ${shown} ${
                selected
                  ? 'font-semibold text-fg'
                  : 'text-fg-muted hover:text-fg'
              }`}
              data-testid={`${testid}-stop-${s.value}`}
            >
              {s.label}
            </button>
          )
        })}
      </div>
      <p className="text-body text-fg-muted" data-testid={`${testid}-hint`}>
        <span className="font-semibold text-fg">{current.label}</span> —{' '}
        {current.hint}
      </p>
    </div>
  )
}

export function QualitySlider({
  value,
  onChange,
}: {
  value: number
  onChange: (value: number) => void
}) {
  return (
    <StopSlider
      stops={QUALITY_STOPS}
      value={value}
      onChange={onChange}
      testid="quality"
      label="How much space do you want to save?"
    />
  )
}

export function AudioSlider({
  value,
  onChange,
}: {
  value: number
  onChange: (value: number) => void
}) {
  return (
    <StopSlider
      stops={AUDIO_STOPS}
      value={value}
      onChange={onChange}
      testid="audio"
      label="Audio tracks"
    />
  )
}

export function RemoveSubtitlesCheckbox({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <div data-testid="subtitles">
      <Checkbox
        checked={checked}
        onChange={onChange}
        label="Remove subtitles"
        help="Deletes subtitle tracks from the files. The originals stay untouched until a smaller copy is ready."
        testId="subtitles-checkbox"
      />
    </div>
  )
}
