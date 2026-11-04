// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Login page (task 51, spec 5.2): one h1 with the real title, a full-viewport
 * screen, 44 px controls, and a wrong-password error shown as a danger Banner
 * under the `login-error` test id. The API is stubbed at the fetch level.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import LoginPage from './LoginPage'
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

const loginOn = {
  setup_complete: true,
  login_enabled: true,
  username: 'admin',
}

let mock: FetchMock

function renderLogin() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/login']}>
          <LoginPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  mock = installFetchMock([
    { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: loginOn },
    {
      method: 'POST',
      match: /\/api\/v1\/auth\/login$/,
      status: 401,
      body: {
        error: 'wrong username or password',
        code: 'invalid_credentials',
      },
    },
  ])
})

afterEach(() => mock.restore())

describe('LoginPage (task 51)', () => {
  it('has one h1 with the title, on a full-viewport screen', async () => {
    const { container } = renderLogin()
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
    expect(headings[0]).toHaveTextContent('Log in')
    expect(container.querySelector('main')).toHaveClass('min-h-dvh')
  })

  it('uses 44 px controls for the fields and the submit button', async () => {
    renderLogin()
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
    expect(screen.getByTestId('login-username')).toHaveClass('min-h-11')
    expect(screen.getByTestId('login-password')).toHaveClass('min-h-11')
    expect(screen.getByTestId('login-submit')).toHaveClass('min-h-11', 'w-full')
  })

  it('labels each field through Field', async () => {
    renderLogin()
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
    expect(screen.getByLabelText('Username')).toBe(
      screen.getByTestId('login-username'),
    )
    expect(screen.getByLabelText('Password')).toBe(
      screen.getByTestId('login-password'),
    )
  })

  it('shows a wrong-password error as a danger Banner above the button', async () => {
    renderLogin()
    await waitFor(() => {
      expect(screen.getByTestId('login-form')).toBeInTheDocument()
    })
    fireEvent.change(screen.getByTestId('login-username'), {
      target: { value: 'admin' },
    })
    fireEvent.change(screen.getByTestId('login-password'), {
      target: { value: 'wrong' },
    })
    fireEvent.click(screen.getByTestId('login-submit'))

    const banner = await screen.findByTestId('login-error')
    expect(banner).toHaveTextContent('Wrong username or password.')
    expect(banner).toHaveAttribute('role', 'alert')
    expect(banner).toHaveClass('bg-danger/10', 'ring-danger/30')
    // The Banner sits before the submit button in the form.
    const submit = screen.getByTestId('login-submit')
    expect(
      banner.compareDocumentPosition(submit) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })
})
