// SPDX-License-Identifier: AGPL-3.0-only
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { ApiError, getHealth, getMe, getSettings, getSetupStatus } from './api'
import type { SetupStatus } from './api'

vi.mock('./api', () => ({
  ApiError: class ApiError extends Error {
    status: number
    code: string
    constructor(message: string, status: number, code: string) {
      super(message)
      this.status = status
      this.code = code
    }
  },
  getHealth: vi.fn(),
  getSetupStatus: vi.fn(),
  getMe: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  completeSetup: vi.fn(),
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
}))

const mockStatus = vi.mocked(getSetupStatus)
const mockMe = vi.mocked(getMe)
const mockHealth = vi.mocked(getHealth)
const mockSettings = vi.mocked(getSettings)

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

const incomplete: SetupStatus = { setup_complete: false, login_enabled: false }
const openNoLogin: SetupStatus = { setup_complete: true, login_enabled: false }
const loginOn: SetupStatus = {
  setup_complete: true,
  login_enabled: true,
  username: 'admin',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockHealth.mockResolvedValue({ status: 'ok', version: 'v0.0.0' })
})

describe('route guard', () => {
  it('redirects every route to /setup while setup is incomplete', async () => {
    mockStatus.mockResolvedValue(incomplete)
    renderAppAt('/settings')
    await waitFor(() => {
      expect(screen.getByTestId('wizard-step-1')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('settings-page')).not.toBeInTheDocument()
  })

  it('redirects to /login when login is enabled and there is no session', async () => {
    mockStatus.mockResolvedValue(loginOn)
    mockMe.mockRejectedValue(
      new ApiError('login required', 401, 'unauthorized'),
    )
    renderAppAt('/')
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
  })

  it('shows the app shell when login is enabled and the session is valid', async () => {
    mockStatus.mockResolvedValue(loginOn)
    mockMe.mockResolvedValue(loginOn)
    renderAppAt('/')
    await waitFor(() => {
      expect(
        screen.getByRole('navigation', { name: 'Main' }),
      ).toBeInTheDocument()
    })
    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.getByText('Libraries')).toBeInTheDocument()
    expect(screen.getByText('Nodes')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('shows the app without login when login is disabled', async () => {
    mockStatus.mockResolvedValue(openNoLogin)
    mockSettings.mockResolvedValue({
      worker_concurrency: 1,
      scratch_dir: '',
      default_result_policy: 'replace_if_smaller',
      login_enabled: false,
      preferred_languages: ['eng'],
      default_video_codec: 'hevc',
    })
    renderAppAt('/settings')
    await waitFor(() => {
      expect(screen.getByTestId('settings-page')).toBeInTheDocument()
    })
    // No session was ever requested.
    expect(mockMe).not.toHaveBeenCalled()
  })

  it('sends the user back to / from the setup page once setup is done', async () => {
    mockStatus.mockResolvedValue(openNoLogin)
    renderAppAt('/setup')
    await waitFor(() => {
      expect(
        screen.getByRole('navigation', { name: 'Main' }),
      ).toBeInTheDocument()
    })
  })

  it('keeps the user on /login when credentials are wrong', async () => {
    mockStatus.mockResolvedValue(loginOn)
    mockMe.mockRejectedValue(
      new ApiError('wrong username or password', 401, 'invalid_credentials'),
    )
    renderAppAt('/login')
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
  })
})
