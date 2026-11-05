// SPDX-License-Identifier: AGPL-3.0-only

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import FlowTreePage from './FlowTreePage'
import type { FlowGraph, StepTypeInfo } from './api'
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
    description: 'Reads the file before anything else runs.',
    condition: false,
    params: [],
  },
  {
    type: 'if_bitrate_above',
    label: 'Only when bitrate is above',
    description: 'Runs the next step only above this bitrate.',
    condition: true,
    params: [
      {
        name: 'mbps',
        label: 'megabits per second',
        kind: 'number',
        min: 0.1,
        required: true,
        default: 8,
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
    type: 'skip_if_smaller',
    label: 'Keep the original if smaller',
    description: '',
    condition: false,
    params: [],
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

/** probe, then a bitrate condition gating one encode, then the container. */
function treeGraph(): FlowGraph {
  return {
    schema: 1,
    nodes: [
      { id: 'probe', type: 'probe' },
      { id: 'if_bitrate_above', type: 'if_bitrate_above', params: { mbps: 8 } },
      {
        id: 'transcode_video',
        type: 'transcode_video',
        params: { quality: 3 },
      },
      {
        id: 'remux_container',
        type: 'remux_container',
        params: { container: 'mkv' },
      },
    ],
    edges: [
      { from: 'probe', to: 'if_bitrate_above' },
      { from: 'if_bitrate_above', to: 'transcode_video' },
      { from: 'transcode_video', to: 'remux_container' },
    ],
  }
}

function libraryDoc(source: string) {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: {},
    easy: null,
    flow: null,
    flow_source: source,
    watch: false,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
  }
}

function routes(
  graph: FlowGraph,
  source = 'steps',
  saveStatus = 200,
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
      body: { library_id: 'lib-1', source, graph },
    },
    {
      method: 'PUT',
      match: /\/api\/v1\/libraries\/lib-1\/flow$/,
      statusFn: () => saveStatus,
      bodyFn: ({ body }) => {
        if (saveStatus !== 200) {
          return {
            code: 'invalid_flow',
            error: 'the flow is not valid',
            errors: [
              { path: 'nodes[2].params.quality', message: 'must be 1 to 5' },
            ],
          }
        }
        return {
          library_id: 'lib-1',
          source: (body as { source: string }).source,
          graph: (body as { graph: FlowGraph }).graph,
        }
      },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/flows\/step-types$/,
      body: { items: PALETTE, total: PALETTE.length },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/plugins$/,
      body: { items: [], total: 0 },
    },
  ]
}

function renderTree() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/libraries/lib-1/tree']}>
          <Routes>
            <Route path="/libraries/:id/tree" element={<FlowTreePage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

/** The row ids in the order the page draws them. */
function rowIds(): string[] {
  return screen
    .getAllByTestId(/^tree-row-/)
    .map((el) => el.dataset.testid!.replace('tree-row-', ''))
}

function depthOf(id: string): string | undefined {
  return screen.getByTestId(`tree-row-${id}`).dataset.depth
}

let mock: FetchMock
afterEach(() => mock?.restore())

describe('FlowTreePage rows', () => {
  it('draws the gated action indented under its condition', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    expect(rowIds()).toEqual([
      'probe',
      'if_bitrate_above',
      'transcode_video',
      'remux_container',
    ])
    expect(depthOf('probe')).toBe('0')
    expect(depthOf('if_bitrate_above')).toBe('0')
    expect(depthOf('transcode_video')).toBe('1')
    expect(depthOf('remux_container')).toBe('0')
    // The condition's indent is its depth in rem, so the child sits further in.
    expect(
      screen.getByTestId('tree-row-transcode_video').style.marginLeft,
    ).toBe('1.5rem')
  })

  it('reads the labels and chips from the palette, not from step names', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    const guard = screen.getByTestId('tree-row-if_bitrate_above')
    expect(
      within(guard).getByText('Only when bitrate is above'),
    ).toBeInTheDocument()
    expect(
      within(screen.getByTestId('tree-chips-if_bitrate_above')).getByText(
        'megabits per second 8',
      ),
    ).toBeInTheDocument()
  })
})

describe('FlowTreePage palette', () => {
  it('offers every StepTypes() entry and adds each one', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-add-type'))
    const select = screen.getByTestId('tree-add-type') as HTMLSelectElement
    // The placeholder plus one option per fetched palette entry.
    expect(select.options).toHaveLength(PALETTE.length + 1)
    for (const info of PALETTE) {
      expect(
        within(select).getByRole('option', { name: info.label }),
      ).toBeTruthy()
    }
    const before = rowIds().length
    for (const [n, info] of PALETTE.entries()) {
      fireEvent.change(select, { target: { value: info.type } })
      fireEvent.click(screen.getByTestId('tree-add-button'))
      // A new row for each step, named by its palette entry.
      expect(rowIds()).toHaveLength(before + n + 1)
      expect(rowIds().at(-1)!.startsWith(info.type)).toBe(true)
    }
  })
})

describe('FlowTreePage editing', () => {
  it('adds a condition and its child, saves the whole graph as steps', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    const select = screen.getByTestId('tree-add-type')
    fireEvent.change(select, { target: { value: 'if_bitrate_above' } })
    fireEvent.click(screen.getByTestId('tree-add-button'))
    fireEvent.change(select, { target: { value: 'skip_if_smaller' } })
    fireEvent.click(screen.getByTestId('tree-add-button'))
    expect(depthOf('skip_if_smaller')).toBe('1')
    fireEvent.click(screen.getByTestId('tree-save'))
    await waitFor(() =>
      expect(screen.getByText('Tree saved.')).toBeInTheDocument(),
    )
    const puts = mock.calls('PUT', /\/libraries\/lib-1\/flow$/)
    expect(puts).toHaveLength(1)
    const body = puts[0].body as { source: string; graph: FlowGraph }
    expect(body.source).toBe('steps')
    expect(body.graph.nodes.map((n) => n.id)).toEqual([
      'probe',
      'if_bitrate_above',
      'transcode_video',
      'remux_container',
      'if_bitrate_above-2',
      'skip_if_smaller',
    ])
    expect(body.graph.edges).toHaveLength(5)
  })

  it('removes a row and rebuilds the chain', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    fireEvent.click(screen.getByTestId('tree-remove-transcode_video'))
    expect(rowIds()).toEqual(['probe', 'if_bitrate_above', 'remux_container'])
    // The guard now gates the next step, which is the remux.
    expect(depthOf('remux_container')).toBe('1')
  })

  it('moves a row with the arrow keys on its handle', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    const handle = screen.getByTestId('tree-handle-remux_container')
    fireEvent.keyDown(handle, { key: 'ArrowUp' })
    expect(rowIds()).toEqual([
      'probe',
      'if_bitrate_above',
      'remux_container',
      'transcode_video',
    ])
  })

  it('shows a server field error on the row that owns the field', async () => {
    mock = installFetchMock(routes(treeGraph(), 'steps', 400))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    // Save needs an edit. Removing the last row keeps transcode_video at nodes[2].
    fireEvent.click(screen.getByTestId('tree-remove-remux_container'))
    fireEvent.click(screen.getByTestId('tree-save'))
    await waitFor(() =>
      expect(
        screen.getByTestId('tree-error-transcode_video').textContent,
      ).toContain('must be 1 to 5'),
    )
    // The error sits on the encode row only.
    expect(screen.queryByTestId('tree-error-if_bitrate_above')).toBeNull()
  })

  it('only enables Save after an edit', async () => {
    mock = installFetchMock(routes(treeGraph()))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-rows'))
    expect(screen.getByTestId('tree-save')).toBeDisabled()
    fireEvent.click(screen.getByTestId('tree-remove-remux_container'))
    expect(screen.getByTestId('tree-save')).not.toBeDisabled()
  })
})

describe('FlowTreePage branching graph', () => {
  it('shows a branching flow read only, with no add or save controls', async () => {
    const forked: FlowGraph = {
      schema: 1,
      nodes: [
        { id: 'probe', type: 'probe' },
        {
          id: 'if_bitrate_above',
          type: 'if_bitrate_above',
          params: { mbps: 8 },
        },
        {
          id: 'transcode_video',
          type: 'transcode_video',
          params: { quality: 3 },
        },
        { id: 'skip_if_smaller', type: 'skip_if_smaller' },
      ],
      edges: [
        { from: 'probe', to: 'if_bitrate_above' },
        { from: 'if_bitrate_above', to: 'transcode_video' },
        { from: 'if_bitrate_above', to: 'skip_if_smaller' },
      ],
    }
    mock = installFetchMock(routes(forked))
    renderTree()
    await waitFor(() => screen.getByTestId('tree-readonly-banner'))
    expect(screen.queryByTestId('tree-add-type')).toBeNull()
    expect(screen.queryByTestId('tree-save')).toBeNull()
    expect(depthOf('transcode_video')).toBe('1')
    expect(depthOf('skip_if_smaller')).toBe('1')
  })
})
