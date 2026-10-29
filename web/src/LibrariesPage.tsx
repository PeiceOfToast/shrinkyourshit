// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list (task 07): name, path, per-status counts, space saved,
 * scan-now, and a delete confirmation that states plainly that only the
 * library's records are removed and the files on disk are untouched
 * (task 13 dialog). Every mutation toasts; load failures use ErrorBox and
 * the list loads behind skeletons.
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
import { SkeletonCard } from './skeleton'
import { useToast } from './toastContext'

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

      <ul className="space-y-3">
        {libraries.data.items.map((lib) => {
          const counts = countsFor(lib)
          return (
            <li
              key={lib.id}
              className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
              data-testid={`library-${lib.id}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <Link
                    to={`/libraries/${lib.id}`}
                    className="font-semibold text-slate-100 hover:text-emerald-400"
                  >
                    {lib.name}
                  </Link>
                  <p className="truncate text-sm text-slate-500">{lib.path}</p>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => scan.mutate(lib.id)}
                    disabled={scan.isPending}
                    className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700 disabled:opacity-50"
                    data-testid={`library-scan-${lib.id}`}
                  >
                    {scan.isPending ? 'Scanning…' : 'Scan now'}
                  </button>
                  <Link
                    to={`/libraries/${lib.id}/edit`}
                    className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm hover:bg-slate-700"
                    data-testid={`library-edit-${lib.id}`}
                  >
                    Edit
                  </Link>
                  <button
                    type="button"
                    onClick={() => setToDelete(lib)}
                    className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm text-red-400 hover:bg-slate-700"
                    data-testid={`library-delete-${lib.id}`}
                  >
                    Remove
                  </button>
                </div>
              </div>
              <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-sm text-slate-400">
                <span data-testid={`library-counts-${lib.id}`}>
                  {counts?.satisfied ?? 0} nothing to do · {counts?.queued ?? 0}{' '}
                  waiting · {counts?.done ?? 0} done · {counts?.failed ?? 0}{' '}
                  failed
                </span>
                <span
                  className="text-emerald-400"
                  data-testid={`library-saved-${lib.id}`}
                >
                  {formatBytes(counts?.bytes_saved ?? 0)} saved
                </span>
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
