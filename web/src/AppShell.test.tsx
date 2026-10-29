// SPDX-License-Identifier: AGPL-3.0-only

/**
 * AppShell tests (task 10) with a mocked matchMedia: the narrow layout
 * (390 px phone: top bar, drawer, no fixed rail) and the wide layout
 * (md and up: rail, collapsible to icons). jsdom cannot compute layout,
 * so the assertions pin the structure that produces the geometry: at
 * 390 px there is no fixed 224 px sidebar and the content column is the
 * full-width flexible one, with the nav one tap away in the drawer.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppShell from './AppShell'
import { ToastProvider } from './toast'
import {
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'

function LibrariesProbe() {
  const { pathname } = useLocation()
  return <div data-testid="page-under-test">{pathname}</div>
}

type MediaStub = ReturnType<typeof stubMatchMedia>

/**
 * Installs a matchMedia stub that answers every query with `matches`.
 * Returns the change listeners so a test can simulate a rotation.
 */
function stubMatchMedia(matches: boolean) {
  const listeners: Array<(event: { matches: boolean }) => void> = []
  const impl = (query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: (_: string, cb: (event: { matches: boolean }) => void) =>
      listeners.push(cb),
    removeEventListener: (
      _: string,
      cb: (event: { matches: boolean }) => void,
    ) => {
      const index = listeners.indexOf(cb)
      if (index >= 0) listeners.splice(index, 1)
    },
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })
  vi.stubGlobal('matchMedia', vi.fn(impl))
  return {
    /** Simulate the viewport crossing the breakpoint. */
    set(matchesNow: boolean) {
      for (const cb of [...listeners]) cb({ matches: matchesNow })
    },
  }
}

const openNoLogin = { setup_complete: true, login_enabled: false }

function renderShell(path = '/') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route element={<AppShell />}>
              <Route
                path="*"
                element={<div data-testid="page-under-test">page</div>}
              />
              <Route path="/libraries" element={<LibrariesProbe />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

const runningJob = {
  id: 'job-run-1',
  file_id: 'f1',
  library_id: 'lib-1',
  node_id: 'node-1',
  status: 'running',
  priority: 0,
  attempt: 1,
  encoder_used: '',
  fallback_reasons: [],
  percent: 40,
  speed: 1.5,
  eta_seconds: 30,
  bytes_before: 0,
  bytes_after: 0,
  log: '',
  error: '',
  created_at: '2026-10-29T00:00:00Z',
}

let mock: FetchMock
let media: MediaStub

beforeEach(() => {
  stubEventSource()
  mock = installFetchMock([
    { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: openNoLogin },
    {
      method: 'GET',
      match: /\/api\/v1\/health$/,
      body: { status: 'ok', version: 'v0.0.0' },
    },
    { method: 'GET', match: /\/api\/v1\/jobs$/, body: { items: [], total: 0 } },
  ])
})

afterEach(() => {
  mock.restore()
  vi.unstubAllGlobals()
})

function navLabels(nav: HTMLElement): string[] {
  return Array.from(nav.querySelectorAll('a')).map((a) => a.textContent ?? '')
}

describe('AppShell narrow (phone, min-width: 768px = false)', () => {
  beforeEach(() => {
    media = stubMatchMedia(false)
  })

  it('has no fixed rail; the drawer opens from the menu button with all five entries', async () => {
    renderShell('/')
    await waitFor(() => {
      expect(screen.getByTestId('page-under-test')).toBeInTheDocument()
    })

    // The 224 px sidebar must not exist on a phone; the content column is
    // the full-width flexible one.
    expect(screen.queryByTestId('nav-rail')).not.toBeInTheDocument()
    const column = screen
      .getByTestId('page-under-test')
      .closest('main')?.parentElement
    expect(column?.className).toContain('flex-1')
    expect(column?.className).toContain('min-w-0')
    expect(document.querySelector('.w-56')).toBeNull()

    fireEvent.click(screen.getByTestId('menu-button'))
    const drawer = screen.getByTestId('nav-drawer')
    const nav = drawer.querySelector('nav')
    expect(nav).not.toBeNull()
    expect(navLabels(nav as HTMLElement)).toEqual([
      'Dash',
      'Library',
      'Nodes',
      'Encode settings',
      'App settings',
    ])
    // Touch targets: every drawer link keeps at least 44 px of height.
    for (const link of (nav as HTMLElement).querySelectorAll('a')) {
      expect(link.className).toContain('min-h-[44px]')
    }
  })

  it('closes the drawer on Escape', async () => {
    renderShell('/')
    await waitFor(() =>
      expect(screen.getByTestId('menu-button')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('menu-button'))
    expect(screen.getByTestId('nav-drawer')).toBeInTheDocument()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByTestId('nav-drawer')).not.toBeInTheDocument()
  })

  it('returns focus to the menu button after the drawer closes (task 13)', async () => {
    renderShell('/')
    await waitFor(() =>
      expect(screen.getByTestId('menu-button')).toBeInTheDocument(),
    )
    const menuButton = screen.getByTestId('menu-button')
    menuButton.focus()
    fireEvent.click(menuButton)
    expect(screen.getByTestId('nav-drawer')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('close-menu'))
    await waitFor(() =>
      expect(screen.queryByTestId('nav-drawer')).not.toBeInTheDocument(),
    )
    expect(menuButton).toHaveFocus()
  })

  it('closes the drawer when the route changes', async () => {
    renderShell('/')
    await waitFor(() =>
      expect(screen.getByTestId('menu-button')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('menu-button'))
    fireEvent.click(screen.getByRole('link', { name: 'Library' }))
    await waitFor(() => {
      expect(screen.queryByTestId('nav-drawer')).not.toBeInTheDocument()
    })
    // The route actually changed behind the drawer.
    expect(screen.getByTestId('page-under-test')).toHaveTextContent(
      '/libraries',
    )
  })

  it('shows a live job indicator in the top bar while jobs run', async () => {
    mock.restore()
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/setup\/status$/, body: openNoLogin },
      {
        method: 'GET',
        match: /\/api\/v1\/health$/,
        body: { status: 'ok', version: 'v0.0.0' },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/jobs$/,
        body: { items: [runningJob], total: 1 },
      },
    ])
    renderShell('/')
    await waitFor(() => {
      expect(screen.getByTestId('job-indicator')).toBeInTheDocument()
    })
    expect(screen.getByTestId('job-indicator')).toHaveTextContent('1 running')
  })

  it('hides the job indicator when nothing is running', async () => {
    renderShell('/')
    await waitFor(() => {
      expect(screen.getByTestId('page-under-test')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('job-indicator')).not.toBeInTheDocument()
  })

  it('follows the breakpoint live when the viewport grows', async () => {
    renderShell('/')
    await waitFor(() =>
      expect(screen.getByTestId('menu-button')).toBeInTheDocument(),
    )
    media.set(true)
    await waitFor(() => {
      expect(screen.queryByTestId('menu-button')).not.toBeInTheDocument()
    })
    expect(screen.getByTestId('nav-rail')).toBeInTheDocument()
  })
})

describe('AppShell wide (md and up, min-width: 768px = true)', () => {
  beforeEach(() => {
    media = stubMatchMedia(true)
  })

  it('shows the collapsible left rail instead of the top bar', async () => {
    renderShell('/')
    await waitFor(() => {
      expect(screen.getByTestId('nav-rail')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('menu-button')).not.toBeInTheDocument()

    const nav = screen.getByRole('navigation', { name: 'Main' })
    expect(navLabels(nav)).toEqual([
      'Dash',
      'Library',
      'Nodes',
      'Encode settings',
      'App settings',
    ])

    // Collapse to icons: the labels disappear, the rail narrows.
    const rail = screen.getByTestId('nav-rail')
    expect(rail.className).toContain('w-56')
    fireEvent.click(screen.getByTestId('nav-toggle'))
    expect(rail.className).toContain('w-16')
    expect(screen.queryByText('Encode settings')).not.toBeInTheDocument()
    // Icon-only links still name themselves for assistive tech.
    expect(
      screen.getByRole('link', { name: 'Encode settings' }),
    ).toHaveAttribute('title', 'Encode settings')
  })
})
