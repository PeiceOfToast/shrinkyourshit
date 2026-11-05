// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest'
import type { FlowGraph, FlowStep } from './api'
import {
  buildTree,
  chainSteps,
  flattenTree,
  graphOf,
  isChainGraph,
  moveStep,
  setStepParam,
  treeRows,
} from './treeModel'

// The model takes the condition flag from the palette. These fixtures use
// the guard names the engine ships with, which is what the palette says.
const GUARDS = new Set(['if_codec_not', 'if_bitrate_above', 'if_hdr'])
const isCondition = (type: string) => GUARDS.has(type)

const step = (id: string, type = id): FlowStep => ({ id, type })

function chain(...steps: FlowStep[]): FlowGraph {
  return graphOf(1, steps)
}

/** The rows as "id@depth" strings, the compact form the tests compare. */
function rowText(graph: FlowGraph): string[] {
  return treeRows(graph, isCondition).map((r) => `${r.step.id}@${r.depth}`)
}

describe('treeModel: flat chain', () => {
  it('lists every step at depth 0 in chain order', () => {
    const g = chain(
      step('probe'),
      step('transcode_video'),
      step('remux_container'),
    )
    expect(rowText(g)).toEqual([
      'probe@0',
      'transcode_video@0',
      'remux_container@0',
    ])
    expect(isChainGraph(g, isCondition)).toBe(true)
  })

  it('puts the step after an action back at the top level', () => {
    const g = chain(step('probe'), step('transcode_video'), step('skip'))
    expect(rowText(g)).toEqual(['probe@0', 'transcode_video@0', 'skip@0'])
  })
})

describe('treeModel: a condition with children', () => {
  it('indents the action a condition gates, and only that action', () => {
    // probe -> bitrate guard -> transcode -> remux: the engine gates the
    // transcode alone, so the remux is back at the top level.
    const g = chain(
      step('probe'),
      step('if_bitrate_above'),
      step('transcode_video'),
      step('remux_container'),
    )
    expect(rowText(g)).toEqual([
      'probe@0',
      'if_bitrate_above@0',
      'transcode_video@1',
      'remux_container@0',
    ])
    const tree = buildTree(g, isCondition)
    expect(tree[1].step.id).toBe('if_bitrate_above')
    expect(tree[1].children.map((c) => c.step.id)).toEqual(['transcode_video'])
  })

  it('shows a condition with two children as two indented rows', () => {
    // Model level only: the engine has no branches and the server refuses
    // a fork on save. The tree still lists both children under the
    // condition, so a fork reads correctly if it ever reaches the page.
    const g: FlowGraph = {
      schema: 1,
      nodes: [
        step('probe'),
        step('if_codec_not'),
        step('transcode_video'),
        step('skip_if_smaller'),
      ],
      edges: [
        { from: 'probe', to: 'if_codec_not' },
        { from: 'if_codec_not', to: 'transcode_video' },
        { from: 'if_codec_not', to: 'skip_if_smaller' },
      ],
    }
    const tree = buildTree(g, isCondition)
    const guard = tree[1]
    expect(guard.step.id).toBe('if_codec_not')
    expect(guard.children.map((c) => c.step.id)).toEqual([
      'transcode_video',
      'skip_if_smaller',
    ])
    expect(rowText(g)).toEqual([
      'probe@0',
      'if_codec_not@0',
      'transcode_video@1',
      'skip_if_smaller@1',
    ])
    expect(isChainGraph(g, isCondition)).toBe(false)
  })
})

describe('treeModel: nested conditions', () => {
  it('nests a condition under a condition and gates the action by both', () => {
    // Two guards in a row both gate the one action after them (the engine
    // ANDs pending conditions), so each guard owns the next level down.
    const g = chain(
      step('probe'),
      step('if_codec_not'),
      step('if_hdr'),
      step('transcode_video'),
      step('remux_container'),
    )
    expect(rowText(g)).toEqual([
      'probe@0',
      'if_codec_not@0',
      'if_hdr@1',
      'transcode_video@2',
      'remux_container@0',
    ])
    const tree = buildTree(g, isCondition)
    const outer = tree[1]
    expect(outer.children).toHaveLength(1)
    expect(outer.children[0].step.id).toBe('if_hdr')
    expect(outer.children[0].children[0].step.id).toBe('transcode_video')
  })

  it('flattens to the same order as the chain', () => {
    const g = chain(
      step('probe'),
      step('if_codec_not'),
      step('if_hdr'),
      step('transcode_video'),
    )
    expect(
      flattenTree(buildTree(g, isCondition)).map((r) => r.step.id),
    ).toEqual(chainSteps(g, isCondition).map((s) => s.id))
  })
})

describe('treeModel: robustness', () => {
  it('keeps a step that nothing reaches, in node order, and never loops', () => {
    const g: FlowGraph = {
      schema: 1,
      nodes: [step('probe'), step('remux_container'), step('loose')],
      edges: [
        { from: 'probe', to: 'remux_container' },
        { from: 'remux_container', to: 'probe' },
      ],
    }
    const rows = treeRows(g, isCondition)
    expect(rows.map((r) => r.step.id).sort()).toEqual([
      'loose',
      'probe',
      'remux_container',
    ])
  })

  it('ignores edges that point at unknown steps', () => {
    const g: FlowGraph = {
      schema: 1,
      nodes: [step('probe')],
      edges: [{ from: 'probe', to: 'ghost' }],
    }
    expect(rowText(g)).toEqual(['probe@0'])
    expect(isChainGraph(g, isCondition)).toBe(false)
  })
})

describe('treeModel: edits keep the chain', () => {
  const base = chain(
    step('probe'),
    step('if_bitrate_above'),
    step('skip_if_smaller'),
  )

  it('moveStep reorders and ignores out of range moves', () => {
    const steps = chainSteps(base, isCondition)
    expect(moveStep(steps, 2, 0).map((s) => s.id)).toEqual([
      'skip_if_smaller',
      'probe',
      'if_bitrate_above',
    ])
    expect(moveStep(steps, 0, -1)).toBe(steps)
    expect(moveStep(steps, 2, 3)).toBe(steps)
  })

  it('graphOf rebuilds one chain edge per adjacent pair', () => {
    const g = graphOf(1, [step('probe'), step('skip_if_smaller')])
    expect(g.edges).toEqual([{ from: 'probe', to: 'skip_if_smaller' }])
  })

  it('setStepParam changes one parameter and keeps the others', () => {
    const steps: FlowStep[] = [
      { id: 'if_bitrate_above', type: 'if_bitrate_above', params: { mbps: 1 } },
    ]
    const next = setStepParam(steps, 'if_bitrate_above', 'mbps', 4)
    expect(next[0].params).toEqual({ mbps: 4 })
    expect(steps[0].params).toEqual({ mbps: 1 })
  })
})
