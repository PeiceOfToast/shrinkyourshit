// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ErrorBox (task 13): the error surface that replaced the one-line
 * failures. It names what failed, shows the HTTP status and the server's
 * snake_case code, and puts the raw response body behind a disclosure so
 * the detail is there without shouting it.
 */
import { ApiError } from './api'

export default function ErrorBox({
  error,
  title,
  testId = 'error-box',
}: {
  error: unknown
  title: string
  testId?: string
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
      className="rounded-2xl bg-red-950/40 p-4 ring-1 ring-red-900/60"
      data-testid={testId}
    >
      <p className="text-sm font-medium text-red-200">{title}</p>
      <p className="mt-1 break-words text-sm text-red-300">
        {api ? (
          <>
            HTTP <span data-testid="error-status">{api.status}</span>, code{' '}
            <span data-testid="error-code">{api.code}</span>
            {reason ? ` — ${reason}` : ''}
          </>
        ) : (
          reason
        )}
      </p>
      {api?.raw && (
        <details className="mt-2" data-testid="error-raw">
          <summary className="cursor-pointer text-xs text-red-300/80 hover:text-red-200">
            Technical details
          </summary>
          <pre className="mt-2 max-h-48 overflow-auto rounded-lg bg-slate-950 p-3 text-xs text-red-200">
            {api.raw}
          </pre>
        </details>
      )}
    </div>
  )
}
