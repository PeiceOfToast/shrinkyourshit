// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The task-07 acceptance browser test: choose a folder, slider at 3,
 * remove subs, audio 2, Start — the created library request must carry the
 * easy object that compiles to the expected Target JSON (the Go side
 * asserts the compilation itself).
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it } from 'vitest'
import EasyLibraryForm from './EasyLibraryForm'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <EasyLibraryForm />
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('EasyLibraryForm', () => {
  it('walks the easy screen and posts the easy object', async () => {
    let created = false
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/fs\/browse$/,
        bodyFn: (ctx) => {
          const path = ctx.url.searchParams.get('path') ?? '/'
          if (path === '/') {
            return {
              path,
              parent: '',
              entries: [
                { name: 'media', path: '/media', is_dir: true },
                { name: 'secret', path: '/secret', is_dir: true },
              ],
            }
          }
          if (path === '/media/movies') {
            // The chosen folder itself: no subfolders left.
            return { path, parent: '/media', entries: [] }
          }
          return {
            path,
            parent: '/media',
            entries: [
              { name: 'movies', path: '/media/movies', is_dir: true },
              { name: 'movie.mkv', path: '/media/movie.mkv', is_dir: false },
            ],
          }
        },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries$/,
        status: 201,
        bodyFn: () => {
          created = true
          return {
            id: 'lib-1',
            name: 'movies',
            path: '/media/movies',
            target: {},
            easy: null,
            watch: true,
            schedule: '',
            result_policy: 'replace_if_smaller',
            created_at: '',
            updated_at: '',
          }
        },
      },
    ])

    renderForm()

    // Choose the folder through the picker (directories only).
    fireEvent.click(screen.getByTestId('easy-choose-folder'))
    await waitFor(() =>
      expect(screen.getByTestId('folder-picker-dir-media')).toBeInTheDocument(),
    )
    expect(screen.queryByTestId('folder-picker-dir-secret')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('folder-picker-dir-media'))
    await waitFor(() =>
      expect(
        screen.getByTestId('folder-picker-dir-movies'),
      ).toBeInTheDocument(),
    )
    // Only directories are offered; files are not clickable entries.
    expect(screen.queryByText('movie.mkv')).not.toBeInTheDocument()
    fireEvent.click(screen.getByTestId('folder-picker-dir-movies'))
    await waitFor(() =>
      expect(screen.getByTestId('folder-picker-current')).toHaveTextContent(
        '/media/movies',
      ),
    )
    fireEvent.click(screen.getByTestId('folder-picker-use'))
    await waitFor(() =>
      expect(screen.queryByTestId('folder-picker')).not.toBeInTheDocument(),
    )
    expect(screen.getByTestId('easy-folder')).toHaveTextContent('/media/movies')

    // Slider at 3, remove subs, audio 2; watch stays on by default.
    fireEvent.click(screen.getByTestId('quality-stop-3'))
    fireEvent.click(screen.getByTestId('subtitles-checkbox'))
    fireEvent.click(screen.getByTestId('audio-stop-2'))
    expect(screen.getByTestId('easy-watch-checkbox')).toBeChecked()

    fireEvent.click(screen.getByTestId('easy-start'))

    await waitFor(() => expect(created).toBe(true))
    const posts = mock.calls('POST', /\/api\/v1\/libraries$/)
    require1(posts)
    expect(posts[0].body).toEqual({
      path: '/media/movies',
      easy: { quality: 3, remove_subtitles: true, audio: 2 },
      watch: true,
    })
  })

  it('disables Start until a folder is chosen', () => {
    mock = installFetchMock([])
    renderForm()
    expect(screen.getByTestId('easy-start')).toBeDisabled()
    expect(screen.getByTestId('easy-folder-empty')).toHaveTextContent(
      'No folder chosen yet.',
    )
  })

  it('shows the failure message when the server rejects the request', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/fs\/browse$/,
        body: { path: '/', parent: '', entries: [] },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries$/,
        status: 400,
        body: { error: 'invalid easy audio', code: 'invalid_request' },
      },
    ])
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <EasyLibraryForm initial={{ path: '/media/movies' }} />
      </QueryClientProvider>,
    )
    fireEvent.click(screen.getByTestId('easy-start'))
    await waitFor(() =>
      expect(screen.getByTestId('easy-error')).toHaveTextContent(
        'invalid easy audio',
      ),
    )
  })
})

function require1(posts: unknown[]): asserts posts is [{ body: unknown }] {
  if (posts.length !== 1) {
    throw new Error(`expected exactly one POST, saw ${posts.length}`)
  }
}
