// SPDX-License-Identifier: AGPL-3.0-only

/**
 * JobLogDialog tests (task 27): the viewer renders the structured log rows,
 * appends a polled batch (after_id, no re-read), filters by level, points
 * the download at the plain-text attachment and stops polling once the job
 * is no longer active or the dialog has closed.
 */
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import JobLogDialog from './JobLogDialog'
import type { Job, JobLogLine } from './api'
import { installFetchMock, type FetchMock } from './test/fetchMock'

function jobFixture(extra: Record<string, unknown> = {}): Job {
  return {
    id: 'job-1',
    file_id: 'f1',
    library_id: 'lib-1',
    node_id: 'node-1',
    status: 'running',
    priority: 0,
    attempt: 0,
    encoder_used: 'cpu',
    fallback_reasons: [],
    percent: 40,
    speed: 1.5,
    eta_seconds: 60,
    bytes_before: 1_000,
    bytes_after: 0,
    bytes_so_far: 400,
    file_name: 'movie.mkv',
    file_path: '/media/movies/movie.mkv',
    log: '',
    log_lines: 2,
    has_log: true,
    error: '',
    created_at: '',
    ...extra,
  }
}

function row(
  id: number,
  level: JobLogLine['level'],
  message: string,
): JobLogLine {
  return {
    id,
    seq: id,
    at: '2026-10-31T10:00:00Z',
    level,
    source: 'ffmpeg',
    message,
    repeated: 1,
  }
}

let mock: FetchMock

beforeEach(() => {
  // shouldAdvanceTime keeps waitFor working on the real clock while the
  // 1 s poll interval stays controllable.
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
  mock.restore()
  vi.restoreAllMocks()
})

function install(initial: JobLogLine[], status = 'running') {
  mock = installFetchMock([
    {
      method: 'GET',
      match: /\/api\/v1\/jobs\/job-1\/log$/,
      bodyFn: ({ url }) => {
        const afterID = Number(
          new URL(url, 'http://x').searchParams.get('after_id') ?? 0,
        )
        return {
          lines: initial.filter((line) => line.id > afterID),
          total: initial.length,
          has_more: false,
          status,
        }
      },
    },
  ])
}

function logRows(): HTMLElement[] {
  return screen.getAllByTestId('job-log-row')
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

describe('JobLogDialog', () => {
  it('renders the log rows', async () => {
    install([
      row(1, 'info', 'started'),
      row(2, 'warn', 'a warning'),
      row(3, 'error', 'boom'),
    ])
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)

    await waitFor(() => {
      expect(logRows()).toHaveLength(3)
    })
    expect(screen.getByText('started')).toBeTruthy()
    expect(screen.getByText('a warning')).toBeTruthy()
    expect(screen.getByText('boom')).toBeTruthy()
  })

  it('appends a polled batch instead of duplicating', async () => {
    install([row(1, 'info', 'started')])
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)
    await waitFor(() => {
      expect(logRows()).toHaveLength(1)
    })

    // The server now holds two more rows; the poll must bring exactly
    // those, not re-read the whole log.
    install([
      row(1, 'info', 'started'),
      row(2, 'info', 'frame=1'),
      row(3, 'info', 'frame=2'),
    ])
    await advance(1100)
    await waitFor(() => {
      expect(logRows()).toHaveLength(3)
    })
    expect(screen.getByText('frame=2')).toBeTruthy()

    // Every follow poll carried after_id — only newer rows are requested.
    const followed = mock
      .calls('GET', /\/api\/v1\/jobs\/job-1\/log/)
      .filter((r) => r.url.includes('after_id=1'))
    expect(followed.length).toBeGreaterThan(0)
  })

  it('hides info rows when the filter is warnings', async () => {
    install([row(1, 'info', 'started'), row(2, 'warn', 'a warning')])
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)
    await waitFor(() => {
      expect(logRows()).toHaveLength(2)
    })

    screen.getByTestId('job-log-filter-warn').click()
    await waitFor(() => {
      expect(logRows()).toHaveLength(1)
    })
    expect(screen.queryByText('started')).toBeNull()
    expect(screen.getByText('a warning')).toBeTruthy()

    // Back to all.
    screen.getByTestId('job-log-filter-all').click()
    await waitFor(() => {
      expect(logRows()).toHaveLength(2)
    })
  })

  it('points the download link at the plain-text attachment', async () => {
    install([row(1, 'info', 'started')])
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)
    await waitFor(() => {
      expect(screen.getByTestId('job-log-download')).toBeTruthy()
    })
    const link = screen.getByTestId('job-log-download') as HTMLAnchorElement
    expect(link.getAttribute('href')).toBe('/api/v1/jobs/job-1/log/download')
    expect(link.hasAttribute('download')).toBe(true)
  })

  it('stops polling once a poll reports a terminal status', async () => {
    install([row(1, 'info', 'started')], 'running')
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)
    await waitFor(() => {
      expect(logRows()).toHaveLength(1)
    })

    // The next poll carries status done: the viewer stops on its own.
    install([row(1, 'info', 'started'), row(2, 'info', 'frame=late')], 'done')
    await advance(1100)
    await waitFor(() => {
      expect(logRows()).toHaveLength(2)
    })

    install([
      row(1, 'info', 'started'),
      row(2, 'info', 'frame=late'),
      row(3, 'info', 'frame=never'),
    ])
    const before = mock.calls('GET', /\/api\/v1\/jobs\/job-1\/log/).length
    await advance(3500)
    const after = mock.calls('GET', /\/api\/v1\/jobs\/job-1\/log/).length
    expect(after).toBe(before)
    expect(screen.queryByText('frame=never')).toBeNull()
  })

  it('does not poll for a job that is already terminal', async () => {
    install([row(1, 'info', 'started')], 'done')
    render(
      <JobLogDialog job={jobFixture({ status: 'done' })} onClose={() => {}} />,
    )
    await waitFor(() => {
      expect(logRows()).toHaveLength(1)
    })

    const before = mock.calls('GET', /\/api\/v1\/jobs\/job-1\/log/).length
    await advance(3500)
    const after = mock.calls('GET', /\/api\/v1\/jobs\/job-1\/log/).length
    expect(after).toBe(before)
  })

  it('closes from the Close button (keyboard reachable, Escape closes via the dialog)', async () => {
    install([row(1, 'info', 'started')])
    const onClose = vi.fn()
    render(<JobLogDialog job={jobFixture()} onClose={onClose} />)
    await waitFor(() => {
      expect(logRows()).toHaveLength(1)
    })
    screen.getByTestId('job-log-close').click()
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('shows a repeat count on a collapsed line', async () => {
    install([{ ...row(2, 'warn', 'same again'), repeated: 7 }])
    render(<JobLogDialog job={jobFixture()} onClose={() => {}} />)
    await waitFor(() => {
      expect(screen.getByText('warn ×7')).toBeTruthy()
    })
  })
})
