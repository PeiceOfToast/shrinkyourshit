// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Playwright layout suite (task 12). Three fixed viewports — phone
 * (390x844), tablet (768x1024) and desktop (1280x800) — against the real
 * Go binary serving the built web app, never the Vite dev server.
 *
 * The global setup (e2e/global-setup.ts) either starts `bin/shrinkyourshit`
 * itself or, when SHRINK_E2E_BASE_URL is set, drives an already running
 * server (CI runs the suite against the docker image). It completes
 * first-run setup and seeds one library with a file so every page has
 * real content before any assertion runs.
 */
import { defineConfig } from '@playwright/test'

const external = process.env.SHRINK_E2E_BASE_URL

export default defineConfig({
  testDir: './e2e',
  // One shared server and shared seeded state; workers must not race it.
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 30_000,
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  use: {
    baseURL:
      external ?? `http://127.0.0.1:${process.env.SHRINK_E2E_PORT ?? '8712'}`,
    // One screenshot per failing page per viewport is the CI artifact.
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never' }]]
    : [['list']],
  outputDir: './test-results',
  projects: [
    { name: 'phone', use: { viewport: { width: 390, height: 844 } } },
    {
      name: 'tablet',
      use: { viewport: { width: 768, height: 1024 } },
      // The mobile audit (task 20) sets its own widths per test and runs
      // once — the tablet and desktop projects exist for layout.spec.
      testIgnore: /mobile\.spec\.ts/,
    },
    {
      name: 'desktop',
      use: { viewport: { width: 1280, height: 800 } },
      testIgnore: /mobile\.spec\.ts/,
    },
  ],
})
