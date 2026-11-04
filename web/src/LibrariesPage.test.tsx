// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Libraries list tests (task 07): counts and space saved from /stats,
 * scan-now, and a delete confirmation that promises files on disk are
 * untouched (only the library record delete goes out).
 */
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
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
    file_kinds: ['video'],
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
  it('lists libraries with counts and space saved, matching /stats', async () => {
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

    // Per-library counts come from the per-library breakdown of /stats,
    // shown as status chips (task 19).
    await waitFor(() =>
      expect(
        screen.getByTestId('library-chip-satisfied-lib-1'),
      ).toHaveTextContent('2 Nothing to do'),
    )
    expect(screen.getByTestId('library-chip-queued-lib-1')).toHaveTextContent(
      '1 Waiting',
    )
    // Zero counts render no chip (task 39, spec 2.1 "hide zero").
    expect(screen.queryByTestId('library-chip-done-lib-1')).toBeNull()
    expect(screen.queryByTestId('library-chip-failed-lib-1')).toBeNull()
    expect(screen.getByTestId('library-files-lib-1')).toHaveTextContent('3')
    expect(screen.getByTestId('library-files-lib-2')).toHaveTextContent('2')
    expect(screen.getByTestId('library-saved-lib-1')).toHaveTextContent(
      '2.0 GB',
    )
    expect(screen.getByTestId('library-saved-lib-2')).toHaveTextContent(
      '500 MB',
    )

    // The library name links to the detail page.
    expect(
      screen.getByTestId('library-name-lib-1').closest('a'),
    ).toHaveAttribute('href', '/libraries/lib-1')
    expect(
      screen.getByTestId('library-name-lib-2').closest('a'),
    ).toHaveAttribute('href', '/libraries/lib-2')
  })

  it('renders the phone card list with the same data (task 19)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()

    const cards = await screen.findByTestId('libraries-cards')
    expect(cards).toBeInTheDocument()
    const card = screen.getByTestId('library-card-lib-1')
    expect(card).toHaveTextContent('Movies')
    expect(card).toHaveTextContent('/media/movies')
    expect(screen.getByTestId('library-files-lib-1-card')).toHaveTextContent(
      '3',
    )
    // The card shows only the most urgent chip (spec 5.4): one waiting file.
    expect(
      screen.getByTestId('library-chip-queued-lib-1-card'),
    ).toHaveTextContent('1 Waiting')
    expect(screen.queryByTestId('library-chip-satisfied-lib-1-card')).toBeNull()
    expect(screen.getByTestId('library-saved-lib-1-card')).toHaveTextContent(
      '2.0 GB',
    )
    expect(
      screen.getByTestId('library-saved-lib-1-card'),
    ).not.toHaveTextContent('saved')
    // The card name links to the detail page.
    expect(
      screen.getByTestId('library-name-lib-1-card').closest('a'),
    ).toHaveAttribute('href', '/libraries/lib-1')
    // The table stays in the DOM for assistive tech (task 20).
    expect(screen.getByRole('table')).toBeInTheDocument()
  })

  it('uses 44px controls on the library list (task 20)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await screen.findByTestId('libraries-table')
    // Every link and button keeps the 44 px height, including the whole-row
    // link (DataList rowHref), which is a 44 px block with the row stretched
    // over it.
    for (const el of [
      ...screen.getAllByRole('button'),
      ...screen.getAllByRole('link'),
    ]) {
      expect(el.className).toContain('min-h-11')
    }
  })

  it('offers no Remove on the row: removing moved to the Edit page (task 47)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await screen.findByTestId('libraries-table')
    expect(screen.queryByTestId('library-delete-lib-1')).toBeNull()
    expect(screen.queryByTestId('library-delete-lib-1-card')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
    expect(screen.getByTestId('library-edit-lib-1')).toBeInTheDocument()
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
      'No libraries yet',
    )
    expect(screen.getByTestId('libraries-empty')).toHaveTextContent(
      'Add library',
    )
  })
})

describe('LibrariesPage file kinds (task 34)', () => {
  it('shows what each library looks for, in plain language', async () => {
    const photos = lib('lib-1', 'Movies')
    photos.file_kinds = ['image']
    const series = lib('lib-2', 'Series')
    series.file_kinds = ['video', 'audio']
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [photos, series], total: 2 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await screen.findByTestId('library-kinds-lib-1')
    expect(screen.getByTestId('library-kinds-lib-1')).toHaveTextContent(
      'Photos',
    )
    expect(screen.getByTestId('library-kinds-lib-2')).toHaveTextContent(
      'Video, Audio',
    )
  })
})

describe('LibrariesPage layout (task 43, spec 5.4)', () => {
  it('has a PageHeader titled Libraries with an "Add library" action', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await screen.findByTestId('libraries-table')
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'Libraries',
    )
    const add = screen.getByTestId('libraries-add')
    expect(add).toHaveTextContent('Add library')
    expect(add).toHaveAttribute('href', '/libraries/new')
    expect(add.className).toContain('bg-accent')
  })

  it('shows only the non-zero status chips in the table', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: {
          ...stats,
          libraries: [
            {
              ...stats.libraries[0],
              files: 4,
              new: 0,
              satisfied: 1,
              queued: 2,
              processing: 0,
              done: 0,
              failed: 1,
              missing: 0,
            },
          ],
        },
      },
    ])
    renderPage()
    await screen.findByTestId('library-chip-queued-lib-1')
    expect(screen.getByTestId('library-chip-queued-lib-1')).toHaveTextContent(
      '2 Waiting',
    )
    expect(screen.getByTestId('library-chip-failed-lib-1')).toHaveTextContent(
      '1 Failed',
    )
    expect(
      screen.getByTestId('library-chip-satisfied-lib-1'),
    ).toHaveTextContent('1 Nothing to do')
    for (const s of ['new', 'processing', 'done', 'missing']) {
      expect(
        screen.queryByTestId(`library-chip-${s}-lib-1`),
      ).not.toBeInTheDocument()
    }
    expect(screen.queryByTestId('library-uptodate-lib-1')).toBeNull()
  })

  it('shows "Up to date" when every file is satisfied or done', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: {
          ...stats,
          libraries: [
            {
              ...stats.libraries[0],
              files: 3,
              new: 0,
              satisfied: 2,
              queued: 0,
              processing: 0,
              done: 1,
              failed: 0,
              missing: 0,
            },
          ],
        },
      },
    ])
    renderPage()
    const badge = await screen.findByTestId('library-uptodate-lib-1')
    expect(badge).toHaveTextContent('Up to date')
    expect(badge.className).toContain('bg-success/10')
    // No chips at all: every count is satisfied or done, none is zero-noise.
    expect(screen.queryByTestId('library-chip-queued-lib-1')).toBeNull()
    // The card shows the same badge.
    expect(
      await screen.findByTestId('library-uptodate-lib-1-card'),
    ).toHaveTextContent('Up to date')
  })

  it('puts the path in mono under the name', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [lib('lib-1', 'Movies')], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: stats },
    ])
    renderPage()
    await screen.findByTestId('libraries-table')
    const table = screen.getByTestId('libraries-table')
    const name = within(table).getByTestId('library-name-lib-1')
    const path = within(table).getByText('/media/movies', { selector: 'span' })
    expect(path.className).toContain('font-mono')
    expect(path.className).toContain('text-caption')
    expect(path.className).toContain('truncate')
    // The path is in the same cell, below the name.
    expect(name.closest('td')).toBe(path.closest('td'))
    expect(
      name.compareDocumentPosition(path) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })
})
