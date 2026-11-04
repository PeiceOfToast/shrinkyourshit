// SPDX-License-Identifier: AGPL-3.0-only

/**
 * StatusChip (task 19, restyled and rebuilt on Badge in task 39): one
 * file-status count as a small pill, "3 Waiting" or "2 Shrunk".
 *
 * It renders nothing when `count` is 0 (spec 2.1, "hide zero"). A zero
 * count is not a status worth a chip, and a page that wants to say "none"
 * writes that in a sentence instead.
 *
 * `satisfied` files read "Shrunk" when `bytesSaved` is above 0 (the file
 * was made smaller by this app), and "Nothing to do" when it is 0 or not
 * given. The UI derives this from `bytes_saved`; the API has no separate
 * "shrunk" status.
 */
import Badge, { type BadgeTone } from './Badge'
import { statusLabel } from './easyModel'

function toneFor(status: string, shrunk: boolean): BadgeTone {
  if (status === 'satisfied') return shrunk ? 'success' : 'neutral'
  switch (status) {
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'missing':
      return 'warning'
    case 'processing':
      return 'info'
    default:
      return 'neutral'
  }
}

export default function StatusChip({
  status,
  count,
  testId,
  bytesSaved,
}: {
  status: string
  count: number
  testId: string
  /** Bytes this library saved; used only to read `satisfied` as "Shrunk". */
  bytesSaved?: number
}) {
  if (count === 0) return null
  const shrunk = status === 'satisfied' && (bytesSaved ?? 0) > 0
  const label = shrunk ? 'Shrunk' : statusLabel(status)
  return (
    <Badge
      tone={toneFor(status, shrunk)}
      dot={status === 'processing'}
      pulse={status === 'processing'}
      testId={testId}
    >
      {count} {label}
    </Badge>
  )
}
