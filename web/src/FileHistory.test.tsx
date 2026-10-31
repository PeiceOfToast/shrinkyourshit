// SPDX-License-Identifier: AGPL-3.0-only

/**
 * FileHistory tests (task 19): the per-file panel renders every job's
 * attempts, status, encoder, fallback reasons, bytes before and after,
 * and the log tail behind a disclosure; empty and error states follow the
 * task 13 conventions.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FileHistory from './FileHistory'
import type { FileItem } from './api'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function historyFile(): FileItem {
  return {
    id: 'f1',
    library_id: 'lib-1',
    path: '/media/movies/movie.mkv',
    size: 1_000_000_000,
    mtime: 0,
    fingerprint: '',
    probe: null,
    status: 'failed',
    satisfied_reason: '',
    last_job_id: 'job-2',
    bytes_saved: 0,
    created_at: '',
    updated_at: '',
  }
}

function job(id: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    file_id: 'f1',
    library_id: 'lib-1',
    node_id: 'node-1',
    status: 'failed',
    priority: 0,
    attempt: 0,
    encoder_used: '',
    fallback_reasons: [],
    percent: 0,
    speed: 0,
    eta_seconds: 0,
    bytes_before: 0,
    bytes_after: 0,
    bytes_so_far: 0,
    file_name: 'movie.mkv',
    file_path: '/media/movies/movie.mkv',
    log: '',
    log_lines: 0,
    has_log: false,
    error: '',
    created_at: '',
    ...extra,
  }
}

let mock: FetchMock

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <FileHistory file={historyFile()} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

afterEach(() => mock?.restore())

describe('FileHistory', () => {
  it('renders attempts, encoder, fallback reasons and the log tail', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/files\/f1\/jobs$/,
        body: {
          items: [
            job('job-1', {
              status: 'failed',
              attempt: 0,
              encoder_used: 'qsv',
              fallback_reasons: [],
              bytes_before: 1_000_000_000,
              error: 'encoder init failed',
            }),
            job('job-2', {
              status: 'failed',
              attempt: 1,
              encoder_used: 'nvenc',
              fallback_reasons: ['qsv: init failed', 'vaapi: no device'],
              bytes_before: 1_000_000_000,
              bytes_after: 400_000_000,
              log: 'line one of the ffmpeg log\nline two',
            }),
          ],
          total: 2,
        },
      },
    ])
    renderPanel()

    // Oldest first: attempt 1 then attempt 2.
    const rows = await screen.findAllByTestId('file-history-job')
    expect(rows).toHaveLength(2)
    const attempts = screen.getAllByTestId('file-history-attempt')
    expect(attempts[0]).toHaveTextContent('Attempt 1')
    expect(attempts[1]).toHaveTextContent('Attempt 2')
    const encoders = screen.getAllByTestId('file-history-encoder')
    expect(encoders[0]).toHaveTextContent('qsv')
    expect(encoders[1]).toHaveTextContent('nvenc')
    // Fallback reasons are listed verbatim on the job that had them.
    const fallbacks = screen.getAllByTestId('file-history-fallbacks')
    expect(fallbacks).toHaveLength(1)
    expect(fallbacks[0]).toHaveTextContent('qsv: init failed')
    expect(fallbacks[0]).toHaveTextContent('vaapi: no device')
    // Bytes before and after.
    const bytes = screen.getAllByTestId('file-history-bytes')
    expect(bytes[1]).toHaveTextContent('1.0 GB')
    expect(bytes[1]).toHaveTextContent('400 MB')
    // The log tail sits behind a disclosure.
    const log = screen.getByTestId('file-history-log')
    expect(log).toHaveTextContent('Log')
    fireEvent.click(log.querySelector('summary') as HTMLElement)
    await waitFor(() =>
      expect(screen.getByTestId('file-history-log-text')).toHaveTextContent(
        'line two',
      ),
    )
  })

  it('says so when no jobs have run for the file', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/files\/f1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    renderPanel()
    await waitFor(() =>
      expect(screen.getByTestId('file-history-empty')).toHaveTextContent(
        'No jobs have run for this file yet.',
      ),
    )
  })

  it('shows the status and code when the history cannot load (task 13)', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/files\/f1\/jobs$/,
        status: 500,
        body: { error: 'boom', code: 'internal_error' },
      },
    ])
    renderPanel()
    const box = await screen.findByTestId('error-box')
    expect(box).toHaveTextContent('500')
    expect(box).toHaveTextContent('internal_error')
  })

  it('closes from the close button and calls onClose', async () => {
    const onClose = vi.fn()
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/files\/f1\/jobs$/,
        body: { items: [], total: 0 },
      },
    ])
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <FileHistory file={historyFile()} onClose={onClose} />
      </QueryClientProvider>,
    )
    fireEvent.click(await screen.findByTestId('file-history-close'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
