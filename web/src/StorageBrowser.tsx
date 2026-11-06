// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage browser (task 69, spec 4.6): a folder picker for one network share.
 * It lists folders only, never files, contents or previews. The share's name
 * is the top breadcrumb and the walk never climbs above its mountpoint. A
 * folder the server cannot read is an error, not an empty list.
 */
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ApiError, browseStorage, type Storage } from './api'
import ModalDialog from './dialog'
import ErrorBox from './ErrorBox'
import Icon from './Icon'
import { SkeletonRows } from './skeleton'
import {
  freeSpaceText,
  isInsideStorage,
  storageBrowseKey,
  storageCrumbs,
} from './storageModel'
import { truncateMiddle } from './easyModel'
import { btnGhost, btnIcon, btnPrimary } from './uiControls'

export default function StorageBrowser({
  storage,
  initialPath,
  onClose,
  onPick,
}: {
  storage: Storage
  initialPath?: string
  onClose: () => void
  onPick: (path: string) => void
}) {
  const [path, setPath] = useState(
    initialPath && isInsideStorage(initialPath, storage.mountpoint)
      ? initialPath
      : storage.mountpoint,
  )
  const query = useQuery({
    queryKey: storageBrowseKey(storage.id, path),
    queryFn: () => browseStorage(storage.id, path),
    retry: false,
  })

  const listing = query.data
  // Folders only, and dot folders are hidden as in the server folder picker.
  const dirs = (listing?.entries ?? []).filter(
    (e) => e.is_dir && !e.name.startsWith('.'),
  )
  const loginOff =
    query.error instanceof ApiError && query.error.code === 'login_required'
  const notMounted =
    query.error instanceof ApiError && query.error.code === 'not_mounted'
  const space = listing
    ? freeSpaceText(listing.free_bytes, listing.total_bytes)
    : ''

  return (
    <ModalDialog
      onClose={onClose}
      label={`Choose a folder in ${storage.name}`}
      testId="storage-browser"
      size="lg"
      dismissOnBackdrop
    >
      <div className="flex items-start justify-between gap-4 px-6 pt-6">
        <div className="min-w-0">
          <h2 className="text-heading text-fg">
            Choose a folder in {storage.name}
          </h2>
          <p className="mt-1 flex flex-wrap items-center gap-2 text-caption text-fg-muted">
            <span
              className="rounded-control bg-raised px-2 py-0.5"
              data-testid="storage-browser-badge"
            >
              {storage.mounted ? 'Mounted' : 'Not mounted'}
            </span>
            {space && <span data-testid="storage-browser-space">{space}</span>}
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className={`${btnIcon} -mr-2 -mt-2`}
          data-testid="storage-browser-close"
        >
          <Icon name="close" className="h-5 w-5" />
        </button>
      </div>

      {/* Breadcrumbs: the share's name, then each folder below it. */}
      <nav
        aria-label="Folder path"
        className="flex flex-wrap items-center gap-1 px-6 pt-4 font-mono text-body text-fg-muted"
        data-testid="storage-browser-crumbs"
      >
        {storageCrumbs(path, storage).map((crumb, i, all) => (
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
        {query.isError && loginOff && (
          <div
            className="rounded-card bg-raised p-4 text-body text-fg"
            role="alert"
            data-testid="storage-browser-login"
          >
            <p className="font-semibold">Browsing a storage needs login.</p>
            <p className="mt-1 text-fg-muted">
              Turn on login in{' '}
              <a
                href="/app"
                className="text-accent-text underline-offset-4 hover:underline"
              >
                App settings
              </a>{' '}
              first. Until then this folder list stays closed.
            </p>
          </div>
        )}
        {query.isError && !loginOff && notMounted && (
          <div
            className="rounded-card bg-raised p-4 text-body text-fg"
            role="alert"
            data-testid="storage-browser-unmounted"
          >
            <p className="font-semibold">{storage.name} is not mounted.</p>
            <p className="mt-1 text-fg-muted">
              Its folders cannot be listed until it is mounted.{' '}
              <a
                href="/storage"
                className="text-accent-text underline-offset-4 hover:underline"
              >
                Open the Storage page
              </a>
              .
            </p>
          </div>
        )}
        {query.isError && !loginOff && !notMounted && (
          <ErrorBox
            title="Cannot list this folder."
            error={query.error}
            testId="storage-browser-error"
          />
        )}
        {listing && (
          <ul className="divide-y divide-line">
            {listing.parent && (
              <li>
                <button
                  type="button"
                  onClick={() => setPath(listing.parent)}
                  className="flex min-h-11 w-full items-center gap-3 text-left text-body text-fg-muted transition-colors duration-fast ease-standard hover:bg-raised/40 hover:text-fg"
                  data-testid="storage-browser-up"
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
                  data-testid={`storage-browser-dir-${entry.name}`}
                >
                  <Icon
                    name="folder"
                    className="h-5 w-5 shrink-0 text-fg-muted"
                  />
                  {entry.name}
                </button>
              </li>
            ))}
            {dirs.length === 0 && (
              <li className="flex min-h-11 items-center text-body text-fg-muted">
                No subfolders here.
              </li>
            )}
          </ul>
        )}
      </div>

      <div className="flex flex-col gap-3 border-t border-line px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
        <span
          className="min-w-0 truncate font-mono text-caption text-fg-muted"
          title={path}
          data-testid="storage-browser-current"
        >
          {truncateMiddle(path)}
        </span>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className={btnGhost}
            data-testid="storage-browser-cancel"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onPick(path)}
            disabled={!listing}
            className={btnPrimary}
            data-testid="storage-browser-use"
          >
            Use this folder
          </button>
        </div>
      </div>
    </ModalDialog>
  )
}
