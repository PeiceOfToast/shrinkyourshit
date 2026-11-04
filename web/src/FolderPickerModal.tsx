// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Folder picker modal (task 07, restyled in task 41): server-side directory
 * listing via /api/v1/fs/browse with breadcrumbs and directories only.
 *
 * Task 13: it is a native <dialog> — Escape closes it and focus returns to
 * the trigger (both come from ModalDialog) — and the footer shows the
 * current path in full on hover with a middle-truncated display so long NAS
 * paths stay readable at either end.
 */
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { browse } from './api'
import ModalDialog from './dialog'
import { crumbs, truncateMiddle } from './easyModel'
import ErrorBox from './ErrorBox'
import Icon from './Icon'
import { SkeletonRows } from './skeleton'
import { btnGhost, btnIcon, btnPrimary } from './uiControls'

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
      size="lg"
      dismissOnBackdrop
    >
      <div className="flex items-start justify-between gap-4 px-6 pt-6">
        <h2 className="text-heading text-fg">Choose a folder</h2>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className={`${btnIcon} -mr-2 -mt-2`}
          data-testid="folder-picker-close"
        >
          <Icon name="close" className="h-5 w-5" />
        </button>
      </div>

      {/* The breadcrumb path: each segment is a button, the last is current. */}
      <nav
        aria-label="Folder path"
        className="flex flex-wrap items-center gap-1 px-6 pt-4 font-mono text-body text-fg-muted"
        data-testid="folder-picker-crumbs"
      >
        {crumbs(path).map((crumb, i, all) => (
          <span key={crumb.path} className="flex items-center gap-1">
            {i > 0 && (
              <span aria-hidden="true" className="text-fg-subtle">
                /
              </span>
            )}
            <button
              type="button"
              onClick={() => setPath(crumb.path)}
              aria-current={i === all.length - 1 ? 'step' : undefined}
              className={`inline-flex min-h-11 items-center rounded-md px-1 transition-colors duration-fast ease-standard hover:text-fg ${
                i === all.length - 1 ? 'font-medium text-fg' : ''
              }`}
            >
              {crumb.name}
            </button>
          </span>
        ))}
      </nav>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
        {query.isPending && <SkeletonRows rows={3} />}
        {query.isError && (
          <ErrorBox
            title="Cannot list this folder."
            error={query.error}
            testId="folder-picker-error"
          />
        )}
        <ul className="divide-y divide-line">
          {query.data && query.data.parent && (
            <li>
              <button
                type="button"
                onClick={() => setPath(query.data.parent)}
                className="flex min-h-11 w-full items-center gap-3 text-left text-body text-fg-muted transition-colors duration-fast ease-standard hover:bg-raised/40 hover:text-fg"
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
                className="flex min-h-11 w-full items-center gap-3 text-left text-body text-fg transition-colors duration-fast ease-standard hover:bg-raised/40"
                data-testid={`folder-picker-dir-${entry.name}`}
              >
                <Icon
                  name="folder"
                  className="h-5 w-5 shrink-0 text-fg-muted"
                />
                {entry.name}
              </button>
            </li>
          ))}
          {query.data && dirs.length === 0 && (
            <li className="flex min-h-11 items-center text-body text-fg-muted">
              No subfolders here.
            </li>
          )}
        </ul>
      </div>

      {/* The full path stays reachable (hover/tooltip); the visible text is
          cut in the middle only, so both ends remain readable. */}
      <div className="flex flex-col gap-3 border-t border-line px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
        <span
          className="min-w-0 truncate font-mono text-caption text-fg-muted"
          title={path}
          data-testid="folder-picker-current"
        >
          {truncateMiddle(path)}
        </span>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className={btnGhost}
            data-testid="folder-picker-cancel"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onPick(path)}
            className={btnPrimary}
            data-testid="folder-picker-use"
          >
            Use this folder
          </button>
        </div>
      </div>
    </ModalDialog>
  )
}
