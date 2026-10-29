// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Stops the server the global setup spawned (managed mode only) and
 * removes its throwaway dirs. An external server (CI runs the suite
 * against the docker image) is left alone.
 */
import { readFile, rm } from 'node:fs/promises'
import { runtimeDir, runtimeFile } from './runtime'

interface Runtime {
  pid?: number
  work?: string
}

export default async function globalTeardown(): Promise<void> {
  let rt: Runtime = {}
  try {
    rt = JSON.parse(await readFile(runtimeFile, 'utf8')) as Runtime
  } catch {
    return // nothing this run started
  }
  if (rt.pid) {
    const deadline = Date.now() + 5_000
    try {
      process.kill(rt.pid, 'SIGTERM')
    } catch {
      // already gone
    }
    while (Date.now() < deadline) {
      try {
        process.kill(rt.pid, 0)
      } catch {
        break // exited
      }
      await new Promise((r) => setTimeout(r, 100))
    }
    try {
      process.kill(rt.pid, 'SIGKILL')
    } catch {
      // already gone
    }
  }
  if (rt.work) {
    await rm(rt.work, { recursive: true, force: true })
  }
  await rm(runtimeDir, { recursive: true, force: true })
}
