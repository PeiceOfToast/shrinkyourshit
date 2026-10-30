// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App settings tests (task 13): a successful save toasts, a failed save
 * renders the HTTP status and server code, and the login-off flow still
 * confirms before it changes anything.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import AppSettingsPage from './AppSettingsPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

const settings = {
  worker_concurrency: 2,
  scratch_dir: '/scratch',
  default_result_policy: 'replace_if_smaller',
  login_enabled: true,
  admin_username: 'michael',
  preferred_languages: ['eng'],
  default_video_codec: 'hevc',
  update_check: true,
  update_channel: 'stable',
  backup_before_update: true,
}

/** The About card (task 17) rides on the App settings page. */
const aboutRoutes = [
  {
    method: 'GET',
    match: /\/api\/v1\/version$/,
    body: {
      version: 'v1.2.0',
      commit: 'abc1234',
      date: '2026-01-02T03:04:05Z',
      runtime: 'binary',
      update_check_enabled: true,
      channel: 'stable',
    },
  },
  {
    method: 'GET',
    match: /\/api\/v1\/update\/status$/,
    body: {
      current: 'v1.2.0',
      latest: 'v1.2.0',
      update_available: false,
      self_update_supported: true,
      reason: 'already on the latest release',
    },
  },
]

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <AppSettingsPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('AppSettingsPage', () => {
  it('toasts on a successful save (task 13)', async () => {
    mock = installFetchMock([
      ...aboutRoutes,
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      { method: 'PUT', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('save-settings'))
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Settings saved.')
  })

  it('renders the HTTP status and server code when a save fails (task 13)', async () => {
    mock = installFetchMock([
      ...aboutRoutes,
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'PUT',
        match: /\/api\/v1\/settings$/,
        status: 401,
        body: {
          error: 'current password is wrong',
          code: 'invalid_credentials',
        },
      },
    ])
    renderPage()
    fireEvent.change(await screen.findByTestId('new-password'), {
      target: { value: 'new-secret' },
    })
    fireEvent.change(screen.getByTestId('current-password'), {
      target: { value: 'wrong' },
    })
    fireEvent.click(screen.getByTestId('change-password'))

    const box = await screen.findByTestId('settings-error')
    expect(screen.getByTestId('error-status')).toHaveTextContent('401')
    expect(screen.getByTestId('error-code')).toHaveTextContent(
      'invalid_credentials',
    )
    expect(box).toHaveTextContent('current password is wrong')
  })

  it('turning login off still asks first and then saves', async () => {
    const puts: unknown[] = []
    mock = installFetchMock([
      ...aboutRoutes,
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'PUT',
        match: /\/api\/v1\/settings$/,
        bodyFn: (ctx) => {
          puts.push(ctx.body)
          return { ...settings, login_enabled: false }
        },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('turn-login-off'))
    expect(screen.getByTestId('confirm-login-off')).toHaveTextContent(
      'without logging in',
    )
    expect(puts).toHaveLength(0)

    fireEvent.click(screen.getByTestId('confirm-off'))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]).toMatchObject({ login_enabled: false })
    await screen.findByTestId('toast')
  })

  it('renders the About card with the version and the runtime (task 17)', async () => {
    mock = installFetchMock([
      ...aboutRoutes,
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderPage()
    await screen.findByTestId('about-card')
    await waitFor(() =>
      expect(screen.getByTestId('about-version')).toHaveTextContent('v1.2.0'),
    )
    expect(screen.getByTestId('about-runtime')).toHaveTextContent(
      'App on this machine',
    )
    expect(screen.getByTestId('check-now')).toBeInTheDocument()
  })
})
