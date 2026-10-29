// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list tests (task 07): counts and space saved from /stats,
 * scan-now, and a delete confirmation that promises files on disk are
 * untouched (only the library record delete goes out).
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import LibrariesPage from './LibrariesPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

function lib(id: string, name: string) {
  return {
    id,
    name,
    path: `/media/${name.toLowerCase()}`,
    target: {},
    easy: null,
    watch: true,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
  }
}

const stats = {
  files: 5,
  new: 1,
  satisfied: 2,
  queued: 1,
  processing: 0,
  done: 1,
  failed: 0,
  missing: 0,
  bytes_saved: 2_500_000_000,
  libraries: [
    {
      library_id: 'lib-1',
      name: 'Movies',
      files: 3,
      new: 0,
      satisfied: 2,
      queued: 1,
      processing: 0,
      done: 0,
      failed: 0,
      missing: 0,
      bytes_saved: 2_000_000_000,
    },
    {
      library_id: 'lib-2',
      name: 'Series',
      files: 2,
      new: 1,
      satisfied: 0,
      queued: 0,
      processing: 0,
      done: 1,
      failed: 0,
      missing: 0,
      bytes_saved: 500_000_000,
    },
  ],
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <LibrariesPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('LibrariesPage', () => {
  it('lists libraries with counts and space saved', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: {
          items: [lib('lib-1', 'Movies'), lib('lib-2', 'Series')],
          total: 2,
        },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()

    await waitFor(() =>
      expect(screen.getByTestId('library-counts-lib-1')).toHaveTextContent(
        '2 nothing to do · 1 waiting · 0 done · 0 failed',
      ),
    )
    expect(screen.getByTestId('library-saved-lib-1')).toHaveTextContent(
      '2.0 GB saved',
    )
    expect(screen.getByTestId('library-saved-lib-2')).toHaveTextContent(
      '500 MB saved',
    )
  })

  it('scans a library on request', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries\/lib-1\/scan$/,
        status: 202,
        body: { started: true },
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('library-scan-lib-1')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('library-scan-lib-1'))
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/libraries\/lib-1\/scan$/),
      ).toHaveLength(1),
    )
  })

  it('toasts on a successful scan (task 13)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries\/lib-1\/scan$/,
        status: 202,
        body: { started: true },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('library-scan-lib-1'))
    const toasts = await screen.findAllByTestId('toast')
    expect(toasts[0]).toHaveTextContent('Scan started.')
  })

  it('toasts the status and server code when a scan fails (task 13)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries\/lib-1\/scan$/,
        status: 409,
        body: { error: 'a scan is already running', code: 'scan_running' },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('library-scan-lib-1'))
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Scan failed (409, scan_running)')
    expect(toast).toHaveTextContent('a scan is already running')
  })

  it('deletes only after confirmation, promising the files stay on disk', async () => {
    let deleted = false
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
      {
        method: 'DELETE',
        match: /\/api\/v1\/libraries\/lib-1$/,
        status: 204,
        bodyFn: () => {
          deleted = true
          return null
        },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('library-delete-lib-1'))

    const dialog = screen.getByTestId('confirm-delete')
    expect(dialog).toHaveTextContent('Remove “Movies”?')
    expect(dialog).toHaveTextContent('/media/movies')
    expect(dialog).toHaveTextContent('are not touched')

    // Cancelling keeps the library.
    fireEvent.click(screen.getByTestId('confirm-delete-cancel'))
    await waitFor(() =>
      expect(screen.queryByTestId('confirm-delete')).not.toBeInTheDocument(),
    )
    expect(deleted).toBe(false)

    // Confirming sends exactly one DELETE to the library endpoint —
    // never anything that would touch files on disk.
    fireEvent.click(screen.getByTestId('library-delete-lib-1'))
    fireEvent.click(screen.getByTestId('confirm-delete-yes'))
    await waitFor(() => expect(deleted).toBe(true))
    expect(mock.calls('DELETE', /\/api\/v1\/libraries\/lib-1$/)).toHaveLength(1)
    expect(mock.requests.filter((r) => r.method === 'DELETE')).toHaveLength(1)
    await waitFor(() =>
      expect(screen.queryByTestId('confirm-delete')).not.toBeInTheDocument(),
    )
    // The removal is announced, and the message repeats the promise the
    // dialog made.
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Library removed')
    expect(toast).toHaveTextContent('files on disk are untouched')
  })

  it('offers the easy-mode form when there are no libraries', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [], total: 0 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('libraries-empty')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('libraries-empty')).toHaveTextContent(
      'Add your first folder',
    )
  })
})
