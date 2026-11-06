// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage form model (task 68), pure so it is unit-testable without React.
 * The form never holds a stored password: `password` is what the user typed
 * in this session, and `password_set` only says whether one is saved.
 */
import {
  ApiError,
  type Library,
  type Storage,
  type StorageBody,
  type StorageProtocol,
} from './api'

/** The spec 4.4 default for NFS, shown and editable on the form (task 65). */
export const NFS_DEFAULT_OPTIONS = 'vers=3,soft,timeo=50,retrans=2'

/** The spec 4.4 option allowlist, surfaced as a hint on the form. */
export const ALLOWED_OPTIONS = [
  'vers',
  'nfsvers',
  'soft',
  'hard',
  'timeo',
  'retrans',
  'port',
  'rw',
  'ro',
  'nosuid',
  'nodev',
  'noexec',
  'noatime',
  'actimeo',
  'uid',
  'gid',
  'file_mode',
  'dir_mode',
  'iocharset',
  'sec',
  '_netdev',
] as const

/** Shared by the page and the dashboard warning, so both read one cache entry. */
export const storagesKey = ['storages'] as const

export interface StorageForm {
  /** Empty for "add". */
  id: string
  name: string
  protocol: StorageProtocol
  source: string
  options: string
  username: string
  domain: string
  /** Typed in this session only. Never filled from a response. */
  password: string
  /** Whether the server holds a password. Drives the "(unchanged)" hint. */
  password_set: boolean
  read_only: boolean
  enabled: boolean
}

export function blankStorageForm(): StorageForm {
  return {
    id: '',
    name: '',
    protocol: 'nfs',
    source: '',
    options: NFS_DEFAULT_OPTIONS,
    username: '',
    domain: '',
    password: '',
    password_set: false,
    read_only: false,
    enabled: true,
  }
}

/** The form for "edit": every field from the row, the password blank. */
export function storageFormOf(row: Storage): StorageForm {
  return {
    id: row.id,
    name: row.name,
    protocol: row.protocol,
    source: row.source,
    options: row.options,
    username: row.username,
    domain: row.domain,
    password: '',
    password_set: row.password_set,
    read_only: row.read_only,
    enabled: row.enabled,
  }
}

/**
 * The body for create or update. SMB fields travel only for SMB, so a
 * switch to NFS cannot leave a stale password in the request.
 */
export function storageBodyOf(form: StorageForm): StorageBody {
  const smb = form.protocol === 'smb'
  return {
    name: form.name.trim(),
    protocol: form.protocol,
    source: form.source.trim(),
    options: form.options.trim(),
    username: smb ? form.username.trim() : '',
    domain: smb ? form.domain.trim() : '',
    password: smb ? form.password : '',
    read_only: form.read_only,
    enabled: form.enabled,
  }
}

/** The source placeholder for each protocol (the format the server checks). */
export function sourcePlaceholder(protocol: StorageProtocol): string {
  return protocol === 'smb'
    ? '//192.168.1.10/media'
    : '192.168.1.10:/export/media'
}

/**
 * Field errors from a 400 `invalid_request` body, keyed by the field path.
 * Any other error gives an empty map, so the toast carries the message.
 */
export function fieldErrorsOf(err: unknown): Record<string, string> {
  if (!(err instanceof ApiError) || !err.raw) return {}
  try {
    const body = JSON.parse(err.raw) as {
      errors?: { path?: unknown; message?: unknown }[]
    }
    const out: Record<string, string> = {}
    for (const e of body.errors ?? []) {
      if (typeof e.path === 'string' && typeof e.message === 'string') {
        out[e.path] ??= e.message
      }
    }
    return out
  } catch {
    return {}
  }
}

/**
 * How many libraries keep their folder inside this share. The storage
 * response does not carry this, so it is read from the library paths.
 */
export function librariesUsing(
  storage: Pick<Storage, 'mountpoint'>,
  libraries: Pick<Library, 'path'>[],
): number {
  const prefix = `${storage.mountpoint}/`
  return libraries.filter(
    (lib) => lib.path === storage.mountpoint || lib.path.startsWith(prefix),
  ).length
}

/** What deleting a share does to the libraries that point into it. */
export function removeText(count: number): string {
  if (count === 0) {
    return 'No library uses this share, so nothing else changes.'
  }
  const noun =
    count === 1 ? 'library keeps its folder' : 'libraries keep their folders'
  return `${count} ${noun} inside this share. Their settings stay, but they cannot reach their files until the share is added back.`
}

/** The row's one-word state, the badge text. */
export function storageState(row: Storage): {
  label: string
  tone: 'success' | 'warning' | 'danger' | 'neutral'
} {
  if (row.error) return { label: 'Error', tone: 'danger' }
  if (row.mounted) return { label: 'Mounted', tone: 'success' }
  if (row.enabled) return { label: 'Not mounted', tone: 'warning' }
  return { label: 'Off', tone: 'neutral' }
}
