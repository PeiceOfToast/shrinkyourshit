// SPDX-License-Identifier: AGPL-3.0-only

/**
 * EmptyState (task 38, spec 3.7): the one way a list or section says it has
 * nothing yet. Title says what is missing, body says what adding one does,
 * and the action (when there is one) is a real button or link.
 */
import type { ReactNode } from 'react'
import Icon, { type IconName } from './Icon'

export default function EmptyState({
  icon,
  title,
  body,
  action,
  testId,
}: {
  icon?: IconName
  title: ReactNode
  body?: ReactNode
  action?: ReactNode
  testId?: string
}) {
  return (
    <div
      className="flex flex-col items-center gap-3 py-12 text-center"
      data-testid={testId}
    >
      {icon && (
        <span className="flex h-12 w-12 items-center justify-center rounded-full bg-raised">
          <Icon name={icon} className="h-10 w-10 text-fg-muted" />
        </span>
      )}
      <h2 className="text-heading text-fg">{title}</h2>
      {body && <p className="max-w-prose text-body text-fg-muted">{body}</p>}
      {action}
    </div>
  )
}
