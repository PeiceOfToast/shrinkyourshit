// SPDX-License-Identifier: AGPL-3.0-only

/**
 * StatusChip (task 19): one file-status count as a small colored pill —
 * the count plus its plain-language label, dimmed when zero. Shared by the
 * libraries list and the library detail header so both read the same way.
 */
import { statusLabel } from './easyModel'

function chipColor(status: string): string {
  switch (status) {
    case 'satisfied':
    case 'done':
      return 'bg-emerald-900/60 text-emerald-300 ring-emerald-800'
    case 'failed':
      return 'bg-red-900/60 text-red-300 ring-red-800'
    case 'queued':
      return 'bg-amber-900/60 text-amber-300 ring-amber-800'
    case 'processing':
      return 'bg-sky-900/60 text-sky-300 ring-sky-800'
    case 'missing':
      return 'bg-slate-800 text-slate-400 ring-slate-700'
    default:
      return 'bg-slate-800 text-slate-300 ring-slate-700'
  }
}

export default function StatusChip({
  status,
  count,
  testId,
}: {
  status: string
  count: number
  testId: string
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ${chipColor(status)} ${
        count === 0 ? 'opacity-50' : ''
      }`}
      data-testid={testId}
    >
      {count} {statusLabel(status)}
    </span>
  )
}
