// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Route guard and shell tests (task 06 semantics, task 07 shell): setup
 * incomplete sends everything to /setup; login enabled without a session
 * sends everything to /login; login disabled is open. The API is stubbed
 * at the fetch level so the real api.ts client runs.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import App from './App'
import {
  FakeEventSource,
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'

function renderAppAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const incomplete = { setup_complete: false, login_enabled: false }
const openNoLogin = { setup_complete: true, login_enabled: false }
const loginOn = {
  setup_complete: true,
  login_enabled: true,
  username: 'admin',
}
const settings = {
  worker_concurrency: 1,
  scratch_dir: '',
  default_result_policy: 'replace_if_smaller',
  login_enabled: false,
  preferred_languages: ['eng'],
  default_video_codec: 'hevc',
}

let mock: FetchMock

beforeEach(() => {
  stubEventSource()
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/health$/,
      body: { status: 'ok', version: 'v0.0.0' },
    },
    { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: openNoLogin },
    { method: 'GET', match: /\/api\/v1\/auth\/me$/, body: loginOn },
    { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries$/,
      body: { items: [], total: 0 },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/stats$/,
      body: {
        files: 0,
        new: 0,
        satisfied: 0,
        queued: 0,
        processing: 0,
        done: 0,
        failed: 0,
        missing: 0,
        bytes_saved: 0,
        libraries: [],
      },
    },
    { method: 'GET', match: /\/api\/v1\/jobs$/, body: { items: [], total: 0 } },
    { method: 'GET', match: /\/api\/v1\/nodes$/, body: { items: [], total: 0 } },
  ])
})

afterEach(() => mock.restore())

describe('route guard', () => {
  it('redirects every route to /setup while setup is incomplete', async () => {
    // Serve an incomplete setup for this test only.
    mock.restore()
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: incomplete },
      {
        method: 'GET',
        match: /\/api\/v1\/health$/,
        body: { status: 'ok', version: 'v0.0.0' },
      },
    ])
    renderAppAt('/settings')
    await waitFor(() => {
      expect(screen.getByTestId('wizard-step-1')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('settings-page')).not.toBeInTheDocument()
  })

  it('redirects to /login when login is enabled and there is no session', async () => {
    mock.restore()
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: loginOn },
      {
        method: 'GET',
        match: /\/api\/v1\/auth\/me$/,
        status: 401,
        body: { error: 'login required', code: 'unauthorized' },
      },
    ])
    renderAppAt('/')
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
  })

  it('shows the app shell when login is enabled and the session is valid', async () => {
    renderAppAt('/')
    await waitFor(() => {
      expect(
        screen.getByRole('navigation', { name: 'Main' }),
      ).toBeInTheDocument()
    })
    // Five entries (task 10): Dash, Library, Nodes, Encode settings,
    // App settings.
    const nav = screen.getByRole('navigation', { name: 'Main' })
    const links = nav.querySelectorAll('a')
    expect(links).toHaveLength(5)
    const labels = Array.from(links).map((a) => a.textContent)
    expect(labels).toEqual([
      'Dash',
      'Library',
      'Nodes',
      'Encode settings',
      'App settings',
    ])
  })

  it('renders each of the five nav routes with its page', async () => {
    const cases: Array<[string, string]> = [
      ['/', 'dashboard'],
      ['/libraries', 'libraries-page'],
      ['/nodes', 'nodes-page'],
      ['/encode', 'encode-settings-page'],
      ['/app', 'app-settings-page'],
    ]
    for (const [path, testid] of cases) {
      const { unmount } = renderAppAt(path)
      await waitFor(() => {
        expect(screen.getByTestId(testid)).toBeInTheDocument()
      })
      unmount()
    }
  })

  it('redirects /settings to /app so old links keep working', async () => {
    renderAppAt('/settings')
    await waitFor(() => {
      expect(screen.getByTestId('app-settings-page')).toBeInTheDocument()
    })
  })

  it('paints the page background on html and body', async () => {
    renderAppAt('/')
    await waitFor(() => {
      expect(screen.getByTestId('dashboard')).toBeInTheDocument()
    })
    expect(document.documentElement.classList.contains('bg-slate-950')).toBe(
      true,
    )
    expect(document.body.classList.contains('bg-slate-950')).toBe(true)
  })

  it('shows the app without login when login is disabled', async () => {
    renderAppAt('/app')
    await waitFor(() => {
      expect(screen.getByTestId('app-settings-page')).toBeInTheDocument()
    })
    // No session was ever requested.
    expect(
      mock.requests.filter((r) => r.url === '/api/v1/auth/me'),
    ).toHaveLength(0)
  })

  it('lands on the dashboard empty state from / with no form and no create button', async () => {
    renderAppAt('/')
    await waitFor(() => {
      expect(screen.getByTestId('empty-dashboard')).toBeInTheDocument()
    })
    // Read only (task 10): one line and a link, nothing to fill in.
    expect(
      screen.getByRole('link', { name: 'Add one' }),
    ).toHaveAttribute('href', '/libraries/new')
    expect(screen.queryByTestId('easy-form')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('dashboard-add-library'),
    ).not.toBeInTheDocument()
    expect(
      screen.getByTestId('empty-dashboard').querySelector('form'),
    ).toBeNull()
  })

  it('sends the user back to / from the setup page once setup is done', async () => {
    renderAppAt('/setup')
    await waitFor(() => {
      expect(
        screen.getByRole('navigation', { name: 'Main' }),
      ).toBeInTheDocument()
    })
  })

  it('keeps the user on /login when credentials are wrong', async () => {
    mock.restore()
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: loginOn },
      {
        method: 'GET',
        match: /\/api\/v1\/auth\/me$/,
        status: 401,
        body: {
          error: 'wrong username or password',
          code: 'invalid_credentials',
        },
      },
    ])
    renderAppAt('/login')
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
  })

  it('subscribes to the SSE stream once the shell is up', async () => {
    renderAppAt('/')
    await waitFor(() => {
      expect(
        screen.getByRole('navigation', { name: 'Main' }),
      ).toBeInTheDocument()
    })
    // One stream, mounted by the shell.
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0].url).toBe('/api/v1/events')
  })
})
