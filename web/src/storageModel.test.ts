// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import { ApiError, type Storage } from './api'
import {
  NFS_DEFAULT_OPTIONS,
  blankStorageForm,
  checkTypedPath,
  fieldErrorsOf,
  freeSpaceText,
  librariesUsing,
  removeText,
  storageBodyOf,
  storageContaining,
  storageCrumbs,
  storageFormOf,
  storageState,
} from './storageModel'

function row(overrides: Partial<Storage> = {}): Storage {
  return {
    id: 'st-1',
    name: 'nas',
    protocol: 'nfs',
    source: '192.168.1.10:/export/media',
    options: NFS_DEFAULT_OPTIONS,
    username: '',
    domain: '',
    password_set: false,
    read_only: false,
    enabled: true,
    mountpoint: '/storage/nas',
    mounted: false,
    error: '',
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    ...overrides,
  }
}

describe('blankStorageForm', () => {
  it('starts NFS with the spec 4.4 default options, enabled, writable', () => {
    const form = blankStorageForm()
    expect(form.protocol).toBe('nfs')
    expect(form.options).toBe('vers=3,soft,timeo=50,retrans=2')
    expect(form.enabled).toBe(true)
    expect(form.read_only).toBe(false)
    expect(form.password).toBe('')
  })
})

describe('storageFormOf', () => {
  it('never copies a password into the form, only whether one is set', () => {
    const form = storageFormOf(row({ protocol: 'smb', password_set: true }))
    expect(form.password).toBe('')
    expect(form.password_set).toBe(true)
  })
})

describe('storageBodyOf', () => {
  it('sends SMB fields only for SMB, so a switch to NFS leaves no stale password', () => {
    const form = {
      ...blankStorageForm(),
      name: ' movies ',
      source: ' 192.168.1.10:/export/movies ',
      username: 'media',
      domain: 'LAN',
      password: 'secret',
    }
    const body = storageBodyOf(form)
    expect(body).toEqual({
      name: 'movies',
      protocol: 'nfs',
      source: '192.168.1.10:/export/movies',
      options: NFS_DEFAULT_OPTIONS,
      username: '',
      domain: '',
      password: '',
      read_only: false,
      enabled: true,
    })
  })

  it('keeps the SMB credentials for an SMB share', () => {
    const body = storageBodyOf({
      ...blankStorageForm(),
      protocol: 'smb',
      username: 'media',
      domain: 'LAN',
      password: 'pw',
    })
    expect(body).toMatchObject({
      username: 'media',
      domain: 'LAN',
      password: 'pw',
    })
  })
})

describe('fieldErrorsOf', () => {
  it('maps a 400 invalid_request to its field paths', () => {
    const err = new ApiError(
      'the storage settings are not valid',
      400,
      'invalid_request',
      JSON.stringify({
        error: 'the storage settings are not valid',
        code: 'invalid_request',
        errors: [
          { path: 'options', message: 'unknown option "foo"' },
          { path: 'options', message: 'second message ignored' },
          { path: 'source', message: 'bad source' },
        ],
      }),
    )
    expect(fieldErrorsOf(err)).toEqual({
      options: 'unknown option "foo"',
      source: 'bad source',
    })
  })

  it('returns nothing for a non-API error or a body that is not JSON', () => {
    expect(fieldErrorsOf(new Error('network down'))).toEqual({})
    expect(
      fieldErrorsOf(new ApiError('x', 500, 'internal', 'not json')),
    ).toEqual({})
    expect(fieldErrorsOf(new ApiError('x', 500, 'internal'))).toEqual({})
  })
})

describe('librariesUsing', () => {
  it('counts libraries inside the share, not ones on a similar prefix', () => {
    const libs = [
      { path: '/storage/nas' },
      { path: '/storage/nas/Movies' },
      { path: '/storage/nas2/Movies' },
      { path: '/media/local' },
    ]
    expect(librariesUsing({ mountpoint: '/storage/nas' }, libs)).toBe(2)
    expect(librariesUsing({ mountpoint: '/storage/nas2' }, libs)).toBe(1)
    expect(librariesUsing({ mountpoint: '/storage/other' }, libs)).toBe(0)
  })
})

describe('removeText', () => {
  it('says nothing changes when no library uses the share', () => {
    expect(removeText(0)).toBe(
      'No library uses this share, so nothing else changes.',
    )
  })

  it('names the libraries and what they keep', () => {
    expect(removeText(1)).toContain(
      '1 library keeps its folder inside this share',
    )
    expect(removeText(3)).toContain(
      '3 libraries keep their folders inside this share',
    )
  })
})

describe('storageState', () => {
  it('reads the error first, then mounted, then enabled', () => {
    expect(storageState(row({ error: 'denied', mounted: true })).label).toBe(
      'Error',
    )
    expect(storageState(row({ mounted: true })).label).toBe('Mounted')
    expect(storageState(row({ enabled: true })).label).toBe('Not mounted')
    expect(storageState(row({ enabled: false })).label).toBe('Off')
  })
})

describe('library path picker helpers (task 69)', () => {
  const nas = row({
    id: 'st-nas',
    name: 'nas',
    mountpoint: '/storage/nas',
    mounted: true,
  })
  const off = row({
    id: 'st-off',
    name: 'off',
    mountpoint: '/storage/off',
    mounted: false,
  })

  it('finds the share that holds a path, longest mountpoint winning', () => {
    const inner = row({
      id: 'st-in',
      name: 'nas/movies',
      mountpoint: '/storage/nas/movies',
    })
    expect(storageContaining('/storage/nas/TV', [nas, off])?.id).toBe('st-nas')
    expect(
      storageContaining('/storage/nas/movies/2026', [nas, inner])?.id,
    ).toBe('st-in')
    expect(storageContaining('/storage/nas2/x', [nas])).toBeUndefined()
    expect(storageContaining('/media/movies', [nas])).toBeUndefined()
  })

  it('keeps breadcrumbs inside the share, with the share name first', () => {
    expect(storageCrumbs('/storage/nas/Movies/2026', nas)).toEqual([
      { name: 'nas', path: '/storage/nas' },
      { name: 'Movies', path: '/storage/nas/Movies' },
      { name: '2026', path: '/storage/nas/Movies/2026' },
    ])
    expect(storageCrumbs('/storage/nas', nas)).toEqual([
      { name: 'nas', path: '/storage/nas' },
    ])
  })

  it('accepts a typed path inside a mounted share', () => {
    const r = checkTypedPath('/storage/nas/Movies/', [nas, off])
    expect(r).toEqual({ ok: true, path: '/storage/nas/Movies', storage: nas })
  })

  it('refuses a typed path outside every mounted share, with the reason', () => {
    for (const typed of [
      '/media/movies',
      '/storage/nas2/x',
      '/storage/nasty',
    ]) {
      const r = checkTypedPath(typed, [nas, off])
      expect(r.ok).toBe(false)
      if (!r.ok) expect(r.message).toMatch(/not inside a mounted storage/)
    }
  })

  it('refuses a typed path inside a share that is not mounted', () => {
    const r = checkTypedPath('/storage/off/Movies', [nas, off])
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toMatch(/off is not mounted/)
  })

  it('refuses relative paths and dot dot walks', () => {
    expect(checkTypedPath('storage/nas', [nas]).ok).toBe(false)
    expect(checkTypedPath('/storage/nas/../nas2', [nas]).ok).toBe(false)
    expect(checkTypedPath('   ', [nas]).ok).toBe(false)
  })

  it('states free space only when both figures are known', () => {
    expect(freeSpaceText(2_000_000_000, 4_000_000_000)).toBe(
      '2.0 GB free of 4.0 GB',
    )
    expect(freeSpaceText(null, 4_000_000_000)).toBe('')
  })
})
