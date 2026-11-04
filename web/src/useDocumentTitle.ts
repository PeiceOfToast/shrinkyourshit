// SPDX-License-Identifier: AGPL-3.0-only

/**
 * useDocumentTitle (v1.9 UI redesign, spec section 4.1): sets the browser tab
 * title from the parts of the current page, so every tab reads differently.
 * "Rules · Movies · ShrinkYourShit". Empty and undefined parts are skipped.
 */
import { useEffect } from 'react'

const SEPARATOR = ' · '
const APP_NAME = 'ShrinkYourShit'

/** The title string for a list of parts. Pure, so it can be tested without React. */
export function documentTitle(parts: (string | undefined)[]): string {
  const kept = parts.filter((p): p is string => Boolean(p))
  return [...kept, APP_NAME].join(SEPARATOR)
}

export function useDocumentTitle(parts: (string | undefined)[]): void {
  // Parts are re-joined each render, so the effect keys on the joined string
  // rather than on the array identity.
  const title = documentTitle(parts)
  useEffect(() => {
    document.title = title
  }, [title])
}
