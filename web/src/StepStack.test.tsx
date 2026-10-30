// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The step stack editor (task 14 acceptance): reorders, adds, removes and
 * saves, and a validation failure renders on the offending row.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import StepStackEditor from './StepStackEditor'
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
    type: 'if_codec_not',
    label: 'If the video is not…',
    description: '',
    condition: true,
    params: [
      {
        name: 'codec',
        label: 'codec',
        kind: 'select',
        options: ['hevc', 'av1'],
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
        options: ['hevc', 'av1'],
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

function graph(): FlowGraph {
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

function renderEditor(
  props: Partial<Parameters<typeof StepStackEditor>[0]> = {},
) {
  const changes: FlowGraph[] = []
  // A stateful wrapper: the editor is controlled, so the test feeds each
  // change back in — the way RulesPage does.
  function Harness() {
    const [g, setG] = useState<FlowGraph>(props.graph ?? graph())
    return (
      <StepStackEditor
        graph={g}
        palette={props.palette ?? PALETTE}
        errors={props.errors ?? {}}
        onChange={(next) => {
          changes.push(next)
          setG(next)
        }}
      />
    )
  }
  render(<Harness />)
  return changes
}

describe('StepStackEditor', () => {
  it('reorders rows with the move buttons', () => {
    const changes = renderEditor()
    fireEvent.click(screen.getByTestId('step-up-transcode_video'))
    expect(changes).toHaveLength(1)
    expect(changes[0].nodes.map((n) => n.id)).toEqual([
      'transcode_video',
      'probe',
      'remux_container',
    ])
    // The edges follow the new chain order.
    expect(changes[0].edges).toEqual([
      { from: 'transcode_video', to: 'probe' },
      { from: 'probe', to: 'remux_container' },
    ])
  })

  it('adds a step from the palette with default parameters', () => {
    const changes = renderEditor()
    fireEvent.change(screen.getByTestId('step-add-type'), {
      target: { value: 'transcode_video' },
    })
    fireEvent.click(screen.getByTestId('step-add-button'))
    expect(changes[0].nodes).toHaveLength(4)
    expect(changes[0].nodes[3].type).toBe('transcode_video')
    // The id is unique even when the type already exists.
    expect(changes[0].nodes[3].id).toBe('transcode_video-2')
    // A duplicate type gets a unique id again.
    fireEvent.change(screen.getByTestId('step-add-type'), {
      target: { value: 'transcode_video' },
    })
    fireEvent.click(screen.getByTestId('step-add-button'))
    expect(changes[1].nodes).toHaveLength(5)
    expect(changes[1].nodes[4].id).toBe('transcode_video-3')
  })

  it('removes a row', () => {
    const changes = renderEditor()
    fireEvent.click(screen.getByTestId('step-remove-transcode_video'))
    expect(changes[0].nodes.map((n) => n.id)).toEqual([
      'probe',
      'remux_container',
    ])
  })

  it('edits parameters and shows them as chips', () => {
    const changes = renderEditor()
    expect(
      screen.getByTestId('step-chips-transcode_video').textContent,
    ).toContain('quality 3')
    fireEvent.click(screen.getByTestId('step-edit-transcode_video'))
    fireEvent.change(screen.getByTestId('step-param-quality'), {
      target: { value: '1' },
    })
    expect(changes[0].nodes[1].params).toMatchObject({ quality: 1 })
  })

  it('renders a validation error on the offending row', () => {
    renderEditor({
      errors: { transcode_video: 'params.quality: want 1 to 5, got 9' },
    })
    expect(
      screen.getByTestId('step-error-transcode_video').textContent,
    ).toContain('quality')
    // The row is visually marked.
    const row = screen.getByTestId('step-row-transcode_video')
    expect(row.className).toContain('ring-rose')
  })

  it('shows the gate condition in muted text', () => {
    const g = graph()
    g.nodes.splice(1, 0, {
      id: 'if_codec_not',
      type: 'if_codec_not',
      params: { codec: 'hevc' },
    })
    g.edges = [
      { from: 'probe', to: 'if_codec_not' },
      { from: 'if_codec_not', to: 'transcode_video' },
      { from: 'transcode_video', to: 'remux_container' },
    ]
    renderEditor({ graph: g })
    expect(
      screen.getByTestId('step-condition-if_codec_not').textContent,
    ).toContain('video is not hevc')
    expect(
      screen.getByTestId('step-condition-transcode_video').textContent,
    ).toContain('only when')
  })
})

// --- The full save flow: validate first, field errors on the row. ---

function libraryDoc() {
  return {
    id: 'lib-1',
    name: 'Movies',
    path: '/media/movies',
    target: {},
    easy: null,
    flow: null,
    flow_source: 'steps',
    watch: false,
    schedule: '',
    result_policy: 'replace_if_smaller',
    created_at: '',
    updated_at: '',
  }
}

function flowDoc(): Record<string, unknown> {
  return { library_id: 'lib-1', source: 'steps', graph: graph() }
}

function stepTypesDoc() {
  return { items: PALETTE, total: PALETTE.length }
}

function renderRules() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/libraries/lib-1/rules']}>
          <Routes>
            <Route path="/libraries/:id/rules" element={<RulesPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

describe('RulesPage step stack save', () => {
  let mock: FetchMock
  afterEach(() => mock?.restore())

  it('saves a valid graph through validate and PUT', async () => {
    let putCalled = false
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        body: flowDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: stepTypesDoc(),
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/validate$/,
        body: { valid: true },
      },
      {
        method: 'PUT',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        bodyFn: () => {
          putCalled = true
          return flowDoc()
        },
      },
    ])
    renderRules()
    await waitFor(() => screen.getByTestId('step-stack'))
    fireEvent.click(screen.getByTestId('rules-steps-save'))
    await waitFor(() => expect(putCalled).toBe(true))
  })

  it('renders a server validation failure on the offending row', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1$/,
        body: libraryDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/libraries\/lib-1\/flow$/,
        body: flowDoc(),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: stepTypesDoc(),
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/validate$/,
        status: 400,
        body: {
          error: 'the flow has errors',
          code: 'invalid_flow',
          errors: [
            { path: 'nodes[1].params.quality', message: 'want 1 to 5, got 9' },
          ],
        },
      },
    ])
    renderRules()
    await waitFor(() => screen.getByTestId('step-stack'))
    fireEvent.click(screen.getByTestId('rules-steps-save'))
    await waitFor(() =>
      expect(
        screen.getByTestId('step-error-transcode_video').textContent,
      ).toContain('quality'),
    )
    // The PUT never ran: an invalid graph never saves.
    expect(mock.calls('PUT', /flow$/)).toHaveLength(0)
  })
})
