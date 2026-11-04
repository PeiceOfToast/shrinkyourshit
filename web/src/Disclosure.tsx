// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Disclosure (task 38, spec 4.1): a native `details`/`summary`, so the browser
 * supplies the keyboard behaviour (Enter and Space toggle it) and the state
 * with no script. Use it inside a Card with `padding="none"`.
 */
import type { ReactNode } from 'react'
import Icon from './Icon'

export default function Disclosure({
  summary,
  children,
  defaultOpen = false,
  testId,
}: {
  summary: ReactNode
  children: ReactNode
  defaultOpen?: boolean
  testId?: string
}) {
  return (
    <details
      className="group"
      open={defaultOpen || undefined}
      data-testid={testId}
    >
      <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 rounded-card px-4 sm:px-6 text-body font-semibold text-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus [&::-webkit-details-marker]:hidden">
        <span className="min-w-0">{summary}</span>
        <Icon
          name="chevron-right"
          className="h-5 w-5 shrink-0 text-fg-muted transition-transform duration-base group-open:rotate-90"
        />
      </summary>
      <div className="px-4 pb-4 text-body text-fg-muted sm:px-6">
        {children}
      </div>
    </details>
  )
}
