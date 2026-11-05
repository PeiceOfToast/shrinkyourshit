// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library detail (tasks 07, 13, 19 and 44). Spec 5.6: a PageHeader with the
 * library's actions (Rules, Edit, Scan now), a summary Card with four
 * numbers and the rules in plain language, then the file list. The list
 * carries before and after, the saved size with its ratio, the encoder in
 * plain words and a History action that opens the per-file job history.
 * Search, sort and filtering happen server side (q, sort, dir); the browser
 * never re-sorts a page. The pager and the retry action only appear when
 * they have something to do.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import {
  cancelJob,
  getFiles,
  getLibrary,
  getLibraryFlow,
  getStats,
  previewFlow,
  putLibraryTier,
  retryJob,
  scanLibrary,
  type FileItem,
  type Library,
  type RuleTier,
} from './api'
import Badge, { type BadgeTone } from './Badge'
import Card from './Card'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import { statusLabel, truncateMiddle } from './easyModel'
import EmptyState from './EmptyState'
import ErrorBox from './ErrorBox'
import FileHistory from './FileHistory'
import {
  encoderLabel,
  formatBytes,
  formatCount,
  formatDateTime,
} from './format'
import { kindLabel, KIND_AGNOSTIC_STEPS } from './kinds'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import SegmentedControl from './SegmentedControl'
import StatusChip from './StatusChip'
import { SkeletonRows } from './skeleton'
import { TIER_OPTIONS, isLocked, tierLabel } from './tierModel'
import { UnrepresentableBanner } from './TierNotices'
import { useToast } from './toastContext'
import {
  btn,
  btnGhost,
  btnIcon,
  btnPrimary,
  input,
  linkTouch,
  select,
} from './uiControls'

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

/** Badge tone for a file, using the spec 5.0 status mapping. */
function fileTone(f: FileItem): BadgeTone {
  if (f.status === 'satisfied') return f.bytes_saved > 0 ? 'success' : 'neutral'
  switch (f.status) {
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

/** "Shrunk" for a satisfied file this app made smaller (spec 5.0). */
function fileLabel(f: FileItem): string {
  if (f.status === 'satisfied' && f.bytes_saved > 0) return 'Shrunk'
  return statusLabel(f.status)
}

/** The file's status as a Badge, with the test id the page already uses. */
function FileStatus({ file, testId }: { file: FileItem; testId: string }) {
  return (
    <Badge
      tone={fileTone(file)}
      dot={file.status === 'processing'}
      pulse={file.status === 'processing'}
      testId={testId}
    >
      {fileLabel(file)}
    </Badge>
  )
}

/** A file the user may still cancel: work queued or running for it. */
function isCancellable(f: FileItem): boolean {
  return (
    (f.status === 'queued' || f.status === 'processing') && f.last_job_id !== ''
  )
}

/** The shrink ratio as a multiplier, e.g. "1.7×". */
function formatRatio(before: number, after: number): string {
  if (after <= 0 || before <= 0) return '—'
  return `${(before / after).toFixed(1)}×`
}

/** The tier a library runs, as the word the Rules page uses. */
function tierWord(source: string): string {
  if (source === 'easy') return 'Easy'
  if (source === 'advanced') return 'Advanced'
  return 'Steps'
}

/** The file name, and the path under the library when it differs from it. */
function relativeTo(libPath: string, filePath: string, name: string): string {
  const prefix = `${libPath.replace(/\/$/, '')}/`
  if (!filePath.startsWith(prefix)) return ''
  const rel = filePath.slice(prefix.length)
  return rel === name ? '' : rel
}

/**
 * The plain-language hint (task 34): a library that looks for photos,
 * subtitles or anything else while its flow only does video and audio work
 * says so, so a photos library that is doing nothing looks deliberate.
 */
function NonMediaHint({ lib }: { lib: Library }) {
  const kinds = lib.file_kinds?.length ? lib.file_kinds : ['video']
  const nonMedia = kinds.filter((k) => k !== 'video' && k !== 'audio')
  if (nonMedia.length === 0) return null
  const flowWorks =
    (lib.flow?.nodes ?? []).some((n) => KIND_AGNOSTIC_STEPS.includes(n.type)) ||
    !!lib.last_scan?.skipped_kinds
  if (flowWorks) return null
  const labels = nonMedia.map(kindLabel).join(', ')
  return (
    <span
      className="text-caption text-warning"
      data-testid="library-kind-hint"
      title="Add a rename, move or plugin step to the flow for these files to be processed."
    >
      This library looks for {labels}, but its flow only handles video and audio
      — those files are skipped untouched.
    </span>
  )
}

/** One number in the summary: a small label over a value. */
function SummaryStat({
  label,
  value,
  testId,
  className = '',
}: {
  label: string
  value: string
  testId?: string
  className?: string
}) {
  return (
    <div className="min-w-0">
      <dt className="text-caption text-fg-muted">{label}</dt>
      <dd
        className={`text-heading tabular-nums text-fg ${className}`}
        data-testid={testId}
      >
        {value}
      </dd>
    </div>
  )
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

  // The flow judged at the library's own tier (task 62). Its unrepresentable
  // list drives the downgrade banner, so the banner survives a reload.
  const flow = useQuery({
    queryKey: ['library-flow', id],
    queryFn: () => getLibraryFlow(id),
  })

  // Changing the tier never rewrites the flow (spec 6, decision 4).
  const setTier = useMutation({
    mutationFn: (tier: RuleTier) => putLibraryTier(id, tier),
    onSuccess: (res) => {
      toast.success(`Editor set to ${tierLabel(res.tier)}.`)
      void client.invalidateQueries({ queryKey: ['library', id] })
      void client.invalidateQueries({ queryKey: ['library-flow', id] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
    },
    onError: (err: unknown) => toast.failure('Changing the editor failed', err),
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

  const scan = useMutation({
    mutationFn: () => scanLibrary(id),
    onSuccess: () => {
      toast.success('Scan started.')
      refresh()
    },
    onError: (err: unknown) => toast.failure('Scan failed', err),
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

  const lib = library.data
  useDocumentTitle([lib?.name, 'Libraries'])
  const headerActions = lib ? (
    <>
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
      <button
        type="button"
        onClick={() => scan.mutate()}
        disabled={scan.isPending}
        className={btnPrimary}
        data-testid={`library-scan-${lib.id}`}
      >
        {scan.isPending ? 'Scanning…' : 'Scan now'}
      </button>
    </>
  ) : undefined

  if (library.isPending) {
    return (
      <PageContainer className="space-y-6" data-testid="library-detail">
        <PageHeader
          breadcrumb={[{ label: 'Libraries', to: '/libraries' }]}
          title={undefined}
        />
        <SkeletonRows rows={2} />
      </PageContainer>
    )
  }
  if (library.isError || !lib) {
    return (
      <PageContainer className="space-y-6" data-testid="library-detail">
        <PageHeader
          breadcrumb={[{ label: 'Libraries', to: '/libraries' }]}
          title="Library"
        />
        <ErrorBox title="Cannot load this library." error={library.error} />
      </PageContainer>
    )
  }

  const total = files.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const from = total === 0 ? 0 : page * pageSize + 1
  const to = Math.min(total, (page + 1) * pageSize)
  const failedOnPage = (files.data?.items ?? []).filter(
    (f) => f.status === 'failed',
  )
  const counts = stats.data?.libraries.find((s) => s.library_id === id)
  const anyChip = FILE_STATUSES.some((s) => (counts?.[s] ?? 0) > 0)

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
        className={btnGhost}
        aria-label={`Cancel the job for ${name}`}
        data-testid={`file-cancel-${f.id}${onCard ? '-card' : ''}`}
      >
        Cancel
      </button>
    )
  }
  const historyButton = (f: FileItem, onCard: boolean) => {
    const name = f.path.split('/').pop() ?? f.path
    return (
      <button
        type="button"
        onClick={() => setHistoryFile(f)}
        className={btnGhost}
        aria-label={`Job history for ${name}`}
        data-testid={`file-log-${f.id}${onCard ? '-card' : ''}`}
      >
        History
      </button>
    )
  }

  // The table and the cards carry the same columns (task 20). Spec 5.6:
  // File, Status, Before and after, Saved, Encoder, Why, Actions.
  const columns: Column<FileItem>[] = [
    {
      key: 'name',
      header: 'File',
      primary: true,
      cell: (f) => {
        const name = f.path.split('/').pop() ?? f.path
        const rel = relativeTo(lib.path, f.path, name)
        return (
          <span className="block min-w-0 max-w-[16rem]">
            <span className="block truncate text-fg" title={f.path}>
              {name}
            </span>
            {rel && (
              <span className="block truncate font-mono text-caption font-normal text-fg-muted">
                {rel}
              </span>
            )}
          </span>
        )
      },
      cardCell: (f) => {
        const name = f.path.split('/').pop() ?? f.path
        const rel = relativeTo(lib.path, f.path, name)
        return (
          <span className="block min-w-0 break-words" title={f.path}>
            {name}
            {rel && (
              <span className="mt-0.5 block font-mono text-caption font-normal text-fg-muted">
                {rel}
              </span>
            )}
          </span>
        )
      },
    },
    {
      key: 'status',
      header: 'Status',
      cell: (f) => <FileStatus file={f} testId={`file-status-${f.id}`} />,
      cardCell: (f) => (
        <FileStatus file={f} testId={`file-card-status-${f.id}`} />
      ),
      cardLabel: 'Status',
    },
    {
      key: 'reason',
      header: 'Why',
      cell: (f) => (
        <span
          className="break-words text-fg-muted"
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
      header: 'Before and after',
      align: 'end',
      cell: (f) => (
        <span className="inline-flex items-baseline gap-1.5 whitespace-nowrap tabular-nums">
          <span className="text-fg-muted" data-testid={`file-before-${f.id}`}>
            {formatBytes(f.size + f.bytes_saved)}
          </span>
          <span aria-hidden="true" className="text-fg-subtle">
            →
          </span>
          <span className="text-fg" data-testid={`file-after-${f.id}`}>
            {f.bytes_saved > 0 ? formatBytes(f.size) : '—'}
          </span>
        </span>
      ),
      cardLabel: 'Before and after',
      cardCell: (f) => (
        <span className="inline-flex items-baseline gap-1.5 whitespace-nowrap tabular-nums">
          <span className="text-fg-muted">
            {formatBytes(f.size + f.bytes_saved)}
          </span>
          <span aria-hidden="true" className="text-fg-subtle">
            →
          </span>
          <span className="text-fg">
            {f.bytes_saved > 0 ? formatBytes(f.size) : '—'}
          </span>
        </span>
      ),
    },
    {
      key: 'saved',
      header: 'Saved',
      align: 'end',
      cell: (f) => (
        <span className="block whitespace-nowrap tabular-nums">
          <span className="text-accent-text" data-testid={`file-saved-${f.id}`}>
            {f.bytes_saved > 0 ? formatBytes(f.bytes_saved) : '—'}
          </span>
          <span
            className="block text-caption text-fg-muted"
            data-testid={`file-ratio-${f.id}`}
          >
            {f.bytes_saved > 0
              ? `${formatRatio(f.size + f.bytes_saved, f.size)} smaller`
              : '—'}
          </span>
        </span>
      ),
      cardCell: (f) => (
        <span className="whitespace-nowrap text-accent-text">
          {formatBytes(f.bytes_saved)}
          <span className="ml-2 text-caption text-fg-muted">
            {formatRatio(f.size + f.bytes_saved, f.size)} smaller
          </span>
        </span>
      ),
      cardLabel: 'Saved',
      hideInCard: (f) => f.bytes_saved <= 0,
    },
    {
      key: 'encoder',
      header: 'Encoder',
      cell: (f) => (
        <span className="text-fg-muted" data-testid={`file-encoder-${f.id}`}>
          {f.last_job_encoder ? encoderLabel(f.last_job_encoder) : '—'}
        </span>
      ),
      cardCell: (f) => (
        <span className="text-fg-muted">
          {f.last_job_encoder ? encoderLabel(f.last_job_encoder) : '—'}
        </span>
      ),
      cardLabel: 'Encoder',
    },
    {
      key: 'actions',
      header: '',
      actions: true,
      cell: (f) => (
        <div className="flex flex-wrap justify-end gap-2">
          {historyButton(f, false)}
          {cancelButton(f, false)}
        </div>
      ),
      cardCell: (f) => (
        <div className="flex flex-wrap justify-end gap-2">
          {historyButton(f, true)}
          {cancelButton(f, true)}
        </div>
      ),
    },
  ]

  return (
    <PageContainer className="space-y-6" data-testid="library-detail">
      <PageHeader
        breadcrumb={[{ label: 'Libraries', to: '/libraries' }]}
        title={lib.name}
        meta={lib.watch ? <Badge>Watching</Badge> : undefined}
        subtitle={<span className="break-all font-mono">{lib.path}</span>}
        actions={headerActions}
      />

      {/* Summary (spec 5.6): four numbers, the last scan, and the rules in
          plain language. The status chips keep their test ids; zero hides. */}
      <Card testId="library-summary">
        <div className="flex flex-col gap-4">
          <h2 className="sr-only">Summary</h2>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <SummaryStat
              label="Saved"
              value={formatBytes(counts?.bytes_saved ?? 0)}
              testId="library-summary-saved"
              className="text-title text-accent-text"
            />
            <SummaryStat
              label="Files"
              value={counts ? formatCount(counts.files) : '—'}
              testId="library-summary-files"
            />
            <SummaryStat
              label="Waiting"
              value={counts ? formatCount(counts.queued) : '—'}
              testId="library-summary-waiting"
            />
            <SummaryStat
              label="Failed"
              value={counts ? formatCount(counts.failed) : '—'}
              testId="library-summary-failed-count"
              className={counts && counts.failed > 0 ? 'text-danger' : ''}
            />
          </dl>

          {anyChip && (
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
          )}

          <p
            className="text-caption text-fg-muted"
            data-testid="library-last-scan"
          >
            {lib.last_scan_at
              ? `Last scan: ${formatDateTime(lib.last_scan_at)}`
              : 'Not scanned yet.'}
          </p>
          {!!lib.last_scan?.skipped_kinds && (
            <p
              className="text-caption text-fg-muted"
              data-testid="library-skipped-kinds"
              title="Files whose type this library does not look for: they stay on disk untouched."
            >
              {lib.last_scan.skipped_kinds} skipped — their file type is not
              what this library looks for.
            </p>
          )}
          <NonMediaHint lib={lib} />

          <div className="space-y-2 border-t border-line pt-4">
            <h3 className="text-body font-semibold text-fg">
              Rules · {tierWord(lib.flow_source)}
            </h3>
            <div className="flex flex-col gap-2" data-testid="library-tier">
              <SegmentedControl
                label="Rule editor"
                value={lib.rule_tier ?? 'basic'}
                onChange={(t) => {
                  if (t !== (lib.rule_tier ?? 'basic')) {
                    setTier.mutate(t as RuleTier)
                  }
                }}
                options={TIER_OPTIONS.map((t) => ({
                  id: t.id,
                  label: t.label,
                  testId: `library-tier-${t.id}`,
                }))}
              />
              <p className="text-caption text-fg-muted">
                Basic: sliders. Moderate: a rule tree. Advanced: a visual graph.
                Changing it never changes the steps.
              </p>
            </div>
            {flow.data && isLocked(flow.data.unrepresentable) && (
              <UnrepresentableBanner
                tier={flow.data.tier ?? lib.rule_tier ?? 'basic'}
                steps={flow.data.unrepresentable ?? []}
                libraryId={lib.id}
              />
            )}
            {plain.data && plain.data.steps.length > 0 && (
              <ul
                className="list-disc space-y-1 pl-5 text-body text-fg"
                data-testid="library-target-plain"
              >
                {plain.data.steps.map((s) => (
                  <li key={s.id} data-testid="target-plain-line">
                    {s.plain}
                  </li>
                ))}
              </ul>
            )}
            <Link
              to={`/libraries/${lib.id}/rules`}
              className={linkTouch}
              data-testid="library-change-rules"
            >
              Change rules
            </Link>
            <Link
              to={`/libraries/${lib.id}/tree`}
              className={linkTouch}
              data-testid="library-tree-editor"
            >
              Open the rule tree
            </Link>
          </div>
        </div>
      </Card>

      {/* File list toolbar (spec 5.6): labels above their controls. */}
      <section className="space-y-4">
        <h2 className="sr-only">Files</h2>
        <div
          className="flex flex-wrap items-end gap-3"
          data-testid="file-toolbar"
        >
          <label className="flex min-w-[12rem] flex-1 flex-col gap-1 text-caption text-fg-muted">
            Search
            <input
              type="search"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder="File name…"
              className={input}
              aria-label="Search files by name"
              data-testid="file-search"
            />
          </label>
          <label className="flex flex-col gap-1 text-caption text-fg-muted">
            Status
            <select
              value={status}
              onChange={(e) => changeStatus(e.target.value)}
              className={`${select} max-w-[10rem]`}
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
          <div className="flex flex-col gap-1 text-caption text-fg-muted">
            <span>Sort</span>
            <div className="flex items-center gap-2">
              <select
                value={sort}
                onChange={(e) => changeSort(e.target.value as typeof sort)}
                className={select}
                aria-label="Sort files by"
                data-testid="file-sort"
              >
                {FILE_SORTS.map((s) => (
                  <option key={s.key} value={s.key}>
                    {s.label}
                  </option>
                ))}
              </select>
              <button
                type="button"
                onClick={flipDir}
                className={`${btnIcon} min-h-11 text-body`}
                aria-label={
                  dir === 'asc' ? 'Sort ascending' : 'Sort descending'
                }
                data-testid="file-sort-dir"
              >
                <span aria-hidden="true">{dir === 'asc' ? '↑' : '↓'}</span>
              </button>
            </div>
          </div>
          {failedOnPage.length > 0 && (
            <button
              type="button"
              onClick={() => retry.mutate()}
              disabled={retry.isPending}
              className={btn}
              data-testid="retry-failed"
            >
              {retry.isPending
                ? 'Retrying…'
                : `Retry failed (${failedOnPage.length})`}
            </button>
          )}
        </div>

        {files.isPending ? (
          <SkeletonRows />
        ) : files.isError ? (
          <ErrorBox title="Cannot load the files." error={files.error} />
        ) : files.data.total === 0 ? (
          query ? (
            <EmptyState
              testId="files-empty"
              title={`No files match “${query}”.`}
              action={
                <button
                  type="button"
                  onClick={() => setSearchText('')}
                  className={linkTouch}
                  data-testid="files-clear-search"
                >
                  Clear the search
                </button>
              }
            />
          ) : (
            <EmptyState
              testId="files-empty"
              title="No files here yet"
              body="Wait for a scan or add files to the folder."
            />
          )
        ) : (
          <>
            {/* One column definition, two layouts (task 20): the table from
                `md` up, stacked cards below it. The table wrapper scrolls
                sideways, so long paths and reasons never force the page wide. */}
            <DataList
              rows={files.data.items}
              columns={columns}
              testid="file-table"
              cardTestid="file-cards"
              cardsBelow="md"
              rowTestId={(f) => `file-row-${f.id}`}
              cardRowTestId={(f) => `file-card-${f.id}`}
            />

            {/* Pagination (task 13): server side, so 40k files never load at
                once. Spec 5.6: hidden when everything fits on one page. */}
            {total > pageSize && (
              <div
                className="flex flex-wrap items-end justify-between gap-3"
                data-testid="files-pagination"
              >
                <div className="flex flex-wrap items-end gap-3">
                  <label className="flex flex-col gap-1 text-caption text-fg-muted">
                    Per page
                    <select
                      value={pageSize}
                      onChange={(e) => changePageSize(Number(e.target.value))}
                      className={select}
                      data-testid="files-page-size"
                    >
                      {PAGE_SIZES.map((n) => (
                        <option key={n} value={n}>
                          {n}
                        </option>
                      ))}
                    </select>
                  </label>
                  <span
                    className="text-body tabular-nums text-fg-muted"
                    data-testid="files-page-info"
                  >
                    {from}–{to} of {total}
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
            )}
          </>
        )}
      </section>

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
            <span className="break-all text-fg">
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
