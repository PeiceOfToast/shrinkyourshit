// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Layout regression suite (tasks 12 and 19). For each of the five pages
 * plus the library detail page, at each of the three viewports: no
 * horizontal overflow, the shell covers the viewport (no unpainted region
 * below or beside it), and the nav is reachable — drawer under `md`, left
 * rail at `md` and up. Plus the file-table card assertion at phone width.
 * A failure leaves a screenshot per page per viewport in web/test-results
 * (the CI artifact). The detail page joins when the global setup seeded a
 * library; it skips cleanly when ffmpeg was unavailable.
 */
import { expect, test, type Page } from '@playwright/test'

/** The page's own stable test id, used as the ready marker. */
const PAGES: Array<{ path: string; ready: string }> = [
  { path: '/', ready: 'dashboard' },
  { path: '/libraries', ready: 'libraries-page' },
  { path: '/nodes', ready: 'nodes-page' },
  { path: '/encode', ready: 'encode-settings-page' },
  { path: '/app', ready: 'app-settings-page' },
]

const NAV_LABELS = [
  'Dash',
  'Library',
  'Nodes',
  'Encode settings',
  'App settings',
]

async function open(page: Page, path: string, ready: string): Promise<void> {
  await page.goto(path)
  await expect(page.getByTestId(ready), `${path} did not render`).toBeVisible()
}

/** Registers the three layout assertions for one page (task 19: shared
 * with the library detail page, which needs the seeded library id). */
function layoutSuite(path: string, ready: string): void {
  test.describe(`layout: ${path}`, () => {
    test('no horizontal overflow', async ({ page }) => {
      await open(page, path, ready)
      const { scrollWidth, innerWidth } = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
      }))
      expect(
        scrollWidth,
        `${path}: the page overflows by ${scrollWidth - innerWidth}px (scrollWidth ${scrollWidth} > viewport ${innerWidth})`,
      ).toBeLessThanOrEqual(innerWidth)
    })

    test('shell covers the viewport', async ({ page }) => {
      await open(page, path, ready)
      const s = await page.evaluate(() => {
        const html = document.documentElement
        const shell = document.querySelector('[data-testid="app-shell"]')
        const rect = shell?.getBoundingClientRect()
        return {
          htmlBg: getComputedStyle(html).backgroundColor,
          shell: rect
            ? {
                top: rect.top,
                left: rect.left,
                right: rect.right,
                bottom: rect.bottom,
              }
            : null,
          viewportW: html.clientWidth,
          viewportH: html.clientHeight,
        }
      })
      // A transparent html background is the white-band regression from
      // the UI review: every region outside the shell renders white.
      expect(
        s.htmlBg,
        'html must paint the page background, not stay transparent',
      ).not.toBe('rgba(0, 0, 0, 0)')
      expect(s.shell, 'the app shell must exist').not.toBeNull()
      if (s.shell) {
        expect(
          s.shell.top,
          `${path}: the shell starts below the viewport top`,
        ).toBeLessThanOrEqual(0)
        expect(
          s.shell.left,
          `${path}: the shell starts right of the viewport left`,
        ).toBeLessThanOrEqual(0)
        expect(
          s.shell.bottom,
          `${path}: unpainted region below the shell (${(s.viewportH - s.shell.bottom).toFixed(1)}px)`,
        ).toBeGreaterThanOrEqual(s.viewportH - 1)
        expect(
          s.shell.right,
          `${path}: unpainted region beside the shell (${(s.shell.right - s.viewportW).toFixed(1)}px)`,
        ).toBeGreaterThanOrEqual(s.viewportW - 1)
      }
    })

    test('nav is reachable', async ({ page }) => {
      await open(page, path, ready)
      const phone = test.info().project.name === 'phone'
      if (phone) {
        // Under `md`: the drawer behind the top bar's hamburger.
        const menu = page.getByTestId('menu-button')
        await expect(menu).toBeVisible()
        await menu.click()
        const drawer = page.getByTestId('nav-drawer')
        await expect(drawer).toBeVisible()
        for (const label of NAV_LABELS) {
          await expect(
            drawer.getByRole('link', { name: label }),
            `drawer link "${label}" not visible`,
          ).toBeVisible()
        }
        await page.keyboard.press('Escape')
        await expect(drawer).toBeHidden()
      } else {
        // `md` and up: the left rail.
        const rail = page.getByTestId('nav-rail')
        await expect(rail).toBeVisible()
        for (const label of NAV_LABELS) {
          await expect(
            rail.getByRole('link', { name: label }),
            `rail link "${label}" not visible`,
          ).toBeVisible()
        }
        await expect(page.getByTestId('menu-button')).toBeHidden()
      }
    })
  })
}

for (const { path, ready } of PAGES) {
  layoutSuite(path, ready)
}

// The library detail page (task 19): same assertions, at the seeded
// library. The setup exports the id; without one (no ffmpeg) the page
// would render its error state, which is not a layout fixture.
const libraryId = process.env.SHRINK_E2E_LIBRARY_ID ?? ''
if (libraryId) {
  layoutSuite(`/libraries/${libraryId}`, 'library-detail')
}

test.describe('file table responsive', () => {
  const libraryId = () => process.env.SHRINK_E2E_LIBRARY_ID ?? ''
  const hasFiles = () => process.env.SHRINK_E2E_HAS_FILES === '1'

  test('phone: the file table renders as cards', async ({ page }) => {
    test.skip(test.info().project.name !== 'phone', 'card layout is phone only')
    test.skip(!hasFiles(), 'no seeded file row (ffmpeg was unavailable)')
    test.skip(!libraryId(), 'no seeded library')
    await open(page, `/libraries/${libraryId()}`, 'library-detail')
    const cards = page.getByTestId('file-cards')
    await expect(cards).toBeVisible()
    // The table itself is gone at phone width, so nothing can be squeezed.
    await expect(page.getByTestId('file-table')).toBeHidden()
    const card = cards.locator('> li')
    const count = await card.count()
    expect(count, 'at least one file card').toBeGreaterThan(0)
    const viewportW = page.viewportSize()?.width ?? 390
    for (let i = 0; i < count; i++) {
      const box = await card.nth(i).boundingBox()
      expect(box, `card ${i} has a layout box`).not.toBeNull()
      if (box) {
        expect(
          box.width,
          `file card ${i} is ${box.width.toFixed(1)}px wide at a ${viewportW}px viewport`,
        ).toBeLessThanOrEqual(viewportW + 1)
      }
    }
  })

  test('tablet and desktop: the file table stays a table', async ({ page }) => {
    test.skip(!hasFiles(), 'no seeded file row (ffmpeg was unavailable)')
    test.skip(!libraryId(), 'no seeded library')
    test.skip(test.info().project.name === 'phone', 'phone shows cards')
    await open(page, `/libraries/${libraryId()}`, 'library-detail')
    await expect(page.getByTestId('file-table')).toBeVisible()
    await expect(page.getByTestId('file-cards')).toBeHidden()
  })
})
