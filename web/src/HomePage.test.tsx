// SPDX-License-Identifier: AGPL-3.0-only
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import HomePage from './HomePage'
import { getHealth } from './api'

vi.mock('./api', () => ({
  getHealth: vi.fn(),
}))

const mockGetHealth = vi.mocked(getHealth)

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <HomePage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('HomePage', () => {
  it('shows the project name and the health response', async () => {
    mockGetHealth.mockResolvedValue({ status: 'ok', version: 'v9.9.9' })
    renderPage()

    expect(
      screen.getByRole('heading', { name: 'ShrinkYourShit' }),
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByTestId('health')).toHaveTextContent('API ok · v9.9.9'),
    )
  })

  it('shows a friendly message when the API is unreachable', async () => {
    mockGetHealth.mockRejectedValue(new Error('connection refused'))
    renderPage()

    await waitFor(() =>
      expect(screen.getByTestId('health')).toHaveTextContent('API unreachable'),
    )
  })
})
