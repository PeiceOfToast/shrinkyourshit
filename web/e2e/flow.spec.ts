// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The flow page on the app palette (task 46, spec 2.9 and 5.9). The React
 * Flow canvas must paint the canvas token, not the library's grey, at every
 * viewport. The page needs a seeded library; without one (no ffmpeg in the
 * seed) there is no flow to draw, so the test skips with a reason.
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

test.describe('flow page (task 46)', () => {
  test('the canvas background is the canvas token rgb(2, 6, 23)', async ({
    page,
    request,
  }) => {
    const id = await firstLibraryId(request)
    test.skip(
      !id,
      'no library is seeded, so there is no flow canvas to measure',
    )
    await page.goto(`/libraries/${id}/flow`)
    await expect(page.getByTestId('flow-canvas-page')).toBeVisible()
    const canvas = page.locator('.react-flow').first()
    await expect(canvas).toBeVisible()
    const background = await canvas.evaluate(
      (el) => getComputedStyle(el).backgroundColor,
    )
    expect(background).toBe('rgb(2, 6, 23)')
  })
})
