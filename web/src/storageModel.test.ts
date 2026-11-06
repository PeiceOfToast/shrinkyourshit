// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import { ApiError, type Storage } from './api'
import {
  NFS_DEFAULT_OPTIONS,
  blankStorageForm,
  fieldErrorsOf,
  librariesUsing,
  removeText,
  storageBodyOf,
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
