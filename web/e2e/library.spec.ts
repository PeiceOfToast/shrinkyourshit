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
