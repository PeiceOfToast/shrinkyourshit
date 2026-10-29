// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Nodes page (task 08): every registered worker machine. Pending nodes sit
 * at the top with Approve/Reject; "Add node" mints a one-time pairing
 * token and shows the three ready-to-copy install commands; the edit
 * dialog covers name, on/off, access mode and path mappings.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  approveNode,
  createNodeToken,
  deleteNode,
  getJobs,
  getNodes,
  revokeNode,
  updateNode,
  type Node,
  type PathMapping,
} from './api'
import PageContainer from './PageContainer'

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
  const created = useMutation({ mutationFn: createNodeToken })
  const tok = created.data

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-label="Add a node"
      data-testid="add-node-dialog"
    >
      <div className="max-h-[90vh] w-full max-w-2xl overflow-auto rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
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
                Could not create a code: {created.error.message}
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
              The machine appears here as soon as it asks to join. Without a
              code it still can — you approve it below.
            </p>
          </div>
        )}
      </div>
    </div>
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
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-label={`Edit ${node.name}`}
      data-testid="edit-node-dialog"
    >
      <div className="max-h-[90vh] w-full max-w-xl overflow-auto rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="text-lg font-semibold">Edit node</h2>
        <label className="mt-4 grid gap-1">
          <span className="text-sm text-slate-300">Name</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="rounded-lg bg-slate-800 px-3 py-2"
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
            className="rounded-lg bg-slate-800 px-3 py-2"
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
            className="rounded-lg bg-slate-800 px-3 py-2"
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
                  onChange={(e) => setRow(i, 'server_path', e.target.value)}
                  className="min-w-0 flex-1 rounded-lg bg-slate-800 px-3 py-2 text-sm"
                  data-testid={`mapping-server-${i}`}
                />
                <span className="text-slate-500">→</span>
                <input
                  type="text"
                  value={row.node_path}
                  placeholder="path on this node"
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
              setMappings((rows) => [
                ...rows,
                { server_path: '', node_path: '' },
              ])
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
                  (m) =>
                    m.server_path.trim() !== '' && m.node_path.trim() !== '',
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
      </div>
    </div>
  )
}

function NodeRow({
  node,
  runningJobs,
  onEdit,
}: {
  node: Node
  runningJobs: number
  onEdit: (node: Node) => void
}) {
  const client = useQueryClient()
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['nodes'] })
  }
  const approve = useMutation({ mutationFn: approveNode, onSuccess: refresh })
  const revoke = useMutation({ mutationFn: revokeNode, onSuccess: refresh })
  const reject = useMutation({ mutationFn: deleteNode, onSuccess: refresh })
  const encoders = node.capabilities?.encoders ?? []

  return (
    <li
      className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
      data-testid={`node-${node.id}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold">{node.name}</span>
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusStyles[node.status] ?? 'bg-slate-700 text-slate-200'}`}
          data-testid={`node-status-${node.id}`}
        >
          {statusLabels[node.status] ?? node.status}
        </span>
        {!node.enabled && node.status === 'approved' && (
          <span className="rounded-full bg-slate-700 px-2 py-0.5 text-xs text-slate-300">
            paused
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
                onClick={() => reject.mutate(node.id)}
                disabled={reject.isPending}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-slate-800 disabled:opacity-50"
                data-testid={`node-reject-${node.id}`}
              >
                Reject
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
      <p className="mt-1 text-sm text-slate-500">
        {node.os || '?'}/{node.arch || '?'} · last check-in{' '}
        {timeAgo(node.last_heartbeat_at)}
        {node.status === 'pending' && ' · asked to join just now'}
      </p>
      {encoders.length > 0 && (
        <div
          className="mt-2 flex flex-wrap gap-1"
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
        </div>
      )}
    </li>
  )
}

export default function NodesPage() {
  const client = useQueryClient()
  const nodes = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  // Distinct key from the dashboard's jobs list (different params); the
  // SSE invalidation of ['jobs'] still covers it (prefix match).
  const jobs = useQuery({
    queryKey: ['jobs', 'running'],
    queryFn: () => getJobs({ status: 'running', limit: 100 }),
  })
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<Node | null>(null)

  const save = useMutation({
    mutationFn: (input: {
      id: string
      body: Parameters<typeof updateNode>[1]
    }) => updateNode(input.id, input.body),
    onSuccess: () => {
      setEditing(null)
      void client.invalidateQueries({ queryKey: ['nodes'] })
    },
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

  return (
    <PageContainer className="space-y-4" data-testid="nodes-page">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Nodes</h1>
        <button
          type="button"
          onClick={() => setAdding(true)}
          className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
          data-testid="nodes-add"
        >
          Add node
        </button>
      </div>

      {nodes.isPending && <p className="text-slate-400">Loading nodes…</p>}
      {nodes.isError && <p className="text-red-400">Cannot load the nodes.</p>}
      {!nodes.isPending && !nodes.isError && sorted.length === 0 && (
        <p className="text-slate-400" data-testid="nodes-empty">
          No other machines yet. “Add node” shows how to join one.
        </p>
      )}

      <ul className="space-y-3" data-testid="nodes-list">
        {sorted.map((node) => (
          <NodeRow
            key={node.id}
            node={node}
            runningJobs={runningByNode.get(node.id) ?? 0}
            onEdit={setEditing}
          />
        ))}
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
