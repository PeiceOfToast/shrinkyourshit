// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The toast context and hook (task 13), split from the provider component
 * so each file exports one kind of thing. The failure text names the
 * action and carries the HTTP status and the server's error code, e.g.
 * "Scan failed (500, internal): no such library".
 */
import { createContext, useContext } from 'react'
import { ApiError } from './api'

export interface ToastApi {
  success: (message: string) => void
  /** Failure toast: "<action> (<status>, <code>): <message>". */
  failure: (action: string, err: unknown) => void
}

export const ToastContext = createContext<ToastApi | null>(null)

/** The toast helpers; requires the provider (mounted in main.tsx). */
export function useToast(): ToastApi {
  const toast = useContext(ToastContext)
  if (!toast) {
    throw new Error('useToast must be used inside <ToastProvider>')
  }
  return toast
}

/**
 * The failure text for a toast. Non-API errors (a network cut, a bug) fall
 * back to their own message.
 */
export function describeFailure(action: string, err: unknown): string {
  if (err instanceof ApiError) {
    const detail =
      err.message && err.message !== err.code ? `: ${err.message}` : ''
    return `${action} (${err.status}, ${err.code})${detail}`
  }
  const message = err instanceof Error ? err.message : String(err)
  return `${action}: ${message}`
}
