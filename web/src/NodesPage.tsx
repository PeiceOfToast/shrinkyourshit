// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Nodes page (task 08): every registered worker machine. Pending nodes sit
 * at the top with Approve/Reject; "Add node" mints a one-time pairing
 * token and shows the three ready-to-copy install commands; the edit
 * dialog covers name, on/off, access mode and path mappings.
 *
 * Task 13: every mutation toasts, destructive actions confirm first with
 * dialog text that says what happens (rejecting a node, stopping one mid
 * job), and every dialog is a native <dialog> — Escape closes it and
 * focus returns to the trigger.
 *
 * Task 18: a real on/off switch per card (off stops gracefully — the
 * current file finishes; "Stop now" is the hard stop behind a confirm),
 * one condensed live line for the file being encoded, per node totals in
 * the footer, and page-level Pause all / Resume all in the header.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  approveNode,
  createNodeToken,
  deleteNode,
  getJobs,
  getNodes,
  pauseAllNodes,
  resumeAllNodes,
  revokeNode,
  startNode,
  stopNode,
  updateNode,
  type Job,
  type Node,
  type PathMapping,
} from './api'
import ConfirmDialog from './ConfirmDialog'
import ModalDialog from './dialog'
import NodeJobLine from './NodeJobLine'
import PageContainer from './PageContainer'
import { SkeletonCard } from './skeleton'
import Switch from './Switch'
import { useToast } from './toastContext'
import ErrorBox from './ErrorBox'
import { formatBytes } from './easyModel'

const statusStyles: Record<string, string> = {
  pending: 'bg-amber-500/20 text-amber-300',
  approved: 'bg-emerald-500/20 text-emerald-300',
  revoked: 'bg-red-500/20 text-red-300',
}

const statusLabels: Record<string, string> = {
  pending: 'Waiting for approval',
  approved: 'Ready',
  revoked: 'Revoked',
}

/** Lifecycle pill (task 11): the state the operator drives. */
const lifecycleStyles: Record<string, string> = {
  running: 'bg-emerald-500/20 text-emerald-300',
  stopped: 'bg-slate-700 text-slate-300',
  draining: 'bg-amber-500/20 text-amber-300',
  error: 'bg-red-500/20 text-red-300',
}

const lifecycleLabels: Record<string, string> = {
  running: 'Running',
  stopped: 'Stopped',
  draining: 'Draining',
  error: 'Error',
}

const accessModeLabels: Record<string, string> = {
  auto: 'Find the best way (automatic)',
  shared: 'Reads the folders directly',
  transfer: 'Files are sent over the network',
}

function timeAgo(iso?: string): string {
  if (!iso) return 'never'
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return 'never'
  const s = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

/** Copy-to-clipboard button with a short "copied" confirmation. */
function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      className="shrink-0 rounded-lg bg-slate-700 px-3 py-1.5 text-xs font-semibold text-slate-100 hover:bg-slate-600"
      data-testid={`copy-${label}`}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text)
        } catch {
          // Clipboard can be blocked (http, permissions); select nothing,
          // the command is visible for manual copying either way.
        }
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1500)
      }}
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
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
      className="m-auto max-h-[90vh] w-full max-w-2xl overflow-auto rounded-2xl bg-slate-900 p-6 text-slate-100 shadow-xl ring-1 ring-slate-800"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Add a node</h2>
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800"
          data-testid="add-node-close"
        >
          Close
        </button>
      </div>

      {!tok ? (
        <div className="mt-4">
          <p className="text-sm text-slate-400">
            A one-time code lets one machine join this server without waiting
            for approval. It is valid for 24 hours.
          </p>
          <button
            type="button"
            onClick={() => created.mutate()}
            disabled={created.isPending}
            className="mt-3 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
            data-testid="add-node-generate"
          >
            {created.isPending ? 'Generating…' : 'Generate install commands'}
          </button>
          {created.isError && (
            <p className="mt-2 text-sm text-red-400">
              Could not create a code: {(created.error as Error).message}
            </p>
          )}
        </div>
      ) : (
        <div className="mt-4 space-y-4" data-testid="add-node-commands">
          <p className="text-sm text-slate-400">
            Run one of these on the other machine. The code works once and
            expires {new Date(tok.expires_at).toLocaleString()}.
          </p>
          <div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Docker
              </span>
              <CopyButton text={tok.commands.docker} label="docker" />
            </div>
            <pre className="mt-1 overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-200">
              {tok.commands.docker}
            </pre>
          </div>
          <div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Linux / macOS
              </span>
              <CopyButton text={tok.commands.shell} label="shell" />
            </div>
            <pre className="mt-1 overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-200">
              {tok.commands.shell}
            </pre>
          </div>
          <div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Windows (PowerShell)
              </span>
              <CopyButton text={tok.commands.powershell} label="powershell" />
            </div>
            <pre className="mt-1 overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-200">
              {tok.commands.powershell}
            </pre>
          </div>
          <p className="text-xs text-slate-500">
            The machine appears here as soon as it asks to join. Without a code
            it still can — you approve it below.
          </p>
        </div>
      )}
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
      className="m-auto max-h-[90vh] w-full max-w-xl overflow-auto rounded-2xl bg-slate-900 p-6 text-slate-100 shadow-xl ring-1 ring-slate-800"
    >
      <h2 className="text-lg font-semibold">Edit node</h2>
      <label className="mt-4 grid gap-1">
        <span className="text-sm text-slate-300">Name</span>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded-lg bg-slate-800 px-3 py-2"
          data-testid="edit-node-name"
        />
      </label>
      <label className="mt-3 flex items-center gap-3">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          data-testid="edit-node-enabled"
        />
        <span>This node takes on work</span>
      </label>
      <label className="mt-3 grid gap-1">
        <span className="text-sm text-slate-300">How it reaches files</span>
        <select
          value={mode}
          onChange={(e) => setMode(e.target.value)}
          className="w-full rounded-lg bg-slate-800 px-3 py-2"
          data-testid="edit-node-access-mode"
        >
          {Object.entries(accessModeLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>

      <label className="mt-3 grid gap-1">
        <span className="text-sm text-slate-300">Transfer speed limit</span>
        <span className="text-xs text-slate-500">
          Only used when files are sent over the network. 0 = no limit.
        </span>
        <input
          type="number"
          min={0}
          value={limit}
          onChange={(e) => setLimit(e.target.value)}
          className="w-full rounded-lg bg-slate-800 px-3 py-2"
          data-testid="edit-node-throughput"
        />
      </label>

      <div className="mt-4">
        <span className="text-sm text-slate-300">Folder paths</span>
        <p className="mt-1 text-xs text-slate-500">
          If this machine sees the same folders under different names (a NAS
          mount), map them here: the path on the server and the path on this
          node.
        </p>
        <div className="mt-2 space-y-2">
          {mappings.map((row, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                type="text"
                value={row.server_path}
                placeholder="/media on the server"
                aria-label={`Path on the server, mapping ${i + 1}`}
                onChange={(e) => setRow(i, 'server_path', e.target.value)}
                className="min-w-0 flex-1 rounded-lg bg-slate-800 px-3 py-2 text-sm"
                data-testid={`mapping-server-${i}`}
              />
              <span className="text-slate-500">→</span>
              <input
                type="text"
                value={row.node_path}
                placeholder="path on this node"
                aria-label={`Path on this node, mapping ${i + 1}`}
                onChange={(e) => setRow(i, 'node_path', e.target.value)}
                className="min-w-0 flex-1 rounded-lg bg-slate-800 px-3 py-2 text-sm"
                data-testid={`mapping-node-${i}`}
              />
              <button
                type="button"
                onClick={() =>
                  setMappings((rows) => rows.filter((_, j) => j !== i))
                }
                className="rounded-lg px-2 py-1 text-sm text-red-400 hover:bg-slate-800"
                aria-label={`Remove mapping ${i + 1}`}
              >
                ✕
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={() =>
            setMappings((rows) => [...rows, { server_path: '', node_path: '' }])
          }
          className="mt-2 rounded-lg bg-slate-800 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700"
          data-testid="add-mapping"
        >
          Add a mapping
        </button>
      </div>

      <div className="mt-6 flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"
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
          className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
          data-testid="edit-node-save"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
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
  const [confirming, setConfirming] = useState<null | 'stop' | 'reject'>(null)
  const encoders = node.capabilities?.encoders ?? []
  const ffmpeg = node.capabilities?.ffmpeg_version
  const running = node.desired_state === 'running'

  return (
    <li
      className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
      data-testid={`node-${node.id}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 break-words font-semibold">{node.name}</span>
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusStyles[node.status] ?? 'bg-slate-700 text-slate-200'}`}
          data-testid={`node-status-${node.id}`}
        >
          {statusLabels[node.status] ?? node.status}
        </span>
        {node.status === 'approved' && (
          <span
            className={`rounded-full px-2 py-0.5 text-xs font-semibold ${lifecycleStyles[node.effective_state] ?? 'bg-slate-700 text-slate-200'}`}
            data-testid={`node-lifecycle-${node.id}`}
          >
            {lifecycleLabels[node.effective_state] ?? node.effective_state}
          </span>
        )}
        {node.state_error && (
          <span
            className="text-xs text-red-400"
            data-testid={`node-state-error-${node.id}`}
          >
            {node.state_error}
          </span>
        )}
        {runningJobs > 0 && (
          <span className="rounded-full bg-sky-500/20 px-2 py-0.5 text-xs text-sky-300">
            working on {runningJobs} file{runningJobs > 1 ? 's' : ''}
          </span>
        )}
        <span className="ml-auto flex gap-2">
          {node.status === 'pending' && (
            <>
              <button
                type="button"
                onClick={() => approve.mutate(node.id)}
                disabled={approve.isPending}
                className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
                data-testid={`node-approve-${node.id}`}
              >
                {approve.isPending ? 'Approving…' : 'Approve'}
              </button>
              <button
                type="button"
                onClick={() => setConfirming('reject')}
                disabled={reject.isPending}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-slate-800 disabled:opacity-50"
                data-testid={`node-reject-${node.id}`}
              >
                {reject.isPending ? 'Rejecting…' : 'Reject'}
              </button>
            </>
          )}
          {node.status === 'approved' && (
            <>
              <button
                type="button"
                onClick={onEdit ? () => onEdit(node) : undefined}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-slate-200 hover:bg-slate-800"
                data-testid={`node-edit-${node.id}`}
              >
                Edit
              </button>
              <button
                type="button"
                onClick={() => revoke.mutate(node.id)}
                disabled={revoke.isPending}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-slate-800 disabled:opacity-50"
                data-testid={`node-revoke-${node.id}`}
              >
                {revoke.isPending ? 'Revoking…' : 'Revoke'}
              </button>
            </>
          )}
        </span>
      </div>

      {node.status === 'approved' && (
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <Switch
            checked={running}
            onChange={(on) =>
              on
                ? start.mutate(node.id)
                : stop.mutate({ id: node.id, drain: true })
            }
            label={`Encoding on ${node.name}`}
            testId={`node-switch-${node.id}`}
            disabled={start.isPending || stop.isPending}
          />
          {(node.desired_state === 'running' ||
            node.desired_state === 'draining') && (
            <button
              type="button"
              onClick={() => setConfirming('stop')}
              disabled={stop.isPending}
              className="rounded-lg px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-slate-800 disabled:opacity-50"
              data-testid={`node-stop-now-${node.id}`}
            >
              {stop.isPending ? 'Stopping…' : 'Stop now'}
            </button>
          )}
        </div>
      )}

      {currentJob && node.status === 'approved' && (
        <div data-testid={`node-current-job-${node.id}`}>
          <NodeJobLine job={currentJob} />
        </div>
      )}

      <p className="mt-1 text-sm text-slate-500">
        {node.os || '?'}/{node.arch || '?'} · last check-in{' '}
        {timeAgo(node.last_heartbeat_at)}
        {node.status === 'pending' && ' · asked to join just now'}
      </p>
      {encoders.length > 0 && (
        <div
          className="mt-2 flex flex-wrap items-center gap-1"
          data-testid={`node-encoders-${node.id}`}
        >
          {encoders.map((e) => (
            <span
              key={e.name}
              className="rounded-md bg-slate-800 px-2 py-0.5 text-xs text-slate-300"
            >
              {e.name}
            </span>
          ))}
          {ffmpeg && <span className="text-xs text-slate-500">{ffmpeg}</span>}
        </div>
      )}

      {/* Footer (task 18): this node's own totals, and its parallel-job cap. */}
      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-slate-800 pt-3 text-xs text-slate-400">
        <span data-testid={`node-stats-${node.id}`}>
          {node.stats?.files_done ?? 0}{' '}
          {(node.stats?.files_done ?? 0) === 1 ? 'file' : 'files'},{' '}
          {formatBytes(node.stats?.bytes_saved ?? 0)} saved
        </span>
        {node.status === 'approved' && (
          <span
            className="flex items-center gap-1"
            data-testid={`node-concurrency-${node.id}`}
          >
            Parallel jobs
            <button
              type="button"
              onClick={() =>
                setConcurrency.mutate({
                  id: node.id,
                  concurrency: Math.max(1, node.concurrency - 1),
                })
              }
              disabled={setConcurrency.isPending || node.concurrency <= 1}
              className="rounded-md bg-slate-800 px-2 py-0.5 font-semibold text-slate-200 hover:bg-slate-700 disabled:opacity-40"
              aria-label="Fewer parallel jobs"
              data-testid={`node-concurrency-down-${node.id}`}
            >
              −
            </button>
            <span className="min-w-[1.5rem] text-center font-semibold text-slate-200">
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
              disabled={setConcurrency.isPending || node.concurrency >= 16}
              className="rounded-md bg-slate-800 px-2 py-0.5 font-semibold text-slate-200 hover:bg-slate-700 disabled:opacity-40"
              aria-label="More parallel jobs"
              data-testid={`node-concurrency-up-${node.id}`}
            >
              +
            </button>
          </span>
        )}
      </div>

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
  const client = useQueryClient()
  const toast = useToast()
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

  const refresh = () => void client.invalidateQueries({ queryKey: ['nodes'] })
  const pauseAll = useMutation({
    mutationFn: pauseAllNodes,
    onSuccess: (r) => {
      toast.success(
        r.paused > 0
          ? `Paused ${r.paused} node${r.paused === 1 ? '' : 's'}. Running files stop; the queue waits.`
          : 'Everything was already paused.',
      )
      refresh()
    },
    onError: (err: unknown) => toast.failure('Pausing all nodes failed', err),
  })
  const resumeAll = useMutation({
    mutationFn: resumeAllNodes,
    onSuccess: (r) => {
      toast.success(
        r.resumed > 0
          ? `Resumed ${r.resumed} node${r.resumed === 1 ? '' : 's'}.`
          : 'There is nothing remembered to resume.',
      )
      refresh()
    },
    onError: (err: unknown) => toast.failure('Resuming all nodes failed', err),
  })

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
  // Page-level controls (task 18). The switch is per node; pause-all is
  // page level. Disabled when they would be a no-op: pausing needs
  // something running or draining, resuming needs every node stopped —
  // the state a pause leaves behind.
  const anythingRunning = approved.some(
    (n) => n.desired_state === 'running' || n.desired_state === 'draining',
  )
  const allStopped =
    approved.length > 0 && approved.every((n) => n.desired_state === 'stopped')

  return (
    <PageContainer className="space-y-4" data-testid="nodes-page">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-bold">Nodes</h1>
        <div className="flex flex-wrap items-center gap-2">
          {approved.length > 0 && (
            <>
              <button
                type="button"
                onClick={() => pauseAll.mutate()}
                disabled={!anythingRunning || pauseAll.isPending}
                title={
                  anythingRunning ? undefined : 'Nothing is running right now'
                }
                className="rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold text-slate-100 hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40"
                data-testid="nodes-pause-all"
              >
                {pauseAll.isPending ? 'Pausing…' : 'Pause all'}
              </button>
              <button
                type="button"
                onClick={() => resumeAll.mutate()}
                disabled={!allStopped || resumeAll.isPending}
                title={allStopped ? undefined : 'Stop every node first'}
                className="rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold text-slate-100 hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40"
                data-testid="nodes-resume-all"
              >
                {resumeAll.isPending ? 'Resuming…' : 'Resume all'}
              </button>
            </>
          )}
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
            data-testid="nodes-add"
          >
            Add node
          </button>
        </div>
      </div>

      {nodes.isPending && (
        <>
          <SkeletonCard className="h-24" />
          <SkeletonCard className="h-24" />
        </>
      )}
      {nodes.isError && (
        <ErrorBox title="Cannot load the nodes." error={nodes.error} />
      )}
      {!nodes.isPending && !nodes.isError && sorted.length === 0 && (
        <p className="text-slate-400" data-testid="nodes-empty">
          No other machines yet. “Add node” shows how to join one.
        </p>
      )}

      <ul className="space-y-3" data-testid="nodes-list">
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
