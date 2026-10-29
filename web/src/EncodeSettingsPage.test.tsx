// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode settings tests (task 13): a successful save toasts, and a failed
 * save renders the HTTP status, the server code and the raw body behind a
 * disclosure — never a generic one-liner.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import EncodeSettingsPage from './EncodeSettingsPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

const settings = {
  worker_concurrency: 1,
  scratch_dir: '',
  default_result_policy: 'replace_if_smaller',
  login_enabled: false,
  admin_username: '',
  preferred_languages: ['eng'],
  default_video_codec: 'hevc',
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <EncodeSettingsPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('EncodeSettingsPage', () => {
  it('toasts on a successful save (task 13)', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'PUT',
        match: /\/api\/v1\/settings$/,
        body: settings,
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('save-settings'))
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Settings saved.')
  })

  it('renders the HTTP status, server code and raw JSON when a save fails (task 13)', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'PUT',
        match: /\/api\/v1\/settings$/,
        status: 500,
        body: { error: 'the database is read only', code: 'internal' },
      },
    ])
    renderPage()
    fireEvent.click(await screen.findByTestId('save-settings'))

    const box = await screen.findByTestId('settings-error')
    expect(box).toHaveTextContent('Saving the settings failed.')
    expect(screen.getByTestId('error-status')).toHaveTextContent('500')
    expect(screen.getByTestId('error-code')).toHaveTextContent('internal')
    expect(box).toHaveTextContent('the database is read only')

    // The raw body is one click away, not shouted by default.
    const raw = screen.getByTestId('error-raw')
    expect(raw).not.toBeVisible()
    fireEvent.click(raw.querySelector('summary')!)
    await waitFor(() => expect(screen.getByTestId('error-raw')).toBeVisible())
    expect(screen.getByTestId('error-raw')).toHaveTextContent(
      '"code":"internal"',
    )
  })

  it('renders the load failure with status and code (task 13)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/settings$/,
        status: 503,
        body: { error: 'coming up', code: 'unavailable' },
      },
    ])
    renderPage()
    const box = await screen.findByTestId('error-box')
    expect(box).toHaveTextContent('Cannot load the settings.')
    expect(screen.getByTestId('error-status')).toHaveTextContent('503')
    expect(screen.getByTestId('error-code')).toHaveTextContent('unavailable')
  })
})
