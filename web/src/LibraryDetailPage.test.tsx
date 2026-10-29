// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library detail tests (task 07): file table with status, reasons and
 * bytes saved; status filter; retry failed.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import LibraryDetailPage from './LibraryDetailPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function file(id: string, status: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    library_id: 'lib-1',
    path: `/media/movies/${id}.mkv`,
    size: 1_000_000_000,
    mtime: 0,
    fingerprint: '',
    probe: null,
    status,
    satisfied_reason: '',
    last_job_id: '',
    bytes_saved: 0,
    created_at: '',
    updated_at: '',
    ...extra,
  }
}

let mock: FetchMock

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/libraries/lib-1']}>
        <Routes>
          <Route path="/libraries/:id" element={<LibraryDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => mock?.restore())

describe('LibraryDetailPage', () => {
  it('lists files with status, reason and saved bytes', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: {
          id: 'lib-1',
          name: 'Movies',
          path: '/media/movies',
          target: {},
          easy: null,
          watch: true,
          schedule: '',
          result_policy: 'replace_if_smaller',
          created_at: '',
          updated_at: '',
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: {
          items: [
            file('f1', 'satisfied'),
            file('f2', 'failed', {
              satisfied_reason: 'video is h264, target is hevc',
              last_job_id: 'job-9',
            }),
            file('f3', 'done', { bytes_saved: 700_000_000 }),
          ],
          total: 3,
        },
      },
    ])
    renderPage()

    await waitFor(() =>
      expect(screen.getByTestId('file-table')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('file-status-f1')).toHaveTextContent(
      'Nothing to do',
    )
    expect(screen.getByTestId('file-status-f2')).toHaveTextContent('Failed')
    expect(screen.getByTestId('file-reason-f2')).toHaveTextContent(
      'video is h264, target is hevc',
    )
    expect(screen.getByTestId('file-row-f3')).toHaveTextContent('700 MB')
    expect(screen.getByTestId('retry-failed')).toHaveTextContent('(1)')
  })

  // Task 12: under `sm` the file list renders as stacked cards — the
  // primary columns (name, status) stay visible and the secondary
  // columns (why, size, saved) collapse into label/value pairs. Both
  // render trees are always in the DOM; CSS picks the visible one, so
  // the structural contract is what the unit test can assert.
  it('renders the phone card list with label/value pairs', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: {
          id: 'lib-1',
          name: 'Movies',
          path: '/media/movies',
          target: {},
          easy: null,
          watch: true,
          schedule: '',
          result_policy: 'replace_if_smaller',
          created_at: '',
          updated_at: '',
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: {
          items: [
            file('f1', 'new', {
              satisfied_reason: 'video is h264, target is hevc',
            }),
          ],
          total: 1,
        },
      },
    ])
    renderPage()

    const cards = await screen.findByTestId('file-cards')
    expect(cards).toBeInTheDocument()
    const card = screen.getByTestId('file-card-f1')
    // Primary columns stay visible in the card: name and status.
    expect(screen.getByTestId('file-card-status-f1')).toHaveTextContent('New')
    // Secondary columns collapse into label and value pairs.
    expect(screen.getByTestId('file-card-reason-f1')).toHaveTextContent(
      'Why: video is h264, target is hevc',
    )
    expect(card).toHaveTextContent('Size:')
    expect(card).toHaveTextContent('Saved:')
    // The long path is deliberately wrapped or truncated, never forcing
    // width: the name span carries break-words.
    expect(card.querySelector('.break-words')).not.toBeNull()
  })

  it('filters files by status', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: {
          id: 'lib-1',
          name: 'Movies',
          path: '/media/movies',
          target: {},
          easy: null,
          watch: true,
          schedule: '',
          result_policy: 'replace_if_smaller',
          created_at: '',
          updated_at: '',
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: { items: [], total: 0 },
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('file-status-filter')).toBeInTheDocument(),
    )

    fireEvent.change(screen.getByTestId('file-status-filter'), {
      target: { value: 'failed' },
    })
    await waitFor(() => {
      const failed = mock.requests.find(
        (r) => r.url === '/api/v1/libraries/lib-1/files?status=failed',
      )
      expect(failed).toBeDefined()
    })
  })

  it('retries every failed file once', async () => {
    const retried: string[] = []
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: {
          id: 'lib-1',
          name: 'Movies',
          path: '/media/movies',
          target: {},
          easy: null,
          watch: true,
          schedule: '',
          result_policy: 'replace_if_smaller',
          created_at: '',
          updated_at: '',
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: {
          items: [
            file('f1', 'failed', { last_job_id: 'job-1' }),
            file('f2', 'failed', { last_job_id: 'job-2' }),
            file('f3', 'satisfied'),
          ],
          total: 3,
        },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/jobs\/job-[12]\/retry$/,
        status: 200,
        bodyFn: (ctx) => {
          retried.push(ctx.url.pathname)
          return { id: 'x' }
        },
      },
    ])
    renderPage()
    const retry = await screen.findByTestId('retry-failed')
    await waitFor(() => expect(retry).not.toBeDisabled())
    fireEvent.click(retry)

    await waitFor(() =>
      expect(retried).toEqual(
        expect.arrayContaining([
          '/api/v1/jobs/job-1/retry',
          '/api/v1/jobs/job-2/retry',
        ]),
      ),
    )
    await waitFor(() =>
      expect(screen.getByTestId('library-notice')).toHaveTextContent(
        'Retrying 2 job(s).',
      ),
    )
  })
})
