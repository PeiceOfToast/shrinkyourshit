// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage page end to end (task 68, spec 4.6).
 *
 * - Login off, real server: the seeded server runs with login disabled, so
 *   the page must render read only, say why, and link to App settings.
 * - Login on, add, test and mount: the e2e runner cannot mount a share, so
 *   for this spec only the storage routes and the settings read are answered
 *   by the test. Nothing on the server changes. The page's own behaviour is
 *   what is under test: the form, the toasts and the state each action leaves.
 *
 * Every row action exists twice on the page (table and card view, one shown
 * per width), so each lookup keeps the visible one.
 */
import { expect, test, type Page, type Route } from '@playwright/test'

interface MockShare {
  id: string
  name: string
  protocol: string
  source: string
  options: string
  username: string
  domain: string
  password_set: boolean
  read_only: boolean
  enabled: boolean
  mountpoint: string
  mounted: boolean
  error: string
  created_at: string
  updated_at: string
}

/**
 * The visible copy of a row: the table row on wide screens, the card on
 * phones. The page renders both, one hidden by CSS.
 */
function row(page: Page, id: string) {
  return page
    .locator(
      `[data-testid="storage-row-${id}"], [data-testid="storage-card-${id}"]`,
    )
    .filter({ visible: true })
}

/** The visible copy of a row action: table id, or the card's -card id. */
function action(page: Page, name: string, id: string) {
  return page
    .locator(
      `[data-testid="storage-${name}-${id}"], [data-testid="storage-${name}-${id}-card"]`,
    )
    .filter({ visible: true })
}

test.describe('storage page with login off (real server)', () => {
  test('renders read only with the reason and a link to App settings', async ({
    page,
  }) => {
    await page.goto('/storage')
    await expect(page.getByTestId('storage-page')).toBeVisible()
    await expect(page.getByTestId('storage-readonly')).toContainText(
      'read only while login is off',
    )
    await expect(page.getByTestId('storage-add')).toBeDisabled()

    await page.getByTestId('storage-readonly-link').click()
    await expect(page).toHaveURL(/\/app$/)
    await expect(page.getByTestId('app-settings-page')).toBeVisible()
  })
})

test.describe('storage page with login on (storage routes answered by the test)', () => {
  let shares: MockShare[]

  test.beforeEach(async ({ page }) => {
    shares = []

    // Login reads as on for this page only. Everything else still reaches
    // the real server.
    await page.route('**/api/v1/settings', async (route: Route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await route.fulfill({
        json: {
          login_enabled: true,
          worker_concurrency: 1,
          preferred_languages: [],
        },
      })
    })

    await page.route(/\/api\/v1\/storages(\/|$|\?)/, async (route: Route) => {
      const req = route.request()
      const path = new URL(req.url()).pathname
      const method = req.method()

      if (path === '/api/v1/storages' && method === 'GET') {
        return route.fulfill({
          json: {
            items: shares,
            total: shares.length,
            host_policy: {
              restricted: false,
              allowed: [],
              message:
                'Any host can be mounted. Set storage_allowed_hosts to limit which hosts this server may mount; setting it is recommended.',
            },
          },
        })
      }

      if (path === '/api/v1/storages' && method === 'POST') {
        const body = req.postDataJSON() as Partial<MockShare> & {
          password?: string
        }
        const created: MockShare = {
          id: `st-${body.name}`,
          name: body.name ?? '',
          protocol: body.protocol ?? 'nfs',
          source: body.source ?? '',
          options: body.options ?? '',
          username: body.username ?? '',
          domain: body.domain ?? '',
          // The response never carries the password itself.
          password_set: Boolean(body.password),
          read_only: body.read_only ?? false,
          enabled: body.enabled ?? true,
          mountpoint: `/storage/${body.name}`,
          mounted: false,
          error: '',
          created_at: '2026-11-06T10:00:00Z',
          updated_at: '2026-11-06T10:00:00Z',
        }
        shares.push(created)
        return route.fulfill({ status: 201, json: created })
      }

      const match = /^\/api\/v1\/storages\/([^/]+)\/(test|mount|unmount)$/.exec(
        path,
      )
      if (match && method === 'POST') {
        const [, id, action] = match
        const share = shares.find((s) => s.id === id)
        if (!share) {
          return route.fulfill({
            status: 404,
            json: { error: 'no such storage', code: 'not_found' },
          })
        }
        if (action === 'test') return route.fulfill({ json: { ok: true } })
        share.mounted = action === 'mount'
        return route.fulfill({ json: share })
      }

      return route.fulfill({
        status: 404,
        json: { error: 'not mocked', code: 'not_found' },
      })
    })
  })

  test('add a share, test it, then mount it', async ({ page }) => {
    await page.goto('/storage')
    await expect(page.getByTestId('storage-empty')).toBeVisible()
    await expect(page.getByTestId('storage-readonly')).toHaveCount(0)

    // Add: the NFS defaults are prefilled and the source is the only input.
    await expect(page.getByTestId('storage-add')).toBeEnabled()
    await page.getByTestId('storage-add').click()
    const dialog = page.getByTestId('storage-form')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByTestId('storage-options')).toHaveValue(
      'vers=4.2,soft,timeo=50,retrans=2',
    )
    await dialog.getByTestId('storage-name').fill('nas')
    await dialog
      .getByTestId('storage-source')
      .fill('192.168.1.10:/export/media')
    await dialog.getByTestId('storage-save').click()

    await expect(page.getByText('Share “nas” saved.')).toBeVisible()
    await expect(dialog).toHaveCount(0)
    await expect(row(page, 'st-nas')).toBeVisible()
    await expect(row(page, 'st-nas')).toContainText('Not mounted')

    // Test: the share mounts, checks and unmounts, and the page says so.
    await action(page, 'test', 'st-nas').click()
    await expect(page.getByTestId('storage-test-result')).toContainText(
      'The test passed',
    )

    // Mount: the state follows the server's answer.
    await action(page, 'mount', 'st-nas').click()
    await expect(page.getByText('Mounted at /storage/nas.')).toBeVisible()
    await expect(row(page, 'st-nas')).toContainText('Mounted')
    await expect(action(page, 'unmount', 'st-nas')).toBeVisible()
  })
})
