// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The unsaved-changes guard (task 15). React Router's useBlocker needs a
 * data router and this app renders plain routers, so the guard combines:
 * - a beforeunload listener for reloads and tab close, and
 * - a capture-phase click interceptor on in-app links plus a confirm
 *   dialog for SPA navigation (it runs before React Router's own link
 *   handler, so the navigation is stopped and the dialog asks first).
 * Known limit: browser back/forward inside the SPA cannot be intercepted
 * without a data router.
 */
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

export function useUnsavedGuard(dirty: boolean) {
  const navigate = useNavigate()
  const [pending, setPending] = useState<string | null>(null)

  useEffect(() => {
    if (!dirty) return
    const beforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', beforeUnload)
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented) return
      const target = e.target as HTMLElement | null
      const href = target?.closest?.('a[href]')?.getAttribute('href') ?? null
      if (!href || !href.startsWith('/') || href.startsWith('//')) return
      e.preventDefault()
      e.stopPropagation()
      setPending(href)
    }
    document.addEventListener('click', onClick, true)
    return () => {
      window.removeEventListener('beforeunload', beforeUnload)
      document.removeEventListener('click', onClick, true)
    }
  }, [dirty])

  const leave = useCallback(() => {
    if (!pending) return
    const href = pending
    setPending(null)
    navigate(href)
  }, [pending, navigate])

  const stay = useCallback(() => setPending(null), [])
  return { pending, leave, stay }
}
