// SPDX-License-Identifier: AGPL-3.0-only

/**
 * useMediaQuery (task 10): subscribes to a CSS media query and follows it
 * live. Where matchMedia is unavailable (jsdom tests, SSR) it defaults to
 * true — desktop is the fallback layout; AppShell tests stub matchMedia to
 * force the narrow layout explicitly.
 */
import { useEffect, useState } from 'react'

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => {
    if (
      typeof window === 'undefined' ||
      typeof window.matchMedia !== 'function'
    ) {
      return true
    }
    return window.matchMedia(query).matches
  })

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia(query)
    const onChange = (event: MediaQueryListEvent) => setMatches(event.matches)
    setMatches(mq.matches)
    window.addEventListener('change', onChange)
    return () => window.removeEventListener('change', onChange)
  }, [query])

  return matches
}
