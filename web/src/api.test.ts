// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, getHealth } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(
  body: unknown,
  ok: boolean,
  status: number,
  statusText = '',
) {
  return {
    ok,
    status,
    statusText,
    json: async () => body,
    text: async () => JSON.stringify(body),
  }
}

describe('api client', () => {
  it('returns parsed JSON on 2xx', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse({ status: 'ok', version: 'v0.0.0' }, true, 200),
        ),
    )
    await expect(getHealth()).resolves.toEqual({
      status: 'ok',
      version: 'v0.0.0',
    })
  })

  it('throws ApiError with the API.md error convention on non-2xx', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse(
            { error: 'no such API endpoint', code: 'not_found' },
            false,
            404,
            'Not Found',
          ),
        ),
    )
    const err = await api('/api/v1/nope').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('not_found')
    expect((err as ApiError).status).toBe(404)
    expect((err as ApiError).message).toBe('no such API endpoint')
  })

  it('falls back to statusText and a generic code when the body is not JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Internal',
        json: async () => {
          throw new Error('not json')
        },
        text: async () => 'exploded in a non-JSON way',
      }),
    )
    const err = await api('/api/v1/boom').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('unknown')
    expect((err as ApiError).message).toBe('Internal')
  })

  it('keeps the raw body for the error surface disclosure (task 13)', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse({ error: 'nope', code: 'invalid_request' }, false, 400),
        ),
    )
    const err = (await api('/api/v1/nope').catch((e: unknown) => e)) as ApiError
    expect(err.raw).toBe('{"error":"nope","code":"invalid_request"}')
  })
})
