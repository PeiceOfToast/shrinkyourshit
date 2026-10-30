// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Client-side model for the flow graph (task 14). The server owns
 * validation and compilation; this module only shapes the graph for the
 * step stack editor: chain order, default parameters from the palette and
 * the row labels (condition text, chips).
 */
import type { FlowEdge, FlowGraph, FlowStep, StepTypeInfo } from './api'

/** The steps in run order (the single chain), falling back to node order. */
export function orderedSteps(graph: FlowGraph): FlowStep[] {
  if (graph.nodes.length === 0) return []
  const next = new Map<string, string>()
  for (const e of graph.edges) next.set(e.from, e.to)
  const incoming = new Set<string>(graph.edges.map((e) => e.to))
  const entry = graph.nodes.find((n) => !incoming.has(n.id)) ?? graph.nodes[0]
  const seen = new Set<string>()
  const out: FlowStep[] = []
  let cur: FlowStep | undefined = entry
  while (cur && !seen.has(cur.id)) {
    seen.add(cur.id)
    out.push(cur)
    cur = graph.nodes.find((n) => n.id === next.get(cur!.id))
  }
  // Anything the walk missed (disconnected drafts) keeps node order.
  for (const n of graph.nodes) if (!seen.has(n.id)) out.push(n)
  return out
}

/** Rebuilds the single-chain edges for a node list. */
export function chainEdges(nodes: FlowStep[]): FlowEdge[] {
  const edges: FlowEdge[] = []
  for (let i = 1; i < nodes.length; i++) {
    edges.push({ from: nodes[i - 1].id, to: nodes[i].id })
  }
  return edges
}

/** A unique id for a new step of the given type. */
export function newStepID(type: string, nodes: FlowStep[]): string {
  const base = type
  let id = base
  let n = 2
  while (nodes.some((s) => s.id === id)) {
    id = `${base}-${n}`
    n++
  }
  return id
}

/** Initial parameter values from a palette entry's schema. */
export function defaultParams(info: StepTypeInfo): Record<string, unknown> {
  const params: Record<string, unknown> = {}
  for (const f of info.params) {
    if (f.default !== undefined) params[f.name] = f.default
    else if (f.kind === 'select') params[f.name] = f.options?.[0] ?? ''
    else if (f.kind === 'number') params[f.name] = f.min ?? 1
    else if (f.kind === 'toggle') params[f.name] = false
    else if (f.kind === 'languages') params[f.name] = []
    else params[f.name] = ''
  }
  return params
}

/** A new step from a palette entry: unique id among `nodes`, default
 * parameters, no params key at all when the type takes none. */
export function stepFromPalette(
  info: StepTypeInfo,
  nodes: FlowStep[],
): FlowStep {
  const params = defaultParams(info)
  return {
    id: newStepID(info.type, nodes),
    type: info.type,
    params: Object.keys(params).length ? params : undefined,
  }
}

/** The palette entry for a step type. */
export function paletteFor(
  palette: StepTypeInfo[],
  type: string,
): StepTypeInfo | undefined {
  return palette.find((p) => p.type === type)
}

/** Short human label for one parameter value. */
function paramText(kind: string, value: unknown): string {
  if (kind === 'toggle') return value ? 'yes' : 'no'
  if (kind === 'languages') {
    const langs = Array.isArray(value) ? (value as string[]) : []
    return langs.length > 0 ? langs.join(', ') : '—'
  }
  if (value === undefined || value === null || value === '') return '—'
  return String(value)
}

/** The chips of one row: "label value" for each parameter. */
export function chipsFor(
  step: FlowStep,
  info: StepTypeInfo | undefined,
): string[] {
  if (!info) return []
  const params = (step.params ?? {}) as Record<string, unknown>
  return info.params.map(
    (f) => `${f.label} ${paramText(f.kind, params[f.name])}`,
  )
}

/** The muted condition text of a row: guards state their rule, action rows
 * state the guards that gate them. */
export function conditionText(
  step: FlowStep,
  gatedBy: FlowStep[],
  info: StepTypeInfo | undefined,
): string {
  if (info?.condition) {
    const params = (step.params ?? {}) as Record<string, unknown>
    if (step.type === 'if_codec_not') {
      return `runs the next step only when the video is not ${params.codec ?? '?'}`
    }
    if (step.type === 'if_bitrate_above') {
      return `runs the next step only when the video bitrate is above ${params.mbps ?? '?'} megabits per second`
    }
    return info.description
  }
  if (gatedBy.length === 0) return ''
  const parts = gatedBy.map((g) => {
    const params = (g.params ?? {}) as Record<string, unknown>
    if (g.type === 'if_codec_not')
      return `the video is not ${params.codec ?? '?'}`
    if (g.type === 'if_bitrate_above')
      return `the bitrate is above ${params.mbps ?? '?'} megabits per second`
    return g.id
  })
  return `only when ${parts.join(' and ')}`
}

/** Guards that directly precede a step in the chain. */
export function gatesFor(steps: FlowStep[], index: number): FlowStep[] {
  const gates: FlowStep[] = []
  for (let i = index - 1; i >= 0; i--) {
    if (
      steps[i].type === 'if_codec_not' ||
      steps[i].type === 'if_bitrate_above'
    ) {
      gates.push(steps[i])
    } else {
      break
    }
  }
  return gates.reverse()
}

/** Field errors keyed for the editor: a step id → message, plus "graph"
 * for graph-level problems. Path shape: nodes[<index>]... The remaining
 * path (e.g. "params.quality") is kept so the row shows which parameter
 * failed. */
export function mapErrors(
  errors: Array<{ path: string; message: string }>,
  steps: FlowStep[],
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const e of errors) {
    const m = e.path.match(/^nodes\[(\d+)\]\.?/)
    if (m) {
      const idx = Number(m[1])
      const step = steps[idx]
      if (step) {
        const rest = e.path.slice(m[0].length)
        out[step.id] = rest ? `${rest}: ${e.message}` : e.message
        continue
      }
    }
    out['graph'] = e.message
  }
  return out
}
