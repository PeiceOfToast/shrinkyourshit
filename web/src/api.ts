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
  /** Update settings (task 16); the About card edits the first two. */
  update_check: boolean
  update_channel: string
  backup_before_update: boolean
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
  /** Update settings (task 16; the About card edits them). */
  update_check?: boolean
  update_channel?: string
  backup_before_update?: boolean
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
  /** When a scan last finished without error (task 19); absent when none. */
  last_scan_at?: string
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
  /** Encoder of the file's most recent job (task 19); absent with none. */
  last_job_encoder?: string
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
  /** The live output size the worker reported (task 18). */
  bytes_so_far: number
  /** The file's basename and full path, resolved from file_id (task 18). */
  file_name: string
  file_path: string
  log: string
  error: string
  /** The structured log (task 27): row count and whether a viewer
   * has something to open. */
  log_lines: number
  has_log: boolean
  created_at: string
}

/** One structured job log row (task 27). */
export interface JobLogLine {
  id: number
  seq: number
  at: string
  level: 'info' | 'warn' | 'error'
  source: 'ffmpeg' | 'sys'
  message: string
  repeated: number
}

/** GET /jobs/{id}/log — one page plus the follow cursor (task 27). */
export interface JobLogResponse {
  lines: JobLogLine[]
  total: number
  has_more: boolean
  /** The job's current status: a viewer stops polling when it is terminal. */
  status: string
}

export interface JobLogParams {
  /** Only rows newer than this id (a running job is followed, not
   * re-read). */
  after_id?: number
  level?: 'info' | 'warn' | 'error'
  limit?: number
}

export function getJobLog(
  jobId: string,
  params: JobLogParams = {},
): Promise<JobLogResponse> {
  const search = new URLSearchParams()
  if (params.after_id !== undefined)
    search.set('after_id', String(params.after_id))
  if (params.level) search.set('level', params.level)
  if (params.limit) search.set('limit', String(params.limit))
  const qs = search.toString()
  return api<JobLogResponse>(`/api/v1/jobs/${jobId}/log${qs ? `?${qs}` : ''}`)
}

/** The plain-text download of one job's whole log (task 27). */
export function jobLogDownloadURL(jobId: string): string {
  return `/api/v1/jobs/${jobId}/log/download`
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

export interface FileListParams {
  status?: string
  /** Case-insensitive substring of the file's name; server side (task 19). */
  q?: string
  /** Sort key: name, size or saved; the server orders, never the browser. */
  sort?: 'name' | 'size' | 'saved'
  dir?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export function getFiles(
  id: string,
  params: FileListParams = {},
): Promise<ListResponse<FileItem>> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.q) search.set('q', params.q)
  if (params.sort) search.set('sort', params.sort)
  if (params.dir) search.set('dir', params.dir)
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

/** GET /files/{id}/jobs — one file's job history, oldest first (task 19). */
export function getFileJobs(
  fileId: string,
  params: { status?: string; limit?: number; offset?: number } = {},
): Promise<ListResponse<Job>> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.limit) search.set('limit', String(params.limit))
  if (params.offset) search.set('offset', String(params.offset))
  const qs = search.toString()
  return api<ListResponse<Job>>(
    `/api/v1/files/${fileId}/jobs${qs ? `?${qs}` : ''}`,
  )
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
  /** Finished-job totals (task 18): files done, bytes saved. */
  stats?: { files_done: number; bytes_saved: number }
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

/**
 * GET /api/v1/flows/step-types. The server marshals a step type without
 * parameters as `"params": null` (a nil Go slice), which crashed every
 * consumer that maps over `params` — the whole canvas page went blank in
 * a real browser (caught by the task 22 audit; the jsdom fixture always
 * used `[]`). Normalized here so every consumer sees an array.
 */
export function getFlowStepTypes(): Promise<ListResponse<StepTypeInfo>> {
  return api<ListResponse<StepTypeInfo>>('/api/v1/flows/step-types').then(
    (resp) => ({
      ...resp,
      items: resp.items.map((info) => ({
        ...info,
        params: info.params ?? [],
      })),
    }),
  )
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

/**
 * Version and in-app update (task 17). Checking is on by default and
 * nothing here applies an update: the apply call only ever follows an
 * explicit click in the About card.
 */
export interface VersionInfo {
  version: string
  commit: string
  date: string
  /** How this install runs: binary or container. */
  runtime: 'binary' | 'container'
  update_check_enabled: boolean
  channel: string
}

export function getVersion(): Promise<VersionInfo> {
  return api<VersionInfo>('/api/v1/version')
}

/** The stages the About card renders while an update applies. */
export interface UpdateProgress {
  active: boolean
  stage?: string
  error?: string
}

export interface UpdateStatus {
  current: string
  /** Empty until a check succeeded. */
  latest?: string
  update_available: boolean
  release_url?: string
  release_notes?: string
  published_at?: string
  self_update_supported: boolean
  /** Why the answer is "no": not checked yet, a quiet failure, the
   * container fix, or "already on the latest release". */
  reason?: string
  progress?: UpdateProgress
}

/** GET /update/status — the last check result, never a network call. */
export function getUpdateStatus(): Promise<UpdateStatus> {
  return api<UpdateStatus>('/api/v1/update/status')
}

/** POST /update/check — forces a refresh, rate limited to once a minute. */
export function checkForUpdate(): Promise<UpdateStatus> {
  return api<UpdateStatus>('/api/v1/update/check', { method: 'POST' })
}

/** POST /update/apply — starts a binary self update; 409 in a container. */
export function applyUpdate(): Promise<{ started: boolean }> {
  return api<{ started: boolean }>('/api/v1/update/apply', { method: 'POST' })
}

/** The integrations (task 23): media servers and download clients the
 * server notifies after a job ends. The API key never comes back out —
 * a response carries api_key_set instead. */
export interface Integration {
  id: string
  kind: 'jellyfin' | 'emby' | 'plex' | 'sonarr' | 'radarr'
  name: string
  base_url: string
  api_key_set: boolean
  enabled: boolean
  refresh_on_done: boolean
}

/** The create/update body. On update an empty api_key keeps the stored
 * key; the form's field shows (unchanged) for it. */
export interface IntegrationBody {
  kind: Integration['kind']
  name: string
  base_url: string
  api_key?: string
  enabled: boolean
  refresh_on_done: boolean
}

/** GET /integrations — every configured integration, oldest first. */
export function getIntegrations(): Promise<ListResponse<Integration>> {
  return api<ListResponse<Integration>>('/api/v1/integrations')
}

/** POST /integrations — create one; the key is stored, never echoed. */
export function createIntegration(body: IntegrationBody): Promise<Integration> {
  return api<Integration>('/api/v1/integrations', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** PUT /integrations/{id} — replace one; an empty api_key keeps the
 * stored key. */
export function updateIntegration(
  id: string,
  body: IntegrationBody,
): Promise<Integration> {
  return api<Integration>(`/api/v1/integrations/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/** DELETE /integrations/{id} — removes the integration and its key. */
export function deleteIntegration(id: string): Promise<void> {
  return api<void>(`/api/v1/integrations/${id}`, { method: 'DELETE' })
}

/** POST /integrations/{id}/test — the one-shot call, answered with what
 * the remote said. The HTTP status is always 200. */
export function testIntegration(id: string): Promise<IntegrationTestResult> {
  return api<IntegrationTestResult>(`/api/v1/integrations/${id}/test`, {
    method: 'POST',
  })
}

/** The Test button's answer: the remote's status and error text. */
export interface IntegrationTestResult {
  ok: boolean
  status: number
  error?: string
}

/** The webhook events (task 25). Documented in API.md and docs/openapi.yaml. */
export const WEBHOOK_EVENTS = [
  { id: 'job.done', label: 'A file is shrunk' },
  { id: 'job.failed', label: 'A file fails' },
  { id: 'library.scanned', label: 'A library is scanned' },
] as const

/** The events a webhook can subscribe to. */
export type WebhookEvent = (typeof WEBHOOK_EVENTS)[number]['id']

/** A webhook (task 25): the server posts event summaries to this URL. The
 * secret is generated by the server and returned exactly once, on create —
 * a response carries secret_set instead. */
export interface Webhook {
  id: string
  name: string
  url: string
  events: WebhookEvent[]
  enabled: boolean
  secret_set: boolean
  created_at: string
  updated_at: string
}

/** The create body. The secret is generated by the server; the create
 * response carries it. On update an empty secret keeps the stored one. */
export interface WebhookBody {
  name: string
  url: string
  events: WebhookEvent[]
  enabled: boolean
}

/** The create response: the normal shape plus the secret, once. */
export interface WebhookCreated extends Webhook {
  secret: string
}

/** GET /webhooks — every configured webhook, keyless. */
export function getWebhooks(): Promise<ListResponse<Webhook>> {
  return api<ListResponse<Webhook>>('/api/v1/webhooks')
}

/** POST /webhooks — create one; the secret comes back exactly once. */
export function createWebhook(body: WebhookBody): Promise<WebhookCreated> {
  return api<WebhookCreated>('/api/v1/webhooks', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** PUT /webhooks/{id} — replace one; an empty secret keeps the stored one. */
export function updateWebhook(id: string, body: WebhookBody): Promise<Webhook> {
  return api<Webhook>(`/api/v1/webhooks/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/** DELETE /webhooks/{id} — removes the webhook and its delivery history. */
export function deleteWebhook(id: string): Promise<void> {
  return api<void>(`/api/v1/webhooks/${id}`, { method: 'DELETE' })
}

/** POST /webhooks/{id}/test — one delivery with a fixed sample payload,
 * answered with what the remote said. The HTTP status is always 200. */
export function testWebhook(id: string): Promise<IntegrationTestResult> {
  return api<IntegrationTestResult>(`/api/v1/webhooks/${id}/test`, {
    method: 'POST',
  })
}

/** One recorded delivery outcome: status 0 means the request never got an
 * answer. */
export interface WebhookDelivery {
  id: string
  event: string
  status: number
  error?: string
  at: string
}

/** GET /webhooks/{id}/deliveries — the recorded outcomes, newest first. */
export function getWebhookDeliveries(
  id: string,
  limit?: number,
): Promise<ListResponse<WebhookDelivery>> {
  const query = limit ? `?limit=${limit}` : ''
  return api<ListResponse<WebhookDelivery>>(
    `/api/v1/webhooks/${id}/deliveries${query}`,
  )
}

/** The plugins (task 31): hook plugins installed from a git URL or a
 * directory. A plugin runs as the server user at the moment its manifest
 * declares. Installed plugins are disabled until enabled on purpose. */
export type PluginType = 'preflight' | 'postprocess' | 'notify'

export interface PluginSetting {
  key: string
  label: string
  required: boolean
}

export interface Plugin {
  id: string
  name: string
  version: string
  type: PluginType
  enabled: boolean
  source: string
  description?: string
  timeout_seconds: number
  env?: string[]
  settings?: PluginSetting[]
  values?: Record<string, string>
  manifest_error?: string
  installed_at: string
}

/** POST /plugins/install — install from a git URL (+ ref) or a path.
 * Exactly one source is sent. */
export function installPlugin(body: {
  git_url?: string
  ref?: string
  path?: string
}): Promise<Plugin> {
  return api<Plugin>('/api/v1/plugins/install', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** PUT /plugins/{id} — the enabled switch and/or new settings values. */
export function updatePlugin(
  id: string,
  body: {
    enabled?: boolean
    settings?: Record<string, string>
  },
): Promise<Plugin> {
  return api<Plugin>(`/api/v1/plugins/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/** The test run's answer: the captured output and the exit code, 200
 * even when the plugin failed. */
export interface PluginTestResult {
  ok: boolean
  skipped: boolean
  skip_reason?: string
  exit_code: number
  error?: string
  output?: string[]
}

/** POST /plugins/{id}/test — run once against a synthetic job. */
export function testPlugin(id: string): Promise<PluginTestResult> {
  return api<PluginTestResult>(`/api/v1/plugins/${id}/test`, {
    method: 'POST',
  })
}

/** DELETE /plugins/{id} — removes a disabled plugin's row and directory. */
export function deletePlugin(id: string): Promise<void> {
  return api<void>(`/api/v1/plugins/${id}`, { method: 'DELETE' })
}

/** GET /plugins — every installed plugin, oldest first. */
export function getPlugins(): Promise<ListResponse<Plugin>> {
  return api<ListResponse<Plugin>>('/api/v1/plugins')
}
