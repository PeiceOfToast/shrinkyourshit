// SPDX-License-Identifier: AGPL-3.0-only

/**
 * EncodingStatus tests (task 40, spec 3.1.1): the four variants, the state
 * derivation from nodes and active jobs, and the switch calling pauseAll
 * and resumeAll. Runs without ffmpeg (fetch is stubbed).
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import EncodingStatus, { type EncodingVariant } from './EncodingStatus'
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function node(id: string, desired: string) {
  return { id, name: id, desired_state: desired, status: 'approved' }
}

function job(id: string, status: string) {
  return {
    id,
    file_id: 'f1',
    library_id: 'lib-1',
    node_id: 'node-1',
    status,
    priority: 0,
    attempt: 1,
    encoder_used: '',
    fallback_reasons: [],
    percent: 10,
    speed: 1,
    eta_seconds: 10,
    bytes_before: 0,
    bytes_after: 0,
    log: '',
    error: '',
    created_at: '2026-10-29T00:00:00Z',
  }
}

let mock: FetchMock

function setup(
  variant: EncodingVariant,
  opts: {
    nodes?: Array<ReturnType<typeof node>>
    jobs?: Array<ReturnType<typeof job>>
    waiting?: number
  } = {},
) {
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/nodes$/,
      body: { items: opts.nodes ?? [], total: (opts.nodes ?? []).length },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/jobs$/,
      body: { items: opts.jobs ?? [], total: (opts.jobs ?? []).length },
    },
    {
      method: 'POST',
      match: /\/api\/v1\/nodes\/pause-all$/,
      body: { paused: 1 },
    },
    {
      method: 'POST',
      match: /\/api\/v1\/nodes\/resume-all$/,
      body: { resumed: 1 },
    },
  ])
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <EncodingStatus variant={variant} waiting={opts.waiting} />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  mock = installFetchMock([])
})

afterEach(() => {
  mock.restore()
})

describe('EncodingStatus rail variant', () => {
  it('shows "1 running" and an on switch when one job runs on a running node', async () => {
    setup('rail', {
      nodes: [node('n1', 'running')],
      jobs: [job('j1', 'running')],
    })
    expect(await screen.findByTestId('job-indicator')).toHaveTextContent(
      '1 running',
    )
    const sw = screen.getByRole('switch', { name: 'Encoding' })
    expect(sw).toHaveAttribute('aria-checked', 'true')
    expect(sw).toBeEnabled()
  })

  it('reads Idle with no active jobs and drops the job indicator', async () => {
    setup('rail', { nodes: [node('n1', 'running')] })
    expect(await screen.findByText('Idle')).toBeInTheDocument()
    expect(screen.queryByTestId('job-indicator')).not.toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Encoding' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
  })

  it('switch off means paused: every node stopped, and turning it on resumes', async () => {
    setup('rail', { nodes: [node('n1', 'stopped'), node('n2', 'draining')] })
    expect(await screen.findByText('Paused')).toBeInTheDocument()
    const sw = screen.getByRole('switch', { name: 'Encoding' })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    fireEvent.click(sw)
    await waitFor(() => {
      expect(mock.calls('POST', /\/api\/v1\/nodes\/resume-all$/).length).toBe(1)
    })
    expect(mock.calls('POST', /\/api\/v1\/nodes\/pause-all$/).length).toBe(0)
  })

  it('switch on calls pauseAll', async () => {
    setup('rail', { nodes: [node('n1', 'running')] })
    const sw = await screen.findByRole('switch', { name: 'Encoding' })
    await waitFor(() => expect(sw).toHaveAttribute('aria-checked', 'true'))
    fireEvent.click(sw)
    await waitFor(() => {
      expect(mock.calls('POST', /\/api\/v1\/nodes\/pause-all$/).length).toBe(1)
    })
    expect(mock.calls('POST', /\/api\/v1\/nodes\/resume-all$/).length).toBe(0)
  })

  it('with no nodes the switch is off and disabled', async () => {
    setup('rail', { nodes: [] })
    expect(await screen.findByText('No nodes')).toBeInTheDocument()
    const sw = screen.getByRole('switch', { name: 'Encoding' })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(sw).toBeDisabled()
  })
})

describe('EncodingStatus compact variant (phone top bar)', () => {
  it('shows nothing when idle', async () => {
    setup('compact', { nodes: [node('n1', 'running')] })
    await waitFor(() => {
      expect(mock.calls('GET', /\/api\/v1\/nodes$/).length).toBeGreaterThan(0)
    })
    await waitFor(() => {
      expect(mock.calls('GET', /\/api\/v1\/jobs$/).length).toBeGreaterThan(0)
    })
    // Let the queries settle, then check nothing rendered.
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.queryByText(/running|Idle|Paused/)).not.toBeInTheDocument()
  })

  it('shows a read-only warning pill when paused, linking to /nodes', async () => {
    setup('compact', { nodes: [node('n1', 'stopped')] })
    const pill = await screen.findByText('Paused')
    expect(pill.closest('a')).toHaveAttribute('href', '/nodes')
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })

  it('shows the running count in the job indicator', async () => {
    setup('compact', {
      nodes: [node('n1', 'running')],
      jobs: [job('j1', 'running'), job('j2', 'claimed')],
    })
    expect(await screen.findByTestId('job-indicator')).toHaveTextContent(
      '2 running',
    )
  })
})

describe('EncodingStatus icon variant (tablet rail)', () => {
  it('is a 44 px link to /nodes that names the state', async () => {
    setup('icon', { nodes: [node('n1', 'stopped')] })
    const link = await screen.findByRole('link', { name: 'Encoding paused' })
    expect(link).toHaveAttribute('href', '/nodes')
    expect(link.className).toContain('min-h-11')
    expect(link.className).toContain('w-11')
  })
})

describe('EncodingStatus panel variant (Dashboard and Nodes)', () => {
  it('says the paused state with the waiting count and resumes from one action', async () => {
    setup('panel', { nodes: [node('n1', 'stopped')], waiting: 3 })
    expect(
      await screen.findByText('Encoding is paused. 3 files are waiting.'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Resume encoding' }))
    await waitFor(() => {
      expect(mock.calls('POST', /\/api\/v1\/nodes\/resume-all$/).length).toBe(1)
    })
  })

  it('says it is paused with nothing waiting when the count is zero', async () => {
    setup('panel', { nodes: [node('n1', 'stopped')], waiting: 0 })
    expect(
      await screen.findByText(
        'Encoding is paused. Nothing is waiting right now.',
      ),
    ).toBeInTheDocument()
  })

  it('reports the shrinking count while running and all caught up when idle', async () => {
    setup('panel', {
      nodes: [node('n1', 'running')],
      jobs: [job('j1', 'running')],
      waiting: 4,
    })
    expect(
      await screen.findByText('Shrinking 1 file · 4 waiting'),
    ).toBeInTheDocument()
  })

  it('says all caught up when idle', async () => {
    setup('panel', { nodes: [node('n1', 'running')] })
    expect(await screen.findByText('All caught up.')).toBeInTheDocument()
  })
})
