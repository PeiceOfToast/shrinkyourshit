// SPDX-License-Identifier: AGPL-3.0-only

/**
 * New library page (task 41): the FormSection layout, the "Add library" and
 * Cancel action bar, and the disabled state that explains itself.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import NewLibraryPage from './NewLibraryPage'
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

let mock: FetchMock

afterEach(() => mock?.restore())

function Where() {
  const loc = useLocation()
  return <p data-testid="where">{loc.pathname}</p>
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/libraries/new']}>
          <Routes>
            <Route
              path="/libraries/new"
              element={
                <>
                  <NewLibraryPage />
                  <Where />
                </>
              }
            />
            <Route path="/libraries" element={<Where />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

describe('NewLibraryPage (task 41)', () => {
  it('lays the form out as FormSections with the field column capped at max-w-2xl', () => {
    mock = installFetchMock([])
    renderPage()
    for (const title of ['Folder', 'How much to shrink', 'Watching']) {
      const heading = screen.getByRole('heading', { level: 2, name: title })
      const column = heading.closest('section')!.querySelector('.max-w-2xl')
      expect(column, `${title} has no max-w-2xl column`).not.toBeNull()
    }
  })

  it('names the primary action "Add library" and offers Cancel back to the list', () => {
    mock = installFetchMock([])
    renderPage()
    expect(screen.getByTestId('easy-start')).toHaveTextContent('Add library')
    fireEvent.click(screen.getByTestId('easy-cancel'))
    expect(screen.getByTestId('where')).toHaveTextContent('/libraries')
  })

  it('explains the disabled state with help linked to the button', () => {
    mock = installFetchMock([])
    renderPage()
    const start = screen.getByTestId('easy-start')
    expect(start).toBeDisabled()
    const help = screen.getByText('Pick a folder first.')
    expect(start).toHaveAttribute('aria-describedby', help.id)
  })

  it('drops the explanation once a folder is chosen', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/fs\/browse$/,
        body: { path: '/', parent: '', entries: [] },
      },
    ])
    renderPage()
    fireEvent.click(screen.getByTestId('easy-choose-folder'))
    await waitFor(() =>
      expect(screen.getByTestId('folder-picker-use')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('folder-picker-use'))
    await waitFor(() =>
      expect(screen.getByTestId('easy-start')).not.toBeDisabled(),
    )
    expect(screen.queryByText('Pick a folder first.')).not.toBeInTheDocument()
    expect(screen.getByTestId('easy-start')).not.toHaveAttribute(
      'aria-describedby',
    )
  })

  it('keeps the action bar sticky on phones', () => {
    mock = installFetchMock([])
    renderPage()
    const bar = screen.getByTestId('easy-start').parentElement!
    expect(bar.className).toContain('sticky')
    expect(bar.className).toContain('bottom-0')
  })
})
