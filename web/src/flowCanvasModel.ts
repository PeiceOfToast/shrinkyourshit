// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Canvas-side model (task 15): pure helpers over the FlowGraph the API
 * returns. The graph is the single source; the canvas re-shapes it for
 * React Flow (positions are presentation only, the graph schema has none)
 * and makes structural edits (connect, disconnect, add, remove). Validity
 * is never decided here — every change goes to POST /api/v1/flows/validate
 * and the returned field errors render on the nodes verbatim.
 */
import type {
  FlowEdge,
  FlowFieldError,
  FlowGraph,
  FlowStep,
  StepTypeInfo,
} from './api'
import { gatesFor, orderedSteps, stepFromPalette } from './flowModel'

/** One node's place on the canvas. Presentation only. */
export interface XYPosition {
  x: number
  y: number
}

/** Layout constants of the vertical chain: one node per row. */
export const NODE_LEFT = 40
export const NODE_TOP = 24
export const NODE_GAP = 150

/** Chain layout: the entry step at the top, one node per row; drafts that
 * are not connected yet append below the chain. */
export function layoutChain(graph: FlowGraph): Record<string, XYPosition> {
  const out: Record<string, XYPosition> = {}
  orderedSteps(graph).forEach((step, i) => {
    out[step.id] = { x: NODE_LEFT, y: NODE_TOP + i * NODE_GAP }
  })
  return out
}

/**
 * The guards that gate each step: the run of condition steps directly
 * before it in the chain — only when the step actually has an incoming
 * edge, so a disconnected draft is never labelled with a neighbour's rule.
 */
export function gateMap(graph: FlowGraph): Record<string, FlowStep[]> {
  const chain = orderedSteps(graph)
  const index = new Map(chain.map((s, i) => [s.id, i]))
  const out: Record<string, FlowStep[]> = {}
  for (const n of graph.nodes) {
    const i = index.get(n.id)
    if (i === undefined) {
      out[n.id] = []
      continue
    }
    out[n.id] = graph.edges.some((e) => e.to === n.id) ? gatesFor(chain, i) : []
  }
  return out
}

/** The canvas id of one edge. */
export function edgeKey(edge: Pick<FlowEdge, 'from' | 'to'>): string {
  return `e-${edge.from}-${edge.to}`
}

/**
 * Server field errors keyed for the canvas: `nodes[<index>]` paths map by
 * the graph's array order — the server's index, not the chain order. The
 * rest of the path (e.g. "params.quality") is kept so the node shows which
 * parameter failed. Anything else (edges, schema, the graph itself) lands
 * on "graph".
 */
export function mapNodeErrors(
  errors: FlowFieldError[],
  nodes: FlowStep[],
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const e of errors) {
    const m = e.path.match(/^nodes\[(\d+)\]\.?(.*)/)
    if (m) {
      const step = nodes[Number(m[1])]
      if (step) {
        out[step.id] = m[2] ? `${m[2]}: ${e.message}` : e.message
        continue
      }
    }
    out['graph'] = e.message
  }
  return out
}

/**
 * Connect from→to, keeping the single-chain shape: the displaced outgoing
 * edge of `from` and the displaced incoming edge of `to` are replaced by
 * the new edge. Self loops and duplicates are no-ops.
 */
export function applyConnect(
  graph: FlowGraph,
  from: string,
  to: string,
): FlowGraph {
  if (from === to) return graph
  if (graph.edges.some((e) => e.from === from && e.to === to)) return graph
  const edges: FlowEdge[] = graph.edges.filter(
    (e) => e.from !== from && e.to !== to,
  )
  edges.push({ from, to })
  return { ...graph, edges }
}

/** Removes one edge. */
export function removeEdge(
  graph: FlowGraph,
  from: string,
  to: string,
): FlowGraph {
  return {
    ...graph,
    edges: graph.edges.filter((e) => !(e.from === from && e.to === to)),
  }
}

/** Removes steps and every edge touching them. */
export function removeSteps(graph: FlowGraph, ids: string[]): FlowGraph {
  if (ids.length === 0) return graph
  const drop = new Set(ids)
  return {
    ...graph,
    nodes: graph.nodes.filter((n) => !drop.has(n.id)),
    edges: graph.edges.filter((e) => !drop.has(e.from) && !drop.has(e.to)),
  }
}

/** Appends a new step of the palette type with its default parameters —
 * an orphan until connected, which the server flags on the next validate. */
export function addStep(
  graph: FlowGraph,
  info: StepTypeInfo,
): { graph: FlowGraph; step: FlowStep } {
  const step = stepFromPalette(info, graph.nodes)
  return { graph: { ...graph, nodes: [...graph.nodes, step] }, step }
}

/** What the inspector is looking at: one step, one connection, or nothing. */
export interface CanvasSelection {
  nodeId: string | null
  edge: { from: string; to: string } | null
}

export const NO_SELECTION: CanvasSelection = { nodeId: null, edge: null }
