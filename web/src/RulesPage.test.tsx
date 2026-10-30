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
