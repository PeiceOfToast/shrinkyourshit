// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The tier switch round trip (task 62). A seeded library gets an advanced
 * graph with a raw_args step. Downgrading it to Basic shows the banner that
 * names the step, and the Basic editor renders read only with Save off.
 * Upgrading back to Advanced clears the banner and the canvas still shows
 * the step, with its arguments unchanged. Without a seeded library (no
 * ffmpeg in the seed) there is nothing to switch, so the test skips.
 */
import { expect, test, type APIRequestContext } from '@playwright/test'

/** The first library's id from the API, or '' when none exists. */
async function firstLibraryId(request: APIRequestContext): Promise<string> {
  try {
    const res = await request.get('/api/v1/libraries')
    if (!res.ok()) return ''
    const body = (await res.json()) as { items?: Array<{ id?: string }> }
    return body.items?.find((l) => l.id)?.id ?? ''
  } catch {
    return ''
  }
}

/** Probe, one encode, one container, then a raw arguments step at the end. */
const GRAPH = {
  schema: 1,
  nodes: [
    { id: 'probe', type: 'probe' },
    {
      id: 'transcode_video',
      type: 'transcode_video',
      params: { codec: 'hevc', quality: 3 },
    },
    {
      id: 'remux_container',
      type: 'remux_container',
      params: { container: 'mkv' },
    },
    {
      id: 'raw_args',
      type: 'raw_args',
      params: { stage: 'output', args: '-threads 2', note: 'Keep threads low' },
    },
  ],
  edges: [
    { from: 'probe', to: 'transcode_video' },
    { from: 'transcode_video', to: 'remux_container' },
    { from: 'remux_container', to: 'raw_args' },
  ],
}

/** The stored graph's raw_args step, read straight from the API. */
async function storedRawArgs(
  request: APIRequestContext,
  id: string,
): Promise<{ type?: string; params?: Record<string, unknown> } | undefined> {
  const res = await request.get(`/api/v1/libraries/${id}/flow`)
  const body = (await res.json()) as {
    graph: {
      nodes: Array<{
        id: string
        type: string
        params?: Record<string, unknown>
      }>
    }
  }
  return body.graph.nodes.find((n) => n.id === 'raw_args')
}

test.describe('tier switch (task 62)', () => {
  test('downgrade to basic shows the banner and locks the editor, upgrade keeps the step', async ({
    page,
    request,
  }) => {
    const id = await firstLibraryId(request)
    test.skip(!id, 'no library is seeded, so there is no tier to switch')

    const put = await request.put(`/api/v1/libraries/${id}/flow`, {
      data: { source: 'advanced', graph: GRAPH },
    })
    expect(put.ok(), await put.text()).toBeTruthy()
    const up = await request.put(`/api/v1/libraries/${id}/tier`, {
      data: { tier: 'advanced' },
    })
    expect(up.ok(), await up.text()).toBeTruthy()

    // Downgrade on the library page: the banner names the raw_args step.
    await page.goto(`/libraries/${id}`)
    await page.getByTestId('library-tier-basic').click()
    await expect(page.getByText('Editor set to Basic.')).toBeVisible()
    const banner = page.getByTestId('tier-banner')
    await expect(banner).toBeVisible()
    // The raw_args step is named, with the tier that can edit it. The graph
    // has no if_codec_not guard, so Basic also lists that graph-level problem.
    const rawItem = page
      .getByTestId('tier-step-item')
      .filter({ hasText: 'raw_args' })
    await expect(rawItem).toHaveCount(1)
    await expect(rawItem).toContainText('Advanced')
    await expect(banner).toContainText('Nothing was changed')

    // The Basic editor is read only: the sliders are off and Save is gone.
    await page.goto(`/libraries/${id}/rules`)
    await page.getByTestId('tier-easy').click()
    await expect(page.getByTestId('easy-locked-reason')).toBeVisible()
    // Every control inside the fieldset is disabled, not only the first.
    const fieldInputs = page.locator('[data-testid="easy-fieldset"] input')
    const total = await fieldInputs.count()
    expect(total).toBeGreaterThan(0)
    await expect(
      page.locator('[data-testid="easy-fieldset"] input:disabled'),
    ).toHaveCount(total)
    await expect(page.getByTestId('rules-easy-save')).toBeDisabled()
    expect(await storedRawArgs(request, id)).toMatchObject({
      type: 'raw_args',
      params: { args: '-threads 2' },
    })

    // Upgrade back: no banner, and the canvas still shows the same step.
    await page.goto(`/libraries/${id}`)
    await page.getByTestId('library-tier-advanced').click()
    await expect(page.getByText('Editor set to Advanced.')).toBeVisible()
    await expect(page.getByTestId('tier-banner')).toHaveCount(0)

    await page.goto(`/libraries/${id}/flow`)
    await expect(page.getByTestId('canvas-node-raw_args')).toBeVisible()
    expect(await storedRawArgs(request, id)).toMatchObject({
      type: 'raw_args',
      params: { args: '-threads 2', note: 'Keep threads low', stage: 'output' },
    })
  })
})

/**
 * The library folder picker (task 69, spec 4.6). The e2e runner cannot mount
 * a share, so for this spec only the storage routes are answered by the test:
 * one mounted share and its folder listing. The form, the browser, the path
 * the library is created with, and the refusal of a typed path are what is
 * under test. The created library request is captured, not stored.
 */
test.describe('library folder picker (task 69)', () => {
  test('picks a folder inside a storage and creates the library with that path', async ({
    page,
  }) => {
    const nas = {
      id: 'st-nas',
      name: 'nas',
      protocol: 'nfs',
      source: 'nas.lan:/export/media',
      options: 'vers=3,soft,timeo=50,retrans=2',
      username: '',
      domain: '',
      password_set: false,
      read_only: false,
      enabled: true,
      mountpoint: '/storage/nas',
      mounted: true,
      error: '',
      created_at: '',
      updated_at: '',
    }
    const folders: Record<string, string[]> = {
      '/storage/nas': ['Movies', 'TV'],
      '/storage/nas/Movies': ['2026'],
    }
    let created: { path?: string } | null = null

    await page.route(/\/api\/v1\/storages(\/|$|\?)/, async (route) => {
      const req = route.request()
      const url = new URL(req.url())
      if (url.pathname === '/api/v1/storages' && req.method() === 'GET') {
        return route.fulfill({
          json: {
            items: [nas],
            total: 1,
            host_policy: { restricted: false, allowed: [], message: '' },
          },
        })
      }
      if (url.pathname === '/api/v1/storages/st-nas/browse') {
        const path = url.searchParams.get('path') ?? '/storage/nas'
        const parent =
          path === '/storage/nas' ? '' : path.replace(/\/[^/]+$/, '')
        return route.fulfill({
          json: {
            id: 'st-nas',
            name: 'nas',
            mounted: true,
            mountpoint: '/storage/nas',
            path,
            parent,
            free_bytes: 2_000_000_000,
            total_bytes: 4_000_000_000,
            entries: (folders[path] ?? []).map((name) => ({
              name,
              path: `${path}/${name}`,
              is_dir: true,
            })),
          },
        })
      }
      return route.continue()
    })

    // Login reads as on for this page only, so the browse is not refused.
    await page.route('**/api/v1/libraries', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      created = route.request().postDataJSON() as { path?: string }
      return route.fulfill({
        status: 201,
        json: {
          id: 'lib-picked',
          name: 'Movies',
          path: created?.path ?? '',
          target: {},
          easy: null,
          watch: true,
          schedule: '',
          result_policy: 'replace_if_smaller',
          created_at: '',
          updated_at: '',
        },
      })
    })

    await page.goto('/libraries/new')
    await expect(page.getByTestId('new-library-page')).toBeVisible()

    await page.getByTestId('easy-storage-select').selectOption('st-nas')
    await expect(page.getByTestId('easy-storage-badge')).toHaveText('Mounted')

    await page.getByTestId('easy-choose-folder').click()
    await expect(page.getByTestId('storage-browser')).toBeVisible()
    await page.getByTestId('storage-browser-dir-Movies').click()
    await expect(page.getByTestId('storage-browser-current')).toContainText(
      'Movies',
    )
    await expect(page.getByTestId('storage-browser-use')).toBeEnabled()
    await page.getByTestId('storage-browser-use').click()

    await expect(page.getByTestId('easy-folder')).toHaveText(
      '/storage/nas/Movies',
    )
    await page.getByTestId('easy-start').click()

    await expect.poll(() => created?.path).toBe('/storage/nas/Movies')
  })

  test('refuses a typed path outside every mounted storage', async ({
    page,
  }) => {
    await page.route(/\/api\/v1\/storages(\/|$|\?)/, async (route) =>
      route.fulfill({
        json: {
          items: [],
          total: 0,
          host_policy: { restricted: false, allowed: [], message: '' },
        },
      }),
    )
    await page.goto('/libraries/new')
    await expect(page.getByTestId('new-library-page')).toBeVisible()

    await page.getByText('Type the path instead').click()
    await page.getByTestId('easy-typed-path-input').fill('/media/movies')
    await page.getByTestId('easy-use-typed-path').click()

    await expect(page.getByTestId('easy-typed-path-error')).toContainText(
      'not inside a mounted storage',
    )
    await expect(page.getByTestId('easy-folder-empty')).toBeVisible()
  })
})
