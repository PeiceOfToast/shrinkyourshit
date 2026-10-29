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

  constructor(message: string, status: number, code: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorBody | null
    throw new ApiError(
      body?.error ?? response.statusText,
      response.status,
      body?.code ?? 'unknown',
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
}

export function getSetupStatus(): Promise<SetupStatus> {
  return api<SetupStatus>('/api/v1/setup/status')
}

/** POST /setup — first-run setup. Only works while setup is incomplete. */
export interface SetupRequest {
  mode: 'server'
  login_enabled: boolean
  username?: string
  password?: string
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
