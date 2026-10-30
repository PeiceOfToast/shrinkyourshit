// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library detail (tasks 07, 13 and 19): the header summarizes the
 * library — file counts by status, bytes saved, the target in plain
 * language (the server's easy-mode description path) and the last scan
 * time. The file table shows before, after, ratio, the encoder that
 * produced the file and a link to the per-file job history (the job log).
 * Search, sort and filtering happen server side (q, sort, dir); the
 * browser never re-sorts a page. Task 13 adds server side pagination, the
 * cancel-job confirmation and toasts.
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import {
  cancelJob,
  getFiles,
  getLibrary,
  getStats,
  previewFlow,
  retryJob,
  type FileItem,
} from './api'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import { formatBytes, statusLabel, truncateMiddle } from './easyModel'
import ErrorBox from './ErrorBox'
import FileHistory from './FileHistory'
import PageContainer from './PageContainer'
import StatusChip from './StatusChip'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn } from './uiControls'

const FILE_STATUSES = [
  'new',
  'satisfied',
  'queued',
  'processing',
  'done',
  'failed',
  'missing',
] as const

const PAGE_SIZES = [25, 50, 100, 200]

/** The sorts the server knows (task 19); anything else is a 400. */
const FILE_SORTS = [
  { key: 'name', label: 'Name' },
  { key: 'size', label: 'Size' },
  { key: 'saved', label: 'Space saved' },
] as const

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

/** A file the user may still cancel: work queued or running for it. */
function isCancellable(f: FileItem): boolean {
  return (
    (f.status === 'queued' || f.status === 'processing') && f.last_job_id !== ''
  )
}

/** "30 Oct 2026, 12:00" style timestamp for the header. */
function formatScanTime(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
}

/** The shrink ratio as a multiplier, e.g. "1.7×". */
function formatRatio(before: number, after: number): string {
  if (after <= 0 || before <= 0) return '—'
  return `${(before / after).toFixed(1)}×`
}

export default function LibraryDetailPage() {
  const { id = '' } = useParams()
  const client = useQueryClient()
  const toast = useToast()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(50)
  const [toCancel, setToCancel] = useState<FileItem | null>(null)
  const [historyFile, setHistoryFile] = useState<FileItem | null>(null)
  const [searchText, setSearchText] = useState('')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<'name' | 'size' | 'saved'>('name')
  const [dir, setDir] = useState<'asc' | 'desc'>('asc')

  // The search box debounces: typing stays responsive, the server sees
  // one request per settled query.
  useEffect(() => {
    const timer = setTimeout(() => setQuery(searchText.trim()), 300)
    return () => clearTimeout(timer)
  }, [searchText])

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })
  const files = useQuery({
    queryKey: ['files', id, status, page, pageSize, query, sort, dir],
    queryFn: () =>
      getFiles(id, {
        status: status || undefined,
        q: query || undefined,
        sort,
        dir,
        limit: pageSize,
        offset: page * pageSize,
      }),
  })
  // The target in plain language: the same server-rendered lines the easy
  // screen's "what this does" panel shows — the UI holds no second copy.
  const plain = useQuery({
    queryKey: ['target-plain', id],
    // enabled guarantees the library and its flow exist when this runs.
    queryFn: () => previewFlow({ graph: library.data!.flow! }),
    enabled: !!library.data?.flow,
  })

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['files'] })
    void client.invalidateQueries({ queryKey: ['library'] })
    void client.invalidateQueries({ queryKey: ['stats'] })
  }

  // Retries every failed file, not just the visible page: the failed set
  // is fetched server-side (up to the API's own page cap) so pagination
  // cannot hide a failed file from the retry action.
  const retry = useMutation({
    mutationFn: async () => {
      const failed = await getFiles(id, { status: 'failed', limit: 1000 })
      const ids = [
        ...new Set(failed.items.map((f) => f.last_job_id).filter(Boolean)),
      ]
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
      toast.success(n > 0 ? `Retrying ${n} job(s).` : 'Nothing to retry.')
      refresh()
    },
    onError: (err: unknown) => toast.failure('Retrying failed', err),
  })

  const cancel = useMutation({
    mutationFn: (jobId: string) => cancelJob(jobId),
    onSuccess: () => {
      setToCancel(null)
      toast.success(
        'Job cancelled. The unfinished temporary copy is thrown away; the original file is untouched.',
      )
      refresh()
    },
    onError: (err: unknown) => toast.failure('Cancelling the job failed', err),
  })

  // A page past the end (files removed under us) falls back to page 0.
  useEffect(() => {
    if (
      files.data &&
      page > 0 &&
      files.data.items.length === 0 &&
      files.data.total > 0
    ) {
      setPage(0)
    }
  }, [files.data, page])

  if (library.isPending) {
    return (
      <PageContainer className="space-y-4" data-testid="library-detail">
        <SkeletonRows rows={2} />
      </PageContainer>
    )
  }
  if (library.isError || !library.data) {
    return (
      <PageContainer className="space-y-4" data-testid="library-detail">
        <ErrorBox title="Cannot load this library." error={library.error} />
      </PageContainer>
    )
  }
  const lib = library.data
  const total = files.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const failedOnPage = (files.data?.items ?? []).filter(
    (f) => f.status === 'failed',
  )
  const counts = stats.data?.libraries.find((s) => s.library_id === id)

  const changeStatus = (next: string) => {
    setStatus(next)
    setPage(0)
  }
  const changePageSize = (next: number) => {
    setPageSize(next)
    setPage(0)
  }
  const changeSort = (next: 'name' | 'size' | 'saved') => {
    setSort(next)
    setPage(0)
  }
  const flipDir = () => {
    setDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    setPage(0)
  }

  const cancelButton = (f: FileItem, onCard: boolean) => {
    if (!isCancellable(f)) return null
    const name = f.path.split('/').pop() ?? f.path
    return (
      <button
        type="button"
        onClick={() => setToCancel(f)}
        className={`${btn} !text-amber-300 hover:!bg-slate-700 ${
          onCard ? 'shrink-0' : ''
        }`}
        aria-label={`Cancel the job for ${name}`}
        data-testid={`file-cancel-${f.id}${onCard ? '-card' : ''}`}
      >
        Cancel
      </button>
    )
  }
  const logButton = (f: FileItem, onCard: boolean) => {
    const name = f.path.split('/').pop() ?? f.path
    return (
      <button
        type="button"
        onClick={() => setHistoryFile(f)}
        className={btn}
        aria-label={`Job history and log for ${name}`}
        data-testid={`file-log-${f.id}${onCard ? '-card' : ''}`}
      >
        Log
      </button>
    )
  }

  // The table and the phone cards carry the same columns (task 20).
  const columns: Column<FileItem>[] = [
    {
      key: 'name',
      header: 'File',
      primary: true,
      cell: (f) => {
        const name = f.path.split('/').pop() ?? f.path
        return (
          <span
            className="block max-w-[16rem] truncate text-slate-200"
            title={f.path}
          >
            {name}
          </span>
        )
      },
      cardCell: (f) => {
        const name = f.path.split('/').pop() ?? f.path
        return (
          <span
            className="min-w-0 break-words font-medium text-slate-200"
            title={f.path}
          >
            {name}
          </span>
        )
      },
    },
    {
      key: 'status',
      header: 'Status',
      cell: (f) => (
        <span
          className={`font-medium ${statusColor(f.status)}`}
          data-testid={`file-status-${f.id}`}
        >
          {statusLabel(f.status)}
        </span>
      ),
      cardCell: (f) => (
        <span
          className={`font-medium ${statusColor(f.status)}`}
          data-testid={`file-card-status-${f.id}`}
        >
          {statusLabel(f.status)}
        </span>
      ),
      cardLabel: 'Status',
    },
    {
      key: 'reason',
      header: 'Why',
      cell: (f) => (
        <span
          className="break-words text-slate-500"
          data-testid={`file-reason-${f.id}`}
        >
          {f.satisfied_reason || '—'}
        </span>
      ),
      cardCell: (f) => (
        <span className="break-words" data-testid={`file-card-reason-${f.id}`}>
          Why: {f.satisfied_reason}
        </span>
      ),
      cardNoLabel: true,
      hideInCard: (f) => !f.satisfied_reason,
    },
    {
      key: 'before',
      header: 'Before',
      cell: (f) => (
        <span className="text-slate-400" data-testid={`file-before-${f.id}`}>
          {formatBytes(f.size + f.bytes_saved)}
        </span>
      ),
      cardCell: (f) => (
        <span className="text-slate-400">
          {formatBytes(f.size + f.bytes_saved)}
          {f.bytes_saved > 0
            ? ` → ${formatBytes(f.size)} (${formatRatio(
                f.size + f.bytes_saved,
                f.size,
              )})`
            : ''}
        </span>
      ),
      cardLabel: 'Size:',
    },
    {
      key: 'after',
      header: 'After',
      cell: (f) => (
        <span className="text-slate-300" data-testid={`file-after-${f.id}`}>
          {f.bytes_saved > 0 ? formatBytes(f.size) : '—'}
        </span>
      ),
      hideInCard: true,
    },
    {
      key: 'saved',
      header: 'Saved',
      cell: (f) => (
        <span className="text-emerald-400" data-testid={`file-saved-${f.id}`}>
          {f.bytes_saved > 0 ? formatBytes(f.bytes_saved) : '—'}
        </span>
      ),
      cardCell: (f) => (
        <span className="whitespace-nowrap text-emerald-400">
          {formatBytes(f.bytes_saved)}
        </span>
      ),
      cardLabel: 'Saved',
      hideInCard: (f) => f.bytes_saved <= 0,
    },
    {
      key: 'ratio',
      header: 'Ratio',
      cell: (f) => (
        <span className="text-emerald-400" data-testid={`file-ratio-${f.id}`}>
          {f.bytes_saved > 0
            ? formatRatio(f.size + f.bytes_saved, f.size)
            : '—'}
        </span>
      ),
      hideInCard: true,
    },
    {
      key: 'encoder',
      header: 'Encoder',
      cell: (f) => (
        <span className="text-slate-400" data-testid={`file-encoder-${f.id}`}>
          {f.last_job_encoder || '—'}
        </span>
      ),
      cardCell: (f) => (
        <span className="text-slate-400">{f.last_job_encoder || '—'}</span>
      ),
      cardLabel: 'Encoder:',
    },
    {
      key: 'actions',
      header: '',
      cell: (f) => (
        <div className="flex flex-wrap justify-end gap-2">
          {logButton(f, false)}
          {cancelButton(f, false)}
        </div>
      ),
      cardCell: (f) => (
        <div className="flex flex-wrap justify-end gap-2">
          {logButton(f, true)}
          {cancelButton(f, true)}
        </div>
      ),
      cardNoLabel: true,
    },
  ]

  return (
    <PageContainer className="space-y-4" data-testid="library-detail">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold">{lib.name}</h1>
          <p className="truncate text-sm text-slate-500">{lib.path}</p>
        </div>
        <div className="flex gap-2">
          <Link
            to={`/libraries/${lib.id}/rules`}
            className={btn}
            data-testid="library-detail-rules"
          >
            Rules
          </Link>
          <Link
            to={`/libraries/${lib.id}/edit`}
            className={btn}
            data-testid="library-detail-edit"
          >
            Edit
          </Link>
        </div>
      </div>

      {/* Header summary (task 19): counts by status, space saved, the
          target in plain language and the last scan time. */}
      <div
        className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
        data-testid="library-summary"
      >
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <div
            className="flex flex-wrap gap-1.5"
            data-testid="library-summary-chips"
          >
            {FILE_STATUSES.map((s) => (
              <StatusChip
                key={s}
                status={s}
                count={counts?.[s] ?? 0}
                testId={`library-summary-${s}`}
              />
            ))}
          </div>
          <span
            className="text-sm font-medium text-emerald-400"
            data-testid="library-summary-saved"
          >
            {formatBytes(counts?.bytes_saved ?? 0)} saved
          </span>
          <span
            className="text-sm text-slate-400"
            data-testid="library-last-scan"
          >
            {lib.last_scan_at
              ? `Last scan: ${formatScanTime(lib.last_scan_at)}`
              : 'Not scanned yet.'}
          </span>
        </div>
        {plain.data && plain.data.steps.length > 0 && (
          <div
            className="mt-3 border-t border-slate-800 pt-3"
            data-testid="library-target-plain"
          >
            <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              What this library does
            </p>
            <ul className="mt-2 space-y-1">
              {plain.data.steps.map((s) => (
                <li
                  key={s.id}
                  className="flex gap-2 text-sm text-slate-300"
                  data-testid="target-plain-line"
                >
                  <span className="text-slate-500">•</span>
                  <span>{s.plain}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm text-slate-400">
          Status
          <select
            value={status}
            onChange={(e) => changeStatus(e.target.value)}
            className="min-h-11 rounded-lg bg-slate-800 px-2 py-1.5 text-sm text-slate-200"
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
        <label className="flex min-w-[12rem] flex-1 items-center gap-2 text-sm text-slate-400 sm:max-w-xs">
          <span className="shrink-0">Search</span>
          <input
            type="search"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="File name…"
            className="min-h-11 w-full min-w-0 rounded-lg bg-slate-800 px-2 py-1.5 text-sm text-slate-200 placeholder:text-slate-500"
            aria-label="Search files by name"
            data-testid="file-search"
          />
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-400">
          Sort
          <select
            value={sort}
            onChange={(e) => changeSort(e.target.value as typeof sort)}
            className="min-h-11 rounded-lg bg-slate-800 px-2 py-1.5 text-sm text-slate-200"
            data-testid="file-sort"
          >
            {FILE_SORTS.map((s) => (
              <option key={s.key} value={s.key}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          onClick={flipDir}
          className={btn}
          aria-label="Flip the sort direction"
          data-testid="file-sort-dir"
        >
          {dir === 'asc' ? '↑ Ascending' : '↓ Descending'}
        </button>
        <button
          type="button"
          onClick={() => retry.mutate()}
          disabled={failedOnPage.length === 0 || retry.isPending}
          className={btn}
          data-testid="retry-failed"
        >
          {retry.isPending
            ? 'Retrying…'
            : `Retry failed (${failedOnPage.length})`}
        </button>
      </div>

      {files.isPending ? (
        <SkeletonRows />
      ) : files.isError ? (
        <ErrorBox title="Cannot load the files." error={files.error} />
      ) : files.data.total === 0 ? (
        query ? (
          <p className="text-slate-400" data-testid="files-empty">
            No files match “{query}”.{' '}
            <button
              type="button"
              onClick={() => setSearchText('')}
              className="inline-flex min-h-11 items-center text-emerald-400 underline"
              data-testid="files-clear-search"
            >
              Clear the search.
            </button>
          </p>
        ) : (
          <p className="text-slate-400" data-testid="files-empty">
            No files here yet. Wait for a scan or add files to the folder.
          </p>
        )
      ) : (
        <>
          {/* One column definition, two layouts (task 20): the table from
              `sm` up, stacked cards below it. The table wrapper scrolls
              sideways, so long paths and reasons never force the page wide. */}
          <DataList
            rows={files.data.items}
            columns={columns}
            testid="file-table"
            cardTestid="file-cards"
            rowTestId={(f) => `file-row-${f.id}`}
            cardRowTestId={(f) => `file-card-${f.id}`}
          />

          {/* Pagination (task 13): server side, so 40k files never load at
              once. The status filter, search, sort and page size reset to
              the first page when they change. */}
          <div
            className="flex flex-wrap items-center justify-between gap-3 text-sm text-slate-400"
            data-testid="files-pagination"
          >
            <div className="flex items-center gap-2">
              <label className="flex items-center gap-2">
                Per page
                <select
                  value={pageSize}
                  onChange={(e) => changePageSize(Number(e.target.value))}
                  className="min-h-11 rounded-lg bg-slate-800 px-2 py-1.5 text-sm text-slate-200"
                  data-testid="files-page-size"
                >
                  {PAGE_SIZES.map((n) => (
                    <option key={n} value={n}>
                      {n}
                    </option>
                  ))}
                </select>
              </label>
              <span data-testid="files-page-info">
                Page {page + 1} of {pages} · {total} file
                {total === 1 ? '' : 's'}
              </span>
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0}
                className={btn}
                data-testid="files-prev"
              >
                ← Previous
              </button>
              <button
                type="button"
                onClick={() => setPage((p) => Math.min(pages - 1, p + 1))}
                disabled={page >= pages - 1}
                className={btn}
                data-testid="files-next"
              >
                Next →
              </button>
            </div>
          </div>
        </>
      )}

      {historyFile && (
        <FileHistory file={historyFile} onClose={() => setHistoryFile(null)} />
      )}

      {toCancel && (
        <ConfirmDialog
          title="Cancel this job?"
          confirmLabel="Cancel job"
          cancelLabel="Keep it running"
          danger={false}
          busy={cancel.isPending}
          onConfirm={() => cancel.mutate(toCancel.last_job_id)}
          onClose={() => setToCancel(null)}
          testId="confirm-cancel-job"
          confirmTestId="confirm-cancel-job-yes"
          cancelTestId="confirm-cancel-job-no"
        >
          <p>
            The job for{' '}
            <span className="break-all text-slate-100">
              {truncateMiddle(toCancel.path, 64)}
            </span>{' '}
            stops. The partially encoded temporary file is thrown away, the job
            goes back to waiting, and the original file is never touched.
          </p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
