// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One API client for the whole UI. Wraps fetch, throws ApiError on any
 * non-2xx response using the error convention from API.md:
 * {"error": "human readable", "code": "snake_case_code"}.
 */

export interface Health {
  status: string
  version: string
}

interface ErrorBody {
  error?: string
  code?: string
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  /** The raw response body, shown behind the error surface's disclosure. */
  readonly raw?: string

  constructor(message: string, status: number, code: string, raw?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.raw = raw
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const raw = await response.text().catch(() => '')
    let body: ErrorBody | null = null
    try {
      body = raw ? (JSON.parse(raw) as ErrorBody) : null
    } catch {
      body = null
    }
    throw new ApiError(
      body?.error ?? response.statusText,
      response.status,
      body?.code ?? 'unknown',
      raw || undefined,
    )
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export function getHealth(): Promise<Health> {
  return api<Health>('/api/v1/health')
}

/** GET /setup/status — is setup done, is login on. */
export interface SetupStatus {
  setup_complete: boolean
  login_enabled: boolean
  username?: string
  /** Present in the POST /setup response for a join (task 08). */
  mode?: 'server' | 'join'
  server?: string
}

export function getSetupStatus(): Promise<SetupStatus> {
  return api<SetupStatus>('/api/v1/setup/status')
}

/** POST /setup — first-run setup. Only works while setup is incomplete. */
export interface SetupRequest {
  mode: 'server' | 'join'
  login_enabled?: boolean
  username?: string
  password?: string
  /** Join target and optional one-time pairing token (mode join). */
  server?: string
  token?: string
  /** The wizard's explicit "start the built-in node now" choice
   * (mode server, task 11). Default false: never start encoding because
   * a wizard was completed. */
  start_node?: boolean
}

export function completeSetup(body: SetupRequest): Promise<SetupStatus> {
  return api<SetupStatus>('/api/v1/setup', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** POST /auth/login — sets the sys_session cookie. */
export function login(body: {
  username: string
  password: string
}): Promise<SetupStatus> {
  return api<SetupStatus>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** POST /auth/logout — invalidates the session. */
export function logout(): Promise<void> {
  return api<void>('/api/v1/auth/logout', { method: 'POST' })
}

/** GET /auth/me — the account the route guard checks. */
export function getMe(): Promise<SetupStatus> {
  return api<SetupStatus>('/api/v1/auth/me')
}

/** GET /settings — settings values (no secrets). */
export interface Settings {
  worker_concurrency: number
  scratch_dir: string
  default_result_policy: string
  login_enabled: boolean
  admin_username?: string
  preferred_languages: string[]
  default_video_codec: string
}

export function getSettings(): Promise<Settings> {
  return api<Settings>('/api/v1/settings')
}

/** PUT /settings — settings values; password rules per API.md. */
export interface SettingsUpdate {
  worker_concurrency?: number
  scratch_dir?: string
  default_result_policy?: string
  preferred_languages?: string[]
  default_video_codec?: string
  login_enabled?: boolean
  admin_username?: string
  /** Initial password when enabling login, or a password change. */
  password?: string
  /** Required for a password change when login is enabled. */
  current_password?: string
}

export function updateSettings(body: SettingsUpdate): Promise<Settings> {
  return api<Settings>('/api/v1/settings', {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/**
 * Library, file and job resources (API.md tasks 03 and 07). Target is the
 * desired end state of a library; Easy is the easy-screen input that
 * compiled to it (null when the library has no easy representation).
 */
export interface Target {
  video: {
    codec: string
    quality: number
    reencode_if_same_codec: boolean
  }
  audio: {
    mode: string
    preferred_languages: string[]
    compact_codec: string
  }
  subtitles: {
    mode: string
    preferred_languages: string[]
    extract_removed: boolean
  }
  container: string
}

/** The easy-screen state: quality 1..5, audio 1..4. */
export interface EasyInput {
  quality: number
  remove_subtitles: boolean
  audio: number
}

export interface Library {
  id: string
  name: string
  path: string
  target: Target
  easy: EasyInput | null
  /** The canonical rule graph (task 14); null only on transport errors. */
  flow: FlowGraph | null
  /** Who owns the flow: easy, steps or advanced. */
  flow_source: string
  watch: boolean
  schedule: string
  result_policy: string
  created_at: string
  updated_at: string
}

export interface FileItem {
  id: string
  library_id: string
  path: string
  size: number
  mtime: number
  fingerprint: string
  probe: unknown
  status: string
  satisfied_reason: string
  last_job_id: string
  bytes_saved: number
  created_at: string
  updated_at: string
}

export interface Job {
  id: string
  file_id: string
  library_id: string
  node_id: string
  status: string
  priority: number
  attempt: number
  encoder_used: string
  fallback_reasons: string[]
  percent: number
  speed: number
  eta_seconds: number
  bytes_before: number
  bytes_after: number
  log: string
  error: string
  created_at: string
}

export interface ListResponse<T> {
  items: T[]
  total: number
}

/** Create/update body: exactly one of target and easy. */
export interface LibraryBody {
  name?: string
  path: string
  target?: Target
  easy?: EasyInput
  watch: boolean
  schedule?: string
  result_policy?: string
}

export function getLibraries(): Promise<ListResponse<Library>> {
  return api<ListResponse<Library>>('/api/v1/libraries')
}

export function createLibrary(body: LibraryBody): Promise<Library> {
  return api<Library>('/api/v1/libraries', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function getLibrary(id: string): Promise<Library> {
  return api<Library>(`/api/v1/libraries/${id}`)
}

export function updateLibrary(id: string, body: LibraryBody): Promise<Library> {
  return api<Library>(`/api/v1/libraries/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/** Deletes the library's records only; files on disk are untouched. */
export function deleteLibrary(id: string): Promise<void> {
  return api<void>(`/api/v1/libraries/${id}`, { method: 'DELETE' })
}

export function scanLibrary(id: string): Promise<{ started: boolean }> {
  return api<{ started: boolean }>(`/api/v1/libraries/${id}/scan`, {
    method: 'POST',
  })
}

export function getFiles(
  id: string,
  params: { status?: string; limit?: number; offset?: number } = {},
): Promise<ListResponse<FileItem>> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.limit) search.set('limit', String(params.limit))
  if (params.offset) search.set('offset', String(params.offset))
  const qs = search.toString()
  return api<ListResponse<FileItem>>(
    `/api/v1/libraries/${id}/files${qs ? `?${qs}` : ''}`,
  )
}

export function getJobs(
  params: { status?: string; order?: string; limit?: number } = {},
): Promise<ListResponse<Job>> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.order) search.set('order', params.order)
  if (params.limit) search.set('limit', String(params.limit))
  const qs = search.toString()
  return api<ListResponse<Job>>(`/api/v1/jobs${qs ? `?${qs}` : ''}`)
}

export function cancelJob(id: string): Promise<void> {
  return api<void>(`/api/v1/jobs/${id}/cancel`, { method: 'POST' })
}

export function retryJob(id: string): Promise<Job> {
  return api<Job>(`/api/v1/jobs/${id}/retry`, { method: 'POST' })
}

/** Per-status file counts and space saved, per library and in total. */
export interface LibraryStats {
  library_id: string
  name: string
  files: number
  new: number
  satisfied: number
  queued: number
  processing: number
  done: number
  failed: number
  missing: number
  bytes_saved: number
}

export interface Stats {
  files: number
  new: number
  satisfied: number
  queued: number
  processing: number
  done: number
  failed: number
  missing: number
  bytes_saved: number
  libraries: LibraryStats[]
}

export function getStats(): Promise<Stats> {
  return api<Stats>('/api/v1/stats')
}

export interface BrowseEntry {
  name: string
  path: string
  is_dir: boolean
}

export interface BrowseResponse {
  path: string
  parent: string
  entries: BrowseEntry[]
}

/** Server-side directory listing for the folder picker. */
export function browse(path = '/'): Promise<BrowseResponse> {
  return api<BrowseResponse>(
    `/api/v1/fs/browse?path=${encodeURIComponent(path)}`,
  )
}

/**
 * Nodes (task 08): registered worker machines, pairing tokens and the
 * discovered-server list for the setup wizard's join card.
 */
export interface NodeCapabilities {
  encoders?: { name: string; codecs: string[] }[]
  cpu_cores?: number
  ffmpeg_version?: string
}

export interface PathMapping {
  server_path: string
  node_path: string
}

export interface Node {
  id: string
  name: string
  os: string
  arch: string
  capabilities: NodeCapabilities | null
  access_mode: string
  path_mappings: PathMapping[] | null
  /** Transfer speed cap in MB/s; 0 = unlimited (task 09). */
  throughput_limit_mbps: number
  /** Operator intent: stopped | running | draining (task 11). */
  desired_state: string
  /** stopped | running | draining | error (mirrors desired_state or error). */
  effective_state: string
  /** Parallel-job cap, 1 to 16 (task 11). */
  concurrency: number
  /** Why a lifecycle transition failed to apply; empty when healthy. */
  state_error?: string
  /** The oldest claimed/running job on this node; absent when idle. */
  current_job_id?: string
  status: string
  enabled: boolean
  last_heartbeat_at?: string
  created_at: string
}

export function getNodes(): Promise<ListResponse<Node>> {
  return api<ListResponse<Node>>('/api/v1/nodes')
}

/** Node lifecycle controls (task 11): start, stop, drain, pause everything. */
export function startNode(id: string): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}/start`, { method: 'POST' })
}

export function stopNode(id: string, drain: boolean): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}/stop`, {
    method: 'POST',
    body: JSON.stringify({ drain }),
  })
}

export function drainNode(id: string): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}/drain`, { method: 'POST' })
}

export function pauseAllNodes(): Promise<{ paused: number }> {
  return api<{ paused: number }>('/api/v1/nodes/pause-all', { method: 'POST' })
}

export function resumeAllNodes(): Promise<{ resumed: number }> {
  return api<{ resumed: number }>('/api/v1/nodes/resume-all', {
    method: 'POST',
  })
}

/** The one-time token and the ready-to-copy install commands. */
export interface InstallCommands {
  docker: string
  shell: string
  powershell: string
}

export interface PairingToken {
  token: string
  expires_at: string
  commands: InstallCommands
}

export function createNodeToken(): Promise<PairingToken> {
  return api<PairingToken>('/api/v1/nodes/tokens', { method: 'POST' })
}

export function approveNode(id: string): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}/approve`, { method: 'POST' })
}

export function revokeNode(id: string): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}/revoke`, { method: 'POST' })
}

export function deleteNode(id: string): Promise<void> {
  return api<void>(`/api/v1/nodes/${id}`, { method: 'DELETE' })
}

export interface NodeUpdate {
  name?: string
  enabled?: boolean
  access_mode?: string
  path_mappings?: PathMapping[]
  /** Transfer speed cap in MB/s; 0 = unlimited. */
  throughput_limit_mbps?: number
  /** Parallel-job cap, 1 to 16 (task 11). Lifecycle moves only through
   * the start/stop/drain endpoints, never through this. */
  concurrency?: number
}

export function updateNode(id: string, body: NodeUpdate): Promise<Node> {
  return api<Node>(`/api/v1/nodes/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/** A server found on the LAN (setup wizard join card). */
export interface DiscoveredServer {
  id: string
  name: string
  url: string
  version: string
}

export function getSetupServers(): Promise<{ servers: DiscoveredServer[] }> {
  return api<{ servers: DiscoveredServer[] }>('/api/v1/setup/servers')
}

/**
 * Flow API (task 14). The graph is the canonical rule object every tier
 * generates and edits; the server owns validation, compilation and the
 * plain-language rendering, so the UI never hardcodes the step list twice.
 */
export interface FlowStep {
  id: string
  type: string
  params?: Record<string, unknown>
}

export interface FlowEdge {
  from: string
  to: string
  when?: string
}

export interface FlowGraph {
  schema: number
  nodes: FlowStep[]
  edges: FlowEdge[]
}

export interface FlowFieldError {
  path: string
  message: string
}

export interface FlowResponse {
  library_id: string
  source: string
  graph: FlowGraph
}

export function getLibraryFlow(id: string): Promise<FlowResponse> {
  return api<FlowResponse>(`/api/v1/libraries/${id}/flow`)
}

export function putLibraryFlow(
  id: string,
  body: { source: string; graph: FlowGraph },
): Promise<FlowResponse> {
  return api<FlowResponse>(`/api/v1/libraries/${id}/flow`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function validateFlow(
  graph: FlowGraph,
): Promise<{ valid: boolean; errors?: FlowFieldError[] }> {
  return api<{ valid: boolean; errors?: FlowFieldError[] }>(
    '/api/v1/flows/validate',
    { method: 'POST', body: JSON.stringify({ graph }) },
  )
}

/** One parameter schema entry of a step type. */
export interface StepParamField {
  name: string
  label: string
  kind: 'select' | 'number' | 'text' | 'languages' | 'toggle'
  options?: string[]
  min?: number
  max?: number
  required?: boolean
  default?: unknown
  help?: string
}

export interface StepTypeInfo {
  type: string
  label: string
  description: string
  condition: boolean
  params: StepParamField[]
}

export function getFlowStepTypes(): Promise<ListResponse<StepTypeInfo>> {
  return api<ListResponse<StepTypeInfo>>('/api/v1/flows/step-types')
}

export interface FlowPreviewStep {
  id: string
  type: string
  plain: string
}

export interface FlowPreviewResponse {
  graph: FlowGraph
  steps: FlowPreviewStep[]
}

/** POST /flows/preview: canonical graph + plain lines, nothing saved. */
export function previewFlow(body: {
  easy?: EasyInput
  graph?: FlowGraph
}): Promise<FlowPreviewResponse> {
  return api<FlowPreviewResponse>('/api/v1/flows/preview', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}
