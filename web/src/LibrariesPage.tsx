// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list (tasks 07 and 19): one row per library with the file
 * counts and bytes saved from the per-library breakdown of GET /stats,
 * status chips, a Scan now action, and the name linking to the detail
 * page. From `sm` up it is a real table; under `sm` the same data renders
 * as stacked cards (the task 12 pattern: both trees stay in the DOM, CSS
 * picks the visible one). Delete keeps its task 13 confirmation that only
 * the library's records are removed — files on disk are untouched.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  deleteLibrary,
  getLibraries,
  getStats,
  scanLibrary,
  type Library,
  type LibraryStats,
} from './api'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import { formatBytes } from './easyModel'
import ErrorBox from './ErrorBox'
import PageContainer from './PageContainer'
import StatusChip from './StatusChip'
import { SkeletonCard } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnPrimary } from './uiControls'

/** The per-status chips a row shows, in table order. */
const SHOWN_STATUSES = ['satisfied', 'queued', 'done', 'failed'] as const

export default function LibrariesPage() {
  const client = useQueryClient()
  const toast = useToast()
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })
  const [toDelete, setToDelete] = useState<Library | null>(null)

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
  const del = useMutation({
    mutationFn: deleteLibrary,
    onSuccess: () => {
      setToDelete(null)
      toast.success('Library removed. The files on disk are untouched.')
      refresh()
    },
    onError: (err: unknown) =>
      toast.failure('Removing the library failed', err),
  })

  const countsFor = (lib: Library): LibraryStats | undefined =>
    stats.data?.libraries.find((s) => s.library_id === lib.id)

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
      className={btn}
      data-testid={`library-edit-${lib.id}${onCard ? '-card' : ''}`}
    >
      Edit
    </Link>
  )
  const removeButton = (lib: Library, onCard: boolean) => (
    <button
      type="button"
      onClick={() => setToDelete(lib)}
      className={`${btn} !text-red-300 hover:!bg-red-950/40`}
      data-testid={`library-delete-${lib.id}${onCard ? '-card' : ''}`}
    >
      Remove
    </button>
  )

  const columns: Column<Library>[] = [
    {
      key: 'name',
      header: 'Library',
      primary: true,
      cell: (lib) => (
        <Link
          to={`/libraries/${lib.id}`}
          className="inline-flex min-h-11 items-center font-semibold text-slate-100 hover:text-emerald-400"
          data-testid={`library-name-${lib.id}`}
        >
          {lib.name}
        </Link>
      ),
      cardCell: (lib) => (
        <Link
          to={`/libraries/${lib.id}`}
          className="inline-flex min-h-11 items-center break-words font-semibold text-slate-100 hover:text-emerald-400"
          data-testid={`library-name-${lib.id}-card`}
        >
          {lib.name}
        </Link>
      ),
    },
    {
      key: 'path',
      header: 'Path',
      cell: (lib) => (
        <span
          className="block max-w-[18rem] truncate text-xs text-slate-500"
          title={lib.path}
        >
          {lib.path}
        </span>
      ),
      cardCell: (lib) => (
        <span className="break-words text-xs text-slate-500">{lib.path}</span>
      ),
      cardLabel: 'Path',
    },
    {
      key: 'files',
      header: 'Files',
      cell: (lib) => (
        <span
          className="text-slate-300"
          data-testid={`library-files-${lib.id}`}
        >
          {countsFor(lib)?.files ?? 0}
        </span>
      ),
      cardCell: (lib) => (
        <span
          className="text-slate-300"
          data-testid={`library-files-${lib.id}-card`}
        >
          {countsFor(lib)?.files ?? 0}
        </span>
      ),
      cardLabel: 'Files',
    },
    {
      key: 'status',
      header: 'Status',
      cell: (lib) => (
        <div className="flex max-w-[18rem] flex-wrap gap-1.5">
          {SHOWN_STATUSES.map((s) => (
            <StatusChip
              key={s}
              status={s}
              count={countsFor(lib)?.[s] ?? 0}
              testId={`library-chip-${s}-${lib.id}`}
            />
          ))}
        </div>
      ),
      cardCell: (lib) => (
        <div className="flex flex-wrap justify-end gap-1.5">
          {SHOWN_STATUSES.map((s) => (
            <StatusChip
              key={s}
              status={s}
              count={countsFor(lib)?.[s] ?? 0}
              testId={`library-chip-${s}-${lib.id}-card`}
            />
          ))}
        </div>
      ),
      cardLabel: 'Status',
    },
    {
      key: 'saved',
      header: 'Saved',
      cell: (lib) => (
        <span
          className="text-emerald-400"
          data-testid={`library-saved-${lib.id}`}
        >
          {formatBytes(countsFor(lib)?.bytes_saved ?? 0)}
        </span>
      ),
      cardCell: (lib) => (
        <span
          className="whitespace-nowrap text-emerald-400"
          data-testid={`library-saved-${lib.id}-card`}
        >
          {formatBytes(countsFor(lib)?.bytes_saved ?? 0)} saved
        </span>
      ),
      cardLabel: 'Saved',
    },
    {
      key: 'actions',
      header: '',
      cell: (lib) => (
        <div className="flex flex-wrap justify-end gap-2">
          {scanButton(lib, false)}
          {editLink(lib, false)}
          {removeButton(lib, false)}
        </div>
      ),
      cardCell: (lib) => (
        <div className="flex flex-wrap justify-end gap-2">
          {scanButton(lib, true)}
          {editLink(lib, true)}
          {removeButton(lib, true)}
        </div>
      ),
      cardNoLabel: true,
    },
  ]

  if (libraries.isPending) {
    return (
      <PageContainer className="space-y-4" data-testid="libraries-page">
        <h1 className="text-2xl font-bold">Libraries</h1>
        <SkeletonCard className="h-20" />
        <SkeletonCard className="h-20" />
      </PageContainer>
    )
  }
  if (libraries.isError) {
    return (
      <PageContainer className="space-y-4" data-testid="libraries-page">
        <h1 className="text-2xl font-bold">Libraries</h1>
        <ErrorBox title="Cannot load the libraries." error={libraries.error} />
      </PageContainer>
    )
  }

  return (
    <PageContainer className="space-y-4" data-testid="libraries-page">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Libraries</h1>
        <Link
          to="/libraries/new"
          className={btnPrimary}
          data-testid="libraries-add"
        >
          Add folder
        </Link>
      </div>

      {libraries.data.total === 0 && (
        <p className="text-slate-400" data-testid="libraries-empty">
          No libraries yet.{' '}
          <Link to="/libraries/new" className="text-emerald-400 underline">
            Add your first folder
          </Link>
          .
        </p>
      )}

      {/* One column definition, two layouts (task 20): the table from
          `sm` up, stacked cards below it. */}
      <DataList
        rows={libraries.data.items}
        columns={columns}
        testid="libraries-table"
        cardTestid="libraries-cards"
        rowTestId={(lib) => `library-${lib.id}`}
        cardRowTestId={(lib) => `library-card-${lib.id}`}
      />

      {toDelete && (
        <ConfirmDialog
          title={`Remove “${toDelete.name}”?`}
          confirmLabel="Remove library"
          busy={del.isPending}
          onConfirm={() => del.mutate(toDelete.id)}
          onClose={() => setToDelete(null)}
          testId="confirm-delete"
          confirmTestId="confirm-delete-yes"
          cancelTestId="confirm-delete-cancel"
        >
          <p>
            This removes the library and its job records from the database and
            forgets what ShrinkYourShit did there. The files in{' '}
            <span className="text-slate-100">{toDelete.path}</span> are not
            touched — nothing on disk is deleted or renamed.
          </p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
