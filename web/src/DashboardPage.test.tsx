// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dashboard tests (task 07 live progress, task 10 read-only + warnings):
 * live job progress updating from SSE events (fake EventSource), the
 * one-line empty state with no form and no create button, and the
 * warning surface driven by real node and job state.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import DashboardPage from './DashboardPage'
import { LiveEvents } from './events'
import { updateStatusKey } from './queries'
import { ToastProvider } from './toast'
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

/** A node fixture; heartbeats by default so only targeted tests warn. */
function nodeFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 'node-1',
    name: 'media-box (built-in)',
    os: 'linux',
    arch: 'amd64',
    capabilities: { encoders: [{ name: 'cpu', codecs: ['hevc'] }] },
    access_mode: 'shared',
    path_mappings: [],
    throughput_limit_mbps: 0,
    status: 'approved',
    enabled: true,
    desired_state: 'running',
    effective_state: 'running',
    concurrency: 1,
    last_heartbeat_at: new Date(Date.now() - 30_000).toISOString(),
    created_at: '2026-10-29T00:00:00Z',
    ...overrides,
  }
}

function renderDashboard(
  withEvents = false,
  updateStatus: Record<string, unknown> = {
    current: 'v1.0.0',
    update_available: false,
    self_update_supported: true,
    reason: 'already on the latest release',
  },
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // The update status (task 17) is seeded so the warning logic is the
  // only thing under test here; the update-specific flow lives in the
  // AboutCard tests.
  client.setQueryData(updateStatusKey, updateStatus)
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          {withEvents && <LiveEvents />}
          <DashboardPage />
        </MemoryRouter>
      </ToastProvider>
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
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
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

  it('shows the EmptyState that links to /libraries/new, with no form and no create button', async () => {
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
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()

    await waitFor(() =>
      expect(screen.getByTestId('empty-dashboard')).toBeInTheDocument(),
    )
    const empty = screen.getByTestId('empty-dashboard')
    expect(empty).toHaveTextContent('Add your first library')
    expect(screen.getByRole('link', { name: 'Add library' })).toHaveAttribute(
      'href',
      '/libraries/new',
    )
    // Read only (task 10): nothing to fill in, nothing to create here.
    expect(empty.querySelector('form')).toBeNull()
    expect(screen.queryByTestId('easy-form')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('dashboard-add-library'),
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId('dashboard')).not.toBeInTheDocument()
  })

  it('renders the four stats as fluid StatCards, 2 columns below lg and 4 from lg up', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()
    const grid = await screen
      .findByTestId('stat-space-saved')
      .then((card) => card.parentElement)
    expect(grid?.className).toContain('grid-cols-2')
    expect(grid?.className).toContain('lg:grid-cols-4')
    // One grid produces the geometry jsdom cannot measure: 2 columns below
    // lg, 4 from lg up (spec 5.3). Testids and order are the dashboard's contract.
    const ids = Array.from(grid?.children ?? []).map(
      (el) => (el as HTMLElement).dataset.testid ?? '',
    )
    expect(ids).toEqual([
      'stat-space-saved',
      'stat-files',
      'stat-satisfied',
      'stat-queued',
    ])
    // Every tile is a StatCard: an aria-hidden icon and a fluid value.
    for (const id of ids) {
      const card = screen.getByTestId(id)
      expect(card.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
      expect(card.querySelector('p.font-semibold')?.className).toContain(
        'text-[clamp(',
      )
    }
  })

  it('shows recent history with sizes and per-node activity', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [
            nodeFixture(),
            nodeFixture({
              id: 'node-2',
              name: 'attic-box',
              os: 'linux',
              arch: 'arm64',
              capabilities: null,
              access_mode: 'auto',
              status: 'pending',
            }),
          ],
          total: 2,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: {
          items: [
            { ...job(100, 'done'), id: 'done-1' },
            {
              ...job(40, 'failed'),
              id: 'fail-1',
              error: 'encoder exploded',
              created_at: new Date(Date.now() - 3_600_000).toISOString(),
            },
            { ...job(60), id: 'run-2', node_id: 'node-2' },
          ],
          total: 3,
        },
      },
    ])

    renderDashboard()

    await waitFor(() =>
      expect(screen.getByTestId('history-job-done-1')).toHaveTextContent(
        'Shrunk',
      ),
    )
    expect(screen.getByTestId('history-job-done-1')).toHaveTextContent(
      '4.0 GB → 2.0 GB',
    )
    expect(screen.getByTestId('history-job-fail-1')).toHaveTextContent('Failed')

    // The Nodes card is live: each node with its state, and the working
    // node's progress bar.
    await waitFor(() =>
      expect(screen.getByTestId('nodes-activity')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('node-activity-node-1')).toHaveTextContent(
      'media-box (built-in)',
    )
    expect(screen.getByTestId('node-activity-node-1')).toHaveTextContent(
      'Running',
    )
    expect(screen.getByTestId('node-activity-node-2')).toHaveTextContent(
      'Waiting for approval',
    )
    await waitFor(() =>
      expect(screen.getByTestId('node-progress-node-2')).toHaveAttribute(
        'aria-valuenow',
        '60',
      ),
    )
  })

  it('warns when every node is stopped or disabled while files wait', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ enabled: false })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()
    const warning = await waitFor(() =>
      screen.getByTestId('warning-nodes-unavailable'),
    )
    // The wording names the cause.
    expect(warning).toHaveTextContent(
      'Every node is stopped or disabled, so nothing is being processed',
    )
    expect(warning).toHaveTextContent('3 files are waiting')
    // ...and links to the fix.
    expect(screen.getByRole('link', { name: 'Go to Nodes' })).toHaveAttribute(
      'href',
      '/nodes',
    )
  })

  it('does not warn about a stopped queue while a job is still running', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ enabled: false })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [job(50)], total: 1 },
      },
    ])

    renderDashboard()
    await waitFor(() => expect(screen.getByTestId('dashboard')).toBeTruthy())
    expect(
      screen.queryByTestId('warning-nodes-unavailable'),
    ).not.toBeInTheDocument()
  })

  it('warns when an approved, enabled node stops checking in', async () => {
    const stale = new Date(Date.now() - 10 * 60 * 1000).toISOString()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [nodeFixture({ last_heartbeat_at: stale })],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()
    const warning = await waitFor(() =>
      screen.getByTestId('warning-node-heartbeat-node-1'),
    )
    expect(warning).toHaveTextContent('media-box (built-in) has not checked in')
    expect(screen.getByRole('link', { name: 'Go to Nodes' })).toHaveAttribute(
      'href',
      '/nodes',
    )
  })

  it('warns about jobs that failed in the last 24 hours', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: {
          items: [
            {
              ...job(40, 'failed'),
              id: 'fail-1',
              error: 'encoder exploded',
              created_at: new Date(Date.now() - 3_600_000).toISOString(),
            },
          ],
          total: 1,
        },
      },
    ])

    renderDashboard()
    const warning = await waitFor(() =>
      screen.getByTestId('warning-failed-jobs'),
    )
    expect(warning).toHaveTextContent('failed in the last 24 hours')
    expect(
      screen.getByRole('link', { name: 'Go to Libraries' }),
    ).toHaveAttribute('href', '/libraries')
  })

  it('shows no warnings on a healthy state', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: { ...zeros, queued: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])

    renderDashboard()
    await waitFor(() => expect(screen.getByTestId('dashboard')).toBeTruthy())
    expect(screen.queryByTestId('dashboard-warnings')).not.toBeInTheDocument()
  })

  it('warns about an available update and points at the About card (task 17)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: {
          current: 'v1.0.0',
          latest: 'v1.1.0',
          update_available: true,
          self_update_supported: true,
          release_url: 'https://example.com/releases/tag/v1.1.0',
        },
      },
    ])

    renderDashboard(true, undefined)
    const warning = await waitFor(() =>
      screen.getByTestId('warning-update-available'),
    )
    expect(warning).toHaveTextContent('Version v1.1.0 is available')
    expect(
      screen.getByRole('link', { name: 'Go to App settings' }),
    ).toHaveAttribute('href', '/app')
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

  it('shows the paused panel with the waiting count and a resume action', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ desired_state: 'stopped' })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    renderDashboard()
    expect(
      await screen.findByText('Encoding is paused. 3 files are waiting.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Resume encoding' }),
    ).toBeEnabled()
  })

  it('says the paused panel with nothing waiting when the queue is empty', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: { ...zeros, queued: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ desired_state: 'stopped' })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    renderDashboard()
    expect(
      await screen.findByText(
        'Encoding is paused. Nothing is waiting right now.',
      ),
    ).toBeInTheDocument()
  })

  it('says how many files are shrinking while encoding runs', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [job(20)], total: 1 },
      },
    ])
    renderDashboard()
    expect(
      await screen.findByText('Shrinking 1 file · 3 waiting'),
    ).toBeInTheDocument()
  })

  it('says all caught up when nothing runs and nothing is paused', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: { ...zeros, queued: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    renderDashboard()
    expect(await screen.findByText('All caught up.')).toBeInTheDocument()
  })

  it('names the running file, never its job id, and splits Working now and Nodes 8/4 at lg', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: {
          items: [
            {
              ...job(30),
              file_name: 'sample-1080p.mkv',
              file_path: '/media/movies/sample-1080p.mkv',
            },
          ],
          total: 1,
        },
      },
    ])
    renderDashboard()
    const row = await screen.findByTestId('running-job-job-1')
    expect(row).toHaveTextContent('sample-1080p.mkv')
    expect(row).not.toHaveTextContent('job job-1')
    expect(screen.getByTestId('dashboard-working').className).toContain(
      'lg:col-span-8',
    )
    expect(screen.getByTestId('dashboard-nodes').className).toContain(
      'lg:col-span-4',
    )
  })

  it('shows the stat sub-lines: files from the nodes, nothing to do, and failures in danger', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/stats$/,
        body: { ...zeros, failed: 2 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [nodeFixture({ stats: { files_done: 5, bytes_saved: 1 } })],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    renderDashboard()
    expect(await screen.findByTestId('stat-space-saved')).toHaveTextContent(
      'from 5 files',
    )
    expect(screen.getByTestId('stat-files')).toHaveTextContent('Files watched')
    expect(screen.getByTestId('stat-satisfied')).toHaveTextContent(
      'nothing to do',
    )
    expect(screen.getByTestId('stat-queued')).toHaveTextContent('2 failed')
  })

  it('shows the file name and the size change in Recent activity', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: {
          items: [
            {
              ...job(100, 'done'),
              id: 'done-1',
              file_name: 'sample-51.mkv',
              bytes_before: 1_000_000,
              bytes_after: 900_000,
            },
          ],
          total: 1,
        },
      },
    ])
    renderDashboard()
    const row = await screen.findByTestId('history-job-done-1')
    expect(row).toHaveTextContent('sample-51.mkv')
    expect(row).toHaveTextContent('Movies')
    expect(row).toHaveTextContent('1.0 MB → 900 KB')
    expect(row).toHaveTextContent('−10 %')
    expect(row).not.toHaveTextContent('job done-1')
  })
})

describe('DashboardPage storage login warning (task 68)', () => {
  function storageMock(loginEnabled: boolean, enabled: boolean) {
    return installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [library], total: 1 },
      },
      { method: 'GET', match: /\/api\/v1\/stats$/, body: zeros },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/storages$/,
        body: {
          items: [
            {
              id: 'st-1',
              name: 'nas',
              protocol: 'nfs',
              source: '192.168.1.10:/export',
              options: '',
              username: '',
              domain: '',
              password_set: false,
              read_only: false,
              enabled,
              mountpoint: '/storage/nas',
              mounted: enabled,
              error: '',
              created_at: '2026-11-01T10:00:00Z',
              updated_at: '2026-11-01T10:00:00Z',
            },
          ],
          total: 1,
          host_policy: { restricted: false, allowed: [], message: '' },
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/settings$/,
        body: { login_enabled: loginEnabled },
      },
    ])
  }

  it('shows the standing warning when a share is mounted and login is off', async () => {
    mock = storageMock(false, true)
    renderDashboard()
    const warning = await waitFor(() =>
      screen.getByTestId('warning-storage-login-off'),
    )
    expect(warning).toHaveTextContent(
      'A network share is mounted while login is off',
    )
    expect(
      screen.getByRole('link', { name: 'Go to App settings' }),
    ).toHaveAttribute('href', '/app')
  })

  it('stays quiet with login on', async () => {
    mock = storageMock(true, true)
    renderDashboard()
    await waitFor(() =>
      expect(mock.calls('GET', /\/api\/v1\/settings$/)).not.toHaveLength(0),
    )
    expect(
      screen.queryByTestId('warning-storage-login-off'),
    ).not.toBeInTheDocument()
  })
})
