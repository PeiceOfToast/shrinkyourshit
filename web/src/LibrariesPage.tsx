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
import { formatBytes } from './easyModel'
import ErrorBox from './ErrorBox'
import PageContainer from './PageContainer'
import StatusChip from './StatusChip'
import { SkeletonCard } from './skeleton'
import { useToast } from './toastContext'

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
      className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700 disabled:opacity-50"
      data-testid={`library-scan-${lib.id}${onCard ? '-card' : ''}`}
    >
      {scan.isPending ? 'Scanning…' : 'Scan now'}
    </button>
  )
  const editLink = (lib: Library, onCard: boolean) => (
    <Link
      to={`/libraries/${lib.id}/edit`}
      className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700"
      data-testid={`library-edit-${lib.id}${onCard ? '-card' : ''}`}
    >
      Edit
    </Link>
  )
  const removeButton = (lib: Library, onCard: boolean) => (
    <button
      type="button"
      onClick={() => setToDelete(lib)}
      className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm text-red-400 hover:bg-slate-700"
      data-testid={`library-delete-${lib.id}${onCard ? '-card' : ''}`}
    >
      Remove
    </button>
  )

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
          className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
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

      {/* From `sm` up: the real table, scrollable so nothing forces the
          page wide. */}
      <div className="hidden sm:block">
        <div className="overflow-x-auto rounded-2xl ring-1 ring-slate-800">
          <table
            className="w-full text-left text-sm"
            data-testid="libraries-table"
          >
            <thead className="bg-slate-900 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-3">Library</th>
                <th className="px-4 py-3">Files</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Saved</th>
                <th className="px-4 py-3">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800 bg-slate-900/50">
              {libraries.data.items.map((lib) => {
                const counts = countsFor(lib)
                return (
                  <tr key={lib.id} data-testid={`library-${lib.id}`}>
                    <td className="max-w-[14rem] px-4 py-3">
                      <Link
                        to={`/libraries/${lib.id}`}
                        className="font-semibold text-slate-100 hover:text-emerald-400"
                        data-testid={`library-name-${lib.id}`}
                      >
                        {lib.name}
                      </Link>
                      <p className="truncate text-xs text-slate-500">
                        {lib.path}
                      </p>
                    </td>
                    <td className="px-4 py-3 text-slate-300">
                      <span data-testid={`library-files-${lib.id}`}>
                        {counts?.files ?? 0}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex max-w-[18rem] flex-wrap gap-1.5">
                        {SHOWN_STATUSES.map((s) => (
                          <StatusChip
                            key={s}
                            status={s}
                            count={counts?.[s] ?? 0}
                            testId={`library-chip-${s}-${lib.id}`}
                          />
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-emerald-400">
                      <span data-testid={`library-saved-${lib.id}`}>
                        {formatBytes(counts?.bytes_saved ?? 0)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        {scanButton(lib, false)}
                        {editLink(lib, false)}
                        {removeButton(lib, false)}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* Under `sm`: the same rows as stacked cards. */}
      <ul className="space-y-3 sm:hidden" data-testid="libraries-cards">
        {libraries.data.items.map((lib) => {
          const counts = countsFor(lib)
          return (
            <li
              key={lib.id}
              className="min-w-0 rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
              data-testid={`library-card-${lib.id}`}
            >
              <div className="flex items-baseline justify-between gap-3">
                <Link
                  to={`/libraries/${lib.id}`}
                  className="min-w-0 break-words font-semibold text-slate-100 hover:text-emerald-400"
                  data-testid={`library-name-${lib.id}-card`}
                >
                  {lib.name}
                </Link>
                <span className="shrink-0 text-sm text-slate-400">
                  <span data-testid={`library-files-${lib.id}-card`}>
                    {counts?.files ?? 0}
                  </span>{' '}
                  file{(counts?.files ?? 0) === 1 ? '' : 's'}
                </span>
              </div>
              <p className="mt-0.5 break-words text-xs text-slate-500">
                {lib.path}
              </p>
              <div className="mt-3 flex flex-wrap gap-1.5">
                {SHOWN_STATUSES.map((s) => (
                  <StatusChip
                    key={s}
                    status={s}
                    count={counts?.[s] ?? 0}
                    testId={`library-chip-${s}-${lib.id}-card`}
                  />
                ))}
              </div>
              <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                <span
                  className="text-sm text-emerald-400"
                  data-testid={`library-saved-${lib.id}-card`}
                >
                  {formatBytes(counts?.bytes_saved ?? 0)} saved
                </span>
                <div className="flex shrink-0 gap-2">
                  {scanButton(lib, true)}
                  {editLink(lib, true)}
                  {removeButton(lib, true)}
                </div>
              </div>
            </li>
          )
        })}
      </ul>

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
