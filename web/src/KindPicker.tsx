// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The file-kind picker (task 34, restyled in task 41): what a library looks
 * for, in plain language. Each kind is a Checkbox in a tile; the kind values
 * travel as the API's file_kinds strings.
 */
import Checkbox from './Checkbox'
import { KIND_CHOICES } from './kinds'

interface KindPickerProps {
  /** The selected kinds, in the library's stored order. */
  value: string[]
  onChange: (next: string[]) => void
  testIdPrefix?: string
}

export default function KindPicker({
  value,
  onChange,
  testIdPrefix = 'kind',
}: KindPickerProps) {
  const toggle = (kind: string) => {
    if (value.includes(kind)) {
      // A library must look for at least one kind (task 34): the last
      // checkbox stays put.
      if (value.length === 1) return
      onChange(value.filter((k) => k !== kind))
      return
    }
    onChange([...value, kind])
  }
  return (
    <fieldset className="grid gap-2" data-testid={`${testIdPrefix}-picker`}>
      <legend className="text-body font-medium text-fg">
        What should this library look for?
      </legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {KIND_CHOICES.map((c) => {
          const checked = value.includes(c.value)
          return (
            <div
              key={c.value}
              className={`rounded-control px-3 ring-1 ${
                checked ? 'bg-accent/5 ring-accent/60' : 'ring-line'
              }`}
            >
              <Checkbox
                checked={checked}
                onChange={() => toggle(c.value)}
                label={c.label}
                testId={`${testIdPrefix}-${c.value}`}
              />
            </div>
          )
        })}
      </div>
    </fieldset>
  )
}
