// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Moderate rule tree (task 60): a seeded library gets a known graph,
 * the page adds a condition and its child, saves, reloads and draws the
 * same tree. A phone-width run proves the rows stack with no sideways
 * scroll. Without a seeded library (no ffmpeg in the seed) there is no
 * library to edit, so the tests skip with a reason.
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

test.describe('rule tree (task 60)', () => {
  test('adds a condition and its child, saves, reloads and shows the same tree', async ({
    page,
    request,
  }) => {
    const id = await firstLibraryId(request)
    test.skip(!id, 'no library is seeded, so there is no rule tree to edit')

    const put = await request.put(`/api/v1/libraries/${id}/flow`, {
      data: { source: 'steps', graph: START },
    })
    expect(put.ok(), await put.text()).toBeTruthy()

    await page.goto(`/libraries/${id}/tree`)
    await expect(page.getByTestId('tree-rows')).toBeVisible()

    // The condition lands after the container; the action after it is its child.
    await page.getByTestId('tree-add-type').selectOption('if_bitrate_above')
    await page.getByTestId('tree-add-button').click()
    await page.getByTestId('tree-add-type').selectOption('skip_if_smaller')
    await page.getByTestId('tree-add-button').click()
    await expect(page.getByTestId('tree-row-skip_if_smaller')).toHaveAttribute(
      'data-depth',
      '1',
    )

    await page.getByTestId('tree-save').click()
    await expect(page.getByText('Tree saved.')).toBeVisible()

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
  })

  test('at 390 px the rows stack and nothing scrolls sideways', async ({
    page,
    request,
  }) => {
    const id = await firstLibraryId(request)
    test.skip(!id, 'no library is seeded, so there is no rule tree to edit')
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/libraries/${id}/tree`)
    await expect(page.getByTestId('tree-rows')).toBeVisible()
    const overflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    )
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
