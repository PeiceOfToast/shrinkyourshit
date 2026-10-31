// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Plugins tab tests (task 31): the tab lists the installed plugins with
 * the security statement, the install form posts the typed source, the
 * enable switch is an explicit PUT, the settings form builds from the
 * manifest and saves the typed values, the Test button shows the output,
 * and Remove is confirmed.
 */
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  installFetchMock,
  stubEventSource,
  type FetchMock,
} from './test/fetchMock'
import PluginsPage from './PluginsPage'
import { ToastProvider } from './toast'

const plugin = {
  id: 'pl-1',
  name: 'notify-telegram',
  version: '1.0.0',
  type: 'notify',
  enabled: false,
  source: '/plugins/notify-telegram',
  description: 'Sends a message when a file finishes.',
  timeout_seconds: 30,
  settings: [{ key: 'chat_id', label: 'Chat id', required: true }],
  values: {},
  installed_at: '2026-01-01T00:00:00Z',
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <PluginsPage />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock
beforeEach(() => {
  stubEventSource()
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/plugins$/,
      body: { items: [plugin], total: 1 },
    },
  ])
})
afterEach(() => {
  mock.restore()
  cleanup()
  vi.unstubAllGlobals()
})

describe('the plugins tab', () => {
  it('lists the installed plugin with the security statement', async () => {
    renderPage()
    expect(await screen.findByTestId('plugins-list')).toBeTruthy()
    const row = screen.getByTestId('plugin-row-pl-1')
    expect(row.textContent).toContain('notify-telegram')
    expect(row.textContent).toContain('1.0.0')
    // The statement says plainly what a plugin runs as, and that there
    // is no sandbox.
    const text = screen.getByTestId('plugins-page').textContent ?? ''
    expect(text).toContain('no sandbox')
    // Installed plugins are off until enabled.
    expect(
      screen.getByTestId('plugin-switch-pl-1').getAttribute('aria-checked'),
    ).toBe('false')
  })

  it('installs from a git URL plus ref', async () => {
    renderPage()
    fireEvent.click(await screen.findByTestId('plugins-add'))
    await waitFor(() => {
      expect(screen.getByTestId('plugin-install-form')).toBeTruthy()
    })
    fireEvent.change(screen.getByTestId('plugin-git-url'), {
      target: { value: 'https://github.com/you/notify-telegram' },
    })
    fireEvent.change(screen.getByTestId('plugin-git-ref'), {
      target: { value: 'v1' },
    })
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/plugins\/install$/,
      status: 201,
      body: plugin,
    })
    fireEvent.click(screen.getByTestId('plugin-install-submit'))
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/plugins\/install/)
      expect(calls).toHaveLength(1)
      expect(calls[0].body).toEqual({
        git_url: 'https://github.com/you/notify-telegram',
        ref: 'v1',
      })
    })
  })

  it('installs from a folder path', async () => {
    renderPage()
    fireEvent.click(await screen.findByTestId('plugins-add'))
    await waitFor(() => {
      expect(screen.getByTestId('plugin-install-form')).toBeTruthy()
    })
    fireEvent.click(screen.getByTestId('plugin-install-mode-path'))
    fireEvent.change(screen.getByTestId('plugin-path'), {
      target: { value: '/media/plugins/notify-telegram' },
    })
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/plugins\/install$/,
      status: 201,
      body: plugin,
    })
    fireEvent.click(screen.getByTestId('plugin-install-submit'))
    await waitFor(() => {
      const calls = mock.calls('POST', /\/api\/v1\/plugins\/install/)
      expect(calls).toHaveLength(1)
      expect(calls[0].body).toEqual({
        path: '/media/plugins/notify-telegram',
      })
    })
  })

  it('enabling is an explicit PUT of the enabled switch', async () => {
    renderPage()
    mock.routes.push({
      method: 'PUT',
      match: /\/api\/v1\/plugins\/pl-1$/,
      body: { ...plugin, enabled: true },
    })
    fireEvent.click(await screen.findByTestId('plugin-switch-pl-1'))
    await waitFor(() => {
      const calls = mock.calls('PUT', /\/api\/v1\/plugins\/pl-1$/)
      expect(calls).toHaveLength(1)
      expect(calls[0].body).toEqual({ enabled: true })
    })
  })

  it('the settings form builds from the manifest and saves the typed values', async () => {
    renderPage()
    fireEvent.click(await screen.findByTestId('plugin-settings-pl-1'))
    await waitFor(() => {
      expect(screen.getByTestId('plugin-settings-dialog')).toBeTruthy()
    })
    const field = screen.getByTestId(
      'plugin-setting-chat_id',
    ) as HTMLInputElement
    expect(field.getAttribute('required')).toBe('')
    fireEvent.change(field, { target: { value: '@channel' } })
    mock.routes.push({
      method: 'PUT',
      match: /\/api\/v1\/plugins\/pl-1$/,
      body: { ...plugin, values: { chat_id: '@channel' } },
    })
    fireEvent.click(screen.getByTestId('plugin-settings-save'))
    await waitFor(() => {
      const calls = mock.calls('PUT', /\/api\/v1\/plugins\/pl-1$/)
      expect(calls).toHaveLength(1)
      expect(calls[0].body).toEqual({ settings: { chat_id: '@channel' } })
    })
  })

  it('the test button shows the captured output', async () => {
    renderPage()
    mock.routes.push({
      method: 'POST',
      match: /\/api\/v1\/plugins\/pl-1\/test$/,
      body: {
        ok: true,
        skipped: false,
        exit_code: 0,
        output: ['from git', 'from git2'],
      },
    })
    fireEvent.click(await screen.findByTestId('plugin-test-pl-1'))
    await waitFor(() => {
      expect(screen.getByTestId('plugin-test-output')).toBeTruthy()
    })
    expect(screen.getByTestId('plugin-test-output').textContent).toContain(
      'from git',
    )
    expect(screen.getByTestId('plugin-test-output').textContent).toContain(
      'from git2',
    )
  })

  it('remove is confirmed', async () => {
    renderPage()
    fireEvent.click(await screen.findByTestId('plugin-delete-pl-1'))
    await waitFor(() => {
      expect(screen.getByTestId('plugin-delete-dialog')).toBeTruthy()
    })
    mock.routes.push({
      method: 'DELETE',
      match: /\/api\/v1\/plugins\/pl-1$/,
      status: 204,
    })
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'))
    await waitFor(() => {
      expect(mock.calls('DELETE', /\/api\/v1\/plugins\/pl-1$/)).toHaveLength(1)
    })
  })
})
