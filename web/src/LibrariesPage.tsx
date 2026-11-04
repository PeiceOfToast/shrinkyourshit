// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list (tasks 07, 19 and 43). One row per library: its name and
 * path, a status column with only the non-zero chips (or one "Up to date"
 * badge), the file count, the bytes saved, what it looks for, and the row
 * actions. The whole row links to the library's detail page (spec 5.4).
 *
 * The table is used from `sm` up and stacked cards below it (DataList, one
 * column definition, two layouts). On a card the status column shows only
 * the most urgent chip: failed, then missing, working, waiting, new, and
 * otherwise "Up to date" when every file is satisfied or done.
 *
 * Remove is not on this list: it lives in the Danger zone of the Edit page
 * (spec 5.7, task 47), so Scan now and Edit are the only row actions.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  getLibraries,
  getStats,
  scanLibrary,
  type Library,
  type LibraryStats,
} from './api'
import Badge from './Badge'
import Card from './Card'
import DataList, { type Column } from './DataList'
import EmptyState from './EmptyState'
import ErrorBox from './ErrorBox'
import { formatBytes, formatCount } from './format'
import { kindLabel } from './kinds'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import StatusChip from './StatusChip'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnGhost, btnPrimary } from './uiControls'

type StatusKey =
  'new' | 'satisfied' | 'queued' | 'processing' | 'done' | 'failed' | 'missing'

/** The status chips a row can show, in table order. Zero counts render nothing. */
const CHIP_STATUSES: StatusKey[] = [
  'new',
  'satisfied',
  'queued',
  'processing',
  'done',
  'failed',
  'missing',
]

/** The one chip a card shows, most urgent first. */
const URGENT_STATUSES: StatusKey[] = [
  'failed',
  'missing',
  'processing',
  'queued',
  'new',
]

/** True when the library has files and every one is satisfied or done. */
function isUpToDate(c: LibraryStats | undefined): boolean {
  if (!c || c.files === 0) return false
  return c.satisfied + c.done === c.files
}

export default function LibrariesPage() {
  useDocumentTitle(['Libraries'])
  const client = useQueryClient()
  const toast = useToast()
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['libraries'] })
    void client.invalidateQueries({ queryKey: ['stats'] })
  }

  const scan = useMutation({
    mutationFn: scanLibrary,
    onSuccess: () => {
      toast.success('Scan started.')
      refresh()
    },
    onError: (err: unknown) => toast.failure('Scan failed', err),
  })

  const countsFor = (lib: Library): LibraryStats | undefined =>
    stats.data?.libraries.find((s) => s.library_id === lib.id)

  const kindsText = (lib: Library) =>
    (lib.file_kinds ?? ['video']).map(kindLabel).join(', ')

  /** Every non-zero status as a chip, in a wrapping row. */
  const chipsFor = (lib: Library, suffix: string) => {
    const c = countsFor(lib)
    return (
      <div className="flex flex-wrap gap-1">
        {CHIP_STATUSES.map((s) => (
          <StatusChip
            key={s}
            status={s}
            count={c?.[s] ?? 0}
            testId={`library-chip-${s}-${lib.id}${suffix}`}
          />
        ))}
        {isUpToDate(c) && (
          <Badge tone="success" testId={`library-uptodate-${lib.id}${suffix}`}>
            Up to date
          </Badge>
        )}
      </div>
    )
  }

  /** The card badge: the single most urgent chip, or "Up to date". */
  const urgentFor = (lib: Library, suffix: string) => {
    const c = countsFor(lib)
    const urgent = URGENT_STATUSES.find((s) => (c?.[s] ?? 0) > 0)
    if (urgent) {
      return (
        <StatusChip
          status={urgent}
          count={c?.[urgent] ?? 0}
          testId={`library-chip-${urgent}-${lib.id}${suffix}`}
        />
      )
    }
    if (isUpToDate(c)) {
      return (
        <Badge tone="success" testId={`library-uptodate-${lib.id}${suffix}`}>
          Up to date
        </Badge>
      )
    }
    return null
  }

  const scanButton = (lib: Library, onCard: boolean) => (
    <button
      type="button"
      onClick={() => scan.mutate(lib.id)}
      disabled={scan.isPending}
      className={btn}
      data-testid={`library-scan-${lib.id}${onCard ? '-card' : ''}`}
    >
      {scan.isPending ? 'Scanning…' : 'Scan now'}
    </button>
  )
  const editLink = (lib: Library, onCard: boolean) => (
    <Link
      to={`/libraries/${lib.id}/edit`}
      className={btnGhost}
      data-testid={`library-edit-${lib.id}${onCard ? '-card' : ''}`}
    >
      Edit
    </Link>
  )

  /** The name over the mono path. The row link wraps it (DataList rowHref). */
  const nameAndPath = (lib: Library, suffix: string) => (
    <>
      <span
        className="block break-words"
        data-testid={`library-name-${lib.id}${suffix}`}
      >
        {lib.name}
      </span>
      <span
        className="mt-0.5 block max-w-[28rem] truncate font-mono text-caption font-normal text-fg-muted"
        title={lib.path}
      >
        {lib.path}
      </span>
    </>
  )

  const columns: Column<Library>[] = [
    {
      key: 'name',
      header: 'Library',
      primary: true,
      cell: (lib) => nameAndPath(lib, ''),
      cardCell: (lib) => nameAndPath(lib, '-card'),
    },
    {
      key: 'status',
      header: 'Status',
      cardPlacement: 'badge',
      cell: (lib) => chipsFor(lib, ''),
      cardCell: (lib) => urgentFor(lib, '-card'),
    },
    {
      key: 'files',
      header: 'Files',
      align: 'end',
      cell: (lib) => (
        <span className="text-fg" data-testid={`library-files-${lib.id}`}>
          {formatCount(countsFor(lib)?.files ?? 0)}
        </span>
      ),
      cardCell: (lib) => (
        <span className="text-fg" data-testid={`library-files-${lib.id}-card`}>
          {formatCount(countsFor(lib)?.files ?? 0)}
        </span>
      ),
      cardLabel: 'Files',
    },
    {
      key: 'saved',
      header: 'Saved',
      align: 'end',
      cell: (lib) => (
        <span
          className="text-accent-text"
          data-testid={`library-saved-${lib.id}`}
        >
          {formatBytes(countsFor(lib)?.bytes_saved ?? 0)}
        </span>
      ),
      cardCell: (lib) => (
        <span
          className="whitespace-nowrap text-accent-text"
          data-testid={`library-saved-${lib.id}-card`}
        >
          {formatBytes(countsFor(lib)?.bytes_saved ?? 0)}
        </span>
      ),
      cardLabel: 'Saved',
    },
    {
      key: 'kinds',
      header: 'Looking for',
      cell: (lib) => (
        <span className="text-fg-muted" data-testid={`library-kinds-${lib.id}`}>
          {kindsText(lib)}
        </span>
      ),
      cardCell: (lib) => (
        <span
          className="text-fg-muted"
          data-testid={`library-kinds-${lib.id}-card`}
        >
          {kindsText(lib)}
        </span>
      ),
      cardLabel: 'Looking for',
    },
    {
      key: 'actions',
      header: '',
      actions: true,
      cell: (lib) => (
        <>
          {scanButton(lib, false)}
          {editLink(lib, false)}
        </>
      ),
      cardCell: (lib) => (
        <>
          {scanButton(lib, true)}
          {editLink(lib, true)}
        </>
      ),
    },
  ]

  const header = (
    <PageHeader
      title="Libraries"
      actions={
        <Link
          to="/libraries/new"
          className={btnPrimary}
          data-testid="libraries-add"
        >
          Add library
        </Link>
      }
    />
  )

  if (libraries.isPending) {
    return (
      <PageContainer data-testid="libraries-page">
        <PageHeader title="Libraries" />
        <Card padding="none">
          <SkeletonRows rows={3} />
        </Card>
      </PageContainer>
    )
  }
  if (libraries.isError) {
    return (
      <PageContainer data-testid="libraries-page">
        <PageHeader title="Libraries" />
        <ErrorBox
          title="Cannot load the libraries."
          error={libraries.error}
          onRetry={() => void libraries.refetch()}
        />
      </PageContainer>
    )
  }

  return (
    <PageContainer data-testid="libraries-page">
      {header}

      {/* One column definition, two layouts (task 20): the table from
          `sm` up, stacked cards below it. The whole row links to the
          library's detail page. */}
      <DataList
        rows={libraries.data.items}
        columns={columns}
        testid="libraries-table"
        cardTestid="libraries-cards"
        rowTestId={(lib) => `library-${lib.id}`}
        cardRowTestId={(lib) => `library-card-${lib.id}`}
        rowHref={(lib) => `/libraries/${lib.id}`}
        empty={
          <EmptyState
            icon="library"
            testId="libraries-empty"
            title="No libraries yet"
            body="A library is a folder ShrinkYourShit watches. Add one and it starts looking for files to shrink."
            action={
              <Link to="/libraries/new" className={btnPrimary}>
                Add library
              </Link>
            }
          />
        }
      />
    </PageContainer>
  )
}
