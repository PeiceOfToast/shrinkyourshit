// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Checkbox (task 39, spec 4.1): one checkbox style for every choice in the
 * app. The visible box is 20 px. The real input is 44 px square and sits
 * over the box with opacity 0, so the touch target meets the 44 px rule
 * without a large visible control. `data-testid` goes on the input, so
 * `click` and `toBeChecked` work the same as on a native checkbox.
 */
import { useId } from 'react'
import Icon from './Icon'

export default function Checkbox({
  checked,
  onChange,
  label,
  help,
  testId,
  disabled = false,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  label: string
  help?: string
  testId: string
  disabled?: boolean
}) {
  const helpId = useId()
  const labelId = useId()
  return (
    <label
      className={`flex min-h-11 items-start gap-3 py-2 ${
        disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
      }`}
    >
      <span className="relative mt-0.5 h-5 w-5 shrink-0">
        <input
          type="checkbox"
          data-testid={testId}
          checked={checked}
          disabled={disabled}
          aria-labelledby={labelId}
          aria-describedby={help ? helpId : undefined}
          onChange={(event) => onChange(event.target.checked)}
          className="peer absolute -inset-3 h-11 w-11 cursor-pointer appearance-none opacity-0 disabled:cursor-not-allowed"
        />
        <span
          aria-hidden="true"
          className="flex h-5 w-5 items-center justify-center rounded-md border border-line-strong bg-canvas transition-colors duration-fast ease-standard peer-checked:border-accent peer-checked:bg-accent peer-focus-visible:ring-2 peer-focus-visible:ring-focus peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-canvas [&>svg]:opacity-0 peer-checked:[&>svg]:opacity-100"
        >
          <Icon
            name="check"
            className="h-4 w-4 text-accent-fg transition-opacity duration-fast"
          />
        </span>
      </span>
      <span className="min-w-0">
        <span id={labelId} className="block text-body font-medium text-fg">
          {label}
        </span>
        {help && (
          <span id={helpId} className="mt-0.5 block text-caption text-fg-muted">
            {help}
          </span>
        )}
      </span>
    </label>
  )
}
