// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit tests (task 07): easy sliders reopen for easy-shaped
 * libraries; the collapsed Advanced panel is the only place codec names
 * appear; saving sends easy or target depending on the mode.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import type { Target } from './api'
import LibraryEditPage from './LibraryEditPage'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function library(easy: unknown) {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: {
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
    },
    easy,
    watch: true,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
  }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/libraries/lib-1/edit']}>
        <Routes>
          <Route path="/libraries/:id/edit" element={<LibraryEditPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('LibraryEditPage', () => {
  it('reopens the easy sliders for an easy library and saves the easy object', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library({ quality: 3, remove_subtitles: false, audio: 1 }),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
    ])
    renderPage()

    // Sliders reflect the stored easy state.
    await waitFor(() =>
      expect(screen.getByTestId('easy-edit')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('quality-slider')).toHaveValue('3')
    expect(screen.getByTestId('subtitles-checkbox')).not.toBeChecked()

    // Move to best quality, tick remove subtitles, save.
    fireEvent.click(screen.getByTestId('quality-stop-5'))
    fireEvent.click(screen.getByTestId('subtitles-checkbox'))
    fireEvent.click(screen.getByTestId('library-edit-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      expect(puts[0].body).toMatchObject({
        name: 'Movies',
        path: '/media/movies',
        watch: true,
        easy: { quality: 5, remove_subtitles: true, audio: 1 },
      })
      expect((puts[0].body as Record<string, unknown>).target).toBeUndefined()
    })
  })

  it('exposes the raw Target only inside the collapsed Advanced panel', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library({ quality: 3, remove_subtitles: true, audio: 2 }),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
    ])
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('easy-edit')).toBeInTheDocument(),
    )

    // Advanced starts collapsed; the codec fields are hidden.
    expect(screen.getByTestId('advanced-panel')).toBeInTheDocument()
    expect(screen.queryByTestId('advanced-fields')).not.toBeVisible()

    fireEvent.click(screen.getByTestId('advanced-codec')) // inside details; opens it
    fireEvent.click(
      screen.getByTestId('advanced-panel').querySelector('summary')!,
    )

    const fields = screen.getByTestId('advanced-fields')
    expect(fields).toBeVisible()
    expect(screen.getByTestId('advanced-codec')).toHaveValue('hevc')
    expect(screen.getByTestId('advanced-container')).toHaveValue('mkv')
    expect(screen.getByTestId('advanced-reencode')).not.toBeChecked()
    expect(screen.getByTestId('advanced-languages')).toHaveValue('eng')

    // Saving with Advanced open sends the full target and no easy object.
    fireEvent.change(screen.getByTestId('advanced-languages'), {
      target: { value: 'eng, deu' },
    })
    fireEvent.click(screen.getByTestId('library-edit-save'))
    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      const body = puts[0].body as { easy?: unknown; target?: Target }
      expect(body.easy).toBeUndefined()
      expect(body.target).toEqual({
        video: { codec: 'hevc', quality: 3, reencode_if_same_codec: false },
        audio: {
          mode: 'keep_preferred',
          preferred_languages: ['eng', 'deu'],
          compact_codec: 'aac',
        },
        subtitles: {
          mode: 'remove',
          preferred_languages: ['eng', 'deu'],
          extract_removed: false,
        },
        container: 'mkv',
      })
    })
  })

  it('shows only the advanced editor for a hand-tuned library', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: library(null),
      },
    ])
    renderPage()

    await waitFor(() =>
      expect(screen.getByTestId('library-edit')).toBeInTheDocument(),
    )
    expect(screen.queryByTestId('easy-edit')).not.toBeInTheDocument()
    expect(screen.getByTestId('advanced-fields')).toBeVisible()
  })
})
