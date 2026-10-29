// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library detail (task 07): the file table with status, reasons and bytes
 * saved, a status filter and a retry-failed action.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { getFiles, getLibrary, retryJob } from './api'
import { formatBytes, statusLabel } from './easyModel'

const FILE_STATUSES = [
  'new',
  'satisfied',
  'queued',
  'processing',
  'done',
  'failed',
  'missing',
]

function statusColor(status: string): string {
  switch (status) {
    case 'satisfied':
    case 'done':
      return 'text-emerald-400'
    case 'failed':
      return 'text-red-400'
    case 'processing':
      return 'text-sky-400'
    case 'queued':
      return 'text-amber-400'
    default:
      return 'text-slate-400'
  }
}

export default function LibraryDetailPage() {
  const { id = '' } = useParams()
  const client = useQueryClient()
  const [status, setStatus] = useState('')
  const [notice, setNotice] = useState<string | null>(null)

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  const files = useQuery({
    queryKey: ['files', id, status],
    queryFn: () => getFiles(id, status ? { status } : { limit: 500 }),
  })

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['files'] })
    void client.invalidateQueries({ queryKey: ['library'] })
    void client.invalidateQueries({ queryKey: ['stats'] })
  }

  const retry = useMutation({
    mutationFn: async (failed: Array<{ last_job_id: string }>) => {
      const ids = [...new Set(failed.map((f) => f.last_job_id).filter(Boolean))]
      for (const jobId of ids) {
        try {
          await retryJob(jobId)
        } catch {
          // A job that is no longer retryable (e.g. the file turned
          // satisfied) is skipped; the rest still retry.
        }
      }
      return ids.length
    },
    onSuccess: (n) => {
      setNotice(n > 0 ? `Retrying ${n} job(s).` : 'Nothing to retry.')
      refresh()
    },
  })

  if (library.isPending) {
    return <p className="text-slate-400">Loading library…</p>
  }
  if (library.isError || !library.data) {
    return <p className="text-red-400">Cannot load this library.</p>
  }
  const lib = library.data
  const failedFiles = (files.data?.items ?? []).filter(
    (f) => f.status === 'failed',
  )

  return (
    <div className="space-y-4" data-testid="library-detail">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold">{lib.name}</h1>
          <p className="truncate text-sm text-slate-500">{lib.path}</p>
        </div>
        <div className="flex gap-2">
          <Link
            to={`/libraries/${lib.id}/edit`}
            className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700"
            data-testid="library-detail-edit"
          >
            Edit
          </Link>
        </div>
      </div>

      <div className="flex items-center justify-between gap-3">
        <label className="flex items-center gap-2 text-sm text-slate-400">
          Status
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="rounded-lg bg-slate-800 px-2 py-1.5 text-sm text-slate-200"
            data-testid="file-status-filter"
          >
            <option value="">All</option>
            {FILE_STATUSES.map((s) => (
              <option key={s} value={s}>
                {statusLabel(s)}
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          onClick={() => retry.mutate(failedFiles)}
          disabled={failedFiles.length === 0 || retry.isPending}
          className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700 disabled:opacity-50"
          data-testid="retry-failed"
        >
          Retry failed ({failedFiles.length})
        </button>
      </div>

      {notice && (
        <p
          className="text-sm text-emerald-400"
          role="status"
          data-testid="library-notice"
        >
          {notice}
        </p>
      )}

      {files.isPending ? (
        <p className="text-slate-400">Loading files…</p>
      ) : files.isError ? (
        <p className="text-red-400">Cannot load the files.</p>
      ) : files.data.total === 0 ? (
        <p className="text-slate-400" data-testid="files-empty">
          No files here yet. Wait for a scan or add files to the folder.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-2xl ring-1 ring-slate-800">
          <table className="w-full text-left text-sm" data-testid="file-table">
            <thead className="bg-slate-900 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-3">File</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Why</th>
                <th className="px-4 py-3">Size</th>
                <th className="px-4 py-3">Saved</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800 bg-slate-900/50">
              {files.data.items.map((f) => {
                const name = f.path.split('/').pop() ?? f.path
                return (
                  <tr key={f.id} data-testid={`file-row-${f.id}`}>
                    <td
                      className="max-w-[16rem] truncate px-4 py-2 text-slate-200"
                      title={f.path}
                    >
                      {name}
                    </td>
                    <td
                      className={`px-4 py-2 font-medium ${statusColor(f.status)}`}
                      data-testid={`file-status-${f.id}`}
                    >
                      {statusLabel(f.status)}
                    </td>
                    <td
                      className="px-4 py-2 text-slate-500"
                      data-testid={`file-reason-${f.id}`}
                    >
                      {f.satisfied_reason || '—'}
                    </td>
                    <td className="px-4 py-2 text-slate-400">
                      {formatBytes(f.size)}
                    </td>
                    <td className="px-4 py-2 text-emerald-400">
                      {f.bytes_saved > 0 ? formatBytes(f.bytes_saved) : '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
