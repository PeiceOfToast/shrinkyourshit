// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dashboard (task 10, restyled in task 42 to spec 5.3): read only, always.
 * It answers "is it working, and how much has it saved" in this order: the
 * EncodingStatus panel, warnings, four stats, the live work and the nodes,
 * and the recent history. Creating a library happens only at /libraries/new,
 * which the empty state links to.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  getJobs,
  getLibraries,
  getNodes,
  getSettings,
  getStats,
  getStorages,
  type Job,
  type Node,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import Card from './Card'
import EmptyState from './EmptyState'
import EncodingStatus from './EncodingStatus'
import Icon from './Icon'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { computeWarnings, type DashboardWarning } from './dashboardWarnings'
import { useProfiles } from './queries'
import { storagesKey } from './storageModel'
import {
  formatBytes,
  formatCount,
  formatEta,
  formatPercent,
  formatRelative,
} from './format'
import { useUpdateStatus } from './queries'
import { Skeleton, SkeletonRows, SkeletonStatCards } from './skeleton'
import StatCard from './StatCard'
import { btn, btnPrimary, linkTouch } from './uiControls'

/** Recent activity shows at most this many finished jobs. */
const HISTORY_ROWS = 10

function progressWidth(percent: number): string {
  return `${Math.max(0, Math.min(100, percent))}%`
}

function SpeedEta({ job }: { job: Job }) {
  const bits: string[] = []
  if (job.speed > 0) bits.push(`${job.speed.toFixed(1)}×`)
  const eta = formatEta(job.eta_seconds)
  if (eta) bits.push(`${eta} left`)
  if (bits.length === 0) return null
  return <span className="text-caption text-fg-muted">{bits.join(' · ')}</span>
}

/**
 * One running file: its name (never a job id), the percent and a progress
 * bar, then speed and ETA. The bar keeps `job-progress-{id}` for the live
 * tests.
 */
function RunningJob({ job }: { job: Job }) {
  const name = job.file_name || job.file_id
  return (
    <li className="flex flex-col gap-1" data-testid={`running-job-${job.id}`}>
      <div className="flex min-w-0 items-baseline justify-between gap-3">
        <span
          className="truncate text-body font-medium text-fg"
          title={job.file_path || name}
        >
          {name}
        </span>
        <span className="shrink-0 text-body tabular-nums text-fg-muted">
          {Math.round(job.percent)}%
        </span>
      </div>
      <div
        className="h-2 overflow-hidden rounded-full bg-raised"
        role="progressbar"
        aria-label={`Progress of ${name}`}
        aria-valuenow={Math.round(job.percent)}
        aria-valuemin={0}
        aria-valuemax={100}
        data-testid={`job-progress-${job.id}`}
      >
        <div
          className="h-full rounded-full bg-info transition-all"
          style={{ width: progressWidth(job.percent) }}
        />
      </div>
      <SpeedEta job={job} />
    </li>
  )
}

/**
 * The lifecycle badge (spec 5.0). Approval comes first: a node that is not
 * approved yet has no lifecycle to show. An approved node shows its state.
 */
function NodeBadge({ node }: { node: Node }) {
  if (node.status === 'pending') {
    return <Badge tone="warning">Waiting for approval</Badge>
  }
  if (node.status === 'revoked') {
    return <Badge tone="danger">Revoked</Badge>
  }
  switch (node.effective_state) {
    case 'running':
      return (
        <Badge tone="success" dot>
          Running
        </Badge>
      )
    case 'draining':
      return <Badge tone="warning">Draining</Badge>
    case 'error':
      return <Badge tone="danger">Error</Badge>
    default:
      return <Badge>Stopped</Badge>
  }
}

/** One node: name, lifecycle badge and last check-in, plus its bar while it works. */
function NodeRow({ node, job }: { node: Node; job?: Job }) {
  return (
    <li
      className="flex flex-col gap-2 py-3"
      data-testid={`node-activity-${node.id}`}
    >
      <div className="flex min-w-0 items-center justify-between gap-3">
        <span className="truncate text-body font-medium text-fg">
          {node.name}
        </span>
        <NodeBadge node={node} />
      </div>
      <p className="text-caption text-fg-muted">
        Checked in {formatRelative(node.last_heartbeat_at)}
      </p>
      {job && (
        <div
          className="h-2 overflow-hidden rounded-full bg-raised"
          role="progressbar"
          aria-label={`Progress on ${node.name}`}
          aria-valuenow={Math.round(job.percent)}
          aria-valuemin={0}
          aria-valuemax={100}
          data-testid={`node-progress-${node.id}`}
        >
          <div
            className="h-full rounded-full bg-info transition-all"
            style={{ width: progressWidth(job.percent) }}
          />
        </div>
      )}
    </li>
  )
}

function NodeList({
  nodes,
  nodesPending,
  jobs,
}: {
  nodes: Node[]
  nodesPending: boolean
  jobs: Job[]
}) {
  if (nodesPending) {
    return <SkeletonRows rows={2} />
  }
  if (nodes.length === 0) {
    return (
      <p className="text-body text-fg-muted" data-testid="nodes-empty">
        This machine is doing the work. Add other machines on the{' '}
        <Link to="/nodes" className={linkTouch}>
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
    <ul className="-my-3 divide-y divide-line" data-testid="nodes-activity">
      {nodes.map((node) => (
        <NodeRow key={node.id} node={node} job={activeByNode.get(node.id)} />
      ))}
    </ul>
  )
}

/** Warnings as Banners (spec 3.7), each with its one fix as a button. */
function Warnings({ warnings }: { warnings: DashboardWarning[] }) {
  if (warnings.length === 0) {
    return null
  }
  return (
    <div className="flex flex-col gap-3" data-testid="dashboard-warnings">
      {warnings.map((warning) => (
        <Banner
          key={warning.id}
          tone="warning"
          testId={`warning-${warning.id}`}
          action={
            <Link to={warning.linkTo} className={btn}>
              {warning.linkLabel}
            </Link>
          }
        >
          {warning.message}
        </Banner>
      ))}
    </div>
  )
}

/**
 * One finished job in Recent activity. The file name leads; the caption says
 * what happened, where and when; the sizes sit on the right. A failed job
 * shows its error under the caption.
 */
function HistoryRow({
  job,
  libraryName,
}: {
  job: Job
  libraryName: string | undefined
}) {
  const done = job.status === 'done'
  const name = job.file_name || job.file_id
  const when = formatRelative(job.created_at)
  const caption = [done ? 'Shrunk' : 'Failed', libraryName, when]
    .filter(Boolean)
    .join(' · ')
  const change =
    job.bytes_before > 0
      ? ((job.bytes_after - job.bytes_before) / job.bytes_before) * 100
      : 0
  return (
    <li
      className="grid grid-cols-[auto_1fr_auto] items-center gap-3 px-4 py-3 sm:px-6"
      data-testid={`history-job-${job.id}`}
    >
      <Icon
        name={done ? 'check' : 'alert'}
        className={`h-5 w-5 shrink-0 ${done ? 'text-success' : 'text-danger'}`}
      />
      <div className="min-w-0">
        <p className="truncate text-body text-fg" title={job.file_path || name}>
          {name}
        </p>
        <p className="truncate text-caption text-fg-muted">{caption}</p>
        {!done && job.error && (
          <p className="truncate text-caption text-danger">{job.error}</p>
        )}
      </div>
      {done && job.bytes_before > 0 && (
        <div className="text-right">
          <p className="whitespace-nowrap text-body tabular-nums text-fg-muted">
            {formatBytes(job.bytes_before)} → {formatBytes(job.bytes_after)}
          </p>
          <p className="whitespace-nowrap text-caption tabular-nums text-accent-text">
            {formatPercent(change)}
          </p>
        </div>
      )}
    </li>
  )
}

export default function DashboardPage() {
  useDocumentTitle(['Dashboard'])
  const stats = useQuery({ queryKey: ['stats'], queryFn: getStats })
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })
  const jobs = useQuery({
    queryKey: ['jobs'],
    queryFn: () => getJobs({ order: 'desc', limit: 50 }),
  })
  const nodes = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  const update = useUpdateStatus()
  const storages = useQuery({ queryKey: storagesKey, queryFn: getStorages })
  const settings = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const profiles = useProfiles()

  if (libraries.data && libraries.data.total === 0) {
    return (
      <PageContainer data-testid="empty-dashboard">
        <PageHeader title="Dashboard" />
        <EmptyState
          icon="library"
          title="Add your first library"
          body="Pick a folder of videos. ShrinkYourShit makes smaller copies and swaps them in only when they are smaller and verified."
          action={
            <Link className={btnPrimary} to="/libraries/new">
              Add library
            </Link>
          }
        />
      </PageContainer>
    )
  }

  const allNodes = nodes.data?.items ?? []
  const allJobs = jobs.data?.items ?? []
  const active = allJobs.filter(
    (j) => j.status === 'running' || j.status === 'claimed',
  )
  const history = allJobs
    .filter((j) => j.status === 'done' || j.status === 'failed')
    .slice(0, HISTORY_ROWS)
  const libraryNames = new Map(
    (libraries.data?.items ?? []).map((lib) => [lib.id, lib.name]),
  )
  // Paused is every known node not set to run. Unknown nodes are not paused.
  const paused =
    allNodes.length > 0 && !allNodes.some((n) => n.desired_state === 'running')
  // Files finished across every node (node.stats is per node, task 18).
  const filesDone = allNodes.reduce(
    (sum, n) => sum + (n.stats?.files_done ?? 0),
    0,
  )
  const warnings = computeWarnings({
    nodes: allNodes,
    jobs: allJobs,
    stats: { queued: stats.data?.queued ?? 0 },
    now: Date.now(),
    // Profiles whose own tier cannot show a step (task 73): one warning per
    // library that follows one. Quiet until the list has answered.
    profiles: profiles.data,
    // Quiet when the check failed or has not run (task 17).
    update: update.data
      ? { available: update.data.update_available, version: update.data.latest }
      : undefined,
    // Storage login warning (task 68). Quiet until both queries have answered.
    storage:
      storages.data && settings.data
        ? {
            mounts: storages.data.items.filter((s) => s.enabled).length,
            loginEnabled: settings.data.login_enabled,
          }
        : undefined,
  })

  return (
    <PageContainer data-testid="dashboard">
      <PageHeader title="Dashboard" />

      <EncodingStatus variant="panel" waiting={stats.data?.queued} />

      <Warnings warnings={warnings} />

      {stats.isPending ? (
        <SkeletonStatCards />
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            testid="stat-space-saved"
            label="Space saved"
            tone="good"
            value={formatBytes(stats.data?.bytes_saved ?? 0)}
            icon="encode"
            sub={
              filesDone > 0 ? `from ${formatCount(filesDone)} files` : undefined
            }
          />
          <StatCard
            testid="stat-files"
            label="Files watched"
            value={formatCount(stats.data?.files ?? 0)}
            icon="library"
          />
          <StatCard
            testid="stat-satisfied"
            label="Already small"
            value={formatCount(stats.data?.satisfied ?? 0)}
            icon="check"
            sub="nothing to do"
          />
          <StatCard
            testid="stat-queued"
            label="Waiting"
            value={formatCount(stats.data?.queued ?? 0)}
            icon="clock"
            sub={
              (stats.data?.failed ?? 0) > 0 ? (
                <span className="text-danger">
                  {formatCount(stats.data?.failed ?? 0)} failed
                </span>
              ) : undefined
            }
          />
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-12">
        <Card
          title="Working now"
          className="lg:col-span-8"
          testId="dashboard-working"
        >
          {jobs.isPending ? (
            <div
              role="status"
              aria-label="Loading"
              data-testid="jobs-skeleton"
              className="flex flex-col gap-2"
            >
              <Skeleton className="h-5 w-full" />
              <Skeleton className="h-5 w-full" />
            </div>
          ) : active.length === 0 ? (
            <p className="text-body text-fg-muted" data-testid="dashboard-idle">
              Nothing is being shrunk right now.
              {paused && ' Encoding is paused.'}
            </p>
          ) : (
            <ul className="flex flex-col gap-4">
              {active.map((job) => (
                <RunningJob key={job.id} job={job} />
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Nodes"
          className="lg:col-span-4"
          testId="dashboard-nodes"
          actions={
            <Link to="/nodes" className={linkTouch}>
              Manage
            </Link>
          }
        >
          <NodeList
            nodes={allNodes}
            nodesPending={nodes.isPending}
            jobs={active}
          />
        </Card>
      </div>

      <Card title="Recent activity" padding="none">
        {history.length === 0 ? (
          <EmptyState
            testId="history-empty"
            title="No activity yet"
            body="Finished files show up here with their sizes before and after."
          />
        ) : (
          <ul className="divide-y divide-line" data-testid="history">
            {history.map((job) => (
              <HistoryRow
                key={job.id}
                job={job}
                libraryName={libraryNames.get(job.library_id)}
              />
            ))}
          </ul>
        )}
      </Card>
    </PageContainer>
  )
}
