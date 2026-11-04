// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Rules page tier switching (task 15 acceptance): the Advanced tier opens
 * the flow canvas and keeps every step; switching back to Steps from a
 * manual graph warns that the manual graph structure will be lost.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import FlowCanvasPage from './FlowCanvasPage'
import RulesPage from './RulesPage'
import type { FlowGraph, StepTypeInfo } from './api'
import { installFetchMock, type FetchMock } from './test/fetchMock'
import { ToastProvider } from './toast'

const PALETTE: StepTypeInfo[] = [
  {
    type: 'probe',
    label: 'Check the file',
    description: '',
    condition: false,
    params: [],
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
]

function stepsGraph(): FlowGraph {
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
        id: 'remux_container',
        type: 'remux_container',
        params: { container: 'mkv' },
      },
    ],
    edges: [
      { from: 'probe', to: 'transcode_video' },
      { from: 'transcode_video', to: 'remux_container' },
    ],
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

function routes(flowSource: string) {
  return [
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/lib-1$/,
      body: libraryDoc(flowSource),
    },
    {
      method: 'GET',
      match: /\/api\/v1\/libraries\/lib-1\/flow$/,
      body: { library_id: 'lib-1', source: flowSource, graph: stepsGraph() },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/flows\/step-types$/,
      body: { items: PALETTE, total: PALETTE.length },
    },
    {
      method: 'POST',
      match: /\/api\/v1\/flows\/validate$/,
      body: { valid: true },
    },
  ]
}

function renderTiers(initial = '/libraries/lib-1/rules') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={[initial]}>
          <Routes>
            <Route path="/libraries/:id/rules" element={<RulesPage />} />
            <Route path="/libraries/:id/flow" element={<FlowCanvasPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock
afterEach(() => mock?.restore())

describe('RulesPage tier switching', () => {
  it('switching a Steps library to Advanced keeps every step', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await waitFor(() => screen.getByTestId('rules'))
    // The step stack shows the three steps.
    await waitFor(() => screen.getByTestId('step-stack'))
    expect(screen.getAllByTestId(/^step-row-/)).toHaveLength(3)
    // The Advanced tier is the canvas on its own route.
    fireEvent.click(screen.getByTestId('tier-advanced'))
    await waitFor(() => screen.getByTestId('flow-canvas-page'))
    // Read-only first: the same graph, every step still there.
    expect(screen.getByTestId('flow-summary').textContent).toBe(
      '3 steps · 2 connections',
    )
    expect(screen.getByTestId('canvas-node-probe')).toBeInTheDocument()
    expect(
      screen.getByTestId('canvas-node-transcode_video'),
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('canvas-node-remux_container'),
    ).toBeInTheDocument()
  })

  it('switching back to Steps warns that the manual graph structure will be lost', async () => {
    mock = installFetchMock(routes('advanced'))
    renderTiers()
    await waitFor(() => screen.getByTestId('advanced-gateway'))
    fireEvent.click(screen.getByTestId('tier-steps'))
    const warning = await waitFor(() =>
      screen.getByTestId('steps-replace-warning'),
    )
    expect(warning.textContent).toContain('manual graph structure')
    // The easy tier warns just the same (task 14 behaviour kept).
    fireEvent.click(screen.getByTestId('tier-easy'))
    await waitFor(() => screen.getByTestId('easy-replace-warning'))
  })
})

// --- Task 45: the restyled rules page (spec 5.8). ---

const nodeFs = 'node:fs'

describe('RulesPage restyle (task 45)', () => {
  it('marks the tier in use with a neutral selection and an In use badge', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await waitFor(() => screen.getByTestId('step-stack'))
    const steps = screen.getByTestId('tier-steps')
    expect(steps).toHaveAttribute('aria-pressed', 'true')
    // The selected segment is neutral, not the accent fill.
    expect(steps.className).toContain('bg-raised')
    expect(steps.className).not.toContain('bg-accent')
    expect(screen.getByTestId('tier-steps-in-use')).toHaveTextContent('In use')
    expect(screen.queryByTestId('tier-easy-in-use')).toBeNull()
    expect(screen.queryByTestId('tier-advanced-in-use')).toBeNull()
  })

  it('shows the three-tier caption under the selector', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await waitFor(() => screen.getByTestId('step-stack'))
    expect(
      screen.getByText(
        'Easy: three sliders. Steps: an ordered list. Advanced: a visual graph. All three produce the same kind of rules.',
      ),
    ).toBeInTheDocument()
  })

  it('names the save action "Save rules" on the steps tier', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await waitFor(() => screen.getByTestId('step-stack'))
    expect(screen.getByTestId('rules-steps-save')).toHaveTextContent(
      'Save rules',
    )
  })

  it('names the save action "Save rules" and shows the replace banner on the easy tier', async () => {
    mock = installFetchMock([
      ...routes('steps'),
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: { steps: [] },
      },
    ])
    renderTiers()
    await waitFor(() => screen.getByTestId('step-stack'))
    fireEvent.click(screen.getByTestId('tier-easy'))
    const banner = await waitFor(() =>
      screen.getByTestId('easy-replace-warning'),
    )
    expect(banner).toHaveAttribute('role', 'status')
    expect(banner.textContent).toContain('replaces the manual steps')
    expect(screen.getByTestId('rules-easy-save')).toHaveTextContent(
      'Save rules',
    )
  })

  it('gives every step row icon button a 44 px box and an aria-label', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await waitFor(() => screen.getByTestId('step-stack'))
    for (const testId of [
      'step-up-probe',
      'step-down-probe',
      'step-edit-probe',
      'step-remove-probe',
    ]) {
      const button = screen.getByTestId(testId)
      expect(button.className, testId).toContain('h-11 w-11')
      expect(button.getAttribute('aria-label'), testId).toBeTruthy()
    }
    // Remove is the danger action, not the muted one.
    expect(screen.getByTestId('step-remove-probe').className).toContain(
      'text-danger',
    )
  })

  it('leaves no rose palette classes in the rules files', async () => {
    const { readFileSync } = (await import(/* @vite-ignore */ nodeFs)) as {
      readFileSync: (path: string, encoding: 'utf8') => string
    }
    for (const file of [
      'src/RulesPage.tsx',
      'src/StepStackEditor.tsx',
      'src/FlowPreviewPanel.tsx',
      'src/flowParams.tsx',
    ]) {
      expect(readFileSync(file, 'utf8'), file).not.toMatch(/rose-/)
    }
  })
})

// --- Task 47: the Edit page's rule controls moved here (spec 5.7, 8.4). ---

function easyLibrary(easy: {
  quality: number
  remove_subtitles: boolean
  audio: number
}) {
  return { ...libraryDoc('easy'), easy }
}

describe('RulesPage holds the rule controls moved from Edit (task 47)', () => {
  it('the Easy tier sliders save the easy object and no target', async () => {
    const lib = easyLibrary({ quality: 3, remove_subtitles: false, audio: 1 })
    mock = installFetchMock([
      { method: 'GET', match: /\/api\/v1\/libraries\/lib-1$/, body: lib },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        body: { library_id: 'lib-1', source: 'easy', graph: stepsGraph() },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: { items: PALETTE, total: PALETTE.length },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: { steps: [] },
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: lib,
      },
    ])
    renderTiers()
    await screen.findByTestId('rules-easy-save')

    fireEvent.click(screen.getByTestId('quality-stop-5'))
    fireEvent.click(screen.getByTestId('subtitles-checkbox'))
    fireEvent.click(screen.getByTestId('rules-easy-save'))

    await waitFor(() => {
      const puts = mock.calls('PUT', /\/api\/v1\/libraries\/lib-1$/)
      expect(puts).toHaveLength(1)
      expect(puts[0].body).toMatchObject({
        name: 'Movies',
        path: '/media/movies',
        watch: false,
        easy: { quality: 5, remove_subtitles: true, audio: 1 },
      })
      expect((puts[0].body as Record<string, unknown>).target).toBeUndefined()
    })
  })

  it('the Easy tier names its audio choices in plain language', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: easyLibrary({ quality: 3, remove_subtitles: false, audio: 2 }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        body: { library_id: 'lib-1', source: 'easy', graph: stepsGraph() },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: { items: PALETTE, total: PALETTE.length },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: { steps: [] },
      },
    ])
    renderTiers()
    await screen.findByTestId('rules-easy-save')
    expect(screen.getByTestId('audio-stop-1')).toHaveTextContent(
      'Keep everything',
    )
    expect(screen.getByTestId('audio-stop-2')).toHaveTextContent(
      'Keep my languages',
    )
    expect(screen.getByTestId('audio-stop-3')).toHaveTextContent(
      'Keep the best track',
    )
    expect(screen.getByTestId('audio-stop-4')).toHaveTextContent(
      'Keep the best track, make it small',
    )
    // The stored value is unchanged: option 2 is the selected stop.
    expect(screen.getByTestId('audio-stop-2')).toHaveClass('font-semibold')
  })

  it('the Steps tier reaches the codec, quality and container settings', async () => {
    mock = installFetchMock(routes('steps'))
    renderTiers()
    await screen.findByTestId('step-stack')

    fireEvent.click(screen.getByTestId('step-edit-transcode_video'))
    const codec = (await screen.findByTestId(
      'step-param-codec',
    )) as HTMLSelectElement
    expect(codec.value).toBe('hevc')
    expect(Array.from(codec.options).map((o) => o.value)).toEqual([
      'hevc',
      'av1',
      'h264',
    ])
    expect(screen.getByTestId('step-param-quality')).toHaveValue(3)

    fireEvent.click(screen.getByTestId('step-edit-remux_container'))
    const container = (await screen.findByTestId(
      'step-param-container',
    )) as HTMLSelectElement
    expect(container.value).toBe('mkv')
    expect(Array.from(container.options).map((o) => o.value)).toEqual([
      'mkv',
      'mp4',
    ])
  })
})
