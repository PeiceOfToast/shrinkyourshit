// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One Server-Sent-Events subscription for the whole app (DECISIONS.md B6),
 * mounted once in the AppShell. Each event type invalidates the matching
 * react-query caches so open pages refetch and update live. Event types
 * come from API.md; keys match the query keys used across the pages.
 */
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'

const EVENT_QUERY_KEYS: Record<string, string[][]> = {
  'job.updated': [['jobs'], ['stats'], ['files']],
  'file.updated': [['files'], ['stats']],
  'library.updated': [['libraries'], ['library'], ['stats']],
  'node.updated': [['nodes']],
  // The in-app update (task 17): progress, success and failure all change
  // what the About card and the dashboard warning show.
  'update.progress': [['update-status']],
  'update.succeeded': [['update-status']],
  'update.failed': [['update-status']],
}

/**
 * Subscribes to /api/v1/events and invalidates caches while mounted. Tests
 * stub the global EventSource; the component only requires the browser API.
 * Mounted once in the AppShell; component form so tests and pages can
 * render it directly.
 */
export function LiveEvents() {
  const client = useQueryClient()
  useEffect(() => {
    const source = new EventSource('/api/v1/events')
    const listeners: Array<[string, () => void]> = []
    for (const [event, keys] of Object.entries(EVENT_QUERY_KEYS)) {
      const listener = () => {
        for (const key of keys) {
          void client.invalidateQueries({ queryKey: key })
        }
      }
      source.addEventListener(event, listener)
      listeners.push([event, listener])
    }
    return () => {
      for (const [event, listener] of listeners) {
        source.removeEventListener(event, listener)
      }
      source.close()
    }
  }, [client])
  return null
}
