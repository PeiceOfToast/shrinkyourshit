// SPDX-License-Identifier: AGPL-3.0-only

/**
 * An accessible on/off switch (task 18, restyled task 39): renders as a
 * role="switch" with aria-checked. The button keeps a 44 px hit area; the
 * visible track is 44x24 with a 20 px knob that slides on `duration-base`.
 * The state is conveyed by the track colour and the knob position, not by
 * an On/Off word inside the knob.
 *
 * Pass `visibleLabel` to show the name beside the switch. The name then
 * comes from that text (aria-labelledby) instead of `label`.
 */
import { useId } from 'react'

export default function Switch({
  checked,
  onChange,
  label,
  visibleLabel,
  testId,
  disabled = false,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  /** Accessible name, used when there is no `visibleLabel`. */
  label: string
  /** Text shown beside the switch. It becomes the accessible name. */
  visibleLabel?: string
  testId: string
  disabled?: boolean
}) {
  const labelId = useId()
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={visibleLabel ? undefined : label}
      aria-labelledby={visibleLabel ? labelId : undefined}
      data-testid={testId}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className="inline-flex h-11 flex-none items-center gap-3 rounded-full transition-colors duration-fast ease-standard disabled:cursor-not-allowed disabled:opacity-60"
    >
      <span
        aria-hidden="true"
        className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors duration-fast ease-standard ${
          checked ? 'bg-accent' : 'bg-raised ring-1 ring-inset ring-line-strong'
        }`}
      >
        <span
          className={`absolute left-0.5 h-5 w-5 rounded-full bg-fg transition-transform duration-base ease-standard ${
            checked ? 'translate-x-5' : 'translate-x-0'
          }`}
        />
      </span>
      {visibleLabel && (
        <span id={labelId} className="text-body text-fg">
          {visibleLabel}
        </span>
      )}
    </button>
  )
}
