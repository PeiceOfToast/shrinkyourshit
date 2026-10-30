// SPDX-License-Identifier: AGPL-3.0-only

/**
 * About card tests (task 17): the card renders the version, the runtime
 * and the action that matches it — an Update button with a restart
 * warning for a binary, the exact docker commands in a copyable block for
 * a container. Check failures are quiet notes, and nothing on load ever
 * calls apply: an update happens only on an explicit click.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AboutCard from './AboutCard'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

const settings = {
  worker_concurrency: 2,
  scratch_dir: '/scratch',
  default_result_policy: 'replace_if_smaller',
  login_enabled: false,
  admin_username: '',
  preferred_languages: ['eng'],
  default_video_codec: 'hevc',
  update_check: true,
  update_channel: 'stable',
  backup_before_update: true,
}

const version = {
  version: 'v1.2.0',
  commit: 'abc1234',
  date: '2026-01-02T03:04:05Z',
  runtime: 'binary',
  update_check_enabled: true,
  channel: 'stable',
}

const upToDate = {
  current: 'v1.2.0',
  latest: 'v1.2.0',
  update_available: false,
  self_update_supported: true,
  reason: 'already on the latest release',
}

const updateAvailable = {
  current: 'v1.2.0',
  latest: 'v1.3.0',
  update_available: true,
  self_update_supported: true,
  release_url: 'https://example.com/releases/tag/v1.3.0',
  release_notes: 'Fixes a lot, adds a little.',
  published_at: '2026-10-30T00:00:00Z',
  reason: '',
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <AboutCard />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('AboutCard', () => {
  it('renders the version, the runtime and the update controls', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      { method: 'GET', match: /\/api\/v1\/update\/status$/, body: upToDate },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderCard()

    await screen.findByTestId('check-now')
    // The version rows fill in once the version query lands.
    await waitFor(() =>
      expect(screen.getByTestId('about-version')).toHaveTextContent('v1.2.0'),
    )
    expect(screen.getByTestId('about-commit')).toHaveTextContent('abc1234')
    expect(screen.getByTestId('about-date')).toHaveTextContent(
      '2026-01-02T03:04:05Z',
    )
    expect(screen.getByTestId('about-runtime')).toHaveTextContent(
      'App on this machine',
    )
    expect(screen.getByTestId('update-check-toggle')).toBeChecked()
    await waitFor(() =>
      expect(screen.getByTestId('update-channel')).toHaveValue('stable'),
    )
    // Up to date: a quiet note, no banner, no button.
    await waitFor(() =>
      expect(screen.getByTestId('update-note')).toHaveTextContent(
        'already on the latest release',
      ),
    )
    expect(screen.queryByTestId('update-banner')).not.toBeInTheDocument()
  })

  it('offers an update button with a restart warning for a binary install', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: updateAvailable,
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderCard()

    const banner = await screen.findByTestId('update-banner')
    expect(banner).toHaveTextContent('Version v1.3.0 is available')
    expect(banner).toHaveTextContent('Oct 30, 2026')
    const notes = screen.getByTestId('release-notes-link')
    expect(notes).toHaveAttribute('href', updateAvailable.release_url)
    expect(screen.getByTestId('apply-update')).toHaveTextContent('Update now')
    expect(screen.getByTestId('update-restart-warning')).toHaveTextContent(
      'restarts',
    )
    expect(
      screen.queryByTestId('container-instructions'),
    ).not.toBeInTheDocument()
  })

  it('shows the docker commands for a container instead of a button', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/version$/,
        body: { ...version, runtime: 'container' },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: {
          ...updateAvailable,
          self_update_supported: false,
          reason:
            'This app runs in a container, so it cannot update itself. Update it from the host instead:\ndocker compose pull\ndocker compose up -d\n(image: ghcr.io/peiceoftoast/shrinkyourshit)',
        },
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderCard()

    await screen.findByTestId('update-banner')
    const block = screen.getByTestId('container-instructions')
    expect(block).toHaveTextContent('docker compose pull')
    expect(block).toHaveTextContent('docker compose up -d')
    expect(block).toHaveTextContent('ghcr.io/peiceoftoast/shrinkyourshit')
    // A container never gets a self-update button.
    expect(screen.queryByTestId('apply-update')).not.toBeInTheDocument()
  })

  it('checks on demand and shows the result, without ever applying on load', async () => {
    const available = { ...updateAvailable }
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        bodyFn: () => upToDate,
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'POST',
        match: /\/api\/v1\/update\/check$/,
        body: available,
      },
    ])
    renderCard()

    await screen.findByTestId('check-now')
    // Nothing applied itself on load (task 17, the hard rule).
    expect(mock.calls('POST', /\/api\/v1\/update\/apply$/)).toHaveLength(0)

    fireEvent.click(screen.getByTestId('check-now'))
    await screen.findByTestId('update-banner')
    expect(mock.calls('POST', /\/api\/v1\/update\/check$/)).toHaveLength(1)
    expect(mock.calls('POST', /\/api\/v1\/update\/apply$/)).toHaveLength(0)
  })

  it('shows a quiet failure when the check cannot reach the server', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: {
          current: 'v1.2.0',
          update_available: false,
          self_update_supported: true,
        },
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'POST',
        match: /\/api\/v1\/update\/check$/,
        status: 503,
        body: { error: 'release check failed', code: 'internal' },
      },
    ])
    renderCard()

    fireEvent.click(await screen.findByTestId('check-now'))
    const note = await screen.findByTestId('check-error')
    expect(note).toHaveTextContent('HTTP 503')
    // Quiet means quiet: no toast anywhere.
    expect(screen.queryByTestId('toast')).not.toBeInTheDocument()
  })

  it('words the rate limit and a dead connection as their own quiet messages', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: {
          current: 'v1.2.0',
          update_available: false,
          self_update_supported: true,
        },
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'POST',
        match: /\/api\/v1\/update\/check$/,
        status: 429,
        body: { error: 'checked less than a minute ago', code: 'rate_limited' },
      },
    ])
    renderCard()

    fireEvent.click(await screen.findByTestId('check-now'))
    expect(await screen.findByTestId('check-error')).toHaveTextContent(
      'less than a minute ago',
    )

    // A fetch that never answers (no network): the connection words.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockRejectedValue(new TypeError('Failed to fetch')),
    )
    fireEvent.click(screen.getByTestId('check-now'))
    await waitFor(() =>
      expect(screen.getByTestId('check-error')).toHaveTextContent(
        'could not be reached',
      ),
    )
  })

  it('applies only on click and follows the streamed progress', async () => {
    // The status flips to an active progress once the apply POST lands,
    // like the server's event stream would produce.
    let status: Record<string, unknown> = { ...updateAvailable }
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        bodyFn: () => status,
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'POST',
        match: /\/api\/v1\/update\/apply$/,
        bodyFn: () => {
          status = {
            ...updateAvailable,
            progress: { active: true, stage: 'downloading' },
          }
          return { started: true }
        },
      },
    ])
    renderCard()

    const button = await screen.findByTestId('apply-update')
    expect(button).toBeEnabled()
    fireEvent.click(button)
    await waitFor(() =>
      expect(mock.calls('POST', /\/api\/v1\/update\/apply$/)).toHaveLength(1),
    )
    // The progress line follows the update.progress stream: the apply
    // invalidates the status query, whose refetch now carries the stage.
    await screen.findByTestId('update-progress')
    expect(screen.getByTestId('update-progress')).toHaveTextContent(
      'Downloading',
    )
    expect(screen.getByTestId('apply-update')).toBeDisabled()
  })

  it('shows a quiet failure when the apply could not use the release', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      {
        method: 'GET',
        match: /\/api\/v1\/update\/status$/,
        body: {
          ...updateAvailable,
          progress: {
            active: false,
            error: 'no release asset for this platform',
          },
        },
      },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
    ])
    renderCard()

    await screen.findByTestId('update-banner')
    const note = screen.getByTestId('update-error')
    expect(note).toHaveTextContent('no release asset for this platform')
    // Quiet: no toast anywhere.
    expect(screen.queryByTestId('toast')).not.toBeInTheDocument()
  })

  it('saves the automatic checks switch and the channel', async () => {
    const puts: Array<Record<string, unknown>> = []
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/version$/, body: version },
      { method: 'GET', match: /\/api\/v1\/update\/status$/, body: upToDate },
      { method: 'GET', match: /\/api\/v1\/settings$/, body: settings },
      {
        method: 'PUT',
        match: /\/api\/v1\/settings$/,
        bodyFn: (ctx) => {
          const body = { ...(ctx.body as Record<string, unknown>) }
          puts.push(body)
          return { ...settings, ...body }
        },
      },
    ])
    renderCard()

    fireEvent.click(await screen.findByTestId('update-check-toggle'))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]).toMatchObject({ update_check: false })

    fireEvent.change(screen.getByTestId('update-channel'), {
      target: { value: 'prerelease' },
    })
    await waitFor(() => expect(puts).toHaveLength(2))
    expect(puts[1]).toMatchObject({ update_channel: 'prerelease' })
    // Two saves, two success toasts.
    await screen.findAllByTestId('toast')
  })
})
