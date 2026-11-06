// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The old library rule routes (task 73): a library with a profile lands in
 * that profile's editor, the same editor per route; a library without one
 * lands on its edit page to choose a profile.
 */
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import LibraryRulesRedirect from './LibraryRulesRedirect'
import { installFetchMock, type FetchMock } from './test/fetchMock'

let mock: FetchMock | undefined

afterEach(() => {
  mock?.restore()
  mock = undefined
})

function Where() {
  const loc = useLocation()
  return <p data-testid="where">{loc.pathname}</p>
}

function renderAt(path: string, profileId: string) {
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/lib-1$/,
      body: {
        id: 'lib-1',
        name: 'Movies',
        path: '/media/movies',
        profile_id: profileId,
        watch: true,
      },
    },
  ])
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            path="/libraries/:id/rules"
            element={<LibraryRulesRedirect />}
          />
          <Route
            path="/libraries/:id/flow"
            element={<LibraryRulesRedirect to="flow" />}
          />
          <Route
            path="/libraries/:id/tree"
            element={<LibraryRulesRedirect to="tree" />}
          />
          <Route path="*" element={<Where />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('LibraryRulesRedirect (task 73)', () => {
  it('sends the rules route to the profile editor', async () => {
    renderAt('/libraries/lib-1/rules', 'prof-1')
    await waitFor(() =>
      expect(screen.getByTestId('where')).toHaveTextContent(
        '/encode/profiles/prof-1',
      ),
    )
  })

  it('keeps the flow and tree suffixes on the profile route', async () => {
    const first = renderAt('/libraries/lib-1/flow', 'prof-1')
    await waitFor(() =>
      expect(screen.getByTestId('where')).toHaveTextContent(
        '/encode/profiles/prof-1/flow',
      ),
    )
    first.unmount()
    renderAt('/libraries/lib-1/tree', 'prof-1')
    await waitFor(() =>
      expect(screen.getByTestId('where')).toHaveTextContent(
        '/encode/profiles/prof-1/tree',
      ),
    )
  })

  it('sends a library with no profile to its edit page', async () => {
    renderAt('/libraries/lib-1/rules', '')
    await waitFor(() =>
      expect(screen.getByTestId('where')).toHaveTextContent(
        '/libraries/lib-1/edit',
      ),
    )
  })
})
