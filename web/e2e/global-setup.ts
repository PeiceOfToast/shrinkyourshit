// SPDX-License-Identifier: AGPL-3.0-only

/**
 * E2e fixture (task 12): every run drives the real Go binary serving the
 * built web app — never the Vite dev server.
 *
 * - With SHRINK_E2E_BASE_URL set, that server is already running (CI runs
 *   the suite against the docker image); this setup only completes
 *   first-run setup and seeds state through the API.
 * - Without it, spawn bin/shrinkyourshit with throwaway data/scratch
 *   dirs, a private port and discovery off; global-teardown stops it.
 *
 * Seeding (both modes): POST /api/v1/setup with login disabled, then one
 * library over a folder holding a small ffmpeg-generated test clip
 * (AGENTS rule 12: generated on demand, never committed). The first scan
 * records it, so the file table has a row for the card-layout assertion.
 * Facts are exported as SHRINK_E2E_* env vars and read by the specs.
 */
import { spawn } from 'node:child_process'
import { execFile } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdir, mkdtemp, open, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { runtimeDir, runtimeFile } from './runtime'

const PORT = process.env.SHRINK_E2E_PORT ?? '8712'
const baseURL = process.env.SHRINK_E2E_BASE_URL ?? `http://127.0.0.1:${PORT}`
const API = `${baseURL}/api/v1`

interface JSONResponse {
  status: number
  body: { code?: string; [k: string]: unknown }
}

async function api(route: string, init?: RequestInit): Promise<JSONResponse> {
  const res = await fetch(`${API}${route}`, {
    ...init,
    headers: { 'content-type': 'application/json', ...init?.headers },
  })
  const text = await res.text()
  return {
    status: res.status,
    body: text ? (JSON.parse(text) as JSONResponse['body']) : {},
  }
}

async function waitHealthy(): Promise<void> {
  const deadline = Date.now() + 30_000
  for (;;) {
    try {
      const res = await fetch(`${API}/health`)
      if (res.ok) return
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) {
      throw new Error(`the server at ${baseURL} never became healthy`)
    }
    await new Promise((r) => setTimeout(r, 250))
  }
}

/** First-run setup with login off, so every admin route stays open. */
async function completeSetup(): Promise<void> {
  const { status, body } = await api('/setup', {
    method: 'POST',
    body: JSON.stringify({ mode: 'server', login_enabled: false }),
  })
  if (status === 201) return
  // A previous run already completed setup on this server.
  if (status === 409 && body.code === 'setup_complete') return
  throw new Error(`setup failed: ${status} ${JSON.stringify(body)}`)
}

function ffmpegBin(): Promise<string | null> {
  const bin = process.env.SHRINK_E2E_FFMPEG ?? 'ffmpeg'
  return new Promise((resolve) => {
    execFile(bin, ['-version'], (err) => resolve(err ? null : bin))
  })
}

/**
 * One small test clip (1s testsrc + sine) as the seeded library file.
 * libx264 first, mpeg4 as the fallback for builds without libx264.
 * Returns the path, or '' when no encoder produced a file.
 */
async function generateMedia(ffmpeg: string, dir: string): Promise<string> {
  await rm(dir, { recursive: true, force: true })
  await mkdir(dir, { recursive: true })
  const out = path.join(dir, 'sample.mkv')
  const base = [
    '-hide_banner',
    '-loglevel',
    'error',
    '-y',
    '-f',
    'lavfi',
    '-i',
    'testsrc=duration=1:size=320x240:rate=10',
    '-f',
    'lavfi',
    '-i',
    'sine=duration=1:frequency=440',
  ]
  const tryEncode = (vcodec: string[]) =>
    new Promise<boolean>((resolve) => {
      execFile(
        ffmpeg,
        [...base, ...vcodec, '-c:a', 'aac', '-shortest', out],
        (err) => resolve(!err),
      )
    })
  if (await tryEncode(['-c:v', 'libx264', '-pix_fmt', 'yuv420p'])) return out
  if (await tryEncode(['-c:v', 'mpeg4', '-q:v', '6'])) return out
  return ''
}

/** The easy-mode Target the seeded library carries (ARCHITECTURE.md). */
const TARGET = {
  video: { codec: 'hevc', quality: 3, reencode_if_same_codec: false },
  audio: {
    mode: 'keep_all',
    preferred_languages: ['eng'],
    compact_codec: 'aac',
  },
  subtitles: { mode: 'keep_all', preferred_languages: ['eng'] },
  container: 'mkv',
}

/**
 * Creates the library through the API and waits for the scan to record
 * the seeded file. The library is always created so every page renders
 * real content; the file row only exists when ffmpeg probed the clip.
 */
async function seedLibrary(
  serverPath: string,
  hasMedia: boolean,
): Promise<void> {
  const res = await api('/libraries', {
    method: 'POST',
    body: JSON.stringify({
      name: 'Movies',
      path: serverPath,
      watch: false,
      target: TARGET,
    }),
  })
  if (res.status !== 201) {
    throw new Error(
      `library create failed: ${res.status} ${JSON.stringify(res.body)}`,
    )
  }
  const id = res.body.id as string
  process.env.SHRINK_E2E_LIBRARY_ID = id
  if (!hasMedia) return
  // The create kicked off the first scan; the probe runs after the 5s
  // stability window, so poll until the row exists.
  const deadline = Date.now() + 60_000
  for (;;) {
    const files = await api(`/libraries/${id}/files`)
    if ((files.body.total as number) > 0) {
      process.env.SHRINK_E2E_HAS_FILES = '1'
      return
    }
    if (Date.now() > deadline) {
      throw new Error('the scan never recorded the seeded test file')
    }
    await new Promise((r) => setTimeout(r, 500))
  }
}

/** Managed mode: start the real binary with throwaway dirs. */
async function startServer(): Promise<void> {
  // Nothing else may answer on the port — a stale server from an earlier
  // run would silently take over this run's assertions.
  try {
    await fetch(`${API}/health`)
    throw new Error(
      `port ${PORT} already serves a server; set SHRINK_E2E_PORT or stop the old one`,
    )
  } catch (err) {
    if (err instanceof Error && err.message.startsWith('port')) throw err
    // expected: connection refused
  }

  const repo = path.resolve(process.cwd(), '..') // web/ is the cwd
  const bin =
    process.env.SHRINK_E2E_BIN ?? path.join(repo, 'bin', 'shrinkyourshit')
  if (!existsSync(bin)) {
    throw new Error(
      `e2e needs the built binary at ${bin} — run "make e2e" (or "make build") first`,
    )
  }
  const work = await mkdtemp(path.join(tmpdir(), 'shrink-e2e-'))
  const log = await open(path.join(work, 'server.log'), 'w')
  const child = spawn(bin, ['serve'], {
    env: {
      ...process.env,
      SHRINK_PORT: PORT,
      SHRINK_DATA_DIR: path.join(work, 'data'),
      SHRINK_SCRATCH_DIR: path.join(work, 'scratch'),
      SHRINK_DISCOVERY_MDNS: 'false',
      SHRINK_DISCOVERY_BROADCAST: 'false',
    },
    stdio: ['ignore', log.fd, log.fd],
  })
  await log.close()
  child.on('exit', (code) => {
    if (code !== 0 && code !== null) {
      console.error(`[e2e] server exited with ${code}; log: ${work}/server.log`)
    }
  })
  await writeFile(runtimeFile, JSON.stringify({ pid: child.pid, work }))
}

export default async function globalSetup(): Promise<void> {
  process.env.SHRINK_E2E_LIBRARY_ID ??= ''
  await rm(runtimeDir, { recursive: true, force: true })
  await mkdir(runtimeDir, { recursive: true })

  if (!process.env.SHRINK_E2E_BASE_URL) {
    await startServer()
  }
  await waitHealthy()
  await completeSetup()

  const mediaDir =
    process.env.SHRINK_E2E_MEDIA_DIR ??
    path.join(tmpdir(), 'shrink-e2e-media', 'e2e-lib')
  // The path as the server sees it (CI: the container's volume mount).
  const serverPath = process.env.SHRINK_E2E_LIBRARY_PATH ?? mediaDir
  const ffmpeg = await ffmpegBin()
  let hasMedia = false
  if (ffmpeg) {
    hasMedia = (await generateMedia(ffmpeg, mediaDir)) !== ''
    if (!hasMedia) {
      console.warn('[e2e] ffmpeg could not produce a test clip')
    }
  } else {
    console.warn(
      '[e2e] no ffmpeg in PATH; the file table stays empty and the card-layout test skips',
    )
  }
  await seedLibrary(serverPath, hasMedia)
  console.log(
    `[e2e] server ${baseURL}, library ${process.env.SHRINK_E2E_LIBRARY_ID}, files ${
      process.env.SHRINK_E2E_HAS_FILES === '1' ? 'seeded' : 'none'
    }`,
  )
}
