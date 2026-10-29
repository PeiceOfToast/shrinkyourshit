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
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <NodesPage />
      </MemoryRouter>
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

    // Reject deletes the pending row.
    fireEvent.click(screen.getByTestId('node-reject-node-pending'))
    await waitFor(() =>
      expect(
        mock.calls('DELETE', /\/api\/v1\/nodes\/node-pending$/).length,
      ).toBeGreaterThan(0),
    )
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

  it('renders the lifecycle pill and starts a stopped node (task 11)', async () => {
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
    // The primary toggle is Start while stopped.
    expect(screen.getByTestId('node-start-node-1')).toBeInTheDocument()
    expect(screen.queryByTestId('node-stop-node-1')).not.toBeInTheDocument()

    fireEvent.click(screen.getByTestId('node-start-node-1'))
    await waitFor(() =>
      expect(
        mock.calls('POST', /\/api\/v1\/nodes\/node-1\/start$/).length,
      ).toBeGreaterThan(0),
    )
  })

  it('stops a running node, drain unchecked by default (task 11)', async () => {
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
    await waitFor(() => screen.getByTestId('node-lifecycle-node-1'))
    expect(screen.getByTestId('node-lifecycle-node-1')).toHaveTextContent(
      'Running',
    )
    expect(screen.getByTestId('node-drain-node-1')).not.toBeChecked()

    // Stop now: drain false goes in the body.
    fireEvent.click(screen.getByTestId('node-stop-node-1'))
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/nodes\/node-1\/stop$/)
      expect(calls.length).toBeGreaterThan(0)
      expect(calls[0].body).toEqual({ drain: false })
    })
  })

  it('sends drain: true when finishing the current job first is checked (task 11)', async () => {
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
    await waitFor(() => screen.getByTestId('node-drain-node-1'))
    fireEvent.click(screen.getByTestId('node-drain-node-1'))
    fireEvent.click(screen.getByTestId('node-stop-node-1'))
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/nodes\/node-1\/stop$/)
      expect(calls.length).toBeGreaterThan(0)
      expect(calls[0].body).toEqual({ drain: true })
    })
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
})
