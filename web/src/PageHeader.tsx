// SPDX-License-Identifier: AGPL-3.0-only

/**
 * PageHeader (task 38, spec 3.3): the one header every page renders. A
 * breadcrumb replaces the "Back to …" links, the title is the only h1, and
 * the actions sit in order on the right (secondary first, one primary last).
 * While `title` is undefined (data still loading) the h1 keeps its place and
 * shows a skeleton, so the words never change after load.
 */
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

export interface Crumb {
  label: string
  to: string
  testId?: string
}

export default function PageHeader({
  breadcrumb,
  title,
  subtitle,
  meta,
  actions,
}: {
  breadcrumb?: Crumb[]
  /** undefined while the data that names the page is loading. */
  title: string | undefined
  subtitle?: ReactNode
  meta?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2">
      {breadcrumb && breadcrumb.length > 0 && (
        <nav aria-label="Breadcrumb">
          <ol className="flex flex-wrap items-center gap-1 text-caption text-fg-muted">
            {breadcrumb.map((crumb) => (
              <li
                key={`${crumb.to}-${crumb.label}`}
                className="flex items-center gap-1"
              >
                <Link
                  to={crumb.to}
                  data-testid={crumb.testId}
                  className="inline-flex min-h-11 items-center text-fg-muted hover:text-fg"
                >
                  {crumb.label}
                </Link>
                <span aria-hidden="true" className="text-fg-subtle">
                  ›
                </span>
              </li>
            ))}
          </ol>
        </nav>
      )}

      <div className="flex min-h-11 flex-wrap items-start justify-between gap-x-4 gap-y-3">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            {title === undefined ? (
              <h1 aria-busy="true" className="text-title text-fg">
                <span
                  aria-hidden="true"
                  data-testid="page-header-skeleton"
                  className="inline-block h-8 w-48 animate-pulse rounded bg-raised align-middle"
                />
              </h1>
            ) : (
              <h1 className="break-words text-title text-fg">{title}</h1>
            )}
            {meta}
          </div>
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>

      {subtitle && (
        <p className="max-w-prose text-body text-fg-muted">{subtitle}</p>
      )}
    </div>
  )
}
