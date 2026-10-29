// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Folder picker tests (task 13): Escape closes the picker, focus returns
 * to the trigger, and the current path is shown in full for normal paths
 * and middle-truncated for long ones (never cut at the end).
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import FolderPickerModal from './FolderPickerModal'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

const browseRoot = {
  path: '/',
  parent: '',
  entries: [{ name: 'media', path: '/media', is_dir: true }],
}

/** A trigger plus the picker, like the pages use it. */
function Harness({
  initialPath,
  openByDefault = false,
}: {
  initialPath?: string
  openByDefault?: boolean
}) {
  const [open, setOpen] = useState(openByDefault)
  return (
    <div>
      <button
        type="button"
        data-testid="picker-trigger"
        onClick={() => setOpen(true)}
      >
        Choose a folder
      </button>
      {open && (
        <FolderPickerModal
          initialPath={initialPath}
          onClose={() => setOpen(false)}
          onPick={() => setOpen(false)}
        />
      )}
    </div>
  )
}

function renderPicker(initialPath?: string, openByDefault = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <Harness initialPath={initialPath} openByDefault={openByDefault} />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock

afterEach(() => mock?.restore())

describe('FolderPickerModal', () => {
  it('closes on Escape and focus returns to the element that opened it', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/fs\/browse$/, body: browseRoot },
    ])
    renderPicker()

    // jsdom does not focus on click, so focus the trigger first — that is
    // what a real keyboard user's focus looks like when the picker opens.
    const trigger = screen.getByTestId('picker-trigger')
    trigger.focus()
    fireEvent.click(trigger)
    expect(screen.getByTestId('folder-picker')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByTestId('folder-picker')).not.toBeInTheDocument(),
    )
    // Focus is back on the trigger, so keyboard users are not stranded.
    expect(trigger).toHaveFocus()
  })

  it('shows the current path in full for a normal path', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/fs\/browse$/, body: browseRoot },
    ])
    renderPicker('/media/movies', true)
    await waitFor(() =>
      expect(screen.getByTestId('folder-picker-dir-media')).toBeInTheDocument(),
    )

    // A short path is shown in full.
    expect(screen.getByTestId('folder-picker-current')).toHaveTextContent(
      '/media/movies',
    )
    // The full path is always reachable through the tooltip.
    expect(screen.getByTestId('folder-picker-current')).toHaveAttribute(
      'title',
      '/media/movies',
    )
  })

  it('middle-truncates a long path so both ends stay readable', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/fs\/browse$/, body: browseRoot },
    ])
    renderPicker('/media/very-long-library-name/subfolder/deep/movies', true)
    const current = await screen.findByTestId('folder-picker-current')
    const deep = '/media/very-long-library-name/subfolder/deep/movies'
    expect(current).toHaveTextContent('…')
    expect(current.textContent!.length).toBeLessThan(deep.length)
    expect(current.textContent!.startsWith('/media')).toBe(true)
    expect(current.textContent!.endsWith('movies')).toBe(true)
    // The full path stays reachable through the tooltip.
    expect(current).toHaveAttribute('title', deep)
  })

  it('picks the shown folder through the footer button', async () => {
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/fs\/browse$/, body: browseRoot },
    ])
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const picked: string[] = []
    render(
      <QueryClientProvider client={client}>
        <ToastProvider>
          <FolderPickerModal
            initialPath="/media/movies"
            onClose={() => {}}
            onPick={(path) => picked.push(path)}
          />
        </ToastProvider>
      </QueryClientProvider>,
    )
    await screen.findByTestId('folder-picker-use')
    fireEvent.click(screen.getByTestId('folder-picker-use'))
    expect(picked).toEqual(['/media/movies'])
  })
})
