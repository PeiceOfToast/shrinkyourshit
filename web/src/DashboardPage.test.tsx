// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dashboard tests (task 07): live job progress updating from SSE events
 * (fake EventSource) and the "Add your first folder" empty state.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import DashboardPage from './DashboardPage'
import { LiveEvents } from './events'
import {
  FakeEventSource,
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'

function job(percent: number, status = 'running') {
  return {
    id: 'job-1',
    file_id: 'f1',
    library_id: 'lib-1',
    node_id: '',
    status,
    priority: 0,
    attempt: 1,
    encoder_used: '',
    fallback_reasons: [],
    percent,
    speed: 2.5,
    eta_seconds: 90,
    bytes_before: 4_000_000_000,
    bytes_after: 2_000_000_000,
    log: '',
    error: '',
    created_at: '2026-10-29T00:00:00Z',
  }
}

const library = {
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
}

const zeros = {
  files: 12,
  new: 0,
  satisfied: 8,
  queued: 3,
  processing: 1,
  done: 0,
  failed: 0,
  missing: 0,
  bytes_saved: 15_000_000_000,
  libraries: [
    {
      library_id: 'lib-1',
      name: 'Movies',
      files: 12,
      new: 0,
      satisfied: 8,
      queued: 3,
      processing: 1,
      done: 0,
      failed: 0,
      missing: 0,
      bytes_saved: 15_000_000_000,
    },
  ],
}

function renderDashboard(withEvents = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        {withEvents && <LiveEvents />}
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

beforeEach(() => {
  stubEventSource()
})

afterEach(() => mock?.restore())

describe('DashboardPage', () => {
  it("shows a running job's progress updating from SSE events", async () => {
    let jobs = { items: [job(10)], total: 1 }
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        bodyFn: () => jobs,
      },
    ])

    renderDashboard(true)
    await waitFor(() =>
      expect(FakeEventSource.instances[0]?.url).toBe('/api/v1/events'),
    )
    await waitFor(() =>
      expect(screen.getByTestId('job-progress-job-1')).toHaveAttribute(
        'aria-valuenow',
        '10',
      ),
    )
    expect(screen.getByTestId('stat-space-saved')).toHaveTextContent('15.0 GB')

    // The worker reports progress; the server publishes job.updated; the
    // dashboard refetches and the bar moves.
    jobs = { items: [job(50)], total: 1 }
    FakeEventSource.instances[0].emit('job.updated')
    await waitFor(() =>
      expect(screen.getByTestId('job-progress-job-1')).toHaveAttribute(
        'aria-valuenow',
        '50',
      ),
    )
    expect(screen.getByText('50%')).toBeInTheDocument()
    expect(screen.getByText(/2.5×/)).toBeInTheDocument()
    expect(screen.getByText(/1m 30s left/)).toBeInTheDocument()
  })

  it('lands on the easy-mode form when there are no libraries', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: { ...zeros, files: 0, libraries: [], bytes_saved: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()

    await waitFor(() =>
      expect(screen.getByTestId('empty-dashboard')).toBeInTheDocument(),
    )
    expect(
      screen.getByRole('heading', { name: 'Add your first folder' }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('easy-form')).toBeInTheDocument()
  })

  it('shows recent history with sizes and the node placeholder', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: {
          items: [
            { ...job(100, 'done'), id: 'done-1' },
            { ...job(40, 'failed'), id: 'fail-1', error: 'encoder exploded' },
          ],
          total: 2,
        },
      },
    ])

    renderDashboard()

    await waitFor(() =>
      expect(screen.getByTestId('history-job-done-1')).toHaveTextContent(
        'shrunk',
      ),
    )
    expect(screen.getByTestId('history-job-done-1')).toHaveTextContent(
      '4.0 GB → 2.0 GB',
    )
    expect(screen.getByTestId('history-job-fail-1')).toHaveTextContent('failed')
    expect(screen.getByTestId('nodes-placeholder')).toBeInTheDocument()
    expect(screen.getByTestId('dashboard-add-library')).toHaveAttribute(
      'href',
      '/libraries/new',
    )
  })

  it('does not open a stream when the page fails to load', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        status: 500,
        body: { error: 'boom', code: 'internal' },
      },
    ])
    renderDashboard()
    await waitFor(() =>
      expect(screen.getByTestId('dashboard')).toBeInTheDocument(),
    )
    // The shell owns the single SSE stream, not the page.
    expect(FakeEventSource.instances).toHaveLength(0)
    void fireEvent
  })
})
