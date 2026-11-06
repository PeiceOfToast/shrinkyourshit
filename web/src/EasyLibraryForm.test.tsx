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
import { ToastProvider } from './toast'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <EasyLibraryForm />
      </ToastProvider>
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
      'No folder chosen yet',
    )
  })

  it('lists one plain language line per generated step in "what this does"', async () => {
    // The server compiles the easy input; the panel shows one line per
    // step (task 14 acceptance). The lines arrive from POST /flows/preview.
    const steps = [
      {
        id: 'probe',
        type: 'probe',
        plain: 'Look inside the file to see what it is.',
      },
      {
        id: 'transcode_video',
        type: 'transcode_video',
        plain:
          'Only when the video is not HEVC: Encode the video as HEVC at quality 3 of 5.',
      },
      {
        id: 'remux_container',
        type: 'remux_container',
        plain: 'Put everything into a MKV file.',
      },
      {
        id: 'convert_audio',
        type: 'convert_audio',
        plain: 'Keep every audio track exactly as it is.',
      },
      { id: 'subtitles', type: 'subtitles', plain: 'Keep all subtitles.' },
    ]
    mock = installFetchMock([
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        bodyFn: (ctx) => {
          // The preview carries the easy input the panel compiled.
          expect(ctx.body).toMatchObject({ easy: { quality: 3, audio: 1 } })
          return { graph: { schema: 1, nodes: [], edges: [] }, steps }
        },
      },
    ])
    renderForm()

    // Collapsed by default.
    expect(screen.queryByTestId('flow-preview')).not.toBeInTheDocument()
    fireEvent.click(screen.getByTestId('flow-preview-toggle'))

    const lines = await waitFor(() => {
      const els = screen.getAllByTestId('flow-preview-line')
      expect(els).toHaveLength(steps.length)
      return els
    })
    lines.forEach((line, i) => {
      expect(line.textContent).toContain(steps[i].plain)
    })
  })

  it('updates the preview lines when the sliders move', async () => {
    mock = installFetchMock([
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: {
          graph: { schema: 1, nodes: [], edges: [] },
          steps: [
            {
              id: 'probe',
              type: 'probe',
              plain: 'Look inside the file to see what it is.',
            },
          ],
        },
      },
    ])
    renderForm()
    fireEvent.click(screen.getByTestId('flow-preview-toggle'))
    await waitFor(() => screen.getAllByTestId('flow-preview-line'))
    // Moving the audio slider re-runs the preview with the new input.
    fireEvent.click(screen.getByTestId('audio-stop-4'))
    await waitFor(() => {
      const posts = mock.calls('POST', /\/api\/v1\/flows\/preview$/)
      expect(posts.at(-1)?.body).toMatchObject({ easy: { audio: 4 } })
    })
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
        <ToastProvider>
          <EasyLibraryForm initial={{ path: '/media/movies' }} />
        </ToastProvider>
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

/** One mounted share and one unmounted share, as GET /storages answers. */
function shareList() {
  const share = (id: string, name: string, mounted: boolean) => ({
    id,
    name,
    protocol: 'nfs',
    source: 'nas.lan:/export/media',
    options: 'vers=3,soft,timeo=50,retrans=2',
    username: '',
    domain: '',
    password_set: false,
    read_only: false,
    enabled: true,
    mountpoint: `/storage/${name}`,
    mounted,
    error: '',
    created_at: '',
    updated_at: '',
  })
  return {
    items: [share('st-nas', 'nas', true), share('st-off', 'off', false)],
    total: 2,
    host_policy: { restricted: false, allowed: [], message: '' },
  }
}

/** A storage listing: folders for the browser, the space for the form. */
function storageListing(path: string) {
  return {
    id: 'st-nas',
    name: 'nas',
    mounted: true,
    mountpoint: '/storage/nas',
    path,
    parent: path === '/storage/nas' ? '' : '/storage/nas',
    free_bytes: 2_000_000_000,
    total_bytes: 4_000_000_000,
    entries: [{ name: 'Movies', path: '/storage/nas/Movies', is_dir: true }],
  }
}

describe('EasyLibraryForm storage picker (task 69)', () => {
  it('picks a folder inside a storage and posts that path', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/storages$/, body: shareList() },
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        bodyFn: (ctx) =>
          storageListing(ctx.url.searchParams.get('path') ?? '/storage/nas'),
      },
      {
        method: 'POST',
        match: /\/api\/v1\/libraries$/,
        status: 201,
        body: { id: 'lib-s', name: 'Movies', path: '/storage/nas/Movies' },
      },
    ])
    renderForm()

    fireEvent.change(await screen.findByTestId('easy-storage-select'), {
      target: { value: 'st-nas' },
    })
    await waitFor(() =>
      expect(screen.getByTestId('easy-storage-space')).toHaveTextContent(
        '2.0 GB free of 4.0 GB',
      ),
    )
    expect(screen.getByTestId('easy-storage-badge')).toHaveTextContent(
      'Mounted',
    )

    fireEvent.click(screen.getByTestId('easy-choose-folder'))
    await waitFor(() =>
      expect(
        screen.getByTestId('storage-browser-dir-Movies'),
      ).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByTestId('storage-browser-dir-Movies'))
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-current')).toHaveTextContent(
        '/storage/nas/Movies',
      ),
    )
    await waitFor(() =>
      expect(screen.getByTestId('storage-browser-use')).toBeEnabled(),
    )
    fireEvent.click(screen.getByTestId('storage-browser-use'))
    await waitFor(() =>
      expect(screen.getByTestId('easy-folder')).toHaveTextContent(
        '/storage/nas/Movies',
      ),
    )

    fireEvent.click(screen.getByTestId('easy-start'))
    await waitFor(() =>
      expect(mock.calls('POST', /\/api\/v1\/libraries$/)).toHaveLength(1),
    )
    expect(mock.calls('POST', /\/api\/v1\/libraries$/)[0].body).toMatchObject({
      path: '/storage/nas/Movies',
    })
  })

  it('refuses a typed path outside every mounted storage, with the reason', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/storages$/, body: shareList() },
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        bodyFn: (ctx) =>
          storageListing(ctx.url.searchParams.get('path') ?? '/storage/nas'),
      },
    ])
    renderForm()
    await screen.findByTestId('easy-storage-select')

    fireEvent.change(screen.getByTestId('easy-typed-path-input'), {
      target: { value: '/media/movies' },
    })
    fireEvent.click(screen.getByTestId('easy-use-typed-path'))

    expect(screen.getByTestId('easy-typed-path-error')).toHaveTextContent(
      'not inside a mounted storage',
    )
    expect(screen.getByTestId('easy-folder-empty')).toBeInTheDocument()
    expect(screen.getByTestId('easy-start')).toBeDisabled()
  })

  it('accepts a typed path inside a mounted storage', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/storages$/, body: shareList() },
      {
        method: 'GET',
        match: /\/api\/v1\/storages\/st-nas\/browse$/,
        bodyFn: (ctx) =>
          storageListing(ctx.url.searchParams.get('path') ?? '/storage/nas'),
      },
    ])
    renderForm()
    await screen.findByTestId('easy-storage-select')

    fireEvent.change(screen.getByTestId('easy-typed-path-input'), {
      target: { value: '/storage/nas/TV' },
    })
    fireEvent.click(screen.getByTestId('easy-use-typed-path'))

    expect(
      screen.queryByTestId('easy-typed-path-error'),
    ).not.toBeInTheDocument()
    expect(screen.getByTestId('easy-folder')).toHaveTextContent(
      '/storage/nas/TV',
    )
  })

  it('refuses to browse a share that is not mounted', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/storages$/, body: shareList() },
    ])
    renderForm()

    fireEvent.change(await screen.findByTestId('easy-storage-select'), {
      target: { value: 'st-off' },
    })
    expect(screen.getByTestId('easy-choose-folder')).toBeDisabled()
    expect(screen.getByTestId('easy-storage-badge')).toHaveTextContent(
      'Not mounted',
    )
  })
})
