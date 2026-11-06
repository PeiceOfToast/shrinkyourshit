// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode profiles end to end (task 72, spec 5.5).
 *
 * The first three tests drive the real server: the seeded default profile,
 * New, Duplicate, and each editor saving to the profile (Basic form, Moderate
 * tree, Advanced canvas).
 *
 * The shared-profile test answers the profile routes itself. Nothing in this
 * build attaches a profile to two real libraries (task 73 does that), so the
 * two library names come from the test. Everything the page does with them
 * (the confirmation, the save and its request) is the page's own behaviour.
 */
import { expect, test, type Page, type Route } from '@playwright/test'

/** The visible copy of a link or control: the table on wide screens, the card on phones. */
function visible(page: Page, selector: string) {
  return page.locator(selector).filter({ visible: true })
}

/** The Encode page, with the Profiles section in view. */
async function openEncode(page: Page) {
  await page.goto('/encode')
  await expect(page.getByTestId('encode-settings-page')).toBeVisible()
  await expect(page.getByTestId('profiles-section')).toBeVisible()
}

/** New profile, then the editor it opens. */
async function newProfile(page: Page): Promise<string> {
  await openEncode(page)
  await page.getByTestId('profile-new').click()
  await expect(page.getByTestId('profile-page')).toBeVisible()
  await expect(page).toHaveURL(/\/encode\/profiles\/[^/]+$/)
  return new URL(page.url()).pathname.split('/').pop() ?? ''
}

test.describe('encode profiles (real server)', () => {
  test('the Encode page lists the seeded profile, and New and Duplicate open an editor', async ({
    page,
  }) => {
    await openEncode(page)
    // Setup seeds one profile at boot, named after the default tier.
    await expect(
      visible(page, 'a:has-text("Default (basic)")').first(),
    ).toBeVisible()

    // The server is shared by all three viewport projects, so names are unique per run.
    const name = `E2E Films ${Date.now()}`
    const id = await newProfile(page)
    await page.getByTestId('profile-name-input').fill(name)
    await page.getByTestId('profile-save').click()
    await expect(
      page.getByTestId('toast').filter({ hasText: 'Profile saved.' }),
    ).toBeVisible()

    // The save is stored: a reload shows the new name.
    await page.reload()
    await expect(page.getByTestId('profile-name-input')).toHaveValue(name)

    // Duplicate from the list opens the copy.
    await page.getByTestId('profile-back').click()
    await expect(page.getByTestId('profiles-section')).toBeVisible()
    await visible(page, `[data-testid="profile-duplicate-${id}"]`)
      .first()
      .click()
    await expect(page.getByTestId('profile-page')).toBeVisible()
    await expect(page.getByTestId('profile-name-input')).toHaveValue(
      `${name} (copy)`,
    )
  })

  test('the Moderate tree saves to the profile', async ({ page }) => {
    await newProfile(page)
    await page.getByTestId('tier-moderate').click()
    await expect(page.getByTestId('tree-page')).toBeVisible()
    await expect(page).toHaveURL(/\/encode\/profiles\/[^/]+\/tree$/)

    await page.getByTestId('tree-add-type').selectOption('skip_if_smaller')
    await page.getByTestId('tree-add-button').click()
    await page.getByTestId('tree-save').click()
    await expect(
      page.getByTestId('toast').filter({ hasText: 'Profile saved.' }),
    ).toBeVisible()
    // The tree saved the profile at tier moderate.
    await expect(page.getByTestId('tier-moderate-in-use')).toBeVisible()
  })

  test('the Advanced canvas saves to the profile', async ({ page }) => {
    // The canvas edits only on a wide screen (task 22); phones see it read only.
    const width = page.viewportSize()?.width ?? 0
    test.skip(width < 768, 'the canvas is view only below 768 px')

    await newProfile(page)
    await page.getByTestId('tier-advanced').click()
    await expect(page.getByTestId('flow-canvas-page')).toBeVisible()
    await expect(page).toHaveURL(/\/encode\/profiles\/[^/]+\/flow$/)

    // A new step is not connected, so the canvas would refuse it. Change a
    // value on a step the default flow already connects instead.
    await page.getByTestId('flow-edit').click()
    await page.getByTestId('canvas-node-transcode_video').click()
    const quality = page.getByTestId('flow-param-quality')
    await expect(quality).toBeVisible()
    await quality.fill('4')
    await expect(page.getByTestId('flow-save')).toBeEnabled()
    await page.getByTestId('flow-save').click()
    await expect(
      page.getByTestId('toast').filter({ hasText: 'Profile saved.' }),
    ).toBeVisible()
    await expect(page.getByTestId('tier-advanced-in-use')).toBeVisible()
  })
})

test.describe('a profile used by two libraries (routes answered by the test)', () => {
  const profile = {
    id: 'p-shared',
    name: 'Shared films',
    description: '',
    tier: 'basic',
    result_policy: 'replace_if_smaller',
    graph: { schema: 1, nodes: [{ id: 'probe', type: 'probe' }], edges: [] },
    used_by: 2,
    used_by_names: ['Movies', 'TV Shows'],
    unrepresentable: [],
    created_at: '2026-11-06T10:00:00Z',
    updated_at: '2026-11-06T10:00:00Z',
  }

  test('asks first, names both libraries, and saves once confirmed', async ({
    page,
  }) => {
    const puts: Array<Record<string, unknown>> = []
    await page.route(
      /\/api\/v1\/profiles\/p-shared(\?|$)/,
      async (route: Route) => {
        const req = route.request()
        if (req.method() === 'PUT') {
          const body = req.postDataJSON() as Record<string, unknown>
          puts.push(body)
          return route.fulfill({ json: { ...profile, ...body } })
        }
        return route.fulfill({ json: profile })
      },
    )

    await page.goto('/encode/profiles/p-shared')
    await expect(page.getByTestId('profile-page')).toBeVisible()
    await expect(page.getByTestId('profile-used-by')).toContainText(
      'Movies, TV Shows',
    )

    await page.getByTestId('profile-save').click()
    const dialog = page.getByTestId('profile-save-dialog')
    await expect(dialog).toBeVisible()
    const names = page.getByTestId('profile-save-libraries')
    await expect(names).toContainText('Movies')
    await expect(names).toContainText('TV Shows')
    expect(puts).toHaveLength(0)

    await page.getByTestId('profile-save-confirm').click()
    await expect(
      page.getByTestId('toast').filter({ hasText: 'Profile saved.' }),
    ).toBeVisible()
    expect(puts).toHaveLength(1)
    expect(puts[0]).toMatchObject({ name: 'Shared films', tier: 'basic' })
  })
})
