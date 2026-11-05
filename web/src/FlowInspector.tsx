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
import Badge from './Badge'
import ParamField from './flowParams'
import { btn, btnBlock, btnDangerGhost, select } from './uiControls'

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
        <header className="flex flex-col gap-1">
          <h2 className="text-heading text-fg">{info?.label ?? node.type}</h2>
          <p
            className="break-all font-mono text-caption text-fg-muted"
            data-testid="flow-inspector-id"
          >
            {node.id}
          </p>
        </header>

        {info?.unsafe && (
          <p
            className="text-body font-medium text-warning"
            data-testid="flow-inspector-unsafe"
          >
            Passes arguments the app does not model. Check them before you save.
          </p>
        )}

        {!editing && (
          <p
            className="text-caption text-fg-muted"
            data-testid="flow-inspector-readonly-note"
          >
            Read only — choose “Edit flow” to change this flow.
          </p>
        )}

        {info && info.condition && (
          <p className="text-caption text-fg-muted">
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
            <p className="text-caption text-fg-muted">
              This step has no settings.
            </p>
          )
        ) : (
          info &&
          info.params.length > 0 && (
            <div
              className="flex flex-wrap gap-1"
              data-testid={`flow-inspector-chips-${node.id}`}
            >
              {chipsFor(node, info).map((chip, i) => (
                <Badge key={i}>{chip}</Badge>
              ))}
            </div>
          )
        )}

        {editing && (
          <div className="flex flex-col gap-4 border-t border-line pt-4">
            {others.length > 0 && (
              <div className="flex flex-col gap-2">
                <span className="text-body font-medium text-fg">
                  Connect this step to
                </span>
                <div className="flex gap-2">
                  <select
                    value={connectTarget}
                    onChange={(e) => setConnectTarget(e.target.value)}
                    className={`${select} min-w-0 flex-1`}
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
                    className={btn}
                    data-testid="flow-inspector-connect"
                  >
                    Connect
                  </button>
                </div>
                <span className="text-caption text-fg-muted">
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
                className={`${btn} ${btnBlock}`}
                data-testid="flow-inspector-disconnect"
              >
                Disconnect the next step
              </button>
            )}
            <button
              type="button"
              onClick={() => onChange(removeSteps(graph, [node.id]))}
              className={`${btnDangerGhost} ${btnBlock}`}
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
          <h2 className="text-heading text-fg">Connection</h2>
        </header>
        <p className="text-body text-fg" data-testid="flow-inspector-edge">
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
            className={`${btnDangerGhost} ${btnBlock}`}
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
      <h2 className="text-heading text-fg">Nothing selected</h2>
      <p className="text-caption text-fg-muted">
        Select a step to see its settings.
      </p>
      {editing && (
        <p className="text-caption text-fg-muted">
          Add a step from the palette, then connect it into the chain.
        </p>
      )}
    </div>
  )
}
