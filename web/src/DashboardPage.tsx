// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dashboard (task 07): total space saved, running jobs with live progress
 * (SSE invalidates the jobs query), queue count, recent history and a
 * per-node activity placeholder (task 08 fills it). With no libraries yet
 * it shows "Add your first folder" with the easy-mode form.
 */
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import {
  getJobs,
  getLibraries,
  getNodes,
  getStats,
  type Job,
  type Node,
} from './api'
import { formatBytes, formatEta, statusLabel } from './easyModel'
import EasyLibraryForm from './EasyLibraryForm'

function progressWidth(percent: number): string {
  return `${Math.max(0, Math.min(100, percent))}%`
}

function SpeedEta({ job }: { job: Job }) {
  const bits: string[] = []
  if (job.speed > 0) bits.push(`${job.speed.toFixed(1)}×`)
  const eta = formatEta(job.eta_seconds)
  if (eta) bits.push(`${eta} left`)
  if (bits.length === 0) return null
  return <span className="text-xs text-slate-500">{bits.join(' · ')}</span>
}

function ActiveJobs({ jobs }: { jobs: Job[] }) {
  if (jobs.length === 0) {
    return (
      <p className="text-sm text-slate-400" data-testid="dashboard-idle">
        Nothing is running right now.
      </p>
    )
  }
  return (
    <ul className="space-y-4">
      {jobs.map((job) => (
        <li key={job.id} data-testid={`running-job-${job.id}`}>
          <div className="flex items-baseline justify-between gap-2">
            <span className="truncate text-sm font-medium text-slate-200">
              {statusLabel(job.status)} · job {job.id.slice(0, 8)}
            </span>
            <span className="text-sm text-slate-400">
              {Math.round(job.percent)}%
            </span>
          </div>
          <div
            className="mt-1 h-2 overflow-hidden rounded-full bg-slate-800"
            role="progressbar"
            aria-valuenow={Math.round(job.percent)}
            aria-valuemin={0}
            aria-valuemax={100}
            data-testid={`job-progress-${job.id}`}
          >
            <div
              className="h-full rounded-full bg-emerald-500 transition-all"
              style={{ width: progressWidth(job.percent) }}
            />
          </div>
          <div className="mt-1">
            <SpeedEta job={job} />
          </div>
        </li>
      ))}
    </ul>
  )
}

/**
 * One node's live activity: name, state and what it is working on right
 * now (SSE node.updated / job.updated refresh both queries).
 */
function NodeActivity({ node, job }: { node: Node; job?: Job }) {
  const stateLabels: Record<string, string> = {
    pending: 'waiting for approval',
    approved: node.enabled ? 'ready' : 'paused',
    revoked: 'revoked',
  }
  return (
    <li className="space-y-1" data-testid={`node-activity-${node.id}`}>
      <div className="flex items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-slate-200">
          {node.name}
        </span>
        <span className="text-xs text-slate-500">
          {stateLabels[node.status] ?? node.status}
          {job ? ` · working on ${job.file_id.slice(0, 8)}` : ''}
        </span>
      </div>
      {job && (
        <div
          className="h-2 overflow-hidden rounded-full bg-slate-800"
          role="progressbar"
          aria-valuenow={Math.round(job.percent)}
          aria-valuemin={0}
          aria-valuemax={100}
          data-testid={`node-progress-${node.id}`}
        >
          <div
            className="h-full rounded-full bg-sky-500 transition-all"
            style={{ width: progressWidth(job.percent) }}
          />
        </div>
      )}
    </li>
  )
}

/**
 * The Nodes card: every registered machine with its live progress. The
 * built-in node is listed like any other.
 */
function NodeActivityList({ jobs }: { jobs: Job[] }) {
  const nodes = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  if (nodes.isPending) {
    return <p className="text-sm text-slate-400">Loading…</p>
  }
  const items = nodes.data?.items ?? []
  if (items.length === 0) {
    return (
      <p className="text-sm text-slate-400" data-testid="nodes-empty">
        This machine is doing the work. Add other machines on the{' '}
        <Link to="/nodes" className="text-emerald-400 underline">
          Nodes
        </Link>{' '}
        page.
      </p>
    )
  }
  const activeByNode = new Map<string, Job>()
  for (const j of jobs) {
    if ((j.status === 'running' || j.status === 'claimed') && j.node_id) {
      activeByNode.set(j.node_id, j)
    }
  }
  return (
    <ul className="space-y-4" data-testid="nodes-activity">
      {items.map((node) => (
        <NodeActivity
          key={node.id}
          node={node}
          job={activeByNode.get(node.id)}
        />
      ))}
    </ul>
  )
}

export default function DashboardPage() {
  const navigate = useNavigate()
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })
  const jobs = useQuery({
    queryKey: ['jobs'],
    queryFn: () => getJobs({ order: 'desc', limit: 50 }),
  })

  if (libraries.data && libraries.data.total === 0) {
    return (
      <div className="max-w-2xl space-y-4" data-testid="empty-dashboard">
        <h1 className="text-2xl font-bold">Add your first folder</h1>
        <p className="text-slate-400">
          Pick a folder of videos. ShrinkYourShit makes copies that need less
          space and swaps them in when they are smaller.
        </p>
        <EasyLibraryForm
          onSaved={(lib) => void navigate(`/libraries/${lib.id}`)}
        />
      </div>
    )
  }

  const allJobs = jobs.data?.items ?? []
  const active = allJobs.filter(
    (j) => j.status === 'running' || j.status === 'claimed',
  )
  const history = allJobs.filter(
    (j) => j.status === 'done' || j.status === 'failed',
  )

  return (
    <div className="space-y-6" data-testid="dashboard">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <Link
          to="/libraries/new"
          className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
          data-testid="dashboard-add-library"
        >
          Add folder
        </Link>
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div
          className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
          data-testid="stat-space-saved"
        >
          <p className="text-xs uppercase tracking-wide text-slate-500">
            Space saved
          </p>
          <p className="mt-1 text-xl font-bold text-emerald-400">
            {formatBytes(stats.data?.bytes_saved ?? 0)}
          </p>
        </div>
        <div
          className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
          data-testid="stat-files"
        >
          <p className="text-xs uppercase tracking-wide text-slate-500">
            Files
          </p>
          <p className="mt-1 text-xl font-bold">{stats.data?.files ?? 0}</p>
        </div>
        <div
          className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
          data-testid="stat-satisfied"
        >
          <p className="text-xs uppercase tracking-wide text-slate-500">
            Nothing to do
          </p>
          <p className="mt-1 text-xl font-bold">{stats.data?.satisfied ?? 0}</p>
        </div>
        <div
          className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
          data-testid="stat-queued"
        >
          <p className="text-xs uppercase tracking-wide text-slate-500">
            Waiting
          </p>
          <p className="mt-1 text-xl font-bold">{stats.data?.queued ?? 0}</p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
          <h2 className="font-semibold">Working now</h2>
          <div className="mt-4">
            {jobs.isPending ? (
              <p className="text-sm text-slate-400">Loading…</p>
            ) : (
              <ActiveJobs jobs={active} />
            )}
          </div>
        </section>

        <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
          <h2 className="font-semibold">Nodes</h2>
          <div className="mt-4">
            <NodeActivityList jobs={active} />
          </div>
        </section>
      </div>

      <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="font-semibold">Recent history</h2>
        {history.length === 0 ? (
          <p
            className="mt-4 text-sm text-slate-400"
            data-testid="history-empty"
          >
            No finished jobs yet.
          </p>
        ) : (
          <ul className="mt-4 space-y-2" data-testid="history">
            {history.map((job) => (
              <li
                key={job.id}
                className="flex items-center justify-between gap-4 rounded-lg bg-slate-800/50 px-3 py-2 text-sm"
                data-testid={`history-job-${job.id}`}
              >
                <span className="text-slate-300">
                  job {job.id.slice(0, 8)} ·{' '}
                  <span
                    className={
                      job.status === 'done'
                        ? 'text-emerald-400'
                        : 'text-red-400'
                    }
                  >
                    {job.status === 'done' ? 'shrunk' : 'failed'}
                  </span>
                </span>
                {job.status === 'done' && job.bytes_before > 0 && (
                  <span className="text-slate-500">
                    {formatBytes(job.bytes_before)} →{' '}
                    {formatBytes(job.bytes_after)}
                  </span>
                )}
                {job.status === 'failed' && job.error && (
                  <span className="truncate text-slate-500">{job.error}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
