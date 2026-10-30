// SPDX-License-Identifier: AGPL-3.0-only

/**
 * An accessible on/off switch (task 18): renders as a pill with a sliding
 * knob, announces itself with role="switch" and aria-checked (not a
 * checkbox), and keeps a 44px touch target (the UI review's touch rule).
 */
export default function Switch({
  checked,
  onChange,
  label,
  testId,
  disabled = false,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  /** Accessible name, announced instead of the visual state. */
  label: string
  testId: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      data-testid={testId}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-11 w-20 flex-none items-center rounded-full px-1 transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-400 disabled:cursor-not-allowed disabled:opacity-50 ${
        checked ? 'bg-emerald-600' : 'bg-slate-700'
      }`}
    >
      <span
        aria-hidden="true"
        className={`h-9 w-9 rounded-full bg-white shadow transition-transform ${
          checked ? 'translate-x-9' : 'translate-x-0'
        }`}
      />
      <span
        aria-hidden="true"
        className={`absolute inset-y-0 flex w-1/2 items-center justify-center text-[10px] font-bold uppercase tracking-wide ${
          checked ? 'left-0 text-white/80' : 'right-0 text-slate-400'
        }`}
      >
        {checked ? 'On' : 'Off'}
      </span>
    </button>
  )
}
