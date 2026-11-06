// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage browser tests (task 69, spec 4.6): folders only, breadcrumbs that
 * stop at the share, the up control, the login and not-mounted notices, and
 * an unreadable folder shown as an error rather than an empty list.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Storage } from './api'
import StorageBrowser from './StorageBrowser'
import { installFetchMock, type FetchMock } from './test/fetchMock'

const nas: Storage = {
  id: 'st-nas',
  name: 'nas',
  protocol: 'nfs',
  source: '192.168.1.10:/export/media',
  options: 'vers=4.2,soft,timeo=50,retrans=2',
  username: '',
  domain: '',
  password_set: false,
  read_only: false,
  enabled: true,
  mountpoint: '/storage/nas',
  mounted: true,
  error: '',
  created_at: '',
  updated_at: '',
}

function renderBrowser(
  onPick = vi.fn(),
  onClose = vi.fn(),
  initialPath?: string,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <StorageBrowser
        storage={nas}
        initialPath={initialPath}
        onClose={onClose}
        onPick={onPick}
      />
    </QueryClientProvider>,
  )
  return { onPick, onClose }
}

let mock: FetchMock | undefined

afterEach(() => {
  mock?.restore()
  mock = undefined
})

/** A listing answer: folders and one file, which must never appear. */
function listing(path: string) {
  const parent = path === '/storage/nas' ? '' : path.replace(/\/[^/]+$/, '')
  return {
    id: 'st-nas',
    name: 'nas',
    mounted: true,
    mountpoint: '/storage/nas',
    path,
    parent,
    free_bytes: 2_000_000_000,
    total_bytes: 4_000_000_000,
    entries: [
      { name: 'Movies', path: `${path}/Movies`, is_dir: true },
      { name: 'TV', path: `${path}/TV`, is_dir: true },
      { name: '.recycle', path: `${path}/.recycle`, is_dir: true },
      { name: 'notes.txt', path: `${path}/notes.txt`, is_dir: false },
    ],
  }
}

describe('StorageBrowser', () => {
  it('lists folders only, starts at the mountpoint and shows the space', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        bodyFn: (ctx) =>
          listing(ctx.url.searchParams.get('path') ?? '/storage/nas'),
      },
    ])
    renderBrowser()

    await waitFor(() =>
      expect(
        screen.getByTestId('storage-browser-dir-Movies'),
      ).toBeInTheDocument(),
    )
    expect(screen.getByTestId('storage-browser-dir-TV')).toBeInTheDocument()
    expect(
      screen.queryByTestId('storage-browser-dir-.recycle'),
    ).not.toBeInTheDocument()
    expect(screen.queryByText('notes.txt')).not.toBeInTheDocument()
    expect(screen.queryByTestId('storage-browser-up')).not.toBeInTheDocument()
    expect(screen.getByTestId('storage-browser-badge')).toHaveTextContent(
      'Mounted',
    )
    expect(screen.getByTestId('storage-browser-space')).toHaveTextContent(
      '2.0 GB free of 4.0 GB',
    )
    expect(screen.getByTestId('storage-browser-crumbs')).toHaveTextContent(
      'nas',
    )
  })

  it('walks into a folder, goes back up, and picks the folder it is in', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        bodyFn: (ctx) =>
          listing(ctx.url.searchParams.get('path') ?? '/storage/nas'),
      },
    ])
    const { onPick } = renderBrowser()

    fireEvent.click(await screen.findByTestId('storage-browser-dir-Movies'))
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-current')).toHaveTextContent(
        '/storage/nas/Movies',
      ),
    )
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-up')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('storage-browser-up'))
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-current')).toHaveTextContent(
        '/storage/nas',
      ),
    )
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-dir-TV')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('storage-browser-dir-TV'))
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-current')).toHaveTextContent(
        '/storage/nas/TV',
      ),
    )
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-use')).toBeEnabled(),
    )
    fireEvent.click(screen.getByTestId('storage-browser-use'))
    expect(onPick).toHaveBeenCalledWith('/storage/nas/TV')
  })

  it('shows an unreadable folder as an error, not an empty list', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        status: 403,
        body: {
          error:
            'the server cannot read this folder (permission denied), so its contents are not shown',
          code: 'unreadable',
        },
      },
    ])
    const { onPick } = renderBrowser()

    const error = await screen.findByTestId('storage-browser-error')
    expect(error).toHaveTextContent('Cannot list this folder.')
    expect(error).toHaveTextContent('permission denied')
    expect(screen.getByTestId('error-code')).toHaveTextContent('unreadable')
    expect(screen.queryByText('No subfolders here.')).not.toBeInTheDocument()
    expect(screen.getByTestId('storage-browser-use')).toBeDisabled()
    expect(onPick).not.toHaveBeenCalled()
  })

  it('links to App settings when login is off', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        status: 403,
        body: { error: 'login is off', code: 'login_required' },
      },
    ])
    renderBrowser()

    const notice = await screen.findByTestId('storage-browser-login')
    expect(notice).toHaveTextContent('Browsing a storage needs login.')
    expect(notice.querySelector('a')).toHaveAttribute('href', '/app')
    expect(
      screen.queryByTestId('storage-browser-error'),
    ).not.toBeInTheDocument()
  })

  it('says the storage is not mounted and links to the Storage page', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        status: 409,
        body: { error: 'not mounted', code: 'not_mounted' },
      },
    ])
    renderBrowser()

    const notice = await screen.findByTestId('storage-browser-unmounted')
    expect(notice).toHaveTextContent('nas is not mounted.')
    expect(notice.querySelector('a')).toHaveAttribute('href', '/storage')
  })
})
