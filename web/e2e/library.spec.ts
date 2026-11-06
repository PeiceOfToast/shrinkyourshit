// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode profiles on the library pages (task 73, spec 5.5). A library follows
 * one profile; the library pages show which, and what its steps do in plain
 * language. These run against the real server: a profile is made through the
 * API, a library through the form. The seeded library and the profiles this
 * spec makes are put back as they were.
 */
import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test'

interface LibraryItem {
  id: string
  name: string
  path: string
  watch: boolean
  file_kinds: string[]
  target: unknown
  profile_id?: string
}

interface ProfileItem {
  id: string
  name: string
  graph: {
    schema: number
    nodes: Array<{ id: string; type: string; params?: Record<string, unknown> }>
    edges: unknown[]
  }
}

/** The first library from the API, or null when none exists. */
async function firstLibrary(
  request: APIRequestContext,
): Promise<LibraryItem | null> {
  try {
    const res = await request.get('/api/v1/libraries')
    if (!res.ok()) return null
    const body = (await res.json()) as { items?: LibraryItem[] }
    return body.items?.find((l) => l.id) ?? null
  } catch {
    return null
  }
}

/** The graph of the oldest profile (the boot seed), which is basic-representable. */
async function seededGraph(
  request: APIRequestContext,
): Promise<ProfileItem['graph']> {
  const res = await request.get('/api/v1/profiles')
  expect(res.ok(), await res.text()).toBeTruthy()
  const body = (await res.json()) as { items: ProfileItem[] }
  return body.items[0].graph
}

/** A profile made through the API, named so it cannot clash with the seed. */
async function createProfile(
  request: APIRequestContext,
  name: string,
  tier: string,
  graph: ProfileItem['graph'],
): Promise<ProfileItem> {
  const res = await request.post('/api/v1/profiles', {
    data: { name, description: '', tier, result_policy: 'replace', graph },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  return (await res.json()) as ProfileItem
}

/** The graph with its transcode quality changed: a visibly different profile. */
function withQuality(
  graph: ProfileItem['graph'],
  quality: number,
): ProfileItem['graph'] {
  return {
    ...graph,
    nodes: graph.nodes.map((n) =>
      n.type === 'transcode_video'
        ? { ...n, params: { ...n.params, quality } }
        : n,
    ),
  }
}

/** Attaches a library to a profile (or detaches with ''), keeping every other field. */
async function attachProfile(
  request: APIRequestContext,
  lib: LibraryItem,
  profileId: string,
): Promise<void> {
  const res = await request.put(`/api/v1/libraries/${lib.id}`, {
    data: {
      name: lib.name,
      path: lib.path,
      watch: lib.watch,
      file_kinds: lib.file_kinds,
      target: lib.target,
      profile_id: profileId,
    },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
}

/**
 * Walks the folder picker from / down to the folder and picks it. The picker
 * opens at / and lists directories by name, so each segment is one click.
 */
async function pickFolder(page: Page, folder: string): Promise<void> {
  await page.getByTestId('easy-choose-folder').click()
  await expect(page.getByTestId('folder-picker')).toBeVisible()
  for (const seg of folder.split('/').filter(Boolean)) {
    await page.getByTestId(`folder-picker-dir-${seg}`).click()
  }
  await expect(page.getByTestId('folder-picker-current')).toHaveAttribute(
    'title',
    folder,
  )
  await page.getByTestId('folder-picker-use').click()
}

test.describe('library follows its profile (task 73)', () => {
  test('a library whose profile tier cannot show a step says so on its page', async ({
    page,
    request,
  }) => {
    const lib = await firstLibrary(request)
    test.skip(!lib, 'no library is seeded, so there is nothing to attach')
    const graph = await seededGraph(request)
    // An advanced step (raw_args) on a profile kept in the Basic editor.
    const profile = await createProfile(
      request,
      `e2e hidden ${Date.now()}`,
      'basic',
      {
        ...graph,
        nodes: [
          ...graph.nodes,
          {
            id: 'raw_args',
            type: 'raw_args',
            params: {
              stage: 'output',
              args: '-threads 2',
              note: 'Keep threads low',
            },
          },
        ],
        edges: [
          ...graph.edges,
          { from: graph.nodes[graph.nodes.length - 1].id, to: 'raw_args' },
        ],
      } as ProfileItem['graph'],
    )
    const original = lib.profile_id ?? ''
    try {
      await attachProfile(request, lib, profile.id)
      await page.goto(`/libraries/${lib.id}`)
      await expect(page.getByTestId('library-profile-name')).toContainText(
        profile.name,
      )
      const banner = page.getByTestId('tier-banner')
      await expect(banner).toBeVisible()
      const rawItem = page
        .getByTestId('tier-step-item')
        .filter({ hasText: 'raw_args' })
      await expect(rawItem).toHaveCount(1)
      await expect(rawItem).toContainText('Advanced')
      await expect(banner).toContainText('Nothing was changed')
    } finally {
      await attachProfile(request, lib, original)
      await request.delete(`/api/v1/profiles/${profile.id}`)
    }
  })

  test('creates a library by choosing a profile and a folder, then switching the profile changes its summary', async ({
    page,
    request,
  }) => {
    const seed = await firstLibrary(request)
    test.skip(!seed, 'no library is seeded, so there is no folder to pick')
    const graph = await seededGraph(request)
    const profileA = await createProfile(
      request,
      `e2e follows A ${Date.now()}`,
      'basic',
      graph,
    )
    const profileB = await createProfile(
      request,
      `e2e follows B ${Date.now()}`,
      'basic',
      withQuality(graph, 5),
    )
    let createdId = ''
    try {
      await page.goto('/libraries/new')
      await expect(page.getByTestId('new-library-page')).toBeVisible()
      await pickFolder(page, seed.path)
      await expect(page.getByTestId('easy-folder')).toHaveText(seed.path)
      await page.getByTestId('library-profile-select').selectOption(profileA.id)
      await expect(
        page
          .getByTestId('library-form-profile-summary')
          .getByTestId('profile-summary-line')
          .first(),
      ).toBeVisible()
      await page.getByTestId('easy-start').click()

      // The new library's page names the profile it follows.
      // The form's own page is /libraries/new, so the id must not be that.
      await expect(page).toHaveURL(/\/libraries\/(?!new$)[^/]+$/)
      createdId = new URL(page.url()).pathname.split('/').pop() ?? ''
      await expect(page.getByTestId('library-profile-name')).toContainText(
        profileA.name,
      )
      const before = await page
        .getByTestId('target-plain-line')
        .allTextContents()
      expect(before.length).toBeGreaterThan(0)

      // Switch the profile on the edit page: the summary follows.
      await page.getByTestId('library-detail-edit').click()
      await page.getByTestId('library-edit-profile').selectOption(profileB.id)
      await page.getByTestId('library-edit-save').click()
      await expect(page).toHaveURL(new RegExp(`/libraries/${createdId}$`))
      await expect(page.getByTestId('library-profile-name')).toContainText(
        profileB.name,
      )
      await expect
        .poll(() => page.getByTestId('target-plain-line').allTextContents())
        .not.toEqual(before)
    } finally {
      if (createdId) await request.delete(`/api/v1/libraries/${createdId}`)
      await request.delete(`/api/v1/profiles/${profileA.id}`)
      await request.delete(`/api/v1/profiles/${profileB.id}`)
    }
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
      options: 'vers=4.2,soft,timeo=50,retrans=2',
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
