// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Moderate rule tree (task 60, moved to the profile in task 73). Each
 * test makes its own profile through the API, edits its tree on the profile's
 * tree route, saves, reloads and checks the tree drawn. A phone-width run
 * proves the rows stack with no sideways scroll. The profile is deleted at the
 * end, so the seeded library and its profile are left as they were.
 */
import { expect, test, type APIRequestContext } from '@playwright/test'

/** Probe, one encode, one container: a valid chain with nothing gated yet. */
const START = {
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
  ],
  edges: [
    { from: 'probe', to: 'transcode_video' },
    { from: 'transcode_video', to: 'remux_container' },
  ],
}

/** A moderate profile holding START, made through the API for one test. */
async function makeProfile(request: APIRequestContext): Promise<string> {
  const res = await request.post('/api/v1/profiles', {
    data: {
      name: `e2e tree ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      description: '',
      tier: 'moderate',
      result_policy: 'replace',
      graph: START,
    },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  return ((await res.json()) as { id: string }).id
}

test.describe('rule tree (task 60)', () => {
  test('adds a condition and its child, saves, reloads and shows the same tree', async ({
    page,
    request,
  }) => {
    const id = await makeProfile(request)
    try {
      await page.goto(`/encode/profiles/${id}/tree`)
      await expect(page.getByTestId('tree-rows')).toBeVisible()

      // The condition lands after the container; the action after it is its child.
      await page.getByTestId('tree-add-type').selectOption('if_bitrate_above')
      await page.getByTestId('tree-add-button').click()
      await page.getByTestId('tree-add-type').selectOption('skip_if_smaller')
      await page.getByTestId('tree-add-button').click()
      await expect(
        page.getByTestId('tree-row-skip_if_smaller'),
      ).toHaveAttribute('data-depth', '1')

      await page.getByTestId('tree-save').click()
      // The profile's tree saves the profile (task 73).
      await expect(page.getByText('Profile saved.')).toBeVisible()

      await page.reload()
      await expect(page.getByTestId('tree-rows')).toBeVisible()
      const rows = await page
        .locator('li[data-testid^="tree-row-"]')
        .evaluateAll((els) =>
          els.map((el) => [
            el.getAttribute('data-testid'),
            el.getAttribute('data-depth'),
          ]),
        )
      expect(rows).toEqual([
        ['tree-row-probe', '0'],
        ['tree-row-transcode_video', '0'],
        ['tree-row-remux_container', '0'],
        ['tree-row-if_bitrate_above', '0'],
        ['tree-row-skip_if_smaller', '1'],
      ])
    } finally {
      await request.delete(`/api/v1/profiles/${id}`)
    }
  })

  test('at 390 px the rows stack and nothing scrolls sideways', async ({
    page,
    request,
  }) => {
    const id = await makeProfile(request)
    try {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`/encode/profiles/${id}/tree`)
      await expect(page.getByTestId('tree-rows')).toBeVisible()
      const overflow = await page.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth,
      )
      expect(overflow).toBeLessThanOrEqual(0)
    } finally {
      await request.delete(`/api/v1/profiles/${id}`)
    }
  })
})
