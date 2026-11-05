// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Moderate decision tree (task 60): a pure model that turns a flow
 * graph into an ordered tree of condition rows and actions, plus the edit
 * operations the tree page applies. The server owns validation; this only
 * shapes and edits.
 *
 * The tree follows the engine's gating (internal/flow Compile): a condition
 * gates the step it leads to, and an action closes the conditions above it,
 * so the step after an action sits back at the level the conditions started
 * from. A condition is told apart by the palette's `condition` flag, never
 * by its step name, so a new guard step appears here without a change.
 */
import type { FlowGraph, FlowStep } from './api'
import { chainEdges } from './flowModel'

export interface TreeNode {
  step: FlowStep
  /** The steps a condition gates. An action is a leaf. */
  children: TreeNode[]
}

export interface TreeRow {
  step: FlowStep
  /** How many conditions the row sits under (0 at the top level). */
  depth: number
}

/** The deepest indent the tree draws; deeper rows keep this indent and
 * show a marker, so a long chain of conditions never runs off a phone. */
export const MAX_INDENT = 4

export type IsCondition = (type: string) => boolean

/**
 * buildTree turns the graph's edges into the tree. A condition's children
 * are the subtrees its outgoing edges lead to. An action is a leaf, and the
 * step that follows it continues at the level above the action's conditions.
 * Steps nothing points at start a new top-level run, in node order. A step
 * is placed once, so a loop cannot run forever.
 */
export function buildTree(
  graph: FlowGraph,
  isCondition: IsCondition,
): TreeNode[] {
  const byId = new Map<string, FlowStep>()
  const index = new Map<string, number>()
  graph.nodes.forEach((n, i) => {
    if (!byId.has(n.id)) {
      byId.set(n.id, n)
      index.set(n.id, i)
    }
  })
  const out = new Map<string, string[]>()
  const hasIncoming = new Set<string>()
  for (const e of graph.edges) {
    if (!byId.has(e.from) || !byId.has(e.to)) continue
    hasIncoming.add(e.to)
    out.set(e.from, [...(out.get(e.from) ?? []), e.to])
  }
  for (const list of out.values()) {
    list.sort((a, b) => index.get(a)! - index.get(b)!)
  }

  const seen = new Set<string>()
  const subtree = (id: string): TreeNode | null => {
    if (seen.has(id)) return null
    seen.add(id)
    const step = byId.get(id)!
    const children: TreeNode[] = []
    if (isCondition(step.type)) {
      for (const next of out.get(id) ?? []) {
        const child = subtree(next)
        if (child) children.push(child)
      }
    }
    return { step, children }
  }

  // The actions a subtree ends on. The steps after them continue at the top.
  const leaves = (node: TreeNode): TreeNode[] => {
    if (node.children.length === 0) {
      return isCondition(node.step.type) ? [] : [node]
    }
    return node.children.flatMap(leaves)
  }

  const roots: TreeNode[] = []
  const pending: string[] = []
  const drain = () => {
    while (pending.length > 0) {
      const node = subtree(pending.shift()!)
      if (!node) continue
      roots.push(node)
      for (const leaf of leaves(node)) {
        pending.push(...(out.get(leaf.step.id) ?? []))
      }
    }
  }

  for (const n of graph.nodes) {
    if (!hasIncoming.has(n.id) && !seen.has(n.id)) pending.push(n.id)
  }
  drain()
  // Anything still unplaced (a disconnected draft) keeps node order.
  for (const n of graph.nodes) {
    if (!seen.has(n.id)) {
      pending.push(n.id)
      drain()
    }
  }
  return roots
}

/** flattenTree lists the tree in order, each row with its depth. */
export function flattenTree(tree: TreeNode[], depth = 0): TreeRow[] {
  const rows: TreeRow[] = []
  for (const node of tree) {
    rows.push({ step: node.step, depth })
    rows.push(...flattenTree(node.children, depth + 1))
  }
  return rows
}

/** The rows of a graph, in the order the chain runs them. */
export function treeRows(
  graph: FlowGraph,
  isCondition: IsCondition,
): TreeRow[] {
  return flattenTree(buildTree(graph, isCondition))
}

/** The steps in run order: the chain the editor saves. */
export function chainSteps(
  graph: FlowGraph,
  isCondition: IsCondition,
): FlowStep[] {
  return treeRows(graph, isCondition).map((r) => r.step)
}

/** True when the graph is exactly the single chain the tree shows, so the
 * editor can rebuild its edges without losing one. A graph that branches
 * (a step with two outgoing edges) is shown read only. */
export function isChainGraph(
  graph: FlowGraph,
  isCondition: IsCondition,
): boolean {
  const want = new Set(
    chainEdges(chainSteps(graph, isCondition)).map((e) => `${e.from}>${e.to}`),
  )
  const have = graph.edges.map((e) => `${e.from}>${e.to}`)
  return (
    have.length === want.size &&
    have.every((k) => want.has(k)) &&
    new Set(have).size === have.length
  )
}

/** A graph holding these steps as its single chain, in this order. */
export function graphOf(schema: number, steps: FlowStep[]): FlowGraph {
  return { schema, nodes: steps, edges: chainEdges(steps) }
}

/** The steps with one moved. An out of range move returns the list as is. */
export function moveStep(
  steps: FlowStep[],
  from: number,
  to: number,
): FlowStep[] {
  if (to < 0 || to >= steps.length || from === to) return steps
  const next = [...steps]
  const [row] = next.splice(from, 1)
  next.splice(to, 0, row)
  return next
}

/** The steps with one step's parameter set. */
export function setStepParam(
  steps: FlowStep[],
  id: string,
  name: string,
  value: unknown,
): FlowStep[] {
  return steps.map((s) =>
    s.id === id ? { ...s, params: { ...(s.params ?? {}), [name]: value } } : s,
  )
}
