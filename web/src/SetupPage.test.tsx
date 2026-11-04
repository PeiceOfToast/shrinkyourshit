// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Setup page screen rules (task 51, spec 5.1): the brand is a div, not a
 * heading; each step has one h1; the screen is min-h-dvh; and the joined
 * panel has one h1 and an info Banner. The API is stubbed at the fetch level.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import SetupPage from './SetupPage'
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

const incomplete = { setup_complete: false, login_enabled: false }

let mock: FetchMock

function renderSetup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/setup']}>
          <SetupPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  mock = installFetchMock([
    { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: incomplete },
    {
      method: 'GET',
      match: /\/api\/v1\/setup\/servers$/,
      body: { servers: [] },
    },
  ])
})

afterEach(() => mock.restore())

describe('SetupPage (task 51)', () => {
  it('keeps the brand out of the headings and gives the step one h1', async () => {
    const { container } = renderSetup()
    await waitFor(() => {
      expect(screen.getByTestId('wizard-step-1')).toBeInTheDocument()
    })
    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
    expect(headings[0]).toHaveTextContent('How will you use this machine?')
    expect(
      screen.queryByRole('heading', { name: 'ShrinkYourShit' }),
    ).not.toBeInTheDocument()
    expect(screen.getByText('ShrinkYourShit').tagName).toBe('DIV')
    expect(container.querySelector('main')).toHaveClass('min-h-dvh')
  })

  it('shows the joined panel with one h1 and an info Banner', async () => {
    // Routes match in order, so the override goes in front of the default.
    mock.routes.unshift({
      method: 'GET',
      match: /\/api\/v1\/setup\/servers$/,
      body: {
        servers: [
          {
            id: 'srv-1',
            name: 'media-box',
            url: 'http://10.0.0.5:8765',
            version: '',
          },
        ],
      },
    })
    // The real server answers a join with setup_complete: true, which sends
    // RedirectWhenSetupComplete to "/" before the panel shows (pre-existing,
    // see tasks/51-notes.md). Answer with false here so the panel renders.
    mock.routes.unshift({
      method: 'POST',
      match: /\/api\/v1\/setup$/,
      body: {
        setup_complete: false,
        login_enabled: false,
        mode: 'join',
        server: 'media-box',
      },
    })
    renderSetup()

    await waitFor(() => {
      expect(screen.getByTestId('mode-join')).toBeEnabled()
    })
    fireEvent.click(screen.getByTestId('mode-join'))
    fireEvent.click(screen.getByTestId('join-server-srv-1'))
    fireEvent.click(screen.getByTestId('join-confirm'))

    const panel = await screen.findByTestId('joined-panel')
    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
    expect(panel).toContainElement(headings[0])
    expect(headings[0]).toHaveTextContent('Joining media-box')
    // The Banner's role sits on its outer box, above the sentence.
    expect(
      screen
        .getByText("Approve this machine on the server's Nodes page.")
        .closest('[role="status"]'),
    ).not.toBeNull()
  })
})
