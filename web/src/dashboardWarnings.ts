// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dashboard warning rules (task 10), pure and timestamp-injected so they
 * are unit-testable without React. Every warning names the cause and
 * links to the place that fixes it.
 */
import type { Job, Node, Stats } from './api'

export interface DashboardWarning {
  /** Stable id, used as the test id on the surface. */
  id: string
  message: string
  linkTo: string
  linkLabel: string
}

/** The slice of the update status the dashboard warning needs (task 17). */
export interface UpdateHint {
  available: boolean
  version?: string
}

/**
 * An approved, enabled node that has not checked in for 3 minutes counts
 * as lost: the worker heartbeats every 30 s and the server requeues its
 * jobs after 120 s, so past 3 minutes it is definitely not taking work.
 */
export const HEARTBEAT_STALE_MS = 3 * 60 * 1000

const DAY_MS = 24 * 60 * 60 * 1000

/**
 * A node can take work only when it is approved, enabled, and started
 * (task 11: desired_state is the operator's start/stop intent; enabled is
 * the hard off switch). Draining still counts as running for warnings: it
 * finishes its held job.
 */
export function canRun(node: Node): boolean {
  return (
    node.status === 'approved' &&
    node.enabled &&
    node.desired_state === 'running'
  )
}

function ageMinutes(from: string, now: number): number {
  return Math.max(0, Math.round((now - Date.parse(from)) / 60000))
}

export function computeWarnings(input: {
  nodes: Node[]
  jobs: Job[]
  stats: Pick<Stats, 'queued'>
  now: number
  update?: UpdateHint | null
}): DashboardWarning[] {
  const { nodes, jobs, stats, now } = input
  const warnings: DashboardWarning[] = []

  // A newer release is out (task 17): one quiet entry pointing at the
  // About card, where the action matches the install type. The warning
  // stays away when the check failed or has not run — update trouble is
  // never loud.
  if (input.update?.available) {
    warnings.push({
      id: 'update-available',
      message: `Version ${input.update.version ?? 'newer'} is available. See the About card in App settings for what changed and how to update.`,
      linkTo: '/app',
      linkLabel: 'Go to App settings',
    })
  }

  const activeJobs = jobs.filter(
    (j) => j.status === 'running' || j.status === 'claimed',
  ).length

  // Queue not moving: files are waiting, nothing is running, and no node
  // is in a state that could start work.
  if (
    nodes.length > 0 &&
    nodes.every((n) => !canRun(n)) &&
    stats.queued > 0 &&
    activeJobs === 0
  ) {
    warnings.push({
      id: 'nodes-unavailable',
      message: `Every node is stopped or disabled, so nothing is being processed. ${
        stats.queued === 1 ? '1 file is' : `${stats.queued} files are`
      } waiting. Start or enable a node.`,
      linkTo: '/nodes',
      linkLabel: 'Go to Nodes',
    })
  }

  // Heartbeat lost: a node that should be running has not checked in.
  for (const node of nodes) {
    if (!canRun(node)) continue
    const last = node.last_heartbeat_at
    if (last && now - Date.parse(last) <= HEARTBEAT_STALE_MS) continue
    const when = last
      ? `has not checked in for ${ageMinutes(last, now)} minutes`
      : 'has never checked in'
    warnings.push({
      id: `node-heartbeat-${node.id}`,
      message: `${node.name} ${when}, so it cannot take work. Check that the machine is up and ShrinkYourShit is running on it.`,
      linkTo: '/nodes',
      linkLabel: 'Go to Nodes',
    })
  }

  // Failed jobs in the last 24 hours (the job list is the dashboard's
  // newest-first window).
  const failed = jobs.filter(
    (j) => j.status === 'failed' && now - Date.parse(j.created_at) < DAY_MS,
  )
  if (failed.length > 0) {
    warnings.push({
      id: 'failed-jobs',
      message: `${failed.length} ${
        failed.length === 1 ? 'job has' : 'jobs have'
      } failed in the last 24 hours. Open the library to see why and retry.`,
      linkTo: '/libraries',
      linkLabel: 'Go to Libraries',
    })
  }

  return warnings
}
