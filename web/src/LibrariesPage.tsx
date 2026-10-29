// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list (task 07): name, path, per-status counts, space saved,
 * scan-now, edit and a delete confirmation that makes clear the files on
 * disk are untouched.
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
import { formatBytes } from './easyModel'
import PageContainer from './PageContainer'

function ConfirmDelete({
  lib,
  busy,
  onConfirm,
  onCancel,
}: {
  lib: Library
  busy: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-label="Remove library"
      data-testid="confirm-delete"
    >
      <div className="w-full max-w-md rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="text-lg font-semibold">Remove “{lib.name}”?</h2>
        <p className="mt-2 text-sm text-slate-400">
          This removes the folder from ShrinkYourShit and forgets what it has
          done there. The files in{' '}
          <span className="text-slate-200">{lib.path}</span> are not touched —
          nothing on disk is deleted or renamed.
        </p>
        <div className="mt-4 flex gap-2">
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
            data-testid="confirm-delete-yes"
          >
            {busy ? 'Removing…' : 'Remove library'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"
            data-testid="confirm-delete-cancel"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}

export default function LibrariesPage() {
  const client = useQueryClient()
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })
  const [toDelete, setToDelete] = useState<Library | null>(null)

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['libraries'] })
    void client.invalidateQueries({ queryKey: ['stats'] })
  }

  const scan = useMutation({
    mutationFn: scanLibrary,
    onSuccess: refresh,
  })
  const del = useMutation({
    mutationFn: deleteLibrary,
    onSuccess: () => {
      setToDelete(null)
      refresh()
    },
  })

  const countsFor = (lib: Library): LibraryStats | undefined =>
    stats.data?.libraries.find((s) => s.library_id === lib.id)

  if (libraries.isPending) {
    return <p className="text-slate-400">Loading libraries…</p>
  }
  if (libraries.isError) {
    return <p className="text-red-400">Cannot load the libraries.</p>
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
                    Scan now
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
        <ConfirmDelete
          lib={toDelete}
          busy={del.isPending}
          onConfirm={() => del.mutate(toDelete.id)}
          onCancel={() => setToDelete(null)}
        />
      )}
    </PageContainer>
  )
}
