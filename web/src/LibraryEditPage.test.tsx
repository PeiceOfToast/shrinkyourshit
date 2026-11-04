// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit tests. Task 07 and 41 covered the sliders and the exact
 * panel; task 47 (spec 5.7) makes the page properties only: Library, File
 * types, a Rules summary with a link, and a Danger zone that removes the
 * library. The slider and Steps-tier checks now live in RulesPage.test.tsx.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import type { Target } from './api'
import LibraryEditPage from './LibraryEditPage'
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

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
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library({ quality: 3, remove_subtitles: true, audio: 2 }, 'easy'),
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
        easy: { quality: 3, remove_subtitles: true, audio: 2 },
      })
    })
  })

  it('resends the stored target and no easy object for a Steps library', async () => {
    mock = installFetchMock([
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
    for (const title of ['Library', 'File types', 'Rules', 'Danger zone']) {
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

  it('summarises the easy rules and links to the Rules page', async () => {
    mock = installFetchMock([
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
    expect(
      await screen.findByTestId('library-edit-rules-summary'),
    ).toHaveTextContent('Easy · Balanced · keep every audio track')
    const link = screen.getByTestId('library-edit-rules-link')
    expect(link).toHaveTextContent('Change rules')
    expect(link).toHaveAttribute('href', '/libraries/lib-1/rules')
  })

  it('keeps the file-type help line and the Save and Cancel bar sticky', async () => {
    mock = installFetchMock([
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

describe('LibraryEditPage rules summary (task 47)', () => {
  it('names the Steps and Advanced tiers in plain language', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'steps'),
      },
    ])
    const first = renderPage()
    expect(
      await screen.findByTestId('library-edit-rules-summary'),
    ).toHaveTextContent('Steps · an ordered list')
    first.unmount()

    mock.restore()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null, 'advanced'),
      },
    ])
    renderPage()
    expect(
      await screen.findByTestId('library-edit-rules-summary'),
    ).toHaveTextContent('Advanced · a visual flow')
  })

  it('adds the subtitle choice to the Easy summary', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library({ quality: 5, remove_subtitles: true, audio: 4 }, 'easy'),
      },
    ])
    renderPage()
    expect(
      await screen.findByTestId('library-edit-rules-summary'),
    ).toHaveTextContent(
      'Easy · Best quality · keep the best audio track, made small · remove subtitles',
    )
  })
})

describe('LibraryEditPage file kinds (task 34)', () => {
  it('shows the plain-language kind picker and sends the kinds on save', async () => {
    const fixture = library({ quality: 3, remove_subtitles: false, audio: 1 })
    fixture.file_kinds = ['image']
    mock = installFetchMock([
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
