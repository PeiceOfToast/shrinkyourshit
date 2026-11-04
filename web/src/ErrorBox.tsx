// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ErrorBox (task 13, restyled and extended in task 39, spec 3.7): the error
 * surface. It names what failed in plain words, then a technical line with
 * the HTTP status and the server's snake_case code, and puts the raw
 * response body behind a "Technical details" disclosure.
 *
 * `onRetry` adds a "Try again" button. Page-level query errors pass
 * `() => query.refetch()`.
 */
import { ApiError } from './api'
import Icon from './Icon'
import { btn } from './uiControls'

export default function ErrorBox({
  error,
  title,
  testId = 'error-box',
  onRetry,
}: {
  error: unknown
  title: string
  testId?: string
  onRetry?: () => void
}) {
  const api = error instanceof ApiError ? error : null
  const reason = api
    ? api.message && api.message !== api.code
      ? api.message
      : ''
    : error instanceof Error
      ? error.message
      : String(error)

  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-card bg-danger/10 p-4 ring-1 ring-inset ring-danger/30"
      data-testid={testId}
    >
      <Icon name="alert" className="mt-0.5 h-5 w-5 shrink-0 text-danger" />
      <div className="min-w-0 flex-1">
        <p className="text-body font-semibold text-fg">{title}</p>
        {reason && (
          <p className="mt-1 break-words text-body text-fg-muted">{reason}</p>
        )}
        {api && (
          <p className="mt-1 break-words font-mono text-caption text-fg-muted">
            HTTP <span data-testid="error-status">{api.status}</span> ·{' '}
            <span data-testid="error-code">{api.code}</span>
          </p>
        )}
        {api?.raw && (
          <details className="mt-2" data-testid="error-raw">
            <summary className="inline-flex min-h-11 cursor-pointer items-center text-body text-fg-muted hover:text-fg">
              Technical details
            </summary>
            <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-control bg-canvas p-3 font-mono text-caption text-fg-muted">
              {api.raw}
            </pre>
          </details>
        )}
        {onRetry && (
          <button type="button" onClick={onRetry} className={`${btn} mt-3`}>
            Try again
          </button>
        )}
      </div>
    </div>
  )
}
