// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Test helpers for the browser-level tests: a fetch stub that answers from
 * a small route table (no MSW dependency — vitest stubs only) and a fake
 * EventSource that tests can drive manually to simulate SSE.
 */
import { vi } from 'vitest'

export interface CapturedRequest {
  method: string
  url: string
  body: unknown
}

export type BodyFn = (ctx: {
  url: URL
  body: unknown
  request: CapturedRequest
}) => unknown

export interface MockRoute {
  method: string
  /** Matched against the pathname (no query string). */
  match: RegExp
  status?: number
  /** Dynamic status (wins over `status`): a 200/400 pair from one route,
   * e.g. POST /flows/validate answering both valid and invalid graphs. */
  statusFn?: (ctx: { body: unknown }) => number
  /** Static response body (ignored when bodyFn is set). */
  body?: unknown
  /** Dynamic response body: a function of the request context. */
  bodyFn?: BodyFn
}

export interface FetchMock {
  requests: CapturedRequest[]
  /** All requests to `method` + pathname without query. */
  calls(method: string, pathname: RegExp): CapturedRequest[]
  /** The live route list: push extra routes onto it mid-test. */
  routes: MockRoute[]
  restore: () => void
}

export function installFetchMock(routes: MockRoute[]): FetchMock {
  const requests: CapturedRequest[] = []
  const stub = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      const url = new URL(String(input), 'http://localhost')
      const method = (init?.method ?? 'GET').toUpperCase()
      let parsed: unknown
      if (init?.body) {
        try {
          parsed = JSON.parse(String(init.body))
        } catch {
          parsed = init.body
        }
      }
      const request: CapturedRequest = {
        method,
        url: url.pathname + url.search,
        body: parsed,
      }
      requests.push(request)
      for (const route of routes) {
        if (route.method !== method) continue
        if (!route.match.test(url.pathname)) continue
        const status = route.statusFn
          ? route.statusFn({ body: parsed })
          : (route.status ?? 200)
        const payload = route.bodyFn
          ? route.bodyFn({ url, body: parsed, request })
          : route.body
        // 204/304 must not carry a body (the Response constructor throws).
        if (status === 204 || status === 304 || payload === undefined) {
          return new Response(null, {
            status,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response(JSON.stringify(payload), {
          status,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response(
        JSON.stringify({
          error: `no mock for ${method} ${url.pathname}`,
          code: 'no_mock',
        }),
        { status: 500, headers: { 'Content-Type': 'application/json' } },
      )
    },
  )
  vi.stubGlobal('fetch', stub)
  return {
    requests,
    routes,
    calls: (method, pathname) =>
      requests.filter(
        (r) => r.method === method && pathname.test(r.url.split('?')[0]),
      ),
    restore: () => vi.unstubAllGlobals(),
  }
}

/** Minimal EventSource: records listeners, tests emit events by type. */
export class FakeEventSource {
  static instances: FakeEventSource[] = []
  readonly url: string
  closed = false
  private listeners = new Map<string, Array<() => void>>()

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, cb: () => void): void {
    const list = this.listeners.get(type) ?? []
    list.push(cb)
    this.listeners.set(type, list)
  }

  removeEventListener(type: string, cb: () => void): void {
    this.listeners.set(
      type,
      (this.listeners.get(type) ?? []).filter((l) => l !== cb),
    )
  }

  close(): void {
    this.closed = true
  }

  /** Deliver an event to the listeners registered for its type. */
  emit(type: string): void {
    for (const cb of this.listeners.get(type) ?? []) cb()
  }

  static reset(): void {
    FakeEventSource.instances = []
  }
}

/** Installs the fake EventSource global and resets the instance list. */
export function stubEventSource(): void {
  FakeEventSource.reset()
  vi.stubGlobal('EventSource', FakeEventSource)
}
