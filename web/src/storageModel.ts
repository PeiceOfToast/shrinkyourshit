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
import { formatBytes } from './format'

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

/* ------------------------------------------------------------------ */
/* Browse and the library path picker (task 69, spec 4.6)              */
/* ------------------------------------------------------------------ */

/** True when `path` is the mountpoint or lies below it. */
export function isInsideStorage(path: string, mountpoint: string): boolean {
  return path === mountpoint || path.startsWith(`${mountpoint}/`)
}

/** The share whose mountpoint holds this path (the longest match wins). */
export function storageContaining(
  path: string,
  shares: Storage[],
): Storage | undefined {
  let best: Storage | undefined
  for (const share of shares) {
    if (!isInsideStorage(path, share.mountpoint)) continue
    if (!best || share.mountpoint.length > best.mountpoint.length) best = share
  }
  return best
}

/**
 * Breadcrumbs for a folder inside a share: the share's name first, then each
 * folder below its mountpoint. Breadcrumbs never climb above the mountpoint.
 */
export function storageCrumbs(
  path: string,
  share: Pick<Storage, 'name' | 'mountpoint'>,
): Array<{ name: string; path: string }> {
  const out = [{ name: share.name, path: share.mountpoint }]
  if (!isInsideStorage(path, share.mountpoint)) return out
  const rest = path.slice(share.mountpoint.length).split('/').filter(Boolean)
  let acc = share.mountpoint
  for (const part of rest) {
    acc += `/${part}`
    out.push({ name: part, path: acc })
  }
  return out
}

export type TypedPathCheck =
  { ok: true; path: string; storage: Storage } | { ok: false; message: string }

/**
 * A hand typed library path. It must be absolute, must not climb with `..`,
 * and must sit inside a share that is mounted. Anything else is refused with
 * the reason, because a library folder outside a mounted share reads as empty
 * when the share is gone.
 */
export function checkTypedPath(
  input: string,
  shares: Storage[],
): TypedPathCheck {
  const raw = input.trim()
  if (raw === '') {
    return { ok: false, message: 'Type the full folder path first.' }
  }
  if (!raw.startsWith('/')) {
    return {
      ok: false,
      message: 'A folder path starts with /, for example /storage/nas/Movies.',
    }
  }
  const segments = raw.split('/').filter(Boolean)
  if (segments.some((s) => s === '..' || s === '.')) {
    return {
      ok: false,
      message:
        'Remove the . and .. parts. Type the folder path as it is named.',
    }
  }
  const path = `/${segments.join('/')}`
  const share = storageContaining(path, shares)
  if (!share) {
    return {
      ok: false,
      message:
        'That path is not inside a mounted storage, so it is refused. Type a folder inside a storage, for example /storage/nas/Movies, or pick one with Choose.',
    }
  }
  if (!share.mounted) {
    return {
      ok: false,
      message: `${share.name} is not mounted, so this folder cannot be used yet. Mount it on the Storage page first.`,
    }
  }
  return { ok: true, path, storage: share }
}

/** The query key of one storage listing (the browser and the form share it). */
export function storageBrowseKey(id: string, path: string) {
  return ['storage-browse', id, path] as const
}

/** Free space in plain words, or '' when the probe gave no figure. */
export function freeSpaceText(
  free: number | null,
  total: number | null,
): string {
  if (free === null || total === null) return ''
  return `${formatBytes(free)} free of ${formatBytes(total)}`
}
