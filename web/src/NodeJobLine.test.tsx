// SPDX-License-Identifier: AGPL-3.0-only

/**
 * NodeJobLine tests (task 18): the condensed live line carries the file
 * name (full path on hover), the percent with its bar, the speed, the ETA
 * and the original size against the live output size.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import NodeJobLine from './NodeJobLine'
import type { Job } from './api'

function jobFixture(overrides: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    file_id: 'file-1',
    library_id: 'lib-1',
    node_id: 'node-1',
    status: 'running',
    priority: 0,
    attempt: 1,
    encoder_used: 'cpu',
    fallback_reasons: [],
    percent: 61.4,
    speed: 1.8,
    eta_seconds: 95,
    bytes_before: 31_000_000,
    bytes_after: 0,
    bytes_so_far: 12_000_000,
    file_name: 'holiday.video.mkv',
    file_path: '/media/movies/holiday.video.mkv',
    log: '',
    log_lines: 0,
    has_log: false,
    error: '',
    created_at: '2026-10-29T00:00:00Z',
    ...overrides,
  }
}

describe('NodeJobLine', () => {
  it('renders name, percent, speed, ETA and before → so far', () => {
    render(<NodeJobLine job={jobFixture()} />)

    const name = screen.getByTitle('/media/movies/holiday.video.mkv')
    expect(name).toHaveTextContent('holiday.video.mkv')
    expect(screen.getByText('61%')).toBeInTheDocument()
    expect(screen.getByTestId('node-job-line-speed')).toHaveTextContent('1.8x')
    expect(screen.getByTestId('node-job-line-eta')).toHaveTextContent(
      '1m 35s left',
    )
    expect(screen.getByTestId('node-job-line-size')).toHaveTextContent(
      '31.0 MB → 12.0 MB',
    )
  })

  it('the percent bar carries aria-valuenow', () => {
    render(<NodeJobLine job={jobFixture({ percent: 61.4 })} />)
    const bar = screen.getByRole('progressbar')
    expect(bar).toHaveAttribute('aria-valuenow', '61')
    expect(bar).toHaveAttribute('aria-valuemin', '0')
    expect(bar).toHaveAttribute('aria-valuemax', '100')
  })

  it('lays out one line at sm and up, two lines below', () => {
    const { container } = render(<NodeJobLine job={jobFixture()} />)
    const row = container.firstElementChild?.firstElementChild
    expect(row).toHaveClass('flex-col')
    expect(row).toHaveClass('sm:flex-row')
  })

  it('degrades gracefully for a job with no live numbers yet', () => {
    render(
      <NodeJobLine
        job={jobFixture({
          percent: 0,
          speed: 0,
          eta_seconds: 0,
          bytes_before: 0,
          bytes_so_far: 0,
          file_name: '',
        })}
      />,
    )
    // No file name resolved: the id stands in, no speed or ETA shown.
    expect(screen.getByText('file-1')).toBeInTheDocument()
    expect(screen.queryByTestId('node-job-line-speed')).not.toBeInTheDocument()
    expect(screen.queryByTestId('node-job-line-eta')).not.toBeInTheDocument()
    expect(screen.getByTestId('node-job-line-size')).toHaveTextContent(
      '0 B → 0 B',
    )
  })
})
