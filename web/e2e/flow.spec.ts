// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The flow page on the app palette (task 46, spec 2.9 and 5.9). The React
 * Flow canvas must paint the canvas token, not the library's grey, at every
 * viewport. Flows belong to encode profiles since task 73: the library's flow
 * route goes to its profile's canvas, and the editing test makes its own
 * profile, so the seeded library is not changed.
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

/** Probe then one container: a valid chain the test extends by hand. */
const BASE = {
  schema: 1,
  nodes: [
    { id: 'probe', type: 'probe' },
    {
      id: 'remux_container',
      type: 'remux_container',
      params: { container: 'mkv' },
    },
  ],
  edges: [{ from: 'probe', to: 'remux_container' }],
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

test.describe('raw args node (task 61)', () => {
  test('adds a raw_args node, saves, reloads and shows the warning again', async ({
    page,
    request,
  }, testInfo) => {
    // Editing is desktop only (the page is view only below md).
    test.skip(
      testInfo.project.name === 'phone',
      'editing the flow needs a desktop browser',
    )
    // A profile of its own: the flow is saved to the profile (task 73).
    const created = await request.post('/api/v1/profiles', {
      data: {
        name: `e2e raw args ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        description: '',
        tier: 'advanced',
        result_policy: 'replace',
        graph: BASE,
      },
    })
    expect(created.ok(), await created.text()).toBeTruthy()
    const id = ((await created.json()) as { id: string }).id
    try {
      await page.goto(`/encode/profiles/${id}/flow`)
      await page.getByTestId('flow-edit').click()
      await expect(page.getByTestId('flow-palette')).toBeVisible()

      // Add the step, connect it after the container, and fill its arguments.
      await page.getByTestId('palette-add-raw_args').click()
      await page.getByTestId('canvas-node-remux_container').click()
      await page
        .getByTestId('flow-inspector-connect-select')
        .selectOption('raw_args')
      await page.getByTestId('flow-inspector-connect').click()
      await page.getByTestId('canvas-node-raw_args').click()
      await page.getByTestId('flow-param-args').fill('-threads 2')
      await page.getByTestId('flow-param-note').fill('Keep threads low')

      await expect(page.getByTestId('flow-save')).toBeEnabled()
      await page.getByTestId('flow-save').click()
      // The profile's canvas saves the profile (task 73).
      await expect(page.getByText('Profile saved.')).toBeVisible()

      await page.reload()
      await expect(page.getByTestId('canvas-node-raw_args')).toBeVisible()
      await expect(page.getByTestId('canvas-node-raw_args')).toHaveAttribute(
        'data-unsafe',
        'true',
      )
      await expect(page.getByTestId('canvas-node-warning-raw_args')).toHaveText(
        'Passes arguments the app does not model.',
      )
      await expect(page.getByTestId('canvas-node-note-raw_args')).toHaveText(
        'Note: Keep threads low',
      )
    } finally {
      await request.delete(`/api/v1/profiles/${id}`)
    }
  })
})
