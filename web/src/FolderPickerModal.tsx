// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Folder picker modal (task 07): server-side directory listing via
 * /api/v1/fs/browse with breadcrumbs and directories only.
 *
 * Task 13: it is a native <dialog> now — Escape closes it and focus
 * returns to the trigger (both come from ModalDialog) — and the footer
 * shows the current path in full on hover with a middle-truncated display
 * so long NAS paths stay readable at either end.
 */
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { browse } from './api'
import ModalDialog from './dialog'
import { crumbs, truncateMiddle } from './easyModel'
import { SkeletonRows } from './skeleton'
import ErrorBox from './ErrorBox'

export default function FolderPickerModal({
  initialPath,
  onClose,
  onPick,
}: {
  initialPath?: string
  onClose: () => void
  onPick: (path: string) => void
}) {
  const [path, setPath] = useState(
    initialPath && initialPath.startsWith('/') ? initialPath : '/',
  )
  const query = useQuery({
    queryKey: ['browse', path],
    queryFn: () => browse(path),
  })

  const dirs = (query.data?.entries ?? []).filter(
    (e) => e.is_dir && !e.name.startsWith('.'),
  )

  return (
    <ModalDialog
      onClose={onClose}
      label="Choose a folder"
      testId="folder-picker"
      dismissOnBackdrop
      className="m-auto flex max-h-[80vh] w-full max-w-xl flex-col rounded-2xl bg-slate-900 text-slate-100 shadow-xl ring-1 ring-slate-800"
    >
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
        <h2 className="font-semibold">Choose a folder</h2>
        <button
          type="button"
          onClick={onClose}
          className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          data-testid="folder-picker-close"
        >
          Close
        </button>
      </div>

      <div
        className="flex flex-wrap items-center gap-1 border-b border-slate-800 px-4 py-2 text-sm"
        data-testid="folder-picker-crumbs"
      >
        {crumbs(path).map((crumb, i, all) => (
          <span key={crumb.path} className="flex items-center gap-1">
            {i > 0 && <span className="text-slate-600">/</span>}
            <button
              type="button"
              onClick={() => setPath(crumb.path)}
              className={`rounded px-1 hover:bg-slate-800 ${
                i === all.length - 1
                  ? 'font-semibold text-emerald-400'
                  : 'text-slate-300'
              }`}
            >
              {crumb.name}
            </button>
          </span>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {query.isPending && <SkeletonRows rows={3} />}
        {query.isError && (
          <ErrorBox
            title="Cannot list this folder."
            error={query.error}
            testId="folder-picker-error"
          />
        )}
        <ul>
          {query.data && query.data.parent && (
            <li>
              <button
                type="button"
                onClick={() => setPath(query.data.parent)}
                className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm text-slate-300 hover:bg-slate-800"
                data-testid="folder-picker-up"
              >
                ↑ Up one level
              </button>
            </li>
          )}
          {dirs.map((entry) => (
            <li key={entry.path}>
              <button
                type="button"
                onClick={() => setPath(entry.path)}
                className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm text-slate-200 hover:bg-slate-800"
                data-testid={`folder-picker-dir-${entry.name}`}
              >
                <span aria-hidden>📁</span>
                {entry.name}
              </button>
            </li>
          ))}
          {query.data && dirs.length === 0 && (
            <li className="px-3 py-2 text-sm text-slate-500">
              No subfolders here.
            </li>
          )}
        </ul>
      </div>

      <div className="flex items-center justify-between gap-2 border-t border-slate-800 px-4 py-3">
        {/* The full path stays reachable (hover/tooltip); the visible text
            is cut in the middle only, so both ends remain readable. */}
        <span
          className="min-w-0 flex-1 truncate text-sm text-slate-400"
          title={path}
          data-testid="folder-picker-current"
        >
          {truncateMiddle(path)}
        </span>
        <button
          type="button"
          onClick={() => onPick(path)}
          className="shrink-0 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
          data-testid="folder-picker-use"
        >
          Use this folder
        </button>
      </div>
    </ModalDialog>
  )
}
