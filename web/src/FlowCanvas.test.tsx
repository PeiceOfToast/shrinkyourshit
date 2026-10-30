// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The flow canvas (task 15 acceptance): read-only rendering of the graph
 * the API returns, a palette built from the step-types response, an
 * invalid edit showing the server's field error on the node and blocking
 * Save, add → connect → save persisting with source=advanced, and the
 * unsaved-changes guard. The validate mock mirrors the server's rules —
 * the client itself never decides validity.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FlowCanvasPage from './FlowCanvasPage'
import type { FlowFieldError, FlowGraph, StepTypeInfo } from './api'
import {
  installFetchMock,
  type FetchMock,
  type MockRoute,
} from './test/fetchMock'
import { ToastProvider } from './toast'

const PALETTE: StepTypeInfo[] = [
  {
    type: 'probe',
    label: 'Check the file',
    description: 'Read the file details.',
    condition: false,
    params: [],
  },
  {
    type: 'if_codec_not',
    label: 'If the video is not…',
    description: 'A condition on the video.',
    condition: true,
    params: [
      {
        name: 'codec',
        label: 'codec',
        kind: 'select',
        options: ['hevc', 'av1', 'h264'],
        required: true,
        default: 'hevc',
      },
    ],
  },
  {
    type: 'transcode_video',
    label: 'Encode the video',
    description: '',
    condition: false,
    params: [
      {
        name: 'codec',
        label: 'codec',
        kind: 'select',
        options: ['hevc', 'av1', 'h264'],
        required: true,
        default: 'hevc',
      },
      {
        name: 'quality',
        label: 'quality',
        kind: 'number',
        min: 1,
        max: 5,
        required: true,
        default: 3,
      },
    ],
  },
  {
    type: 'convert_audio',
    label: 'Choose the audio tracks',
    description: '',
    condition: false,
    params: [
      {
        name: 'mode',
        label: 'mode',
        kind: 'select',
        options: ['keep_all', 'keep_preferred', 'keep_best', 'compact'],
        required: true,
        default: 'keep_best',
      },
    ],
  },
  {
    type: 'subtitles',
    label: 'Choose the subtitles',
    description: '',
    condition: false,
    params: [
      {
        name: 'mode',
        label: 'mode',
        kind: 'select',
        options: ['keep_all', 'keep_preferred', 'remove'],
        required: true,
        default: 'keep_all',
      },
    ],
  },
  {
    type: 'remux_container',
    label: 'Choose the container',
    description: '',
    condition: false,
    params: [
      {
        name: 'container',
        label: 'container',
        kind: 'select',
        options: ['mkv', 'mp4'],
        required: true,
        default: 'mkv',
      },
    ],
  },
  // The palette is whatever this endpoint returns — a type that only
  // exists in this fixture must appear on it (task 15 acceptance).
  {
    type: 'fancy_step',
    label: 'Fancy step',
    description: 'Only in this test fixture.',
    condition: false,
    params: [],
  },
]

/** The graph an easy-mode library generates (task 14 compiler shape). */
function easyGraph(): FlowGraph {
  return {
    schema: 1,
    nodes: [
      { id: 'probe', type: 'probe' },
      {
        id: 'transcode_video',
        type: 'transcode_video',
        params: { codec: 'hevc', quality: 3 },
      },
      {
        id: 'convert_audio',
        type: 'convert_audio',
        params: { mode: 'keep_best', languages: ['eng'], compact_codec: 'aac' },
      },
      {
        id: 'subtitles',
        type: 'subtitles',
        params: {
          mode: 'keep_all',
          languages: ['eng'],
          extract_removed: false,
        },
      },
      {
        id: 'remux_container',
        type: 'remux_container',
        params: { container: 'mkv' },
      },
    ],
    edges: [
      { from: 'probe', to: 'transcode_video' },
      { from: 'transcode_video', to: 'convert_audio' },
      { from: 'convert_audio', to: 'subtitles' },
      { from: 'subtitles', to: 'remux_container' },
    ],
  }
}

const SHAPING = new Set([
  'transcode_video',
  'remux_container',
  'convert_audio',
  'subtitles',
  'rename',
  'skip_if_smaller',
])

/** A stand-in for the server's validator — the same rules the Go side
 * checks, enough for every scenario in this file. */
function flowProblems(graph: FlowGraph): FlowFieldError[] {
  const errors: FlowFieldError[] = []
  graph.nodes.forEach((n, i) => {
    if (!PALETTE.some((p) => p.type === n.type)) {
      errors.push({
        path: `nodes[${i}].type`,
        message: `unknown step type "${n.type}"`,
      })
    }
  })
  const counts = new Map<string, number>()
  graph.nodes.forEach((n) => counts.set(n.type, (counts.get(n.type) ?? 0) + 1))
  graph.nodes.forEach((n, i) => {
    if (SHAPING.has(n.type) && (counts.get(n.type) ?? 0) > 1) {
      errors.push({
        path: `nodes[${i}].type`,
        message: `only one ${n.type} step is allowed`,
      })
    }
  })
  const params = (n: (typeof graph.nodes)[number]) =>
    (n.params ?? {}) as Record<string, unknown>
  graph.nodes.forEach((n, i) => {
    if (
      (n.type === 'if_codec_not' || n.type === 'transcode_video') &&
      !params(n).codec
    ) {
      errors.push({
        path: `nodes[${i}].params.codec`,
        message: 'want hevc, av1 or h264',
      })
    }
  })
  const out = new Map<string, number>()
  const inn = new Map<string, number>()
  for (const e of graph.edges) {
    out.set(e.from, (out.get(e.from) ?? 0) + 1)
    inn.set(e.to, (inn.get(e.to) ?? 0) + 1)
  }
  graph.nodes.forEach((n, i) => {
    if ((out.get(n.id) ?? 0) > 1 || (inn.get(n.id) ?? 0) > 1) {
      errors.push({
        path: `nodes[${i}].id`,
        message: `step "${n.id}" has more than one connection; v1 flows are a single chain`,
      })
      return
    }
    if (!out.get(n.id) && !inn.get(n.id)) {
      errors.push({
        path: `nodes[${i}].id`,
        message: `step "${n.id}" is not connected to the flow`,
      })
    }
  })
  return errors
}

function validateRoute(): MockRoute {
  return {
    method: 'POST',
    match: /\/api\/v1\/flows\/validate$/,
    statusFn: ({ body }) =>
      flowProblems((body as { graph: FlowGraph }).graph).length > 0 ? 400 : 200,
    bodyFn: ({ body }) => {
      const problems = flowProblems((body as { graph: FlowGraph }).graph)
      return problems.length > 0
        ? {
            error: 'the flow has errors',
            code: 'invalid_flow',
            errors: problems,
          }
        : { valid: true }
    },
  }
}

function libraryDoc(flowSource: string) {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: {},
    easy: null,
    flow: null,
    flow_source: flowSource,
    watch: false,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
  }
}

function flowDoc(source: string, graph: FlowGraph) {
  return { library_id: 'lib-1', source, graph }
}

function standardRoutes(
  source = 'easy',
  graph: FlowGraph = easyGraph(),
): MockRoute[] {
  return [
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/lib-1$/,
      body: libraryDoc(source),
    },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/lib-1\/flow$/,
      body: flowDoc(source, graph),
    },
    {
      method: 'GET',
      match: /\/api\/v1\/flows\/step-types$/,
      body: { items: PALETTE, total: PALETTE.length },
    },
    validateRoute(),
  ]
}

function renderFlow(initial = '/libraries/lib-1/flow') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={[initial]}>
          <Routes>
            <Route path="/libraries/:id/flow" element={<FlowCanvasPage />} />
            <Route
              path="/libraries/:id/rules"
              element={<div data-testid="rules-stub" />}
            />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

async function enterEditMode(source = 'advanced') {
  mock = installFetchMock(standardRoutes(source))
  renderFlow()
  await waitFor(() => screen.getByTestId('flow-edit'))
  fireEvent.click(screen.getByTestId('flow-edit'))
  await waitFor(() => screen.getByTestId('flow-palette'))
}

let mock: FetchMock
afterEach(() => mock?.restore())

describe('FlowCanvas read only (phase 1)', () => {
  it('renders the easy graph read only with the right number of nodes and edges', async () => {
    mock = installFetchMock(standardRoutes('easy'))
    renderFlow()
    await waitFor(() => screen.getByTestId('flow-canvas-page'))
    // The same graph object the API returned: every step is a node and the
    // counts match exactly.
    expect(screen.getByTestId('flow-summary').textContent).toBe(
      '5 steps · 4 connections',
    )
    await waitFor(() =>
      expect(screen.getByTestId('canvas-node-probe')).toBeInTheDocument(),
    )
    for (const id of [
      'transcode_video',
      'convert_audio',
      'subtitles',
      'remux_container',
    ]) {
      expect(screen.getByTestId(`canvas-node-${id}`)).toBeInTheDocument()
    }
    // Read only: no palette, no save, an explicit banner instead.
    expect(screen.getByTestId('flow-readonly-banner')).toBeInTheDocument()
    expect(screen.queryByTestId('flow-palette')).not.toBeInTheDocument()
    expect(screen.queryByTestId('flow-save')).not.toBeInTheDocument()
  })

  it('shows the read-only inspector for a selected step', async () => {
    mock = installFetchMock(standardRoutes('easy'))
    renderFlow()
    await waitFor(() => screen.getByTestId('canvas-node-transcode_video'))
    // jsdom has no layout: the step-types response can re-render the
    // canvas right between the node appearing and the click, so the
    // synthetic click can land on a detached element — no select change,
    // no inspector (seen ~1 in 8 runs, on main too). Clicking again
    // inside waitFor makes it deterministic; a repeated click on an
    // already selected step is idempotent.
    await waitFor(() => {
      fireEvent.click(screen.getByTestId('canvas-node-transcode_video'))
      screen.getByTestId('flow-inspector')
    })
    expect(
      screen.getByTestId('flow-inspector-chips-transcode_video').textContent,
    ).toContain('quality 3')
    expect(screen.queryByTestId('flow-param-quality')).not.toBeInTheDocument()
  })
})

describe('FlowCanvas editing (phase 2)', () => {
  it('builds the palette from the API response, including a fixture-only type', async () => {
    await enterEditMode('advanced')
    // Every type of the response is on the palette — including the one
    // that only exists in the test fixture.
    expect(screen.getByTestId('palette-add-probe')).toBeInTheDocument()
    expect(screen.getByTestId('palette-add-if_codec_not')).toBeInTheDocument()
    expect(screen.getByTestId('palette-add-fancy_step')).toBeInTheDocument()
  })

  it('an orphan step shows the server error on the node and blocks Save', async () => {
    await enterEditMode('advanced')
    fireEvent.click(screen.getByTestId('palette-add-if_codec_not'))
    // The server flags the orphan; the error renders on the node itself.
    await waitFor(() =>
      expect(
        screen.getByTestId('canvas-node-error-if_codec_not').textContent,
      ).toContain('not connected'),
    )
    const save = screen.getByTestId('flow-save')
    await waitFor(() => expect(save).toBeDisabled())
    expect(screen.getByTestId('flow-save-reason').textContent).toContain('red')
    // An invalid graph never saves.
    expect(mock.calls('PUT', /flow$/)).toHaveLength(0)
  })

  it('adding a step, connecting it and saving persists source=advanced', async () => {
    let savedGraph: FlowGraph | undefined
    let savedSource: string | undefined
    mock = installFetchMock([
      ...standardRoutes('advanced'),
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        bodyFn: ({ body }) => {
          const b = body as { source: string; graph: FlowGraph }
          savedSource = b.source
          savedGraph = b.graph
          return flowDoc('advanced', b.graph)
        },
      },
    ])
    renderFlow()
    await waitFor(() => screen.getByTestId('flow-edit'))
    fireEvent.click(screen.getByTestId('flow-edit'))
    await waitFor(() => screen.getByTestId('palette-add-if_codec_not'))
    fireEvent.click(screen.getByTestId('palette-add-if_codec_not'))
    await waitFor(() => screen.getByTestId('canvas-node-if_codec_not'))

    // Connect through the inspector — the menu path, no drag needed.
    // First hop: the orphan connects forward into the chain, which
    // displaces the transcode step's input — the server flags the probe.
    fireEvent.click(screen.getByTestId('canvas-node-if_codec_not'))
    await waitFor(() => screen.getByTestId('flow-inspector-connect-select'))
    fireEvent.change(screen.getByTestId('flow-inspector-connect-select'), {
      target: { value: 'transcode_video' },
    })
    fireEvent.click(screen.getByTestId('flow-inspector-connect'))
    await waitFor(() =>
      expect(
        screen.getByTestId('canvas-node-error-probe').textContent,
      ).toContain('not connected'),
    )
    // Second hop: reconnect the probe in front of the guard.
    fireEvent.click(screen.getByTestId('canvas-node-probe'))
    await waitFor(() => screen.getByTestId('flow-inspector-connect-select'))
    fireEvent.change(screen.getByTestId('flow-inspector-connect-select'), {
      target: { value: 'if_codec_not' },
    })
    fireEvent.click(screen.getByTestId('flow-inspector-connect'))
    await waitFor(() => expect(screen.getByTestId('flow-save')).toBeEnabled())
    fireEvent.click(screen.getByTestId('flow-save'))

    await waitFor(() => expect(savedSource).toBe('advanced'))
    expect(savedGraph!.nodes.map((n) => n.id)).toContain('if_codec_not')
    expect(savedGraph!.edges).toContainEqual({
      from: 'probe',
      to: 'if_codec_not',
    })
    expect(savedGraph!.edges).toContainEqual({
      from: 'if_codec_not',
      to: 'transcode_video',
    })
    // The refetched flow reports the new source.
    await waitFor(() =>
      expect(screen.getByTestId('flow-source').textContent).toBe('advanced'),
    )
  })

  it('returns to read only via Done editing', async () => {
    await enterEditMode('advanced')
    fireEvent.click(screen.getByTestId('flow-done'))
    await waitFor(() =>
      expect(screen.getByTestId('flow-readonly-banner')).toBeInTheDocument(),
    )
  })
})

describe('FlowCanvas unsaved-changes guard', () => {
  it('warns before leaving and can stay or leave', async () => {
    await enterEditMode('advanced')
    fireEvent.click(screen.getByTestId('palette-add-if_codec_not'))
    await waitFor(() => screen.getByTestId('flow-dirty'))
    // Stay: the dialog cancels the navigation.
    fireEvent.click(screen.getByTestId('flow-back'))
    await waitFor(() => screen.getByTestId('flow-guard-dialog'))
    fireEvent.click(screen.getByTestId('flow-guard-stay'))
    await waitFor(() =>
      expect(screen.queryByTestId('flow-guard-dialog')).not.toBeInTheDocument(),
    )
    expect(screen.getByTestId('flow-canvas-page')).toBeInTheDocument()
    // Leave: the dialog navigates.
    fireEvent.click(screen.getByTestId('flow-back'))
    await waitFor(() => screen.getByTestId('flow-guard-dialog'))
    fireEvent.click(screen.getByTestId('flow-guard-leave'))
    await waitFor(() => screen.getByTestId('rules-stub'))
  })
})

describe('FlowCanvas mobile (task 22)', () => {
  /**
   * Forces the phone layout: every media query answers false. Without the
   * stub jsdom has no matchMedia and useMediaQuery defaults to wide —
   * exactly what the desktop tests above rely on.
   */
  function stubNarrow() {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      })),
    )
  }

  it('is view only below md: notice, no editing UI, canvas still works', async () => {
    stubNarrow()
    mock = installFetchMock(standardRoutes('easy'))
    renderFlow()
    await waitFor(() => screen.getByTestId('flow-canvas-page'))
    await waitFor(() =>
      expect(screen.getByTestId('canvas-node-probe')).toBeInTheDocument(),
    )
    // The notice explains why editing is not offered here.
    expect(screen.getByTestId('flow-mobile-notice').textContent).toBe(
      'Viewing only on a phone. Editing the flow needs a desktop browser.',
    )
    // No palette, no inspector, no edit controls, no banner CTA.
    expect(screen.queryByTestId('flow-palette')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('flow-inspector-empty'),
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId('flow-edit')).not.toBeInTheDocument()
    expect(screen.queryByTestId('flow-edit-banner')).not.toBeInTheDocument()
    expect(screen.queryByTestId('flow-save')).not.toBeInTheDocument()
    expect(screen.queryByTestId('flow-readonly-banner')).not.toBeInTheDocument()
    // The graph itself is still readable: same node count as desktop.
    expect(screen.getByTestId('flow-summary').textContent).toBe(
      '5 steps · 4 connections',
    )
    for (const id of [
      'probe',
      'transcode_video',
      'convert_audio',
      'subtitles',
      'remux_container',
    ]) {
      expect(screen.getByTestId(`canvas-node-${id}`)).toBeInTheDocument()
    }
    // The zoom controls keep their accessible names (they are real
    // buttons in main, so the audit counts them).
    expect(screen.getByRole('button', { name: 'Zoom In' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zoom Out' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fit View' })).toBeInTheDocument()
  })
})
