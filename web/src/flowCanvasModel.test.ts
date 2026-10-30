// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The canvas-side graph helpers (task 15): layout, error mapping by the
 * server's array index, and the structural edits (connect, disconnect,
 * add, remove). Pure functions — no canvas, no React.
 */
import { describe, expect, it } from 'vitest'
import type { FlowGraph, StepTypeInfo } from './api'
import {
  addStep,
  applyConnect,
  gateMap,
  layoutChain,
  mapNodeErrors,
  removeEdge,
  removeSteps,
} from './flowCanvasModel'

function chain(): FlowGraph {
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

describe('layoutChain', () => {
  it('places the entry at the top and every node in chain order', () => {
    const g = chain()
    g.nodes.push({ id: 'draft', type: 'subtitles' }) // disconnected draft
    const pos = layoutChain(g)
    expect(Object.keys(pos).sort()).toEqual([
      'draft',
      'probe',
      'remux_container',
      'transcode_video',
    ])
    // The chain runs downward; the draft is below it.
    expect(pos['probe'].y).toBeLessThan(pos['transcode_video'].y)
    expect(pos['transcode_video'].y).toBeLessThan(pos['remux_container'].y)
    expect(pos['draft'].y).toBeGreaterThan(pos['remux_container'].y)
  })
})

describe('mapNodeErrors', () => {
  it('maps by array index — the server indexes graph.nodes, not chain order', () => {
    const g = chain()
    // A graph whose array order differs from its chain order.
    const swapped: FlowGraph = {
      schema: 1,
      nodes: [g.nodes[1], g.nodes[0], g.nodes[2]],
      edges: g.edges,
    }
    const out = mapNodeErrors(
      [{ path: 'nodes[0].params.quality', message: 'want 1 to 5, got 9' }],
      swapped.nodes,
    )
    expect(out).toEqual({
      transcode_video: 'params.quality: want 1 to 5, got 9',
    })
  })

  it('keeps the field prefix for node-level paths and collects the rest on "graph"', () => {
    const out = mapNodeErrors(
      [
        { path: 'nodes[1].id', message: 'step "x" is not connected' },
        { path: 'edges[0].to', message: 'unknown step "x"' },
        {
          path: 'graph',
          message: 'the flow needs exactly one remux_container step',
        },
      ],
      chain().nodes,
    )
    expect(out['transcode_video']).toBe('id: step "x" is not connected')
    expect(out['graph']).toBe('the flow needs exactly one remux_container step')
  })
})

describe('applyConnect', () => {
  it('splices the chain: the displaced out-edge and in-edge are replaced', () => {
    const out = applyConnect(chain(), 'probe', 'remux_container')
    expect(out.edges).toEqual([{ from: 'probe', to: 'remux_container' }])
  })

  it('extends a chain end without disturbing the rest', () => {
    const out = applyConnect(chain(), 'remux_container', 'extra')
    expect(out.edges).toEqual([
      { from: 'probe', to: 'transcode_video' },
      { from: 'transcode_video', to: 'remux_container' },
      { from: 'remux_container', to: 'extra' },
    ])
  })

  it('replaces the out-edge of a node that already has one', () => {
    const out = applyConnect(chain(), 'transcode_video', 'extra')
    expect(out.edges).toEqual([
      { from: 'probe', to: 'transcode_video' },
      { from: 'transcode_video', to: 'extra' },
    ])
  })

  it('is a no-op for self loops and duplicates', () => {
    const g = chain()
    expect(applyConnect(g, 'probe', 'probe')).toEqual(g)
    expect(applyConnect(g, 'probe', 'transcode_video')).toEqual(g)
  })
})

describe('removeEdge and removeSteps', () => {
  it('removes one edge', () => {
    const out = removeEdge(chain(), 'probe', 'transcode_video')
    expect(out.edges).toEqual([
      { from: 'transcode_video', to: 'remux_container' },
    ])
    expect(out.nodes).toHaveLength(3)
  })

  it('removes steps and every edge touching them', () => {
    const out = removeSteps(chain(), ['transcode_video'])
    expect(out.nodes.map((n) => n.id)).toEqual(['probe', 'remux_container'])
    expect(out.edges).toEqual([])
  })

  it('ignores an empty removal', () => {
    const g = chain()
    expect(removeSteps(g, [])).toEqual(g)
  })
})

describe('addStep', () => {
  it('appends a palette step with defaults as an orphan', () => {
    const info: StepTypeInfo = {
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
    }
    const { graph, step } = addStep(chain(), info)
    expect(step).toEqual({
      id: 'if_codec_not',
      type: 'if_codec_not',
      params: { codec: 'hevc' },
    })
    expect(graph.nodes).toHaveLength(4)
    expect(graph.edges).toHaveLength(2) // unchanged: the step is an orphan
  })

  it('makes the id unique when the type already exists', () => {
    const g = chain()
    g.nodes.push({ id: 'transcode_video-2', type: 'transcode_video' })
    const info: StepTypeInfo = {
      type: 'transcode_video',
      label: 'Encode the video',
      description: '',
      condition: false,
      params: [],
    }
    expect(addStep(g, info).step.id).toBe('transcode_video-3')
  })
})

describe('gateMap', () => {
  it('labels a step with the guards before it, but not a disconnected draft', () => {
    const g = chain()
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
    g.nodes.push({ id: 'draft', type: 'convert_audio' })
    const gates = gateMap(g)
    expect(gates['transcode_video'].map((s) => s.id)).toEqual(['if_codec_not'])
    expect(gates['probe']).toEqual([])
    expect(gates['draft']).toEqual([])
  })
})
