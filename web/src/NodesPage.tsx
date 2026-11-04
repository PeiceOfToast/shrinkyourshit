// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Nodes page (task 08, restyled in task 48 to spec 5.10): every registered
 * worker machine. Pending nodes sit at the top with Approve/Reject; "Add
 * node" mints a one-time pairing token and shows the three install
 * commands; the edit dialog covers name, on/off, access mode and path
 * mappings.
 *
 * Task 13: every mutation toasts, destructive actions confirm first with
 * dialog text that says what happens, and every dialog is a native
 * <dialog>. Task 18: a switch per node (off is the graceful drain, "Stop
 * now" is the hard stop behind a confirm) and per node totals.
 *
 * Task 48: one lifecycle Badge per approved node, the switch labelled
 * "Encoding", Pause all and Resume all in the EncodingStatus panel, Revoke
 * behind a ConfirmDialog, FFmpeg details in a Disclosure, and the join
 * commands as CodeBlocks with copy.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  approveNode,
  createNodeToken,
  deleteNode,
  getJobs,
  getNodes,
  revokeNode,
  startNode,
  stopNode,
  updateNode,
  type Job,
  type Node,
  type PathMapping,
  type QSVFeatures,
} from './api'
import Badge, { type BadgeTone } from './Badge'
import Card from './Card'
import Checkbox from './Checkbox'
import CodeBlock from './CodeBlock'
import ConfirmDialog from './ConfirmDialog'
import Disclosure from './Disclosure'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import EmptyState from './EmptyState'
import EncodingStatus from './EncodingStatus'
import ErrorBox from './ErrorBox'
import { Field } from './FormSection'
import {
  encoderLabel,
  formatBytes,
  formatCount,
  formatDateTime,
  formatRelative,
} from './format'
import Icon from './Icon'
import NodeJobLine from './NodeJobLine'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { SkeletonCard } from './skeleton'
import Switch from './Switch'
import { useToast } from './toastContext'
import {
  btn,
  btnDangerGhost,
  btnGhost,
  btnIcon,
  btnPrimary,
  input,
  select,
} from './uiControls'

/** Lifecycle of an approved node (spec 5.0). Running has the dot. */
const lifecycle: Record<
  string,
  { tone: BadgeTone; label: string; dot: boolean }
> = {
  running: { tone: 'success', label: 'Running', dot: true },
  stopped: { tone: 'neutral', label: 'Stopped', dot: false },
  draining: { tone: 'warning', label: 'Draining', dot: true },
  error: { tone: 'danger', label: 'Error', dot: false },
}

const accessModeLabels: Record<string, string> = {
  auto: 'Find the best way (automatic)',
  shared: 'Reads the folders directly',
  transfer: 'Files are sent over the network',
}

/** `ffmpeg version 7.1.4-Jellyfin Copyright …` becomes `7.1.4-Jellyfin`. */
function shortFfmpegVersion(banner: string): string {
  const m = banner.match(/ffmpeg version (\S+)/)
  return m ? m[1] : (banner.split(/\s+/)[0] ?? banner)
}

/** A stepper button: `btnIcon`, with a visible disabled state. */
const stepBtn = `${btnIcon} disabled:cursor-not-allowed disabled:opacity-60`

/** One labelled figure in a node card's facts row. */
function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-caption text-fg-muted">{label}</dt>
      <dd className="text-body font-semibold tabular-nums text-fg">
        {children}
      </dd>
    </div>
  )
}

function JoinCommand({
  label,
  text,
  testId,
}: {
  label: string
  text: string
  testId: string
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-body font-semibold text-fg">{label}</h3>
      <CodeBlock copy testId={testId}>
        {text}
      </CodeBlock>
    </section>
  )
}

function AddNodeDialog({ onClose }: { onClose: () => void }) {
  const toast = useToast()
  const created = useMutation({
    mutationFn: createNodeToken,
    onSuccess: () =>
      toast.success('One-time code created. It expires in 24 hours.'),
    onError: (err: unknown) =>
      toast.failure('Creating the one-time code failed', err),
  })
  const tok = created.data

  return (
    <ModalDialog
      onClose={onClose}
      label="Add a node"
      testId="add-node-dialog"
      size="lg"
    >
      <DialogHeader title="Add a node" onClose={onClose} />
      <DialogBody>
        {!tok ? (
          <div className="flex flex-col gap-4">
            <p>
              A one-time code lets one machine join this server without waiting
              for approval. It is valid for 24 hours.
            </p>
            <div>
              <button
                type="button"
                onClick={() => created.mutate()}
                disabled={created.isPending}
                className={btnPrimary}
                data-testid="add-node-generate"
              >
                {created.isPending
                  ? 'Generating…'
                  : 'Generate install commands'}
              </button>
            </div>
            {created.isError && (
              <p className="text-caption text-danger" role="alert">
                Could not create a code: {(created.error as Error).message}
              </p>
            )}
          </div>
        ) : (
          <div className="flex flex-col gap-5" data-testid="add-node-commands">
            <p>
              Run one of these on the other machine. The code works once and
              expires {formatDateTime(tok.expires_at)}.
            </p>
            <JoinCommand
              label="Docker"
              text={tok.commands.docker}
              testId="add-node-docker"
            />
            <JoinCommand
              label="Linux / macOS"
              text={tok.commands.shell}
              testId="add-node-shell"
            />
            <JoinCommand
              label="Windows (PowerShell)"
              text={tok.commands.powershell}
              testId="add-node-powershell"
            />
            <p className="text-caption text-fg-muted">
              The machine appears here as soon as it asks to join. Without a
              code it still can — you approve it below.
            </p>
          </div>
        )}
      </DialogBody>
    </ModalDialog>
  )
}

function EditNodeDialog({
  node,
  busy,
  onSave,
  onClose,
}: {
  node: Node
  busy: boolean
  onSave: (body: {
    name: string
    enabled: boolean
    access_mode: string
    path_mappings: PathMapping[]
    throughput_limit_mbps: number
  }) => void
  onClose: () => void
}) {
  const [name, setName] = useState(node.name)
  const [enabled, setEnabled] = useState(node.enabled)
  const [mode, setMode] = useState(node.access_mode || 'auto')
  const [mappings, setMappings] = useState<PathMapping[]>(
    node.path_mappings ?? [],
  )
  const [limit, setLimit] = useState(String(node.throughput_limit_mbps ?? 0))

  const setRow = (i: number, key: keyof PathMapping, value: string) => {
    setMappings((rows) =>
      rows.map((row, j) => (j === i ? { ...row, [key]: value } : row)),
    )
  }

  return (
    <ModalDialog
      onClose={onClose}
      label={`Edit ${node.name}`}
      testId="edit-node-dialog"
      size="md"
    >
      <DialogHeader title="Edit node" onClose={onClose} />
      <DialogBody>
        <div className="flex flex-col gap-5">
          <Field label="Name" htmlFor="edit-node-name-input">
            <input
              id="edit-node-name-input"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className={input}
              data-testid="edit-node-name"
            />
          </Field>
          <Checkbox
            checked={enabled}
            onChange={setEnabled}
            label="This node takes on work"
            testId="edit-node-enabled"
          />
          <Field label="How it reaches files" htmlFor="edit-node-access-input">
            <select
              id="edit-node-access-input"
              value={mode}
              onChange={(e) => setMode(e.target.value)}
              className={`${select} max-w-xs`}
              data-testid="edit-node-access-mode"
            >
              {Object.entries(accessModeLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
          <Field
            label="Transfer speed limit"
            htmlFor="edit-node-throughput-input"
            help="Only used when files are sent over the network. 0 = no limit."
          >
            <input
              id="edit-node-throughput-input"
              type="number"
              min={0}
              value={limit}
              onChange={(e) => setLimit(e.target.value)}
              className={`${input} w-32`}
              data-testid="edit-node-throughput"
            />
          </Field>

          <fieldset className="flex flex-col gap-2">
            <legend className="text-body font-medium text-fg">
              Folder paths
            </legend>
            <p className="text-caption text-fg-muted">
              If this machine sees the same folders under different names (a NAS
              mount), map them here: the path on the server and the path on this
              node.
            </p>
            {mappings.map((row, i) => (
              <div
                key={i}
                className="flex flex-col gap-2 sm:flex-row sm:items-center"
              >
                <input
                  type="text"
                  value={row.server_path}
                  placeholder="/media on the server"
                  aria-label={`Path on the server, mapping ${i + 1}`}
                  onChange={(e) => setRow(i, 'server_path', e.target.value)}
                  className={`${input} min-w-0 flex-1`}
                  data-testid={`mapping-server-${i}`}
                />
                <span
                  aria-hidden="true"
                  className="hidden text-fg-muted sm:inline"
                >
                  →
                </span>
                <input
                  type="text"
                  value={row.node_path}
                  placeholder="path on this node"
                  aria-label={`Path on this node, mapping ${i + 1}`}
                  onChange={(e) => setRow(i, 'node_path', e.target.value)}
                  className={`${input} min-w-0 flex-1`}
                  data-testid={`mapping-node-${i}`}
                />
                <button
                  type="button"
                  onClick={() =>
                    setMappings((rows) => rows.filter((_, j) => j !== i))
                  }
                  className={`${btnIcon} self-start hover:text-danger sm:self-auto`}
                  aria-label={`Remove mapping ${i + 1}`}
                >
                  <Icon name="close" className="h-5 w-5" />
                </button>
              </div>
            ))}
            <div>
              <button
                type="button"
                onClick={() =>
                  setMappings((rows) => [
                    ...rows,
                    { server_path: '', node_path: '' },
                  ])
                }
                className={btn}
                data-testid="add-mapping"
              >
                Add a mapping
              </button>
            </div>
          </fieldset>
        </div>
      </DialogBody>
      <DialogFooter>
        <button
          type="button"
          onClick={onClose}
          className={btnGhost}
          data-testid="edit-node-cancel"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() =>
            onSave({
              name: name.trim(),
              enabled,
              access_mode: mode,
              path_mappings: mappings.filter(
                (m) => m.server_path.trim() !== '' && m.node_path.trim() !== '',
              ),
              throughput_limit_mbps: Math.max(0, Number(limit) || 0),
            })
          }
          disabled={busy || name.trim() === ''}
          className={btnPrimary}
          data-testid="edit-node-save"
        >
          {busy ? 'Saving…' : 'Save changes'}
        </button>
      </DialogFooter>
    </ModalDialog>
  )
}

function NodeRow({
  node,
  runningJobs,
  currentJob,
  onEdit,
}: {
  node: Node
  runningJobs: number
  currentJob?: Job
  onEdit: (node: Node) => void
}) {
  const client = useQueryClient()
  const toast = useToast()
  const [confirming, setConfirming] = useState<
    null | 'stop' | 'reject' | 'revoke'
  >(null)
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['nodes'] })
    void client.invalidateQueries({ queryKey: ['jobs'] })
  }
  const approve = useMutation({
    mutationFn: approveNode,
    onSuccess: () => {
      toast.success(`${node.name} approved.`)
      refresh()
    },
    onError: (err: unknown) => toast.failure('Approving failed', err),
  })
  const revoke = useMutation({
    mutationFn: revokeNode,
    onSuccess: () => {
      setConfirming(null)
      toast.success(`${node.name} revoked. Its jobs return to the queue.`)
      refresh()
    },
    onError: (err: unknown) => toast.failure('Revoking failed', err),
  })
  const reject = useMutation({
    mutationFn: deleteNode,
    onSuccess: () => {
      toast.success(`${node.name} rejected. Files on disk are untouched.`)
      refresh()
    },
    onError: (err: unknown) => toast.failure('Rejecting failed', err),
  })
  const start = useMutation({
    mutationFn: startNode,
    onSuccess: () => {
      toast.success(`${node.name} started. It picks up queued files.`)
      refresh()
    },
    onError: (err: unknown) => toast.failure('Starting failed', err),
  })
  // The switch's off side is the graceful stop (drain: the current file
  // finishes); "Stop now" is the hard stop behind a confirmation.
  const stop = useMutation({
    mutationFn: (args: { id: string; drain: boolean }) =>
      stopNode(args.id, args.drain),
    onSuccess: (_node, args) => {
      setConfirming(null)
      toast.success(
        args.drain
          ? `${node.name} will stop after its current job.`
          : `${node.name} stopped.`,
      )
      refresh()
    },
    onError: (err: unknown) => toast.failure('Stopping failed', err),
  })
  const setConcurrency = useMutation({
    mutationFn: (args: { id: string; concurrency: number }) =>
      updateNode(args.id, { concurrency: args.concurrency }),
    onSuccess: (_node, args) => {
      toast.success(`Parallel jobs set to ${args.concurrency}.`)
      refresh()
    },
    onError: (err: unknown) =>
      toast.failure('Changing parallel jobs failed', err),
  })

  const encoders = node.capabilities?.encoders ?? []
  const ffmpeg = node.capabilities?.ffmpeg_version
  const qsvFeatures = encoders.find((e) => e.name === 'qsv')?.qsv_features
  const running = node.desired_state === 'running'
  const approved = node.status === 'approved'
  const stopping =
    node.desired_state === 'running' || node.desired_state === 'draining'
  const life = lifecycle[node.effective_state] ?? {
    tone: 'neutral' as const,
    label: node.effective_state,
    dot: false,
  }

  const header = approved ? (
    <>
      <Badge
        tone={life.tone}
        dot={life.dot}
        testId={`node-lifecycle-${node.id}`}
      >
        {life.label}
      </Badge>
      <span className="text-body font-medium text-fg">Encoding</span>
      <Switch
        checked={running}
        onChange={(on) =>
          on ? start.mutate(node.id) : stop.mutate({ id: node.id, drain: true })
        }
        label={`Encoding on ${node.name}`}
        testId={`node-switch-${node.id}`}
        disabled={start.isPending || stop.isPending}
      />
    </>
  ) : node.status === 'pending' ? (
    <Badge tone="warning" testId={`node-status-${node.id}`}>
      Waiting for approval
    </Badge>
  ) : (
    <Badge tone="danger" testId={`node-status-${node.id}`}>
      Revoked
    </Badge>
  )

  const footer = approved ? (
    <>
      <button
        type="button"
        onClick={() => onEdit(node)}
        className={btnGhost}
        data-testid={`node-edit-${node.id}`}
      >
        Edit
      </button>
      {stopping && (
        <button
          type="button"
          onClick={() => setConfirming('stop')}
          disabled={stop.isPending}
          className={btnDangerGhost}
          data-testid={`node-stop-now-${node.id}`}
        >
          {stop.isPending ? 'Stopping…' : 'Stop now'}
        </button>
      )}
      <button
        type="button"
        onClick={() => setConfirming('revoke')}
        disabled={revoke.isPending}
        className={btnDangerGhost}
        data-testid={`node-revoke-${node.id}`}
      >
        {revoke.isPending ? 'Revoking…' : 'Revoke'}
      </button>
    </>
  ) : node.status === 'pending' ? (
    <>
      <button
        type="button"
        onClick={() => approve.mutate(node.id)}
        disabled={approve.isPending}
        className={btnPrimary}
        data-testid={`node-approve-${node.id}`}
      >
        {approve.isPending ? 'Approving…' : 'Approve'}
      </button>
      <button
        type="button"
        onClick={() => setConfirming('reject')}
        disabled={reject.isPending}
        className={btnDangerGhost}
        data-testid={`node-reject-${node.id}`}
      >
        {reject.isPending ? 'Rejecting…' : 'Reject'}
      </button>
    </>
  ) : null

  return (
    <li className="flex" data-testid={`node-${node.id}`}>
      <Card
        padding="none"
        title={node.name}
        actions={header}
        footer={footer}
        className="w-full"
      >
        {approved ? (
          <div className="flex flex-col gap-4 px-4 pb-4 sm:px-6 sm:pb-6">
            {currentJob ? (
              <div data-testid={`node-current-job-${node.id}`}>
                <NodeJobLine job={currentJob} />
              </div>
            ) : (
              <p className="text-body text-fg-muted">
                {runningJobs > 0
                  ? `Working on ${runningJobs} ${runningJobs === 1 ? 'file' : 'files'}.`
                  : 'Not working on anything.'}
              </p>
            )}

            <dl
              className="grid grid-cols-2 gap-4 sm:grid-cols-4"
              data-testid={`node-stats-${node.id}`}
            >
              <Fact label="Files done">
                {formatCount(node.stats?.files_done ?? 0)}
              </Fact>
              <Fact label="Saved">
                <span className="text-accent-text">
                  {formatBytes(node.stats?.bytes_saved ?? 0)}
                </span>
              </Fact>
              <Fact label="Last check-in">
                {formatRelative(node.last_heartbeat_at)}
              </Fact>
              <Fact label="Parallel jobs">
                <span
                  className="inline-flex items-center gap-1"
                  data-testid={`node-concurrency-${node.id}`}
                >
                  <button
                    type="button"
                    onClick={() =>
                      setConcurrency.mutate({
                        id: node.id,
                        concurrency: Math.max(1, node.concurrency - 1),
                      })
                    }
                    disabled={setConcurrency.isPending || node.concurrency <= 1}
                    className={stepBtn}
                    aria-label="Fewer parallel jobs"
                    data-testid={`node-concurrency-down-${node.id}`}
                  >
                    −
                  </button>
                  <span className="w-8 text-center tabular-nums">
                    {node.concurrency}
                  </span>
                  <button
                    type="button"
                    onClick={() =>
                      setConcurrency.mutate({
                        id: node.id,
                        concurrency: Math.min(16, node.concurrency + 1),
                      })
                    }
                    disabled={
                      setConcurrency.isPending || node.concurrency >= 16
                    }
                    className={stepBtn}
                    aria-label="More parallel jobs"
                    data-testid={`node-concurrency-up-${node.id}`}
                  >
                    +
                  </button>
                </span>
              </Fact>
            </dl>

            <div className="flex flex-col gap-3 border-t border-line pt-4">
              {encoders.length > 0 && (
                <div
                  className="flex flex-wrap items-center gap-2"
                  data-testid={`node-encoders-${node.id}`}
                >
                  {encoders.map((e) => (
                    <Badge key={e.name}>{encoderLabel(e.name)}</Badge>
                  ))}
                </div>
              )}
              <p className="text-caption text-fg-muted">
                {node.os || '?'}/{node.arch || '?'}
              </p>
              {/* QSV extras (task 35): what this machine's probe actually
                  found, in plain language, so a setting that did nothing is
                  explainable. */}
              {qsvFeatures && (
                <QSVFeatureStrip id={node.id} features={qsvFeatures} />
              )}
              {node.state_error && (
                <p
                  className="text-caption text-danger"
                  data-testid={`node-state-error-${node.id}`}
                >
                  {node.state_error}
                </p>
              )}
            </div>
          </div>
        ) : (
          <div className="px-4 pb-4 sm:px-6 sm:pb-6">
            <p className="text-body text-fg-muted">
              {node.os || '?'}/{node.arch || '?'} · asked to join just now
            </p>
          </div>
        )}

        {approved && ffmpeg && (
          <div className="border-t border-line">
            <Disclosure
              summary="Technical details"
              testId={`node-tech-${node.id}`}
            >
              <div className="flex flex-col gap-3 pb-2">
                <p className="text-body text-fg">
                  FFmpeg {shortFfmpegVersion(ffmpeg)}
                </p>
                <CodeBlock>{ffmpeg}</CodeBlock>
              </div>
            </Disclosure>
          </div>
        )}
      </Card>

      {confirming === 'stop' && (
        <ConfirmDialog
          title={`Stop ${node.name} now?`}
          confirmLabel="Stop node"
          cancelLabel="Keep running"
          busy={stop.isPending}
          onConfirm={() => stop.mutate({ id: node.id, drain: false })}
          onClose={() => setConfirming(null)}
          testId={`node-stop-dialog-${node.id}`}
          confirmTestId={`node-stop-confirm-${node.id}`}
          cancelTestId={`node-stop-cancel-${node.id}`}
        >
          {currentJob ? (
            <p>
              The file being worked on right now stops mid-way. Its unfinished
              temporary copy is thrown away, the job returns to the queue to try
              again later, and the original file is never touched.
            </p>
          ) : (
            <p>
              {node.name} stops where it is and takes no new work. Queued files
              wait until it is started again. Nothing is deleted.
            </p>
          )}
        </ConfirmDialog>
      )}
      {confirming === 'revoke' && (
        <ConfirmDialog
          title={`Revoke ${node.name}?`}
          confirmLabel="Revoke node"
          busy={revoke.isPending}
          onConfirm={() => revoke.mutate(node.id)}
          onClose={() => setConfirming(null)}
          testId={`node-revoke-dialog-${node.id}`}
          confirmTestId={`node-revoke-confirm-${node.id}`}
          cancelTestId={`node-revoke-cancel-${node.id}`}
        >
          <p>
            It stops taking work and must be paired again to come back. Jobs it
            holds go back to the queue.
          </p>
        </ConfirmDialog>
      )}
      {confirming === 'reject' && (
        <ConfirmDialog
          title={`Reject ${node.name}?`}
          confirmLabel="Reject node"
          busy={reject.isPending}
          onConfirm={() => reject.mutate(node.id)}
          onClose={() => setConfirming(null)}
          testId={`node-reject-dialog-${node.id}`}
          confirmTestId={`node-reject-confirm-${node.id}`}
          cancelTestId={`node-reject-cancel-${node.id}`}
        >
          <p>
            This removes {node.name} from this server. Its in-flight jobs return
            to the queue so another node can take them, and files on disk are
            never touched.
          </p>
        </ConfirmDialog>
      )}
    </li>
  )
}

export default function NodesPage() {
  useDocumentTitle(['Nodes'])
  const client = useQueryClient()
  // SSE (node.updated / job.updated) invalidates both caches; the 5s
  // refetch is the fallback for a missed or stalled event stream. Last
  // write wins: each refetch carries the server's current heartbeat_at.
  const nodes = useQuery({
    queryKey: ['nodes'],
    queryFn: getNodes,
    refetchInterval: 5000,
  })
  // Distinct key from the dashboard's jobs list (different params); the
  // SSE invalidation of ['jobs'] still covers it (prefix match).
  const jobs = useQuery({
    queryKey: ['jobs', 'running'],
    queryFn: () => getJobs({ status: 'running', limit: 100 }),
    refetchInterval: 5000,
  })
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<Node | null>(null)
  const toast = useToast()

  const save = useMutation({
    mutationFn: (input: {
      id: string
      body: Parameters<typeof updateNode>[1]
    }) => updateNode(input.id, input.body),
    onSuccess: () => {
      setEditing(null)
      toast.success('Node saved.')
      void client.invalidateQueries({ queryKey: ['nodes'] })
    },
    onError: (err: unknown) => toast.failure('Saving the node failed', err),
  })

  const items = nodes.data?.items ?? []
  // Pending first, then ready, then revoked; freshest first within a group.
  const order: Record<string, number> = { pending: 0, approved: 1, revoked: 2 }
  const sorted = [...items].sort(
    (a, b) =>
      (order[a.status] ?? 9) - (order[b.status] ?? 9) ||
      a.created_at.localeCompare(b.created_at),
  )
  const runningByNode = new Map<string, number>()
  for (const j of jobs.data?.items ?? []) {
    if (j.node_id) {
      runningByNode.set(j.node_id, (runningByNode.get(j.node_id) ?? 0) + 1)
    }
  }
  const approved = sorted.filter((n) => n.status === 'approved')

  return (
    <PageContainer data-testid="nodes-page">
      <PageHeader
        title="Nodes"
        subtitle="Machines that do the shrinking. This server is one of them."
        actions={
          <button
            type="button"
            onClick={() => setAdding(true)}
            className={btnPrimary}
            data-testid="nodes-add"
          >
            Add node
          </button>
        }
      />

      {approved.length > 0 && <EncodingStatus variant="panel" controls />}

      {nodes.isPending && (
        <>
          <SkeletonCard className="h-24" />
          <SkeletonCard className="h-24" />
        </>
      )}
      {nodes.isError && (
        <ErrorBox
          title="Cannot load the nodes."
          error={nodes.error}
          onRetry={() => void nodes.refetch()}
        />
      )}
      {!nodes.isPending && !nodes.isError && sorted.length === 0 && (
        <EmptyState
          icon="nodes"
          title="No other machines yet"
          body="Add a node to share the work with another computer."
          action={
            <button
              type="button"
              onClick={() => setAdding(true)}
              className={btnPrimary}
            >
              Add node
            </button>
          }
          testId="nodes-empty"
        />
      )}

      {sorted.length > 0 && (
        <ul className="grid gap-4 lg:grid-cols-2" data-testid="nodes-list">
          {sorted.map((node) => {
            const currentJob = jobs.data?.items.find(
              (j) => j.id === node.current_job_id,
            )
            return (
              <NodeRow
                key={node.id}
                node={node}
                runningJobs={runningByNode.get(node.id) ?? 0}
                currentJob={currentJob}
                onEdit={setEditing}
              />
            )
          })}
        </ul>
      )}

      {adding && <AddNodeDialog onClose={() => setAdding(false)} />}
      {editing && (
        <EditNodeDialog
          node={editing}
          busy={save.isPending}
          onSave={(body) => save.mutate({ id: editing.id, body })}
          onClose={() => setEditing(null)}
        />
      )}
    </PageContainer>
  )
}

/**
 * QSVFeatureStrip (task 35, restyled in task 48): what this machine's
 * capability probe actually found, in plain language, so a setting that had
 * no effect is explainable from the node card. One line per probed feature;
 * the unsupported wording says what the machine cannot do rather than naming
 * an ffmpeg flag.
 */
export function QSVFeatureStrip({
  id,
  features,
}: {
  id: string
  features: QSVFeatures
}) {
  const lines = [
    {
      key: 'look-ahead',
      label: 'Look-ahead',
      yes: 'supported — better quality at the same file size, a little slower (used for the H.264 format)',
      no: 'This machine cannot do look-ahead.',
    },
    {
      key: 'low-power',
      label: 'Low power',
      yes: 'supported — faster, slightly lower quality, less power',
      no: 'This machine cannot do the faster low-power mode.',
    },
    {
      key: 'async-depth',
      label: 'Async depth',
      yes: 'supported — more frames in flight, so it keeps up better',
      no: 'This machine cannot use the deeper multi-frame pipeline.',
    },
  ]
  return (
    <div
      className="rounded-control bg-canvas p-3 ring-1 ring-line"
      data-testid={`node-qsv-${id}`}
    >
      <p className="text-caption font-semibold text-fg">
        Intel Quick Sync on this machine
      </p>
      <ul className="mt-1 flex flex-col gap-1 text-caption text-fg-muted">
        {lines.map((line) => {
          const supported =
            line.key === 'look-ahead'
              ? features.look_ahead
              : line.key === 'low-power'
                ? features.low_power
                : features.async_depth
          return (
            <li key={line.key} data-testid={`node-qsv-${id}-${line.key}`}>
              <span className="text-fg">{line.label}</span> —{' '}
              {supported ? line.yes : line.no}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
