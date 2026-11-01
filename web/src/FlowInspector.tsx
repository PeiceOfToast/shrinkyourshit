// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The inspector (task 15): what the canvas selection is. Read-only mode
 * shows the step's parameters as text; edit mode renders the server's
 * parameter schema as inputs plus the menu alternative for every drag
 * action — connect to a step, disconnect the next hop, remove the step or
 * the selected connection — so drag is never the only path. Nothing here
 * decides validity: it edits the graph and the server re-validates.
 */
import { useEffect, useState } from 'react'
import type { FlowGraph, Plugin, StepTypeInfo } from './api'
import { chipsFor, conditionText, paletteFor } from './flowModel'
import {
  applyConnect,
  removeEdge,
  removeSteps,
  type CanvasSelection,
} from './flowCanvasModel'
import ParamField from './flowParams'

export interface FlowInspectorProps {
  graph: FlowGraph
  palette: StepTypeInfo[]
  /** The installed, enabled step plugins — the plugin kind's select. */
  plugins?: Plugin[]
  selection: CanvasSelection
  editing: boolean
  onChange: (graph: FlowGraph) => void
}

export default function FlowInspector({
  graph,
  palette,
  plugins,
  selection,
  editing,
  onChange,
}: FlowInspectorProps) {
  const [connectTarget, setConnectTarget] = useState('')

  useEffect(() => {
    setConnectTarget('')
  }, [selection.nodeId, selection.edge])

  const node = selection.nodeId
    ? (graph.nodes.find((n) => n.id === selection.nodeId) ?? null)
    : null

  if (node) {
    const info = paletteFor(palette, node.type)
    const outEdge = graph.edges.find((e) => e.from === node.id)
    const others = graph.nodes.filter((n) => n.id !== node.id)
    const updateParams = (name: string, value: unknown) => {
      onChange({
        ...graph,
        nodes: graph.nodes.map((n) =>
          n.id === node.id
            ? { ...n, params: { ...(n.params ?? {}), [name]: value } }
            : n,
        ),
      })
    }
    return (
      <div className="space-y-4" data-testid="flow-inspector">
        <header>
          <h2 className="text-sm font-semibold text-slate-200">
            {info?.label ?? node.type}
          </h2>
          <p className="text-xs text-slate-500">{node.id}</p>
        </header>

        {!editing && (
          <p
            className="text-xs text-slate-500"
            data-testid="flow-inspector-readonly-note"
          >
            Read only — choose “Edit flow” to change this flow.
          </p>
        )}

        {info && info.condition && (
          <p className="text-xs text-slate-500">
            {conditionText(node, [], info)}
          </p>
        )}

        {editing ? (
          info && info.params.length > 0 ? (
            <div className="grid gap-3" data-testid={`flow-params-${node.id}`}>
              {info.params.map((f) => (
                <ParamField
                  key={f.name}
                  field={f}
                  step={node}
                  plugins={plugins}
                  testId={`flow-param-${f.name}`}
                  onChange={updateParams}
                />
              ))}
            </div>
          ) : (
            <p className="text-xs text-slate-500">This step has no settings.</p>
          )
        ) : (
          info &&
          info.params.length > 0 && (
            <div
              className="flex flex-wrap gap-1"
              data-testid={`flow-inspector-chips-${node.id}`}
            >
              {chipsFor(node, info).map((chip, i) => (
                <span
                  key={i}
                  className="rounded-full bg-slate-800 px-2 py-0.5 text-[10px] text-slate-300"
                >
                  {chip}
                </span>
              ))}
            </div>
          )
        )}

        {editing && (
          <div className="grid gap-3 border-t border-slate-800 pt-3">
            {others.length > 0 && (
              <div className="grid gap-1">
                <span className="text-xs font-medium text-slate-400">
                  Connect this step to
                </span>
                <div className="flex gap-2">
                  <select
                    value={connectTarget}
                    onChange={(e) => setConnectTarget(e.target.value)}
                    className="min-w-0 flex-1 rounded-lg bg-slate-800 px-3 py-2 text-sm"
                    aria-label="Step to connect to"
                    data-testid="flow-inspector-connect-select"
                  >
                    <option value="">Choose a step…</option>
                    {others.map((n) => {
                      const otherInfo = paletteFor(palette, n.type)
                      return (
                        <option key={n.id} value={n.id}>
                          {otherInfo?.label ?? n.type} ({n.id})
                        </option>
                      )
                    })}
                  </select>
                  <button
                    type="button"
                    disabled={!connectTarget}
                    onClick={() => {
                      onChange(applyConnect(graph, node.id, connectTarget))
                      setConnectTarget('')
                    }}
                    className="rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-slate-200 hover:bg-slate-700 disabled:opacity-50"
                    data-testid="flow-inspector-connect"
                  >
                    Connect
                  </button>
                </div>
                <span className="text-xs text-slate-500">
                  Connecting replaces the connection it displaces — the chain
                  stays a single line.
                </span>
              </div>
            )}
            {outEdge && (
              <button
                type="button"
                onClick={() =>
                  onChange(removeEdge(graph, outEdge.from, outEdge.to))
                }
                className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-slate-200 hover:bg-slate-700"
                data-testid="flow-inspector-disconnect"
              >
                Disconnect the next step
              </button>
            )}
            <button
              type="button"
              onClick={() => onChange(removeSteps(graph, [node.id]))}
              className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-rose-400 hover:bg-slate-700"
              data-testid="flow-inspector-remove"
            >
              Remove this step
            </button>
          </div>
        )}
      </div>
    )
  }

  if (selection.edge) {
    const from = graph.nodes.find((n) => n.id === selection.edge?.from)
    const to = graph.nodes.find((n) => n.id === selection.edge?.to)
    const label = (n: typeof from) =>
      n ? (paletteFor(palette, n.type)?.label ?? n.type) : '?'
    return (
      <div className="space-y-4" data-testid="flow-inspector">
        <header>
          <h2 className="text-sm font-semibold text-slate-200">Connection</h2>
        </header>
        <p className="text-sm text-slate-300" data-testid="flow-inspector-edge">
          {label(from)} → {label(to)}
        </p>
        {editing && (
          <button
            type="button"
            onClick={() =>
              onChange(
                removeEdge(graph, selection.edge!.from, selection.edge!.to),
              )
            }
            className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm font-semibold text-rose-400 hover:bg-slate-700"
            data-testid="flow-inspector-remove-edge"
          >
            Remove connection
          </button>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-2" data-testid="flow-inspector-empty">
      <h2 className="text-sm font-semibold text-slate-400">Nothing selected</h2>
      <p className="text-xs text-slate-500">
        Select a step on the canvas to see its settings.
      </p>
      {editing && (
        <p className="text-xs text-slate-500">
          Add a step from the palette, then connect it into the chain.
        </p>
      )}
    </div>
  )
}
