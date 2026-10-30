// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library detail tests (tasks 07, 13 and 19): file table with status,
 * reasons and the before/after/ratio columns; the header summary (status
 * counts, bytes saved, plain-language target, last scan time); server side
 * search and sort; status filter; pagination (page size, previous/next);
 * the cancel-job confirmation and toasts; the per-file job history link.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import LibraryDetailPage from './LibraryDetailPage'
import {
  installFetchMock,
  type FetchMock,
  type MockRoute,
} from './test/fetchMock'
import { ToastProvider } from './toast'

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

function libraryDoc(extra: Record<string, unknown> = {}) {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: {},
    easy: null,
    flow: { schema: 1, nodes: [], edges: [] },
    flow_source: 'easy',
    watch: true,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
    ...extra,
  }
}

/** The per-library breakdown the header summary reads (task 19). */
const statsDoc = {
  files: 3,
  new: 0,
  satisfied: 1,
  queued: 1,
  processing: 1,
  done: 0,
  failed: 0,
  missing: 0,
  bytes_saved: 700_000_000,
  libraries: [
    {
      library_id: 'lib-1',
      name: 'Movies',
      files: 3,
      new: 0,
      satisfied: 1,
      queued: 1,
      processing: 1,
      done: 0,
      failed: 0,
      missing: 0,
      bytes_saved: 700_000_000,
    },
  ],
}

/** The server-rendered plain-language target lines (task 14 preview). */
function previewRoutes(): MockRoute[] {
  return [
    {
      method: 'POST',
      match: /\/api\/v1\/flows\/preview$/,
      body: {
        graph: { schema: 1, nodes: [], edges: [] },
        steps: [
          {
            id: 's1',
            type: 'transcode_video',
            plain: 'Make the video smaller with balanced quality.',
          },
          {
            id: 's2',
            type: 'convert_audio',
            plain: 'Keep the best audio track.',
          },
        ],
      },
    },
  ]
}

let mock: FetchMock

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/libraries/lib-1']}>
          <Routes>
            <Route path="/libraries/:id" element={<LibraryDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => mock?.restore())

describe('LibraryDetailPage', () => {
  it('lists files with status, reason and the before/after columns', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
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
              last_job_encoder: 'nvenc',
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
    // The done file shows before, after and ratio (task 19): 1.7 GB
    // before, 1.0 GB after, 1.7× smaller.
    expect(screen.getByTestId('file-before-f3')).toHaveTextContent('1.7 GB')
    expect(screen.getByTestId('file-after-f3')).toHaveTextContent('1.0 GB')
    expect(screen.getByTestId('file-ratio-f3')).toHaveTextContent('1.7×')
    // A file with no reduction shows dashes, not made-up numbers.
    expect(screen.getByTestId('file-after-f1')).toHaveTextContent('—')
    expect(screen.getByTestId('file-ratio-f1')).toHaveTextContent('—')
    // The encoder of the file's most recent job.
    expect(screen.getByTestId('file-encoder-f2')).toHaveTextContent('nvenc')
    expect(screen.getByTestId('file-encoder-f1')).toHaveTextContent('—')
    expect(screen.getByTestId('retry-failed')).toHaveTextContent('(1)')
  })

  it('shows the header summary: counts, saved, plain target, last scan', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc({ last_scan_at: '2026-10-30T12:00:00Z' }),
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: statsDoc },
      ...previewRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: { items: [], total: 0 },
      },
    ])
    renderPage()

    const summary = await screen.findByTestId('library-summary')
    expect(summary).toBeInTheDocument()
    // Status counts from the per-library breakdown of /stats.
    expect(screen.getByTestId('library-summary-satisfied')).toHaveTextContent(
      '1 Nothing to do',
    )
    expect(screen.getByTestId('library-summary-queued')).toHaveTextContent(
      '1 Waiting',
    )
    expect(screen.getByTestId('library-summary-processing')).toHaveTextContent(
      '1 Working',
    )
    expect(screen.getByTestId('library-summary-failed')).toHaveTextContent(
      '0 Failed',
    )
    // Bytes saved.
    expect(screen.getByTestId('library-summary-saved')).toHaveTextContent(
      '700 MB saved',
    )
    // The last scan time.
    expect(screen.getByTestId('library-last-scan')).toHaveTextContent(
      'Last scan:',
    )
    // The target in plain language, one line per compiled step.
    const lines = await screen.findAllByTestId('target-plain-line')
    expect(lines).toHaveLength(2)
    expect(lines[0]).toHaveTextContent(
      'Make the video smaller with balanced quality.',
    )
    expect(lines[1]).toHaveTextContent('Keep the best audio track.')
  })

  it('says a library that never scanned has no scan time', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: statsDoc },
      ...previewRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: { items: [], total: 0 },
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('library-last-scan')).toHaveTextContent(
        'Not scanned yet.',
      ),
    )
  })

  it('opens the job history panel from the log link (task 19)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: {
          items: [file('f2', 'failed', { last_job_id: 'job-9' })],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/files\/f2\/jobs$/,
        body: {
          items: [
            {
              id: 'job-9',
              file_id: 'f2',
              library_id: 'lib-1',
              node_id: '',
              status: 'failed',
              attempt: 1,
              encoder_used: 'nvenc',
              fallback_reasons: ['qsv: init failed'],
              percent: 0,
              speed: 0,
              eta_seconds: 0,
              bytes_before: 1_000_000_000,
              bytes_after: 0,
              bytes_so_far: 0,
              file_name: 'f2.mkv',
              file_path: '/media/movies/f2.mkv',
              log: 'ffmpeg stderr tail',
              error: 'encode failed',
              created_at: '',
            },
          ],
          total: 1,
        },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('file-log-f2'))
    const panel = await screen.findByTestId('file-history')
    expect(panel).toBeInTheDocument()
    expect(screen.getByTestId('file-history-title')).toHaveTextContent(
      'Job history',
    )
  })

  it('renders the phone card list with label/value pairs', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
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
    expect(card).toHaveTextContent('Encoder:')
    // The long path is deliberately wrapped or truncated, never forcing
    // width: the name span carries break-words.
    expect(card.querySelector('.break-words')).not.toBeNull()
  })

  it('filters files by status and keeps the filter in the request', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
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
      const failed = mock.requests.find((r) =>
        r.url.startsWith('/api/v1/libraries/lib-1/files?status=failed'),
      )
      expect(failed).toBeDefined()
    })
  })

  it('searches and sorts server side (task 19)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        bodyFn: (ctx) => {
          const url = ctx.url
          void url
          return { items: [], total: 0 }
        },
      },
    ])
    renderPage()
    const search = await screen.findByTestId('file-search')

    // The default sort rides along on every request.
    await waitFor(() => {
      expect(
        mock.requests.some(
          (r) => r.url.includes('sort=name') && r.url.includes('dir=asc'),
        ),
      ).toBe(true)
    })

    // Typing searches by name, server side (debounced).
    fireEvent.change(search, { target: { value: 'movie' } })
    await waitFor(() => {
      expect(mock.requests.some((r) => r.url.includes('q=movie'))).toBe(true)
    })

    // Changing the sort key and flipping the direction ride along.
    fireEvent.change(screen.getByTestId('file-sort'), {
      target: { value: 'size' },
    })
    await waitFor(() => {
      expect(mock.requests.some((r) => r.url.includes('sort=size'))).toBe(true)
    })
    fireEvent.click(screen.getByTestId('file-sort-dir'))
    await waitFor(() => {
      expect(mock.requests.some((r) => r.url.includes('dir=desc'))).toBe(true)
    })
    // A search change resets to the first page.
    expect(
      mock.requests.some(
        (r) => r.url.includes('q=movie') && r.url.includes('offset=50'),
      ),
    ).toBe(false)
  })

  it('paginates with a page size selector and previous/next (task 13)', async () => {
    const all = Array.from({ length: 120 }, (_, i) => file(`f${i}`, 'new'))
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        bodyFn: (ctx) => {
          const offset = Number(ctx.url.searchParams.get('offset') ?? 0)
          const limit = Number(ctx.url.searchParams.get('limit') ?? 50)
          return { items: all.slice(offset, offset + limit), total: 120 }
        },
      },
    ])
    renderPage()

    await waitFor(() =>
      expect(screen.getByTestId('files-page-info')).toHaveTextContent(
        'Page 1 of 3 · 120 files',
      ),
    )
    // First page: previous disabled, next enabled.
    expect(screen.getByTestId('files-prev')).toBeDisabled()
    const next = screen.getByTestId('files-next')
    expect(next).not.toBeDisabled()

    fireEvent.click(next)
    await waitFor(() => {
      const second = mock.requests.find((r) => r.url.includes('offset=50'))
      expect(second).toBeDefined()
    })
    await waitFor(() =>
      expect(screen.getByTestId('files-page-info')).toHaveTextContent(
        'Page 2 of 3',
      ),
    )
    expect(screen.getByTestId('files-prev')).not.toBeDisabled()

    // Changing the page size resets to the first page with the new limit.
    // (offset 0 is omitted from the query string; the first request of a
    // fresh page is just the limit.)
    fireEvent.change(screen.getByTestId('files-page-size'), {
      target: { value: '25' },
    })
    await waitFor(() => {
      const resized = mock.requests.find((r) => r.url.includes('limit=25'))
      expect(resized).toBeDefined()
    })
    await waitFor(() =>
      expect(screen.getByTestId('files-page-info')).toHaveTextContent(
        'Page 1 of 5',
      ),
    )
  })

  it('retries every failed file once and toasts the count', async () => {
    const retried: string[] = []
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
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
    // Task 13: the result is a toast now, not an inline notice.
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Retrying 2 job(s).')
  })

  it('cancels a running job only after the confirmation says what happens', async () => {
    let cancelled = false
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/files$/,
        body: {
          items: [
            file('f1', 'processing', { last_job_id: 'job-7' }),
            file('f2', 'queued', { last_job_id: 'job-8' }),
            file('f3', 'done', { bytes_saved: 1 }),
          ],
          total: 3,
        },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/jobs\/job-7\/cancel$/,
        bodyFn: () => {
          cancelled = true
          return null
        },
      },
    ])
    renderPage()
    const button = await screen.findByTestId('file-cancel-f1')
    // Done files are not cancellable.
    expect(screen.queryByTestId('file-cancel-f3')).not.toBeInTheDocument()
    expect(screen.queryByTestId('file-cancel-f3-card')).not.toBeInTheDocument()

    fireEvent.click(button)
    const dialog = screen.getByTestId('confirm-cancel-job')
    expect(dialog).toHaveTextContent('Cancel this job?')
    expect(dialog).toHaveTextContent('temporary file is thrown away')
    expect(dialog).toHaveTextContent('original file is never touched')

    // Keeping it running sends nothing.
    fireEvent.click(screen.getByTestId('confirm-cancel-job-no'))
    await waitFor(() =>
      expect(
        screen.queryByTestId('confirm-cancel-job'),
      ).not.toBeInTheDocument(),
    )
    expect(cancelled).toBe(false)

    // Confirming posts exactly one cancel.
    fireEvent.click(screen.getByTestId('file-cancel-f1'))
    fireEvent.click(screen.getByTestId('confirm-cancel-job-yes'))
    await waitFor(() => expect(cancelled).toBe(true))
    expect(mock.calls('POST', /\/api\/v1\/jobs\/job-7\/cancel$/)).toHaveLength(
      1,
    )
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Job cancelled')
  })
})
