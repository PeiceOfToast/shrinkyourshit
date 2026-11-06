// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage page tests (task 68, spec 4.6): the list states, the read-only
 * state with login off, the add form's defaults, the row actions and their
 * toasts, the field errors, and the rule that no password is ever rendered.
 */
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import StoragePage from './StoragePage'
import { ToastProvider } from './toast'
import {
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'
import { NFS_DEFAULT_OPTIONS } from './storageModel'

/** A distinctive value: if it ever reaches the DOM, the test fails. */
const SECRET = 'hunter2-distinctive-smb-secret'

function share(overrides: Record<string, unknown>) {
  return {
    id: 'st-nas',
    name: 'nas',
    protocol: 'nfs',
    source: '192.168.1.10:/export/media',
    options: NFS_DEFAULT_OPTIONS,
    username: '',
    domain: '',
    password_set: false,
    read_only: false,
    enabled: true,
    mountpoint: '/storage/nas',
    mounted: true,
    error: '',
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    ...overrides,
  }
}

const nas = share({})
const movies = share({
  id: 'st-movies',
  name: 'movies',
  protocol: 'smb',
  source: '//192.168.1.20/movies',
  options: '',
  username: 'media',
  password_set: true,
  mountpoint: '/storage/movies',
  mounted: false,
  error: 'mount error(13): Permission denied',
})

const libraries = [
  {
    id: 'lib-1',
    name: 'Films',
    path: '/storage/nas/Movies',
  },
]

function hostPolicy(extra: Record<string, unknown> = {}) {
  return {
    restricted: false,
    allowed: [],
    message:
      'Any host can be mounted. Set storage_allowed_hosts to limit which hosts this server may mount; setting it is recommended.',
    ...extra,
  }
}

/**
 * A write control, once the settings have loaded and it is enabled. Write
 * controls wait for the login check, so a click before then does nothing.
 */
async function enabledButton(testId: string): Promise<HTMLElement> {
  return waitFor(() => {
    const el = screen.getByTestId(testId)
    expect(el).toBeEnabled()
    return el
  })
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <StoragePage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock
let items: unknown[]
let loginEnabled: boolean

beforeEach(() => {
  stubEventSource()
  items = [nas, movies]
  loginEnabled = true
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/settings$/,
      bodyFn: () => ({ login_enabled: loginEnabled, worker_concurrency: 1 }),
    },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries$/,
      body: { items: libraries, total: libraries.length },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/storages$/,
      bodyFn: () => ({
        items,
        total: items.length,
        host_policy: hostPolicy(),
      }),
    },
  ])
})

afterEach(() => {
  cleanup()
  mock.restore()
})

describe('StoragePage list', () => {
  it('shows name, protocol, source, state, error and the library count', async () => {
    renderPage()
    expect(await screen.findByTestId('storage-row-st-nas')).toBeInTheDocument()
    expect(screen.getAllByText('nas').length).toBeGreaterThan(0)
    expect(screen.getAllByText('/storage/nas').length).toBeGreaterThan(0)
    expect(
      screen.getAllByText('192.168.1.10:/export/media').length,
    ).toBeGreaterThan(0)
    expect(
      within(screen.getByTestId('storage-row-st-nas')).getByText('Mounted'),
    ).toBeInTheDocument()
    const movies = within(screen.getByTestId('storage-row-st-movies'))
    expect(movies.getByText('Error')).toBeInTheDocument()
    expect(
      movies.getByText('mount error(13): Permission denied'),
    ).toBeInTheDocument()
    // The library at /storage/nas/Movies sits inside the nas share.
    expect(screen.getAllByText('1 library').length).toBeGreaterThan(0)
    expect(screen.getAllByText('0 libraries').length).toBeGreaterThan(0)
  })

  it('shows the host policy notice with the recommendation', async () => {
    renderPage()
    expect(await screen.findByTestId('storage-host-policy')).toHaveTextContent(
      'storage_allowed_hosts',
    )
  })

  it('shows the invalid host policy as a refusal', async () => {
    items = []
    mock.routes.length = 0
    mock.routes.push(
      {
        method: 'GET',
        match: /\/api\/v1\/settings$/,
        body: { login_enabled: true },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/storages$/,
        body: {
          items: [],
          total: 0,
          host_policy: hostPolicy({
            restricted: true,
            error: 'storage_allowed_hosts has an invalid entry "10.0.0.0/33"',
          }),
        },
      },
    )
    renderPage()
    expect(
      await screen.findByTestId('storage-host-policy-error'),
    ).toHaveTextContent('10.0.0.0/33')
  })

  it('offers the empty state when no share exists', async () => {
    items = []
    mock.routes.length = 0
    mock.routes.push(
      {
        method: 'GET',
        match: /\/api\/v1\/settings$/,
        body: { login_enabled: true },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/storages$/,
        body: { items: [], total: 0, host_policy: hostPolicy() },
      },
    )
    renderPage()
    expect(await screen.findByTestId('storage-empty')).toBeInTheDocument()
  })
})

describe('StoragePage login gate (spec 4.6)', () => {
  it('renders read only with the reason and a link to App settings when login is off', async () => {
    loginEnabled = false
    renderPage()
    const banner = await screen.findByTestId('storage-readonly')
    expect(banner).toHaveTextContent('Storage is read only while login is off')
    expect(screen.getByTestId('storage-readonly-link')).toHaveAttribute(
      'href',
      '/app',
    )
    expect(screen.getByTestId('storage-add')).toBeDisabled()
    expect(screen.getByTestId('storage-test-st-nas')).toBeDisabled()
    expect(screen.getByTestId('storage-mount-st-movies')).toBeDisabled()
    expect(screen.getByTestId('storage-edit-st-nas')).toBeDisabled()
    expect(screen.getByTestId('storage-delete-st-movies')).toBeDisabled()
  })

  it('enables the write controls when login is on', async () => {
    renderPage()
    await screen.findByTestId('storage-row-st-nas')
    expect(screen.queryByTestId('storage-readonly')).not.toBeInTheDocument()
    expect(await enabledButton('storage-add')).toBeEnabled()
    expect(await enabledButton('storage-test-st-nas')).toBeEnabled()
  })
})

describe('StoragePage form', () => {
  it('prefills the NFS defaults and switches to SMB credential fields', async () => {
    renderPage()
    fireEvent.click(await enabledButton('storage-add'))
    const dialog = await screen.findByTestId('storage-form')
    const options = within(dialog).getByTestId(
      'storage-options',
    ) as HTMLInputElement
    expect(options.value).toBe(NFS_DEFAULT_OPTIONS)
    // The reasoning is in the help text, not hidden.
    expect(dialog).toHaveTextContent('soft')
    expect(
      within(dialog).queryByTestId('storage-password'),
    ).not.toBeInTheDocument()

    fireEvent.change(within(dialog).getByTestId('storage-protocol'), {
      target: { value: 'smb' },
    })
    expect(
      (within(dialog).getByTestId('storage-options') as HTMLInputElement).value,
    ).toBe('')
    expect(within(dialog).getByTestId('storage-username')).toBeInTheDocument()
    expect(within(dialog).getByTestId('storage-domain')).toBeInTheDocument()
    expect(within(dialog).getByTestId('storage-password')).toBeInTheDocument()
  })

  it('prefills vers=4.2 and warns that vers=3 needs rpc.statd (task 80)', async () => {
    renderPage()
    fireEvent.click(await enabledButton('storage-add'))
    const dialog = await screen.findByTestId('storage-form')
    const options = within(dialog).getByTestId(
      'storage-options',
    ) as HTMLInputElement
    expect(options.value).toBe('vers=4.2,soft,timeo=50,retrans=2')
    // The help text says what vers=3 needs before the operator picks it.
    expect(dialog).toHaveTextContent('vers=3 is allowed')
    expect(dialog).toHaveTextContent('rpc.statd')
    expect(dialog).toHaveTextContent('"rpc.statd is not running"')
  })

  it('sends the typed password once and never renders a secret', async () => {
    // The API must never carry a password. This response does, on purpose:
    // the page renders named fields only, so the secret stays out of the DOM.
    items = [{ ...movies, password: SECRET }]
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages$/,
      status: 201,
      bodyFn: ({ body }) => ({
        ...movies,
        id: 'st-new',
        name: (body as { name: string }).name,
        password_set: true,
      }),
    })
    renderPage()
    await screen.findByTestId('storage-row-st-movies')
    expect(document.body.innerHTML).not.toContain(SECRET)

    fireEvent.click(screen.getByTestId('storage-add'))
    const dialog = await screen.findByTestId('storage-form')
    fireEvent.change(within(dialog).getByTestId('storage-protocol'), {
      target: { value: 'smb' },
    })
    fireEvent.change(within(dialog).getByTestId('storage-name'), {
      target: { value: 'archive' },
    })
    fireEvent.change(within(dialog).getByTestId('storage-source'), {
      target: { value: '//192.168.1.30/archive' },
    })
    fireEvent.change(within(dialog).getByTestId('storage-password'), {
      target: { value: SECRET },
    })
    fireEvent.click(within(dialog).getByTestId('storage-save'))

    await waitFor(() =>
      expect(mock.calls('POST', /\/api\/v1\/storages$/)).toHaveLength(1),
    )
    const sent = mock.calls('POST', /\/api\/v1\/storages$/)[0]
    expect(sent.body).toMatchObject({ name: 'archive', password: SECRET })
    await waitFor(() =>
      expect(screen.queryByTestId('storage-form')).not.toBeInTheDocument(),
    )
    expect(document.body.innerHTML).not.toContain(SECRET)
  })

  it('edit keeps the password blank and says it is unchanged', async () => {
    renderPage()
    fireEvent.click(await enabledButton('storage-edit-st-movies'))
    const dialog = await screen.findByTestId('storage-form')
    const password = within(dialog).getByTestId(
      'storage-password',
    ) as HTMLInputElement
    expect(password.value).toBe('')
    expect(password.placeholder).toBe('(unchanged)')
    expect(within(dialog).getByTestId('storage-name')).toBeDisabled()
    expect(within(dialog).getByTestId('storage-protocol')).toBeDisabled()
  })

  it('shows a field error from a 400 next to its field and keeps the form open', async () => {
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages$/,
      status: 400,
      body: {
        error: 'the storage settings are not valid',
        code: 'invalid_request',
        errors: [
          { path: 'source', message: 'NFS source must look like host:/path' },
        ],
      },
    })
    renderPage()
    fireEvent.click(await enabledButton('storage-add'))
    const dialog = await screen.findByTestId('storage-form')
    fireEvent.change(within(dialog).getByTestId('storage-name'), {
      target: { value: 'bad' },
    })
    fireEvent.click(within(dialog).getByTestId('storage-save'))
    expect(
      await within(dialog).findByText('NFS source must look like host:/path'),
    ).toBeInTheDocument()
    expect(screen.getByTestId('storage-form')).toBeInTheDocument()
  })
})

describe('StoragePage row actions', () => {
  it('test passes and says so', async () => {
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages\/st-nas\/test$/,
      body: { ok: true },
    })
    renderPage()
    fireEvent.click(await enabledButton('storage-test-st-nas'))
    expect(await screen.findByTestId('storage-test-result')).toHaveTextContent(
      'The test passed',
    )
    expect(mock.calls('POST', /\/storages\/st-nas\/test$/)).toHaveLength(1)
  })

  it('test failure shows the reason as a toast', async () => {
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages\/st-movies\/test$/,
      body: { ok: false, error: 'mount error(13): Permission denied' },
    })
    renderPage()
    fireEvent.click(await enabledButton('storage-test-st-movies'))
    // The toast names the share and carries the reason. The row shows the
    // same stored error too, so the toast is matched by its title.
    expect(
      await screen.findByText(/Testing “movies” failed/),
    ).toHaveTextContent('mount error(13): Permission denied')
  })

  it('mount calls the mount route and names the mountpoint', async () => {
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages\/st-movies\/mount$/,
      body: { ...movies, mounted: true, error: '' },
    })
    renderPage()
    fireEvent.click(await enabledButton('storage-mount-st-movies'))
    expect(
      await screen.findByText('Mounted at /storage/movies.'),
    ).toBeInTheDocument()
    expect(mock.calls('POST', /\/storages\/st-movies\/mount$/)).toHaveLength(1)
  })

  it('unmount is offered for a mounted share', async () => {
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/storages\/st-nas\/unmount$/,
      body: { ...nas, mounted: false },
    })
    renderPage()
    fireEvent.click(await enabledButton('storage-unmount-st-nas'))
    expect(
      await screen.findByText('Unmounted /storage/nas.'),
    ).toBeInTheDocument()
  })

  it('remove is blocked while mounted and the confirm names the libraries inside', async () => {
    renderPage()
    expect(await enabledButton('storage-mount-st-movies')).toBeEnabled()
    expect(screen.getByTestId('storage-delete-st-nas')).toBeDisabled()

    fireEvent.click(await enabledButton('storage-delete-st-movies'))
    await screen.findByText('Remove this share?')
    expect(screen.getByTestId('confirm-dialog')).toHaveTextContent(
      'No library uses this share',
    )
  })

  it('remove confirm for a share with a library names it', async () => {
    // nas is unmounted here, so its Remove is enabled and the library inside
    // it (/storage/nas/Movies) is named in the confirm.
    items = [{ ...nas, mounted: false }, movies]
    renderPage()
    fireEvent.click(await enabledButton('storage-delete-st-nas'))
    await screen.findByText('Remove this share?')
    expect(screen.getByTestId('confirm-dialog')).toHaveTextContent(
      '1 library keeps its folder inside this share',
    )
  })
})
