// SPDX-License-Identifier: AGPL-3.0-only
import react from '@vitejs/plugin-react'
import { configDefaults, defineConfig } from 'vitest/config'

// The dev server proxies /api to the Go server so `make dev` works with one
// command: the UI runs on 5173, the API on 8765.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8765',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    globals: true,
    // e2e/ is the Playwright suite (task 12); it runs via `npm run e2e`.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
})
