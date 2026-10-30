// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Unit tests for the dashboard warning rules (task 10): pure logic, no
 * React, timestamps injected so the windows are exact.
 */
import { describe, expect, it } from 'vitest'
import type { Job, Node } from './api'
import { computeWarnings, HEARTBEAT_STALE_MS } from './dashboardWarnings'

const NOW = Date.parse('2026-10-29T12:00:00Z')

function node(overrides: Partial<Node> = {}): Node {
  return {
    id: 'node-1',
    name: 'media-box',
    os: 'linux',
    arch: 'amd64',
    capabilities: null,
    access_mode: 'auto',
    path_mappings: null,
    throughput_limit_mbps: 0,
    status: 'approved',
    enabled: true,
    desired_state: 'running',
    effective_state: 'running',
    concurrency: 1,
    last_heartbeat_at: new Date(NOW - 30_000).toISOString(),
    created_at: '2026-10-28T00:00:00Z',
    ...overrides,
  }
}

function job(overrides: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    file_id: 'f1',
    library_id: 'lib-1',
    node_id: 'node-1',
    status: 'running',
    priority: 0,
    attempt: 1,
    encoder_used: '',
    fallback_reasons: [],
    percent: 0,
    speed: 0,
    eta_seconds: 0,
    bytes_before: 0,
    bytes_after: 0,
    bytes_so_far: 0,
    file_name: 'movie.mkv',
    file_path: '/media/movie.mkv',
    log: '',
    error: '',
    created_at: new Date(NOW - 60_000).toISOString(),
    ...overrides,
  }
}

const base = {
  nodes: [] as Node[],
  jobs: [] as Job[],
  stats: { queued: 0 },
  now: NOW,
}

describe('computeWarnings', () => {
  it('warns when every node is stopped or disabled, files wait, nothing runs', () => {
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ enabled: false }), node({ id: 'n2', status: 'pending' })],
      stats: { queued: 7 },
    })
    expect(warnings).toHaveLength(1)
    expect(warnings[0].id).toBe('nodes-unavailable')
    expect(warnings[0].message).toContain('stopped or disabled')
    expect(warnings[0].message).toContain('7 files are waiting')
    expect(warnings[0].linkTo).toBe('/nodes')
  })

  it('uses singular wording for one waiting file', () => {
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ enabled: false })],
      stats: { queued: 1 },
    })
    expect(warnings[0].message).toContain('1 file is waiting')
  })

  it('stays quiet while a job is still running, even with all nodes off', () => {
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ enabled: false })],
      jobs: [job({ status: 'running' })],
      stats: { queued: 7 },
    })
    expect(warnings).toHaveLength(0)
  })

  it('stays quiet when nothing is queued or no nodes exist yet', () => {
    expect(
      computeWarnings({ ...base, nodes: [node({ enabled: false })] }),
    ).toHaveLength(0)
    expect(
      computeWarnings({ ...base, nodes: [], stats: { queued: 5 } }),
    ).toHaveLength(0)
  })

  it('stays quiet when at least one node can run', () => {
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ enabled: false }), node({ id: 'n2' })],
      stats: { queued: 7 },
    })
    expect(warnings).toHaveLength(0)
  })

  it('warns for a heartbeat lost beyond the stale threshold', () => {
    const stale = new Date(NOW - HEARTBEAT_STALE_MS - 1000).toISOString()
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ last_heartbeat_at: stale }), node({ id: 'n2' })],
    })
    expect(warnings).toHaveLength(1)
    expect(warnings[0].id).toBe('node-heartbeat-node-1')
    expect(warnings[0].message).toContain('media-box has not checked in')
    expect(warnings[0].message).toContain('cannot take work')
    expect(warnings[0].linkTo).toBe('/nodes')
  })

  it('warns for an approved node that never checked in', () => {
    const warnings = computeWarnings({
      ...base,
      nodes: [node({ last_heartbeat_at: undefined })],
    })
    expect(warnings).toHaveLength(1)
    expect(warnings[0].message).toContain('has never checked in')
  })

  it('does not warn for fresh heartbeats or nodes that may not run', () => {
    const fresh = new Date(NOW - HEARTBEAT_STALE_MS + 1000).toISOString()
    const warnings = computeWarnings({
      ...base,
      nodes: [
        node({ last_heartbeat_at: fresh }),
        node({ id: 'n2', enabled: false }),
        node({ id: 'n3', status: 'pending' }),
        node({ id: 'n4', status: 'revoked' }),
      ],
    })
    expect(warnings).toHaveLength(0)
  })

  it('warns about failed jobs inside the 24 hour window', () => {
    const warnings = computeWarnings({
      ...base,
      jobs: [
        job({
          status: 'failed',
          created_at: new Date(NOW - 3600_000).toISOString(),
        }),
        job({
          id: 'job-2',
          status: 'failed',
          created_at: new Date(NOW - 25 * 3600_000).toISOString(),
        }),
        job({ id: 'job-3', status: 'done' }),
      ],
    })
    expect(warnings).toHaveLength(1)
    expect(warnings[0].id).toBe('failed-jobs')
    expect(warnings[0].message).toContain('1 job has failed')
    expect(warnings[0].linkTo).toBe('/libraries')
  })

  it('warns with plural wording for several failed jobs', () => {
    const warnings = computeWarnings({
      ...base,
      jobs: [
        job({ status: 'failed', id: 'a' }),
        job({ status: 'failed', id: 'b' }),
      ],
    })
    expect(warnings[0].message).toContain('2 jobs have failed')
  })
})
