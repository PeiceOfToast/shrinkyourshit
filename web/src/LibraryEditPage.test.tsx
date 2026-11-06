// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit tests. Task 07 and 41 covered the sliders and the exact
 * panel; task 47 (spec 5.7) made the page properties only. Task 73 replaces
 * the rules summary with the encode profile the library follows: the picker,
 * the plain-language steps of the chosen profile, and the Danger zone.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import type { Target } from './api'
import LibraryEditPage from './LibraryEditPage'
import { ToastProvider } from './toast'
import {
  installFetchMock,
  type FetchMock,
  type MockRoute,
} from './test/fetchMock'

/** Two profiles: the boot seed and one Moderate with three steps. */
function profileRow(id: string, name: string, tier: string, nodes: number) {
  return {
    id,
    name,
    description: '',
    tier,
    result_policy: 'replace_if_smaller',
    graph: {
      schema: 1,
      nodes: Array.from({ length: nodes }, (_, i) => ({
        id: `n${i}`,
        type: 'probe',
      })),
      edges: [],
    },
    used_by: 1,
    used_by_names: ['Movies'],
    unrepresentable: [],
    created_at: '',
    updated_at: '',
  }
}

/** The profile list and the plain-language preview the picker reads. */
function profilesRoutes(): MockRoute[] {
  return [
    {
      method: 'GET',
      match: /\/api\/v1\/profiles$/,
      body: {
        items: [
          profileRow('prof-basic', 'Default (basic)', 'basic', 2),
          profileRow('prof-moderate', 'Small files', 'moderate', 3),
        ],
        total: 2,
      },
    },
    {
      method: 'POST',
      match: /\/api\/v1\/flows\/preview$/,
      bodyFn: (ctx) => {
        const g = (ctx.body as { graph: { nodes: unknown[] } }).graph
        return {
          graph: g,
          steps: [
            {
              id: 'n0',
              type: 'probe',
              plain: `Steps for ${g.nodes.length} nodes.`,
            },
          ],
        }
      },
    },
  ]
}

const STORED_TARGET: Target = {
  video: { codec: 'hevc', quality: 3, reencode_if_same_codec: false },
  audio: {
    mode: 'keep_preferred',
    preferred_languages: ['eng'],
    compact_codec: 'aac',
  },
  subtitles: {
    mode: 'remove',
    preferred_languages: ['eng'],
    extract_removed: false,
  },
  container: 'mkv',
}

function library(
  easy: unknown,
  flowSource = 'steps',
  overrides: Record<string, unknown> = {},
) {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: STORED_TARGET,
    easy,
    flow: null,
    flow_source: flowSource,
    watch: true,
    schedule: '',
    result_policy: 'replace_if_smaller',
    file_kinds: ['video'],
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/libraries/lib-1/edit']}>
          <Routes>
            <Route path="/libraries/:id/edit" element={<LibraryEditPage />} />
            <Route path="/libraries/:id/rules" element={<p>rules page</p>} />
            <Route path="/libraries/:id" element={<p>detail page</p>} />
            <Route path="/libraries" element={<p>libraries page</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('LibraryEditPage saves the rule the library already has (task 47)', () => {
  it('resends the stored easy object unchanged when the easy tier owns the flow', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(
          { quality: 3, remove_subtitles: true, audio: 2 },
          'easy',
          {
            profile_id: 'prof-basic',
          },
        ),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-save')
    fireEvent.click(screen.getByTestId('library-edit-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      expect(puts[0].body).toEqual({
        name: 'Movies',
        path: '/media/movies',
        watch: true,
        file_kinds: ['video'],
        profile_id: 'prof-basic',
        easy: { quality: 3, remove_subtitles: true, audio: 2 },
      })
    })
  })

  it('resends the stored target and no easy object for a Steps library', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps'),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps'),
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-save')
    fireEvent.click(screen.getByTestId('library-edit-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      const body = puts[0].body as { easy?: unknown; target?: Target }
      expect(body.easy).toBeUndefined()
      expect(body.target).toEqual(STORED_TARGET)
    })
  })

  it('never sends a leftover easy object when the Steps tier owns the flow', async () => {
    // A library saved on the Easy tier and later moved to Steps still has
    // its easy object stored. An easy save would regenerate the hand-made
    // steps, so the stored target goes instead.
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(
          { quality: 2, remove_subtitles: false, audio: 1 },
          'steps',
        ),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps'),
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-save')
    fireEvent.click(screen.getByTestId('library-edit-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      const body = puts[0].body as { easy?: unknown; target?: Target }
      expect(body.easy).toBeUndefined()
      expect(body.target).toEqual(STORED_TARGET)
    })
  })
})

describe('LibraryEditPage properties only (task 47, spec 5.7)', () => {
  it('has Library, File types, Rules and Danger zone sections and no rule controls', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(
          { quality: 3, remove_subtitles: false, audio: 1 },
          'easy',
        ),
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-save')
    for (const title of [
      'Library',
      'File types',
      'Encode profile',
      'Danger zone',
    ]) {
      expect(
        screen.getByRole('heading', { level: 2, name: title }),
      ).toBeInTheDocument()
    }
    expect(screen.queryByRole('heading', { name: 'Exact settings' })).toBeNull()
    expect(screen.queryByText('Advanced', { exact: false })).toBeNull()
    // The sliders and the exact-values panel are gone from Edit.
    expect(screen.queryByTestId('easy-edit')).toBeNull()
    expect(screen.queryByTestId('quality-stop-3')).toBeNull()
    expect(screen.queryByTestId('audio-stop-1')).toBeNull()
    expect(screen.queryByTestId('subtitles-checkbox')).toBeNull()
    expect(screen.queryByTestId('advanced-panel')).toBeNull()
    expect(screen.queryByTestId('advanced-codec')).toBeNull()
  })

  it('shows the chosen profile and links to its editor', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps', { profile_id: 'prof-moderate' }),
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(
        screen.getByTestId('library-edit-profile-summary'),
      ).toHaveTextContent('Steps for 3 nodes.'),
    )
    expect(screen.getByTestId('library-edit-profile')).toHaveValue(
      'prof-moderate',
    )
    expect(screen.queryByTestId('library-edit-no-profile')).toBeNull()
    const link = screen.getByTestId('library-edit-rules-link')
    expect(link).toHaveTextContent('Edit profiles in Encode settings')
    expect(link).toHaveAttribute('href', '/encode/profiles/prof-moderate')
  })

  it('warns that a library with no profile does not shrink anything', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps'),
      },
    ])
    renderPage()
    expect(
      await screen.findByTestId('library-edit-no-profile'),
    ).toHaveTextContent('does not shrink anything')
    expect(screen.getByTestId('library-edit-profile')).toHaveValue('')
  })

  it('keeps the file-type help line and the Save and Cancel bar sticky', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-save')
    const bar = screen.getByTestId('library-edit-save').parentElement!
    expect(bar.className).toContain('sticky')
    expect(
      screen.getByText('Files of other types stay untouched.'),
    ).toBeInTheDocument()
  })

  it('removes the library from the Danger zone after confirmation only', async () => {
    let deleted = false
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
      {
        method: 'DELETE',
        match: /\/api\/v1\/libraries\/lib-1$/,
        status: 204,
        bodyFn: () => {
          deleted = true
          return null
        },
      },
    ])
    renderPage()
    const danger = await screen.findByTestId('library-edit-danger')
    expect(danger).toHaveTextContent(
      'Removing a library forgets its history. Files on disk are not touched.',
    )

    fireEvent.click(screen.getByTestId('library-delete-lib-1'))
    const dialog = screen.getByTestId('confirm-delete')
    expect(dialog).toHaveTextContent('Remove “Movies”?')
    expect(dialog).toHaveTextContent('/media/movies')
    expect(dialog).toHaveTextContent('are not touched')

    // Cancelling keeps the library.
    fireEvent.click(screen.getByTestId('confirm-delete-cancel'))
    await waitFor(() =>
      expect(screen.queryByTestId('confirm-delete')).not.toBeInTheDocument(),
    )
    expect(deleted).toBe(false)

    // Confirming sends exactly one DELETE to the library endpoint.
    fireEvent.click(screen.getByTestId('library-delete-lib-1'))
    fireEvent.click(screen.getByTestId('confirm-delete-yes'))
    await waitFor(() => expect(deleted).toBe(true))
    expect(mock.calls('DELETE', /\/api\/v1\/libraries\/lib-1$/)).toHaveLength(1)
    expect(mock.requests.filter((r) => r.method === 'DELETE')).toHaveLength(1)
    const toast = await screen.findByTestId('toast')
    expect(toast).toHaveTextContent('Library removed')
    expect(toast).toHaveTextContent('files on disk are untouched')
    expect(await screen.findByText('libraries page')).toBeInTheDocument()
  })
})

describe('LibraryEditPage encode profile (task 73)', () => {
  it('changing the profile changes the summary and is what gets saved', async () => {
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps', { profile_id: 'prof-basic' }),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps', { profile_id: 'prof-moderate' }),
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(
        screen.getByTestId('library-edit-profile-summary'),
      ).toHaveTextContent('Steps for 2 nodes.'),
    )

    fireEvent.change(screen.getByTestId('library-edit-profile'), {
      target: { value: 'prof-moderate' },
    })
    await waitFor(() =>
      expect(
        screen.getByTestId('library-edit-profile-summary'),
      ).toHaveTextContent('Steps for 3 nodes.'),
    )
    fireEvent.click(screen.getByTestId('library-edit-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      expect(puts[0].body).toMatchObject({ profile_id: 'prof-moderate' })
      // The stored legacy rule is resent unchanged alongside the profile.
      expect(puts[0].body).toHaveProperty('target')
    })
  })
})

describe('LibraryEditPage file kinds (task 34)', () => {
  it('shows the plain-language kind picker and sends the kinds on save', async () => {
    const fixture = library({ quality: 3, remove_subtitles: false, audio: 1 })
    fixture.file_kinds = ['image']
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: fixture,
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: fixture,
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-kind-picker')
    // Plain-language labels, no codec or engine words on the picker.
    expect(screen.getByText('Photos')).toBeInTheDocument()
    expect(screen.getByText('Anything else')).toBeInTheDocument()
    const image = screen.getByTestId(
      'library-edit-kind-image',
    ) as HTMLInputElement
    expect(image.checked).toBe(true)
    expect(
      (screen.getByTestId('library-edit-kind-video') as HTMLInputElement)
        .checked,
    ).toBe(false)

    // Add Video and save: both kinds travel as file_kinds.
    fireEvent.click(screen.getByTestId('library-edit-kind-video'))
    fireEvent.click(screen.getByTestId('library-edit-save'))
    await waitFor(() =>
      expect(mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)).toHaveLength(1),
    )
    const put = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)[0]
    expect(put.body).toMatchObject({ file_kinds: ['image', 'video'] })
  })

  it('keeps the last kind when the picker would go empty', async () => {
    const fixture = library(null)
    fixture.file_kinds = ['video']
    mock = installFetchMock([
      ...profilesRoutes(),
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: fixture,
      },
    ])
    renderPage()
    await screen.findByTestId('library-edit-kind-picker')
    fireEvent.click(screen.getByTestId('library-edit-kind-video'))
    // The library must look for at least one kind: the checkbox stays on.
    expect(
      (screen.getByTestId('library-edit-kind-video') as HTMLInputElement)
        .checked,
    ).toBe(true)
  })
})
