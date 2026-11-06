// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Every route sets the browser tab title (task 52, spec section 3.3 and the
 * useDocumentTitle hook in 4.1). Each route is rendered through the real App
 * with the API stubbed at the fetch level, and the title is checked once the
 * page has committed. Unknown GETs answer an empty list so no page crashes.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import App from './App'
import { ToastProvider } from './toast'
import {
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'

const settings = {
  worker_concurrency: 1,
  scratch_dir: '',
  default_result_policy: 'replace_if_smaller',
  login_enabled: false,
  preferred_languages: ['eng'],
  default_video_codec: 'hevc',
}

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={[path]}>
          <App />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock | undefined
let setup = { setup_complete: true, login_enabled: false }

beforeEach(() => {
  stubEventSource()
  setup = { setup_complete: true, login_enabled: false }
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/health$/,
      body: { status: 'ok', version: 'v0.0.0' },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/setup\/status$/,
      bodyFn: () => setup,
    },
    {
      method: 'GET',
      match: /\/api\/v1\/auth\/me$/,
      body: { username: 'admin' },
    },
    { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    { method: 'GET', match: /\/api\/v1\/nodes$/, body: { items: [] } },
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
        bytes_saved: 0,
        libraries: [],
      },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/[^/]+\/flow$/,
      body: { flow: { nodes: [], edges: [] }, flow_source: 'easy' },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/[^/]+$/,
      body: {
        id: 'lib-1',
        name: 'Movies',
        path: '/media/movies',
        easy: null,
        flow: { nodes: [], edges: [] },
        flow_source: 'easy',
        watch: false,
      },
    },
    { method: 'GET', match: /\/api\/v1\//, body: { items: [], total: 0 } },
  ])
})

afterEach(() => {
  mock?.restore()
  mock = undefined
  document.title = ''
})

const cases: {
  path: string
  title: string
  setup?: { setup_complete: boolean; login_enabled: boolean }
}[] = [
  {
    path: '/setup',
    title: 'Setup · ShrinkYourShit',
    setup: { setup_complete: false, login_enabled: false },
  },
  {
    path: '/login',
    title: 'Log in · ShrinkYourShit',
    setup: { setup_complete: true, login_enabled: true },
  },
  { path: '/', title: 'Dashboard · ShrinkYourShit' },
  { path: '/libraries', title: 'Libraries · ShrinkYourShit' },
  { path: '/libraries/new', title: 'Add a library · ShrinkYourShit' },
  { path: '/libraries/lib-1', title: 'Movies · Libraries · ShrinkYourShit' },
  {
    path: '/libraries/lib-1/edit',
    title: 'Edit library · Movies · ShrinkYourShit',
  },
  { path: '/libraries/lib-1/rules', title: 'Rules · Movies · ShrinkYourShit' },
  { path: '/libraries/lib-1/flow', title: 'Rules · Movies · ShrinkYourShit' },
  { path: '/nodes', title: 'Nodes · ShrinkYourShit' },
  { path: '/storage', title: 'Storage · ShrinkYourShit' },
  { path: '/encode', title: 'Encode settings · ShrinkYourShit' },
  { path: '/app', title: 'App settings · ShrinkYourShit' },
  { path: '/integrations', title: 'Integrations · ShrinkYourShit' },
]

describe('every route sets the document title', () => {
  for (const c of cases) {
    it(`${c.path} → "${c.title}"`, async () => {
      setup = c.setup ?? { setup_complete: true, login_enabled: false }
      renderAt(c.path)
      await waitFor(() => expect(document.title).toBe(c.title))
    })
  }
})
