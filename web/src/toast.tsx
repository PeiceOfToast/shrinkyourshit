// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Toasts (task 13): one small provider, used by every mutation. Success is
 * short and specific; failure names the action and carries the HTTP status
 * and the server's error code. The provider mounts once in main.tsx, above
 * the router, so a toast survives the navigation a mutation may trigger.
 */
import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { ToastContext, describeFailure, type ToastApi } from './toastContext'
import Icon from './Icon'
import { btnIcon } from './uiControls'

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
        className="pointer-events-none fixed inset-x-4 bottom-4 z-toast flex flex-col items-stretch gap-2 sm:bottom-auto sm:top-4 sm:items-end"
        aria-live="polite"
        data-testid="toast-region"
      >
        {items.map((toast) => (
          <div
            key={toast.id}
            role={toast.kind === 'error' ? 'alert' : 'status'}
            className={`pointer-events-auto flex w-full max-w-sm items-start gap-3 self-end rounded-overlay border-l-[3px] bg-surface p-4 text-body text-fg shadow-overlay ring-1 ring-line-strong/40 ${
              toast.kind === 'error' ? 'border-danger' : 'border-success'
            }`}
            data-testid="toast"
          >
            <Icon
              name={toast.kind === 'error' ? 'alert' : 'check'}
              className={`mt-0.5 h-5 w-5 shrink-0 ${
                toast.kind === 'error' ? 'text-danger' : 'text-success'
              }`}
            />
            <span className="min-w-0 flex-1 break-words">{toast.message}</span>
            <button
              type="button"
              onClick={() => dismiss(toast.id)}
              aria-label="Dismiss notification"
              className={`${btnIcon} -mr-2 -mt-2`}
            >
              <Icon name="close" className="h-5 w-5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
