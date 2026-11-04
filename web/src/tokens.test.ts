// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from 'vitest'
import config from '../tailwind.config.js'

// Design tokens, task 36 (docs/design/ui-redesign-2026-11-04.md 2.2 to 2.9).
// These tests read the same files the build uses, so a drift between the
// spec's variables, the Tailwind mapping and the stylesheet fails here.

// Vitest blanks every .css import, including ?raw, so the stylesheets are read
// from disk. Paths are relative to the web/ directory, where vitest runs (make
// test-web and npm run test both cd there). The module id is held in a variable
// so type-checking does not need Node typings, which this project does not install.
const nodeFs = 'node:fs'
const { readFileSync } = (await import(/* @vite-ignore */ nodeFs)) as {
  readFileSync: (path: string, encoding: 'utf8') => string
}
const indexCss = readFileSync('src/index.css', 'utf8')
const reactFlowCss = readFileSync(
  'node_modules/@xyflow/react/dist/style.css',
  'utf8',
)

type Colors = Record<string, string | Record<string, string>>

const extend = (config.theme?.extend ?? {}) as unknown as {
  colors: Colors
  fontSize: Record<string, unknown>
  transitionDuration: Record<string, string>
  fontFamily: Record<string, string[]>
}

const colorKeys = Object.keys(extend.colors)

/** Every `--c-*` name declared in the :root block of index.css. */
function declaredTokens(): string[] {
  const root = indexCss.match(/:root\s*\{([^}]*)\}/)
  expect(root, ':root block present in index.css').not.toBeNull()
  return [...root![1].matchAll(/--(c-[a-z-]+)\s*:/g)].map((m) => m[1])
}

/** Every `rgb(var(--c-…))` name the Tailwind colour map references. */
function referencedTokens(): string[] {
  const out: string[] = []
  const walk = (value: string | Record<string, string>) => {
    if (typeof value === 'string') {
      for (const m of value.matchAll(/var\(--(c-[a-z-]+)\)/g)) out.push(m[1])
    } else {
      Object.values(value).forEach(walk)
    }
  }
  Object.values(extend.colors).forEach(walk)
  return out
}

describe('Tailwind colour tokens', () => {
  it('exposes the colour keys the spec names', () => {
    for (const key of [
      'canvas',
      'surface',
      'raised',
      'line',
      'fg',
      'accent',
      'focus',
      'success',
      'info',
      'warning',
      'danger',
    ]) {
      expect(colorKeys, key).toContain(key)
    }
  })

  it('maps exactly the 20 variables declared in :root (2.2 and 2.9)', () => {
    const declared = declaredTokens()
    expect(declared).toHaveLength(20)
    expect(new Set(referencedTokens())).toEqual(new Set(declared))
  })

  it('declares the sub-keys used by the 2.2 class roots', () => {
    expect(extend.colors.raised).toHaveProperty('hover')
    expect(extend.colors.line).toHaveProperty('strong')
    expect(extend.colors.fg).toHaveProperty('muted')
    expect(extend.colors.fg).toHaveProperty('subtle')
    expect(extend.colors.accent).toHaveProperty('hover')
    expect(extend.colors.accent).toHaveProperty('fg')
    expect(extend.colors.accent).toHaveProperty('text')
    expect(extend.colors.danger).toHaveProperty('solid')
    expect(extend.colors.danger).toHaveProperty('solid-hover')
  })
})

describe('Tailwind type and motion tokens', () => {
  it('exposes the font sizes caption, body, heading, title', () => {
    for (const size of ['caption', 'body', 'heading', 'title']) {
      expect(extend.fontSize, size).toHaveProperty(size)
    }
  })

  it('exposes the durations fast, base, slow', () => {
    for (const d of ['fast', 'base', 'slow']) {
      expect(extend.transitionDuration, d).toHaveProperty(d)
    }
    expect(extend.transitionDuration).toEqual({
      fast: '120ms',
      base: '200ms',
      slow: '300ms',
    })
  })

  it('puts Inter Variable first in the sans stack', () => {
    expect(extend.fontFamily.sans[0]).toBe('"Inter Variable"')
    expect(extend.fontFamily.sans.length).toBeGreaterThan(1)
  })
})

describe('index.css', () => {
  it('draws the focus ring in sky-300 (--c-focus), not emerald', () => {
    const rule = indexCss.match(/:focus-visible\s*\{([^}]*)\}/)
    expect(rule).not.toBeNull()
    expect(rule![1]).toContain('rgb(var(--c-focus))')
    expect(rule![1]).not.toContain('emerald')
  })

  it('has the reduced-motion rule that zeroes durations', () => {
    const media = indexCss.match(
      /@media \(prefers-reduced-motion: reduce\)\s*\{([\s\S]*)\}\s*\/\*/,
    )
    expect(media).not.toBeNull()
    expect(media![1]).toContain('transition-duration: 0.01ms')
    expect(media![1]).toContain('animation-iteration-count: 1')
  })

  it('sets every React Flow variable from 2.9, and each exists in the installed React Flow CSS', () => {
    const xyVars = [
      '--xy-background-color',
      '--xy-background-pattern-color',
      '--xy-edge-stroke',
      '--xy-edge-stroke-selected',
      '--xy-minimap-background-color',
      '--xy-minimap-mask-background-color',
      '--xy-controls-button-background-color',
      '--xy-controls-button-background-color-hover',
      '--xy-controls-button-color',
      '--xy-controls-button-border-color',
      '--xy-attribution-background-color',
    ]
    for (const name of xyVars) {
      expect(indexCss, `${name} set in index.css`).toContain(`${name}:`)
      expect(
        reactFlowCss,
        `${name} exists in @xyflow/react style.css`,
      ).toContain(name)
    }
  })

  it('keeps the 44px React Flow control and attribution rules', () => {
    expect(indexCss).toContain('main .react-flow__controls-button')
    expect(indexCss).toContain('min-height: 44px')
  })

  it('declares the latin Inter face once, with a unicode-range', () => {
    const faces = indexCss.match(/@font-face\s*\{[^}]*\}/g) ?? []
    expect(faces).toHaveLength(1)
    expect(faces[0]).toContain("font-family: 'Inter Variable'")
    expect(faces[0]).toContain('inter-latin-wght-normal.woff2')
    expect(faces[0]).toContain('unicode-range')
  })
})
