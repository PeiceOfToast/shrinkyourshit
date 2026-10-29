// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Toasts (task 13): one small provider, used by every mutation. Success is
 * short and specific; failure names the action and carries the HTTP status
 * and the server's error code. The provider mounts once in main.tsx, above
 * the router, so a toast survives the navigation a mutation may trigger.
 */
import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { ToastContext, describeFailure, type ToastApi } from './toastContext'

type ToastKind = 'success' | 'error'

interface ToastItem {
  id: number
  kind: ToastKind
  message: string
}

const SUCCESS_MS = 4000
const ERROR_MS = 7000

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => {
    setItems((list) => list.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (kind: ToastKind, message: string) => {
      const id = nextId.current++
      setItems((list) => [...list.slice(-3), { id, kind, message }])
      window.setTimeout(
        () => dismiss(id),
        kind === 'success' ? SUCCESS_MS : ERROR_MS,
      )
    },
    [dismiss],
  )

  const api = useMemo<ToastApi>(
    () => ({
      success: (message) => push('success', message),
      failure: (action, err) => push('error', describeFailure(action, err)),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        className="pointer-events-none fixed inset-x-4 top-4 z-[70] flex flex-col items-stretch gap-2 sm:items-end"
        aria-live="polite"
        data-testid="toast-region"
      >
        {items.map((toast) => (
          <div
            key={toast.id}
            role={toast.kind === 'error' ? 'alert' : 'status'}
            className={`pointer-events-auto flex w-full max-w-sm items-start gap-2 self-end rounded-xl px-4 py-3 text-sm shadow-lg ring-1 ${
              toast.kind === 'error'
                ? 'bg-red-950/95 text-red-100 ring-red-800'
                : 'bg-emerald-950/95 text-emerald-100 ring-emerald-800'
            }`}
            data-testid="toast"
          >
            <span aria-hidden="true">{toast.kind === 'error' ? '✕' : '✓'}</span>
            <span className="min-w-0 break-words">{toast.message}</span>
            <button
              type="button"
              onClick={() => dismiss(toast.id)}
              aria-label="Dismiss notification"
              className="ml-auto shrink-0 rounded px-1 opacity-70 hover:opacity-100"
            >
              ✕
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
