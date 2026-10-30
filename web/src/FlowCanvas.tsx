// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The flow canvas (task 15): the library's rule graph on @xyflow/react
 * (STACK.md pins React Flow 12 — no hand-written canvas). The graph object
 * from the API maps 1:1 onto canvas nodes and edges; positions are
 * presentation only. Edit mode makes structural changes (connect,
 * disconnect, add, remove) — the server owns validation and the returned
 * field errors render on the nodes themselves.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type NodeProps,
  type XYPosition as RFPosition,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { FlowGraph, StepTypeInfo } from './api'
import { chipsFor, conditionText, paletteFor } from './flowModel'
import {
  edgeKey,
  gateMap,
  layoutChain,
  NODE_GAP,
  applyConnect,
  removeEdge,
  removeSteps,
  type CanvasSelection,
  type XYPosition,
} from './flowCanvasModel'

interface StepNodeData extends Record<string, unknown> {
  stepId: string
  label: string
  condition: string
  chips: string[]
  error?: string
  readOnly: boolean
}

type StepNode = Node<StepNodeData, 'step'>

/** One rule step on the canvas: its label, its condition line, its
 * parameters as chips, and the server's validation error when there is
 * one. Handles on top (in) and bottom (out) form the chain. */
function StepNodeView({ data, selected }: NodeProps<StepNode>) {
  return (
    <div
      data-testid={`canvas-node-${data.stepId}`}
      className={`min-w-[15rem] max-w-[17rem] rounded-2xl bg-slate-900 px-4 py-3 ring-1 ${
        data.error
          ? 'ring-rose-500/70'
          : selected
            ? 'ring-emerald-500'
            : 'ring-slate-700'
      }`}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!h-2.5 !w-2.5 !border-slate-600 !bg-slate-500"
        isConnectable={!data.readOnly}
      />
      <div className="flex items-baseline gap-2">
        <span className="font-semibold text-slate-100">{data.label}</span>
        <span className="truncate text-[10px] text-slate-500">
          {data.stepId}
        </span>
      </div>
      {data.condition && (
        <p className="mt-1 text-xs text-slate-500">{data.condition}</p>
      )}
      {data.chips.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {data.chips.map((chip, i) => (
            <span
              key={i}
              className="rounded-full bg-slate-800 px-2 py-0.5 text-[10px] text-slate-300"
            >
              {chip}
            </span>
          ))}
        </div>
      )}
      {data.error && (
        <p
          className="mt-2 text-xs text-rose-400"
          data-testid={`canvas-node-error-${data.stepId}`}
        >
          {data.error}
        </p>
      )}
      <Handle
        type="source"
        position={Position.Bottom}
        className="!h-2.5 !w-2.5 !border-slate-600 !bg-slate-500"
        isConnectable={!data.readOnly}
      />
    </div>
  )
}

const nodeTypes = { step: StepNodeView }

export interface FlowCanvasProps {
  graph: FlowGraph
  palette: StepTypeInfo[]
  /** Server field errors keyed by step id ("graph" for graph-level). */
  errors: Record<string, string>
  editing: boolean
  selection: CanvasSelection
  onSelect: (selection: CanvasSelection) => void
  onChange: (graph: FlowGraph) => void
}

export default function FlowCanvas({
  graph,
  palette,
  errors,
  editing,
  selection,
  onSelect,
  onChange,
}: FlowCanvasProps) {
  const [positions, setPositions] = useState<Record<string, XYPosition>>(() =>
    layoutChain(graph),
  )

  // Nodes added later (palette, or a graph swap) need a place to stand:
  // below everything so far. The guard keeps this a no-op unless a node is
  // actually missing a position, so existing arrangements never move.
  useEffect(() => {
    setPositions((prev) => {
      const missing = graph.nodes.filter((n) => !prev[n.id])
      if (missing.length === 0) return prev
      let maxY = 0
      for (const p of Object.values(prev)) maxY = Math.max(maxY, p.y)
      const next = { ...prev }
      missing.forEach((n, i) => {
        next[n.id] = { x: 40, y: maxY + NODE_GAP + i * NODE_GAP }
      })
      return next
    })
  }, [graph])

  const conditions = useMemo(() => gateMap(graph), [graph])

  const nodes = useMemo<StepNode[]>(
    () =>
      graph.nodes.map((n) => {
        const info = paletteFor(palette, n.type)
        return {
          id: n.id,
          type: 'step' as const,
          position: positions[n.id] ?? { x: 40, y: 0 },
          selected: selection.nodeId === n.id,
          draggable: editing,
          connectable: editing,
          deletable: editing,
          data: {
            stepId: n.id,
            label: info?.label ?? n.type,
            condition: conditionText(n, conditions[n.id] ?? [], info),
            chips: chipsFor(n, info),
            error: errors[n.id],
            readOnly: !editing,
          },
        }
      }),
    [graph, positions, palette, errors, editing, selection.nodeId, conditions],
  )

  const edges = useMemo<Edge[]>(
    () =>
      graph.edges.map((e) => ({
        id: edgeKey(e),
        source: e.from,
        target: e.to,
        selected:
          !!selection.edge &&
          selection.edge.from === e.from &&
          selection.edge.to === e.to,
        deletable: editing,
        data: { from: e.from, to: e.to },
      })),
    [graph.edges, editing, selection.edge],
  )

  const edgeLookup = useMemo(
    () => new Map(graph.edges.map((e) => [edgeKey(e), e])),
    [graph.edges],
  )

  // The selection the change handlers read; kept in a ref so a batch of
  // changes sees the current value without re-binding the callback.
  const selectionRef = useRef(selection)
  selectionRef.current = selection

  // In controlled mode React Flow reports every selection as a change; the
  // client applies them into its own selection state, which flows back
  // through the `selected` flags on the nodes and edges below. A batch can
  // carry both the new selection and the old one's deselect — the select
  // wins; a deselect only clears when it names the current selection.
  const handleNodesChange = useCallback(
    (changes: NodeChange<StepNode>[]) => {
      const dropped: string[] = []
      const deselected: string[] = []
      let selectedId: string | null | undefined
      for (const c of changes) {
        if (c.type === 'position' && c.position) {
          const { id, position } = c
          setPositions((prev) => ({ ...prev, [id]: position as RFPosition }))
        } else if (c.type === 'remove') {
          dropped.push(c.id)
        } else if (c.type === 'select') {
          if (c.selected) selectedId = c.id
          else deselected.push(c.id)
        }
      }
      // One change event for all removed steps so the version bumps once.
      if (dropped.length > 0) onChange(removeSteps(graph, dropped))
      if (selectedId !== undefined) {
        onSelect({ nodeId: selectedId, edge: null })
      } else if (deselected.includes(selectionRef.current.nodeId ?? '')) {
        onSelect({ nodeId: null, edge: null })
      }
    },
    [graph, onChange, onSelect],
  )

  const handleEdgesChange = useCallback(
    (changes: EdgeChange<Edge>[]) => {
      let next = graph
      const deselected: string[] = []
      let selected: { from: string; to: string } | null | undefined
      for (const c of changes) {
        if (c.type === 'remove') {
          const e = edgeLookup.get(c.id)
          if (e) next = removeEdge(next, e.from, e.to)
        } else if (c.type === 'select') {
          if (c.selected) {
            const e = edgeLookup.get(c.id)
            if (e) selected = { from: e.from, to: e.to }
          } else {
            deselected.push(c.id)
          }
        }
      }
      if (next !== graph) onChange(next)
      if (selected !== undefined) {
        onSelect({ nodeId: null, edge: selected })
      } else {
        for (const id of deselected) {
          const e = edgeLookup.get(id)
          const cur = selectionRef.current.edge
          if (e && cur && e.from === cur.from && e.to === cur.to) {
            onSelect({ nodeId: null, edge: null })
            break
          }
        }
      }
    },
    [graph, edgeLookup, onChange, onSelect],
  )

  const handleConnect = useCallback(
    (c: Connection) => {
      if (!c.source || !c.target) return
      onChange(applyConnect(graph, c.source, c.target))
    },
    [graph, onChange],
  )

  return (
    <div className="h-full w-full" data-testid="flow-canvas">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={handleConnect}
        nodesDraggable={editing}
        nodesConnectable={editing}
        selectNodesOnDrag={false}
        deleteKeyCode={editing ? ['Backspace', 'Delete'] : null}
        multiSelectionKeyCode={null}
        fitView
        fitViewOptions={{ padding: 0.15, maxZoom: 1 }}
        minZoom={0.2}
        maxZoom={1.75}
        colorMode="dark"
      >
        <Background variant={BackgroundVariant.Dots} gap={28} />
        <Controls showInteractive={false} />
        <MiniMap pannable zoomable className="!hidden md:!block" />
      </ReactFlow>
    </div>
  )
}
