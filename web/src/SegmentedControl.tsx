// SPDX-License-Identifier: AGPL-3.0-only

/**
 * SegmentedControl (task 39, spec 4.1): a row of options where one is
 * current. Two kinds of option:
 *
 * - With `onChange`, each option is a button with `aria-pressed`. This is
 *   the in-page choice (the file filter, the log level).
 * - An option with `to` is a router link with `aria-current="page"`. This
 *   is a choice between routes (the rules tiers, which are three pages).
 *
 * The selected segment is neutral (raised surface, fg text), per spec 2.1,
 * so the tier control does not use the accent fill.
 */
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

export type SegmentOption = {
  id: string
  label: string
  testId?: string
  /** When set, the option is a link to this route. */
  to?: string
  /** Shown after the label, such as a count. */
  badge?: ReactNode
}

export default function SegmentedControl({
  options,
  value,
  onChange,
  label,
}: {
  options: SegmentOption[]
  value: string
  onChange?: (id: string) => void
  /** Accessible name of the group. */
  label: string
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="inline-flex max-w-full flex-wrap gap-1 rounded-control bg-canvas p-1 ring-1 ring-line"
    >
      {options.map((option) => {
        const selected = option.id === value
        const classes = `inline-flex min-h-11 items-center gap-2 rounded-md px-4 text-body font-medium transition-colors duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus ${
          selected ? 'bg-raised text-fg' : 'text-fg-muted hover:text-fg'
        }`
        const content = (
          <>
            {option.label}
            {option.badge}
          </>
        )
        if (option.to !== undefined) {
          return (
            <Link
              key={option.id}
              to={option.to}
              data-testid={option.testId}
              aria-current={selected ? 'page' : undefined}
              className={classes}
            >
              {content}
            </Link>
          )
        }
        return (
          <button
            key={option.id}
            type="button"
            data-testid={option.testId}
            aria-pressed={selected}
            onClick={() => onChange?.(option.id)}
            className={classes}
          >
            {content}
          </button>
        )
      })}
    </div>
  )
}
