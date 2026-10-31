// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Integrations page tests (task 23): the list renders from a mocked
 * getIntegrations, the API key never appears in the DOM, saving the edit
 * form calls updateIntegration with the typed values, and the Test button
 * surfaces the returned error text.
 */
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import IntegrationsPage from './IntegrationsPage'
import { ToastProvider } from './toast'
import {
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'

const secret = 'sk-distinctive-secret-key-9f3a'

const rows = [
  {
    id: 'int-1',
    kind: 'jellyfin',
    name: 'Living room TV',
    base_url: 'http://media.lan:8096',
    api_key_set: true,
    enabled: true,
    refresh_on_done: true,
  },
  {
    id: 'int-2',
    kind: 'radarr',
    name: 'Movies',
    base_url: 'http://radarr.lan:7878',
    api_key_set: true,
    enabled: false,
    refresh_on_done: true,
  },
]

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <IntegrationsPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

beforeEach(() => {
  stubEventSource()
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/integrations$/,
      body: { items: rows, total: rows.length },
    },
  ])
})

afterEach(() => {
  mock.restore()
  vi.unstubAllGlobals()
})

describe('IntegrationsPage', () => {
  it('renders rows from the list: kind, name, address, enabled switch', async () => {
    renderPage()
    const table = await screen.findByRole('table')
    expect(table).toBeInTheDocument()
    // The table row and the stacked card both carry the row (task 20:
    // CSS decides which is visible), so each test id is asserted once.
    expect(screen.getByTestId('integration-row-int-1')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-int-1')).toBeInTheDocument()
    expect(screen.getAllByText('Living room TV').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Jellyfin').length).toBeGreaterThan(0)
    expect(screen.getAllByText('http://media.lan:8096').length).toBeGreaterThan(
      0,
    )
    expect(screen.getAllByText('Movies').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Radarr').length).toBeGreaterThan(0)
    // The enabled switch per row, on for the first, off for the second.
    expect(
      screen
        .getByTestId('integration-switch-int-1')
        .getAttribute('aria-checked'),
    ).toBe('true')
    expect(
      screen
        .getByTestId('integration-switch-int-2')
        .getAttribute('aria-checked'),
    ).toBe('false')
  })

  it('never renders the API key', async () => {
    renderPage()
    await screen.findByRole('table')
    expect(document.body.textContent).not.toContain(secret)
  })

  it('opens the edit form from a row, shows (unchanged) for the key, and saves the typed values', async () => {
    renderPage()
    await screen.findByRole('table')
    fireEvent.click(screen.getByTestId('integration-edit-int-1'))

    const form = screen.getByTestId('integration-form')
    expect(form).toBeInTheDocument()
    expect(screen.getByTestId('integration-name')).toHaveValue('Living room TV')
    expect(screen.getByTestId('integration-base-url')).toHaveValue(
      'http://media.lan:8096',
    )
    // The key field is blank and the placeholder says the key is kept.
    expect(screen.getByTestId('integration-api-key')).toHaveValue('')
    expect(screen.getByTestId('integration-api-key')).toHaveAttribute(
      'placeholder',
      '(unchanged)',
    )
    expect(document.body.textContent).not.toContain(secret)

    fireEvent.change(screen.getByTestId('integration-name'), {
      target: { value: 'Media room TV' },
    })
    fireEvent.change(screen.getByTestId('integration-api-key'), {
      target: { value: 'replacement-key-42' },
    })
    fireEvent.click(screen.getByTestId('integration-save'))

    await waitFor(() => {
      expect(mock.calls('PUT', /\/api\/v1\/integrations\/int-1$/).length).toBe(
        1,
      )
    })
    const call = mock.calls('PUT', /\/api\/v1\/integrations\/int-1$/)[0]
    expect(call.body).toEqual({
      kind: 'jellyfin',
      name: 'Media room TV',
      base_url: 'http://media.lan:8096',
      api_key: 'replacement-key-42',
      enabled: true,
      refresh_on_done: true,
    })
  })

  it('saving an edit with a blank key field keeps the stored key', async () => {
    renderPage()
    await screen.findByRole('table')
    fireEvent.click(screen.getByTestId('integration-edit-int-1'))
    fireEvent.change(screen.getByTestId('integration-name'), {
      target: { value: 'Media room TV' },
    })
    fireEvent.click(screen.getByTestId('integration-save'))

    await waitFor(() => {
      expect(mock.calls('PUT', /\/api\/v1\/integrations\/int-1$/).length).toBe(
        1,
      )
    })
    const call = mock.calls('PUT', /\/api\/v1\/integrations\/int-1$/)[0]
    // An empty api_key means "keep the stored key" (the (unchanged)
    // placeholder), so the secret is never sent around unnecessarily.
    expect((call.body as Record<string, unknown>).api_key).toBe('')
  })

  it('creating through the form calls createIntegration', async () => {
    renderPage()
    await screen.findByRole('table')
    fireEvent.click(screen.getByTestId('integrations-add'))

    fireEvent.change(screen.getByTestId('integration-kind'), {
      target: { value: 'plex' },
    })
    fireEvent.change(screen.getByTestId('integration-name'), {
      target: { value: 'Plex' },
    })
    fireEvent.change(screen.getByTestId('integration-base-url'), {
      target: { value: 'http://plex.lan:32400' },
    })
    fireEvent.change(screen.getByTestId('integration-api-key'), {
      target: { value: 'plex-key-1' },
    })
    fireEvent.click(screen.getByTestId('integration-save'))

    await waitFor(() => {
      expect(mock.calls('POST', /\/api\/v1\/integrations$/).length).toBe(1)
    })
    const call = mock.calls('POST', /\/api\/v1\/integrations$/)[0]
    expect(call.body).toEqual({
      kind: 'plex',
      name: 'Plex',
      base_url: 'http://plex.lan:32400',
      api_key: 'plex-key-1',
      enabled: true,
      refresh_on_done: true,
    })
  })

  it('the Test button surfaces the returned error text', async () => {
    renderPage()
    await screen.findByRole('table')
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/integrations$/,
        body: { items: rows, total: rows.length },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/integrations\/int-1\/test$/,
        status: 200,
        body: {
          ok: false,
          status: 401,
          error: 'unauthorized: the remote refused the call',
        },
      },
    ])
    fireEvent.click(screen.getByTestId('integration-test-int-1'))
    const result = await screen.findByRole('status')
    expect(result).toBeInTheDocument()
    expect(result.textContent).toContain('the remote refused the call')
  })

  it('the Test button reports a healthy remote', async () => {
    renderPage()
    await screen.findByRole('table')
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/integrations$/,
        body: { items: rows, total: rows.length },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/integrations\/int-2\/test$/,
        status: 200,
        body: { ok: true, status: 204 },
      },
    ])
    fireEvent.click(screen.getByTestId('integration-test-int-2'))
    const result = await screen.findByRole('status')
    expect(result.textContent).toContain('Reached Movies')
  })

  it('delete asks for confirmation and removes the row', async () => {
    renderPage()
    await screen.findByRole('table')
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/integrations$/,
        body: { items: rows, total: rows.length },
      },
      {
        method: 'DELETE',
        match: /\/api\/v1\/integrations\/int-1$/,
        status: 204,
      },
    ])
    fireEvent.click(screen.getByTestId('integration-delete-int-1'))
    const confirm = screen.getByTestId('confirm-dialog')
    expect(confirm).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('integration-delete-confirm'))
    await waitFor(() => {
      expect(
        mock.calls('DELETE', /\/api\/v1\/integrations\/int-1$/).length,
      ).toBe(1)
    })
  })
})

describe('IntegrationsPage webhooks tab (task 25)', () => {
  const hooks = [
    {
      id: 'hook-1',
      name: 'Discord',
      url: 'https://discord.com/api/webhooks/1/abc',
      events: ['job.done'],
      enabled: true,
      secret_set: true,
      created_at: '2026-10-30T10:00:00Z',
      updated_at: '2026-10-30T10:00:00Z',
    },
  ]

  const deliveries = [
    {
      id: 'del-2',
      event: 'job.done',
      status: 204,
      created_at: '2026-10-30T10:05:00Z',
    },
    {
      id: 'del-1',
      event: 'job.done',
      status: 500,
      error: 'boom',
      created_at: '2026-10-30T10:03:00Z',
    },
  ]

  beforeEach(() => {
    stubEventSource()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/integrations$/,
        body: { items: rows, total: rows.length },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/webhooks$/,
        body: { items: hooks, total: hooks.length },
      },
    ])
  })

  afterEach(() => {
    mock.restore()
    vi.unstubAllGlobals()
  })

  /** Renders the page, switches to the Webhooks tab and waits for the
   * hook list to be on screen. */
  async function renderWebhooksTab() {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <ToastProvider>
          <MemoryRouter>
            <IntegrationsPage />
          </MemoryRouter>
        </ToastProvider>
      </QueryClientProvider>,
    )
    fireEvent.click(await screen.findByTestId('integrations-tab-webhooks'))
    await screen.findByTestId('webhook-row-hook-1')
  }

  it('renders the hook rows: name, url, enabled switch', async () => {
    await renderWebhooksTab()
    expect(await screen.findByTestId('webhook-row-hook-1')).toBeInTheDocument()
    expect(screen.getAllByText('Discord').length).toBeGreaterThan(0)
    expect(
      screen.getByTestId('webhook-switch-hook-1').getAttribute('aria-checked'),
    ).toBe('true')
    // The secret is never rendered anywhere.
    expect(document.body.textContent).not.toContain('whsec')
  })

  it('creating a webhook shows the secret once with a copy button and never again after a reload', async () => {
    await renderWebhooksTab()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/webhooks$/,
        body: { items: hooks, total: hooks.length },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/webhooks$/,
        status: 201,
        body: {
          id: 'hook-2',
          name: 'New hook',
          url: 'https://example.com/hook',
          events: ['job.done'],
          enabled: true,
          secret_set: true,
          created_at: '2026-10-30T11:00:00Z',
          updated_at: '2026-10-30T11:00:00Z',
          secret: 'whsec-once-only-3b2f9d1c',
        },
      },
    ])

    fireEvent.click(screen.getByTestId('webhooks-add'))
    fireEvent.change(screen.getByTestId('webhook-name'), {
      target: { value: 'New hook' },
    })
    fireEvent.change(screen.getByTestId('webhook-url'), {
      target: { value: 'https://example.com/hook' },
    })
    fireEvent.click(screen.getByTestId('webhook-save'))

    // The secret box appears exactly once, with the copy button.
    const box = await screen.findByTestId('webhook-secret-box')
    expect(box.textContent).toContain('whsec-once-only-3b2f9d1c')
    expect(screen.getByTestId('webhook-secret-copy')).toBeInTheDocument()

    // A reload: a fresh mount shows the list again, and the secret is
    // nowhere in it — not in the rows, not in a "secret" field.
    cleanup()
    await renderWebhooksTab()
    expect(screen.getByTestId('webhook-row-hook-1')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('whsec-once-only-3b2f9d1c')
    expect(document.body.textContent).not.toContain('"secret"')
  })

  it('the deliveries dialog renders each attempt with time, event and status', async () => {
    await renderWebhooksTab()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/webhooks\/hook-1\/deliveries$/,
        body: { items: deliveries, total: deliveries.length },
      },
    ])
    fireEvent.click(screen.getByTestId('webhook-deliveries-hook-1'))
    const dialog = await screen.findByTestId('webhook-deliveries-dialog')
    expect(dialog).toBeInTheDocument()
    expect(
      await screen.findByTestId('delivery-status-del-1'),
    ).toHaveTextContent('500')
    expect(screen.getByTestId('delivery-status-del-2')).toHaveTextContent('204')
    expect(screen.getByTestId('delivery-event-del-1')).toHaveTextContent(
      'A file is shrunk',
    )
    expect(screen.getByTestId('delivery-error-del-1')).toHaveTextContent('boom')
  })

  it('the Test button reports the result', async () => {
    await renderWebhooksTab()
    mock = installFetchMock([
      {
        method: 'POST',
        match: /\/api\/v1\/webhooks\/hook-1\/test$/,
        status: 200,
        body: { ok: false, status: 500, error: 'boom' },
      },
    ])
    fireEvent.click(screen.getByTestId('webhook-test-hook-1'))
    const result = await screen.findByTestId('webhook-test-result')
    expect(result.textContent).toContain('boom')
  })

  it('the enabled switch flips the hook without opening the form, keeping the secret out of the body', async () => {
    await renderWebhooksTab()
    mock = installFetchMock([
      {
        method: 'PUT',
        match: /\/api\/v1\/webhooks\/hook-1$/,
        status: 200,
        body: {
          id: 'hook-1',
          name: 'Discord',
          url: 'https://discord.com/api/webhooks/1/abc',
          events: ['job.done'],
          enabled: false,
          secret_set: true,
          created_at: '2026-10-30T10:00:00Z',
          updated_at: '2026-10-30T10:00:00Z',
        },
      },
    ])
    fireEvent.click(screen.getByTestId('webhook-switch-hook-1'))
    await waitFor(() => {
      expect(mock.calls('PUT', /\/api\/v1\/webhooks\/hook-1$/).length).toBe(1)
    })
    // The body never carries a secret: the server keeps the stored one.
    const call = mock.calls('PUT', /\/api\/v1\/webhooks\/hook-1$/)[0]
    expect(Object.keys(call.body as Record<string, unknown>)).toEqual([
      'name',
      'url',
      'events',
      'enabled',
    ])
  })
})
