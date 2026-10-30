// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Mobile audit (task 20): the CI gate for phone usability. At 320, 390,
 * 414 and 768 px, every page must have no horizontal overflow and no
 * interactive control inside `main` shorter than 44px — the Apple and
 * WCAG touch minimum. Failures name the page, the width and either the
 * overflow in px or the offending tag, label and rendered height.
 *
 * Like layout.spec (task 12), this runs against the real Go binary
 * serving the built web app, seeded by the shared global setup. The
 * library detail page joins when the seed produced a library id; it
 * skips cleanly when ffmpeg was unavailable.
 *
 * Two reportability exclusions, per the task: controls that are hidden
 * (zero rendered box) and controls with no label at all (nothing to name
 * in the failure message) are skipped. Empty strings and hidden elements
 * are excluded, everything else counts.
 *
 * Runs only under the `phone` project (playwright.config.ts sets
 * `testIgnore` on the other two projects, which exist for layout.spec);
 * the widths are set per test below.
 */
import { expect, test, type Page } from '@playwright/test'

const WIDTHS = [320, 390, 414, 768]

/** The page's own stable test id, used as the ready marker. */
const PAGES: Array<{ path: string; ready: string; settled: string }> = [
  // The dashboard settles once the nodes query resolves: warnings, the
  // activity list and the empty state are all driven by it.
  {
    path: '/',
    ready: 'dashboard',
    settled: '[data-testid="nodes-activity"], [data-testid="nodes-empty"]',
  },
  {
    path: '/libraries',
    ready: 'libraries-page',
    settled: '[data-testid="libraries-cards"], [data-testid="libraries-table"]',
  },
  {
    path: '/nodes',
    ready: 'nodes-page',
    settled: '[data-testid="nodes-list"] li, [data-testid="nodes-empty"]',
  },
  {
    path: '/encode',
    ready: 'encode-settings-page',
    // The form (and its Save button) only render once the settings load.
    settled: '[data-testid="save-settings"]',
  },
  {
    path: '/app',
    ready: 'app-settings-page',
    settled: '[data-testid="worker-concurrency"], [data-testid="about-card"]',
  },
]

/** Small controls matching these selectors are audited, per the task. */
const CONTROL_SELECTOR =
  'main button, main a, main input, main select, main textarea'

interface SmallControl {
  tag: string
  label: string
  height: number
}

async function open(page: Page, path: string, ready: string): Promise<void> {
  await page.goto(path)
  await expect(page.getByTestId(ready), `${path} did not render`).toBeVisible()
}

/**
 * Waits for the page's data-driven content so the audit never measures a
 * skeleton. `waitForLoadState('networkidle')` cannot be used: the shell
 * holds a permanent SSE connection, so the network never goes idle.
 */
async function settle(page: Page, settled: string): Promise<void> {
  try {
    await page.waitForSelector(settled, {
      state: 'attached',
      timeout: 10_000,
    })
  } catch {
    // A page in its error state has no data marker; the audit below
    // measures whatever rendered.
  }
}

async function auditPage(
  page: Page,
  path: string,
  width: number,
): Promise<void> {
  const { overflow, scrollWidth, innerWidth } = await page.evaluate(() => ({
    overflow: document.documentElement.scrollWidth - window.innerWidth,
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  expect(
    overflow,
    `${path} at ${width}px: the page overflows horizontally by ${overflow}px (scrollWidth ${scrollWidth} > viewport ${innerWidth})`,
  ).toBeLessThanOrEqual(0)

  const small = await page.evaluate((selector): SmallControl[] => {
    return [...document.querySelectorAll(selector)]
      .map((el) => {
        const box = el.getBoundingClientRect()
        const label = (
          el.textContent ||
          el.getAttribute('aria-label') ||
          el.getAttribute('placeholder') ||
          ''
        )
          .trim()
          .slice(0, 40)
        return {
          tag: el.tagName.toLowerCase(),
          label,
          height: Math.round(box.height),
        }
      })
      .filter((t) => t.label !== '' && t.height > 0 && t.height < 44)
  }, CONTROL_SELECTOR)
  expect(
    small,
    `${path} at ${width}px: ${small.length} control(s) under 44px tall: ${JSON.stringify(small)}`,
  ).toEqual([])
}

// The width loop measures the real layout, so it runs once — under the
// phone project only, with the viewport set per width.
test.describe('mobile audit', () => {
  for (const width of WIDTHS) {
    test(`no overflow and 44px touch targets at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 844 })
      for (const { path, ready, settled } of PAGES) {
        await open(page, path, ready)
        await settle(page, settled)
        await auditPage(page, path, width)
      }
      // The detail page joins when the seeded library exists.
      const libraryId = process.env.SHRINK_E2E_LIBRARY_ID ?? ''
      if (libraryId) {
        const path = `/libraries/${libraryId}`
        await open(page, path, 'library-detail')
        await settle(
          page,
          '[data-testid="file-cards"], [data-testid="file-table"]',
        )
        await auditPage(page, path, width)
      }
    })
  }
})
