// SPDX-License-Identifier: AGPL-3.0-only

/**
 * EncodingStatus (task 40, spec 3.1.1): one control for "is it working".
 * It replaces the shell's PauseAllSwitch and JobIndicator and the Nodes
 * page's pause buttons. It reads the same queries (`nodes`, the `jobs`
 * indicator) and calls the same mutations (pauseAll, resumeAll).
 *
 * The switch is positive: on means encoding, which is any node desired to
 * run. The four variants fit the four places it lives:
 * - `rail`: label, status pill and switch (desktop rail, drawer);
 * - `icon`: a 44 px link to /nodes (tablet icon rail);
 * - `compact`: a read-only pill linking to /nodes (phone top bar);
 * - `panel`: a full-width Banner with one sentence and one action.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  getJobs,
  getNodes,
  pauseAllNodes,
  resumeAllNodes,
  type Job,
  type Node,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import Icon from './Icon'
import Switch from './Switch'
import { useToast } from './toastContext'
import { btn, btnGhost } from './uiControls'

export type EncodingVariant = 'rail' | 'icon' | 'compact' | 'panel'

type EncodingState = 'no-nodes' | 'paused' | 'running' | 'idle'

/**
 * Derives the state from the same two inputs the spec names. "Paused"
 * needs at least one node, so an unknown node list (not loaded yet, or
 * failed) never reads as paused; with jobs active it reads as running.
 */
function deriveState(nodes: Node[], active: number) {
  const hasNodes = nodes.length > 0
  const anyRunning = nodes.some((n) => n.desired_state === 'running')
  const paused = hasNodes && !anyRunning
  let state: EncodingState
  if (paused) {
    state = 'paused'
  } else if (active > 0) {
    state = 'running'
  } else if (hasNodes) {
    state = 'idle'
  } else {
    state = 'no-nodes'
  }
  return { state, hasNodes, anyRunning, active }
}

function runningLabel(active: number): string {
  return `${active} running`
}

export default function EncodingStatus({
  variant,
  waiting,
  controls = false,
}: {
  variant: EncodingVariant
  /** Files waiting in the queue, for the panel sentence when known. */
  waiting?: number
  /**
   * Panel only (Nodes, spec 5.10): the action slot holds Pause all and
   * Resume all instead of the single resume action. Same mutations and
   * toasts as the other variants.
   */
  controls?: boolean
}) {
  const client = useQueryClient()
  const toast = useToast()
  const nodesQuery = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  const jobs = useQuery({
    queryKey: ['jobs', 'indicator'],
    queryFn: () => getJobs({ limit: 50 }),
  })
  const nodes = nodesQuery.data?.items ?? []
  const active = (jobs.data?.items ?? []).filter((j: Job) => {
    return j.status === 'running' || j.status === 'claimed'
  }).length
  const { state, hasNodes, anyRunning } = deriveState(nodes, active)

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['nodes'] })
  }
  const pause = useMutation({
    mutationFn: pauseAllNodes,
    onSuccess: ({ paused: n }) => {
      toast.success(
        n > 0
          ? `Encoding paused on ${n} node${n === 1 ? '' : 's'}.`
          : 'Encoding paused.',
      )
      invalidate()
    },
    onError: (err: unknown) => toast.failure('Pausing all nodes failed', err),
  })
  const resume = useMutation({
    mutationFn: resumeAllNodes,
    onSuccess: ({ resumed: n }) => {
      toast.success(
        n > 0
          ? `Encoding resumed on ${n} node${n === 1 ? '' : 's'}.`
          : 'Nothing to resume.',
      )
      invalidate()
    },
    onError: (err: unknown) => toast.failure('Resuming all nodes failed', err),
  })
  const busy = pause.isPending || resume.isPending

  if (variant === 'icon') {
    const label =
      state === 'running'
        ? runningLabel(active)
        : state === 'paused'
          ? 'Encoding paused'
          : state === 'idle'
            ? 'Encoding idle'
            : 'No nodes'
    return (
      <Link
        to="/nodes"
        aria-label={label}
        title={label}
        className="flex min-h-11 w-11 flex-col items-center justify-center gap-0.5 rounded-control text-fg-muted hover:bg-raised hover:text-fg"
      >
        {state === 'paused' ? (
          <Icon name="pause" className="h-5 w-5 shrink-0 text-warning" />
        ) : state === 'running' ? (
          <>
            <span
              aria-hidden="true"
              className="h-2 w-2 animate-pulse rounded-full bg-info"
            />
            <span className="text-caption font-medium tabular-nums text-info">
              {active}
            </span>
          </>
        ) : (
          <span
            aria-hidden="true"
            className="h-2 w-2 rounded-full bg-fg-muted"
          />
        )}
      </Link>
    )
  }

  if (variant === 'compact') {
    // Read-only: it shows only when something needs a look or is running.
    if (state === 'idle' || state === 'no-nodes') {
      return null
    }
    return (
      <Link
        to="/nodes"
        className="inline-flex min-h-11 items-center rounded-control px-1"
      >
        <span role="status">
          {state === 'paused' ? (
            <Badge tone="warning">Paused</Badge>
          ) : (
            <Badge tone="info" dot pulse testId="job-indicator">
              {runningLabel(active)}
            </Badge>
          )}
        </span>
      </Link>
    )
  }

  if (variant === 'panel') {
    if (controls) {
      const approved = nodes.filter((n) => n.status === 'approved')
      // Pause needs something running or draining; resume needs every
      // approved node stopped (the state a pause leaves behind).
      const canPause = approved.some(
        (n) => n.desired_state === 'running' || n.desired_state === 'draining',
      )
      const canResume =
        approved.length > 0 &&
        approved.every((n) => n.desired_state === 'stopped')
      const sentence =
        state === 'paused'
          ? 'Encoding is paused on all nodes.'
          : state === 'running'
            ? `Shrinking ${active} ${active === 1 ? 'file' : 'files'}${
                waiting === undefined ? '' : ` · ${waiting} waiting`
              }`
            : state === 'no-nodes'
              ? 'No nodes yet.'
              : 'All caught up.'
      const tone =
        state === 'paused'
          ? 'warning'
          : state === 'running'
            ? 'info'
            : 'success'
      return (
        <Banner
          tone={tone}
          testId="encoding-status"
          action={
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => pause.mutate()}
                disabled={!canPause || busy}
                className={canPause ? btn : btnGhost}
                data-testid="nodes-pause-all"
              >
                {pause.isPending ? 'Pausing…' : 'Pause all'}
              </button>
              <button
                type="button"
                onClick={() => resume.mutate()}
                disabled={!canResume || busy}
                className={canResume ? btn : btnGhost}
                data-testid="nodes-resume-all"
              >
                {resume.isPending ? 'Resuming…' : 'Resume all'}
              </button>
            </div>
          }
        >
          {sentence}
        </Banner>
      )
    }
    if (state === 'no-nodes') {
      return (
        <Banner tone="info" testId="encoding-status">
          No nodes yet.
        </Banner>
      )
    }
    if (state === 'paused') {
      const sentence =
        waiting === undefined
          ? 'Encoding is paused.'
          : waiting > 0
            ? `Encoding is paused. ${waiting} ${waiting === 1 ? 'file is' : 'files are'} waiting.`
            : 'Encoding is paused. Nothing is waiting right now.'
      return (
        <Banner
          tone="warning"
          testId="encoding-status"
          action={
            <button
              type="button"
              className={btn}
              disabled={busy}
              onClick={() => resume.mutate()}
            >
              Resume encoding
            </button>
          }
        >
          {sentence}
        </Banner>
      )
    }
    if (state === 'running') {
      const shrinking = `Shrinking ${active} ${active === 1 ? 'file' : 'files'}`
      return (
        <Banner tone="info" testId="encoding-status">
          {waiting === undefined
            ? shrinking
            : `${shrinking} · ${waiting} waiting`}
        </Banner>
      )
    }
    return (
      <Banner tone="success" testId="encoding-status">
        All caught up.
      </Banner>
    )
  }

  // `rail`: the label, the status pill and the switch.
  const pill =
    state === 'running' ? (
      <Badge tone="info" dot pulse testId="job-indicator">
        {runningLabel(active)}
      </Badge>
    ) : state === 'paused' ? (
      <Badge tone="warning">Paused</Badge>
    ) : (
      <Badge>{state === 'idle' ? 'Idle' : 'No nodes'}</Badge>
    )

  return (
    <div className="flex flex-col gap-2 rounded-card bg-raised p-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-body font-medium text-fg">Encoding</span>
        <div data-testid="pause-all-switch">
          <Switch
            checked={anyRunning}
            onChange={(next) => (next ? resume.mutate() : pause.mutate())}
            disabled={busy || !hasNodes}
            label="Encoding"
            testId="pause-all-checkbox"
          />
        </div>
      </div>
      <div role="status">{pill}</div>
    </div>
  )
}
