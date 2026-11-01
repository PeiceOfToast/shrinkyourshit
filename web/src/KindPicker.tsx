// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The file-kind picker (task 34): what a library looks for, in plain
 * language. Multi-select checkboxes at the 44px touch minimum; no codec or
 * engine terms on the label (AGENTS.md rule 14). The kind values travel as
 * the API's file_kinds strings.
 */

/** The kinds the API knows, with their plain-language labels. */
export const KIND_CHOICES = [
  { value: 'video', label: 'Video' },
  { value: 'audio', label: 'Audio' },
  { value: 'image', label: 'Photos' },
  { value: 'subtitle', label: 'Subtitles' },
  { value: 'other', label: 'Anything else' },
] as const

/** The plain-language label of a kind; unknown kinds show as-is. */
export function kindLabel(kind: string): string {
  for (const c of KIND_CHOICES) {
    if (c.value === kind) return c.label
  }
  return kind
}

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
    <fieldset className="grid gap-1" data-testid={`${testIdPrefix}-picker`}>
      <legend className="text-sm font-medium text-slate-300">
        What should this library look for?
      </legend>
      <div className="mt-1 grid gap-1 sm:grid-cols-2 lg:grid-cols-3">
        {KIND_CHOICES.map((c) => (
          <label
            key={c.value}
            className="flex min-h-11 items-center gap-3 rounded-lg bg-slate-900 px-3 text-sm text-slate-300 ring-1 ring-slate-800"
          >
            <input
              type="checkbox"
              checked={value.includes(c.value)}
              onChange={() => toggle(c.value)}
              className="h-4 w-4 accent-emerald-500"
              aria-label={c.label}
              data-testid={`${testIdPrefix}-${c.value}`}
            />
            {c.label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}
