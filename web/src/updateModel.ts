// SPDX-License-Identifier: AGPL-3.0-only

/**
 * About-card model (task 17): wording for the update state, kept out of
 * the component so it is testable without React. The server owns the
 * facts; this file only presents them.
 */
import { ApiError } from './api'

/** Update apply stages (API.md, task 17) in plain words. */
const STAGE_LABELS: Record<string, string> = {
  starting: 'Starting the update…',
  checking: 'Looking up the latest release…',
  backup: 'Backing up the database…',
  downloading: 'Downloading the new version…',
  verifying: 'Verifying the download…',
  swapping: 'Installing…',
  done: 'Installed. The app is restarting…',
}

export function stageLabel(stage?: string): string {
  if (!stage) return 'Working…'
  return STAGE_LABELS[stage] ?? stage
}

export function runtimeLabel(runtime: 'binary' | 'container'): string {
  return runtime === 'container' ? 'Container (Docker)' : 'App on this machine'
}

export function formatPublished(iso?: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Check failures are quiet (task 17): one specific sentence in the card.
 * The rate limit and an unreachable server each get their own words.
 */
export function checkFailureMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429 || err.code === 'rate_limited') {
      return 'Updates were checked less than a minute ago. Try again in a moment.'
    }
    return `The update check failed (HTTP ${err.status}, code ${err.code}).`
  }
  return 'The update server could not be reached. Check your connection and try again.'
}
