// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Card (task 38, spec 3.4): the one surface for a section of a page. It
 * replaces the hand-written `rounded-2xl bg-slate-900 p-6 ring-1 …` copies.
 * Never nest a Card in a Card; divide sub-groups with `border-t border-line`.
 */
import type { ReactNode } from 'react'
import { card, cardPad } from './uiControls'

export default function Card({
  title,
  description,
  actions,
  padding = 'default',
  footer,
  as: Tag = 'div',
  className = '',
  children,
  testId,
}: {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  /** `none` is for a card that holds a DataList or a divided list. */
  padding?: 'default' | 'none'
  footer?: ReactNode
  as?: 'div' | 'section' | 'article'
  className?: string
  children?: ReactNode
  testId?: string
}) {
  const hasHeader = Boolean(title || description || actions)
  // The header carries the top padding; the body follows it with gap spacing.
  const headerClass =
    'flex items-start justify-between gap-4 px-4 pt-4 sm:px-6 sm:pt-6'
  const bodyClass =
    padding === 'none' ? '' : hasHeader ? 'px-4 pb-4 sm:px-6 sm:pb-6' : cardPad

  return (
    <Tag className={`${card} flex flex-col ${className}`} data-testid={testId}>
      {hasHeader && (
        <div className={`${headerClass} ${children || footer ? 'mb-4' : ''}`}>
          <div className="min-w-0">
            {title && <h2 className="text-heading text-fg">{title}</h2>}
            {description && (
              <p className="mt-1 text-body text-fg-muted">{description}</p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>
          )}
        </div>
      )}
      {children !== undefined && <div className={bodyClass}>{children}</div>}
      {footer && (
        <div className="flex flex-wrap items-center gap-2 border-t border-line px-4 py-3 sm:px-6">
          {footer}
        </div>
      )}
    </Tag>
  )
}
