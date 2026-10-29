// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Shared fixture paths (task 12). The global setup writes the managed
 * server's runtime facts here; the teardown reads them. Lives in the
 * system temp dir so no bookkeeping file ever lands in a checkout.
 */
import { tmpdir } from 'node:os'
import path from 'node:path'

export const runtimeDir = path.join(tmpdir(), 'shrinkyourshit-e2e')
export const runtimeFile = path.join(runtimeDir, 'server.json')
