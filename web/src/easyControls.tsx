// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen's controls (task 07): a five-stop quality slider, a
 * four-stop audio slider and the remove-subtitles checkbox. Plain language
 * only — no technical terms on this screen.
 */
import { AUDIO_STOPS, QUALITY_STOPS, type Stop } from './easyModel'

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
  return (
    <div className="grid gap-2" data-testid={testid}>
      <span className="text-sm font-medium text-slate-300">{label}</span>
      <input
        type="range"
        min={stops[0].value}
        max={stops[stops.length - 1].value}
        step={1}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-emerald-500"
        aria-label={label}
        data-testid={`${testid}-slider`}
      />
      <div className="flex justify-between text-xs text-slate-500">
        {stops.map((s) => (
          <button
            key={s.value}
            type="button"
            onClick={() => onChange(s.value)}
            className={`rounded px-1 py-0.5 hover:text-slate-300 ${
              s.value === value ? 'font-semibold text-emerald-400' : ''
            }`}
            data-testid={`${testid}-stop-${s.value}`}
          >
            {s.label}
          </button>
        ))}
      </div>
      <p className="text-sm text-slate-400" data-testid={`${testid}-hint`}>
        <span className="font-semibold text-slate-200">{current.label}</span> —{' '}
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
    <label className="flex items-center gap-3" data-testid="subtitles">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4 accent-emerald-500"
        data-testid="subtitles-checkbox"
      />
      <span>
        <span className="text-sm font-medium text-slate-300">
          Remove subtitles
        </span>
        <span className="block text-xs text-slate-500">
          Deletes subtitle tracks from the files. The originals stay untouched
          until a smaller copy is ready.
        </span>
      </span>
    </label>
  )
}
