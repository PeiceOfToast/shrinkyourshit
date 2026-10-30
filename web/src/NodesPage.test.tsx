// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Nodes page tests (task 08): pending node with Approve/Reject at the top,
 * the add-node dialog with the real install commands (server URL + token),
 * and the edit dialog carrying name/mappings.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import NodesPage from './NodesPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

function nodeFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 'node-1',
    name: 'worker-a',
    os: 'linux',
    arch: 'amd64',
    capabilities: {
      encoders: [{ name: 'cpu', codecs: ['hevc'] }],
      ffmpeg_version: 'ffmpeg version 6.0',
    },
    access_mode: 'auto',
    path_mappings: [],
    throughput_limit_mbps: 0,
    status: 'approved',
    enabled: true,
    desired_state: 'running',
    effective_state: 'running',
    concurrency: 1,
    stats: { files_done: 2, bytes_saved: 1_500_000_000 },
    created_at: '2026-10-29T00:00:00Z',
    ...overrides,
  }
}

function pendingNode() {
  return nodeFixture({
    id: 'node-pending',
    name: 'attic-box',
    status: 'pending',
    capabilities: null,
  })
}

function renderNodes() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <NodesPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

beforeEach(() => {})
afterEach(() => mock?.restore())

describe('NodesPage', () => {
  it('shows the pending node on top with working approve and reject', async () => {
    let status = 'pending'
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        bodyFn: () => ({
          items: [
            nodeFixture(),
            pendingNode(),
            nodeFixture({ id: 'node-old', name: 'old', status: 'revoked' }),
          ],
          total: 3,
        }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-pending\/approve$/,
        bodyFn: () => {
          status = 'approved'
          return nodeFixture({ id: 'node-pending', name: 'attic-box', status })
        },
      },
    ])

    renderNodes()
    await waitFor(() =>
      expect(screen.getByTestId('nodes-list').children[0]).toHaveAttribute(
        'data-testid',
        'node-node-pending',
      ),
    )
    expect(screen.getByText('Waiting for approval')).toBeInTheDocument()

    // Approve: the refetch shows the node ready.
    fireEvent.click(screen.getByTestId('node-approve-node-pending'))
    await waitFor(() => expect(status).toBe('approved'))
    await waitFor(() => expect(screen.getByText('Ready')).toBeInTheDocument())

    // Reject deletes the pending row — but only after the confirmation
    // names what happens (task 13).
    fireEvent.click(screen.getByTestId('node-reject-node-pending'))
    expect(
      screen.getByTestId('node-reject-dialog-node-pending'),
    ).toHaveTextContent('jobs return to the queue')
    expect(
      mock.calls('DELETE', /\/api\/v1\/nodes\/node-pending$/),
    ).toHaveLength(0)
    fireEvent.click(screen.getByTestId('node-reject-confirm-node-pending'))
    await waitFor(() =>
      expect(
        mock.calls('DELETE', /\/api\/v1\/nodes\/node-pending$/).length,
      ).toBeGreaterThan(0),
    )
    // Both actions announced themselves (task 13).
    await screen.findAllByTestId('toast')
  })

  it('generates install commands that carry the server URL and token', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/tokens$/,
        body: {
          token: 'one-time-token-123',
          expires_at: '2026-10-30T00:00:00Z',
          commands: {
            docker:
              'docker run -d --name shrinkyourshit-node -e SHRINK_ROLE=worker -e SHRINK_SERVER=http://media.lan:8765 -e SHRINK_TOKEN=one-time-token-123 ghcr.io/peiceoftoast/shrinkyourshit:latest',
            shell:
              'curl -fsSL http://media.lan:8765/install.sh | sh -s -- --server http://media.lan:8765 --token one-time-token-123',
            powershell:
              "$env:SHRINK_SERVER='http://media.lan:8765'; $env:SHRINK_TOKEN='one-time-token-123'; irm http://media.lan:8765/install.ps1 | iex",
          },
        },
      },
    ])

    renderNodes()
    fireEvent.click(screen.getByTestId('nodes-add'))
    fireEvent.click(screen.getByTestId('add-node-generate'))

    // All three commands are shown and carry the real URL and the token
    // (acceptance item 7, UI side).
    await waitFor(() =>
      expect(screen.getByTestId('add-node-commands')).toBeInTheDocument(),
    )
    const text = screen.getByTestId('add-node-commands').textContent ?? ''
    expect(text).toContain('one-time-token-123')
    expect(text).toContain('http://media.lan:8765')
    expect(text).toContain('docker run')
    expect(text).toContain('install.sh')
    expect(text).toContain('install.ps1')

    // The copy buttons are present for every command.
    expect(screen.getByTestId('copy-docker')).toBeInTheDocument()
    expect(screen.getByTestId('copy-shell')).toBeInTheDocument()
    expect(screen.getByTestId('copy-powershell')).toBeInTheDocument()
    // Clicking copy must not explode when the clipboard is unavailable.
    expect(() =>
      fireEvent.click(screen.getByTestId('copy-docker')),
    ).not.toThrow()
  })

  it('saves name, enabled state and path mappings through the edit dialog', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [
            nodeFixture({
              path_mappings: [{ server_path: '/media', node_path: '/mnt/nas' }],
            }),
          ],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/nodes\/node-1$/,
        bodyFn: (ctx) => ({
          ...nodeFixture(),
          ...(ctx.body as Record<string, unknown>),
        }),
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-edit-node-1'))
    fireEvent.click(screen.getByTestId('node-edit-node-1'))

    const dialog = screen.getByTestId('edit-node-dialog')
    expect(dialog).toBeInTheDocument()

    // Existing mapping is shown; rename, add a second mapping, set a
    // transfer speed cap, save.
    expect(screen.getByTestId('mapping-server-0')).toHaveValue('/media')
    fireEvent.change(screen.getByTestId('edit-node-name'), {
      target: { value: 'renamed-box' },
    })
    fireEvent.click(screen.getByTestId('add-mapping'))
    fireEvent.change(screen.getByTestId('mapping-server-1'), {
      target: { value: '/media2' },
    })
    fireEvent.change(screen.getByTestId('mapping-node-1'), {
      target: { value: '/srv/two' },
    })
    fireEvent.change(screen.getByTestId('edit-node-throughput'), {
      target: { value: '50' },
    })
    fireEvent.click(screen.getByTestId('edit-node-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/nodes\/node-1$/)
      expect(puts.length).toBeGreaterThan(0)
      expect(puts[0].body).toEqual({
        name: 'renamed-box',
        enabled: true,
        access_mode: 'auto',
        path_mappings: [
          { server_path: '/media', node_path: '/mnt/nas' },
          { server_path: '/media2', node_path: '/srv/two' },
        ],
        throughput_limit_mbps: 50,
      })
    })
  })

  it('saves a zero transfer cap as unlimited', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ throughput_limit_mbps: 25 })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/nodes\/node-1$/,
        body: nodeFixture(),
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-edit-node-1'))
    fireEvent.click(screen.getByTestId('node-edit-node-1'))
    expect(screen.getByTestId('edit-node-throughput')).toHaveValue(25)
    fireEvent.change(screen.getByTestId('edit-node-throughput'), {
      target: { value: '0' },
    })
    fireEvent.click(screen.getByTestId('edit-node-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/nodes\/node-1$/)
      expect(puts.length).toBeGreaterThan(0)
      expect(puts[0].body).toMatchObject({ throughput_limit_mbps: 0 })
    })
  })

  it('revokes an approved node', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-1\/revoke$/,
        body: nodeFixture({ status: 'revoked' }),
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-revoke-node-1'))
    fireEvent.click(screen.getByTestId('node-revoke-node-1'))
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/nodes\/node-1\/revoke$/).length,
      ).toBeGreaterThan(0),
    )
  })

  it('shows the lifecycle pill and a switch that starts a stopped node (tasks 11 and 18)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [
            nodeFixture({
              desired_state: 'stopped',
              effective_state: 'stopped',
            }),
          ],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-1\/start$/,
        body: nodeFixture(),
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-lifecycle-node-1'))
    expect(screen.getByTestId('node-lifecycle-node-1')).toHaveTextContent(
      'Stopped',
    )
    // The switch is the primary control: off while stopped, switch role.
    const sw = screen.getByRole('switch', { name: 'Encoding on worker-a' })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByTestId('node-stop-now-node-1')).not.toBeInTheDocument()

    fireEvent.click(sw)
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/nodes\/node-1\/start$/).length,
      ).toBeGreaterThan(0),
    )
    // Task 13: the start is announced.
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('worker-a started.')
  })

  it('switches a running node off with a graceful drain: true stop (task 18)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-1\/stop$/,
        body: nodeFixture({
          desired_state: 'draining',
          effective_state: 'draining',
        }),
      },
    ])

    renderNodes()
    const sw = await screen.findByRole('switch', {
      name: 'Encoding on worker-a',
    })
    expect(sw).toHaveAttribute('aria-checked', 'true')

    // Off means the graceful stop: drain true, no confirmation needed —
    // the current file finishes.
    fireEvent.click(sw)
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/nodes\/node-1\/stop$/)
      expect(calls.length).toBeGreaterThan(0)
      expect(calls[0].body).toEqual({ drain: true })
    })
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('will stop after its current job.')
  })

  it('offers "Stop now" as the hard stop behind a confirmation (task 18)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-1\/stop$/,
        body: nodeFixture({
          desired_state: 'stopped',
          effective_state: 'stopped',
        }),
      },
    ])

    renderNodes()
    fireEvent.click(await screen.findByTestId('node-stop-now-node-1'))
    const dialog = screen.getByTestId('node-stop-dialog-node-1')
    expect(dialog).toHaveTextContent('Stop worker-a now?')
    expect(dialog).toHaveTextContent('stops where it is')
    expect(mock.calls('POST', /\/api\/v1\/nodes\/node-1\/stop$/)).toHaveLength(
      0,
    )

    // Confirming sends the stop with drain false in the body.
    fireEvent.click(screen.getByTestId('node-stop-confirm-node-1'))
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/nodes\/node-1\/stop$/)
      expect(calls.length).toBeGreaterThan(0)
      expect(calls[0].body).toEqual({ drain: false })
    })
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('worker-a stopped.')
  })

  it('tells you what happens to the current file when stopping mid job (task 13)', async () => {
    const runningJob = {
      id: 'job-run-1',
      file_id: 'f1',
      library_id: 'lib-1',
      node_id: 'node-1',
      status: 'running',
      priority: 0,
      attempt: 1,
      encoder_used: '',
      fallback_reasons: [],
      percent: 40,
      speed: 0,
      eta_seconds: 0,
      bytes_before: 0,
      bytes_after: 0,
      bytes_so_far: 0,
      file_name: 'movie.mkv',
      file_path: '/media/movie.mkv',
      log: '',
      error: '',
      created_at: '2026-10-29T00:00:00Z',
    }
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [nodeFixture({ current_job_id: 'job-run-1' })],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [runningJob], total: 1 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/node-1\/stop$/,
        body: nodeFixture({
          desired_state: 'stopped',
          effective_state: 'stopped',
        }),
      },
    ])

    renderNodes()
    await waitFor(() =>
      expect(screen.getByTestId('node-current-job-node-1')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('node-stop-now-node-1'))
    const dialog = screen.getByTestId('node-stop-dialog-node-1')
    expect(dialog).toHaveTextContent('stops mid-way')
    expect(dialog).toHaveTextContent('temporary copy is thrown away')
    expect(dialog).toHaveTextContent('original file is never touched')
    expect(dialog).toHaveTextContent('returns to the queue')
  })

  it('steps concurrency between 1 and 16 (task 11)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture({ concurrency: 1 })], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/nodes\/node-1$/,
        bodyFn: (ctx) => ({
          ...nodeFixture(),
          ...(ctx.body as Record<string, unknown>),
        }),
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-concurrency-node-1'))
    // Down is disabled at the floor of 1.
    expect(screen.getByTestId('node-concurrency-down-node-1')).toBeDisabled()
    expect(screen.getByTestId('node-concurrency-node-1')).toHaveTextContent('1')

    fireEvent.click(screen.getByTestId('node-concurrency-up-node-1'))
    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/nodes\/node-1$/)
      expect(puts.length).toBeGreaterThan(0)
      expect(puts[0].body).toMatchObject({ concurrency: 2 })
    })
  })

  it('shows the error pill with the recorded reason (task 11)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [
            nodeFixture({
              effective_state: 'error',
              state_error: 'pause-all: disk went away',
            }),
          ],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
    ])

    renderNodes()
    await waitFor(() => screen.getByTestId('node-lifecycle-node-1'))
    expect(screen.getByTestId('node-lifecycle-node-1')).toHaveTextContent(
      'Error',
    )
    expect(screen.getByTestId('node-state-error-node-1')).toHaveTextContent(
      'pause-all: disk went away',
    )
  })

  it('primary controls meet the 44px touch minimum (task 20)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [nodeFixture()], total: 1 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
    ])

    renderNodes()
    for (const name of [
      /pause all/i,
      /resume all/i,
      /add node/i,
      /edit/i,
      /stop now/i,
      /revoke/i,
    ]) {
      const el = await screen.findByRole('button', { name })
      expect(el.className).toContain('min-h-11')
    }
  })

  it('shows the empty state when there are no nodes at all', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
    ])
    renderNodes()
    await waitFor(() =>
      expect(screen.getByTestId('nodes-empty')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('nodes-empty')).toHaveTextContent(
      'No other machines yet',
    )
  })

  it('shows per node totals in the card footer (task 18)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [
            nodeFixture({
              stats: { files_done: 2, bytes_saved: 1_500_000_000 },
            }),
          ],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
    ])
    const first = renderNodes()
    const footer = await screen.findByTestId('node-stats-node-1')
    expect(footer).toHaveTextContent('2 files')
    expect(footer).toHaveTextContent('1.5 GB saved')
    first.unmount()

    // A node with no finished jobs reports its zeros explicitly.
    mock.restore()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        body: {
          items: [nodeFixture({ stats: { files_done: 0, bytes_saved: 0 } })],
          total: 1,
        },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
    ])
    renderNodes()
    const fresh = await screen.findByTestId('node-stats-node-1')
    expect(fresh).toHaveTextContent('0 files')
    expect(fresh).toHaveTextContent('0 B saved')
  })

  it('pause all and resume all call their endpoints and disable when redundant (task 18)', async () => {
    // One running, one stopped: pausing means something, resuming does not
    // (not every node is stopped — nothing was paused).
    let running = true
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/nodes$/,
        bodyFn: () => ({
          items: [
            nodeFixture({
              id: 'node-1',
              name: 'worker-a',
              desired_state: running ? 'running' : 'stopped',
              effective_state: running ? 'running' : 'stopped',
            }),
            nodeFixture({
              id: 'node-2',
              name: 'worker-b',
              desired_state: 'stopped',
              effective_state: 'stopped',
            }),
          ],
          total: 2,
        }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/pause-all$/,
        bodyFn: () => {
          running = false
          return { paused: 2 }
        },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/nodes\/resume-all$/,
        body: { resumed: 2 },
      },
    ])

    renderNodes()
    const pause = await screen.findByTestId('nodes-pause-all')
    const resume = screen.getByTestId('nodes-resume-all')
    expect(pause).toBeEnabled()
    expect(resume).toBeDisabled()

    fireEvent.click(pause)
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/nodes\/pause-all$/).length,
      ).toBeGreaterThan(0),
    )
    await waitFor(() =>
      expect(screen.getByTestId('nodes-pause-all')).toBeDisabled(),
    )
    const resumeAfterPause = await screen.findByTestId('nodes-resume-all')
    await waitFor(() => expect(resumeAfterPause).toBeEnabled())

    fireEvent.click(resumeAfterPause)
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/nodes\/resume-all$/).length,
      ).toBeGreaterThan(0),
    )
    // Both actions announced themselves (pause + resume).
    const toasts = await screen.findAllByTestId('toast')
    expect(
      toasts.some((t) => t.textContent?.includes('Resumed 2 nodes.')),
    ).toBe(true)
  })
})
