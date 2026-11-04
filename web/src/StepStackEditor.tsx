// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The step stack editor (task 14, tier 2): an ordered list of steps, each
 * row showing its condition in muted text and its parameters as chips.
 * Rows drag to reorder (plus up/down buttons for keyboard and tests),
 * add from the server's palette and remove. Validation errors from the
 * server render on the row that failed.
 *
 * Styling (task 45, spec 4.3): every row icon button is 44 px with an
 * aria-label, the remove button is the danger tint, and the add control is
 * the shared `btn` beside a `select`. The step rows sit inside the Rules
 * page's Card, so they use the canvas surface rather than a second Card.
 */
import { useState } from 'react'
import Badge from './Badge'
import type { FlowGraph, FlowStep, Plugin, StepTypeInfo } from './api'
import {
  chainEdges,
  chipsFor,
  conditionText,
  gatesFor,
  orderedSteps,
  paletteFor,
  stepFromPalette,
} from './flowModel'
import ParamField from './flowParams'
import { btn, btnIcon, select } from './uiControls'

/** Remove is the one destructive row action: the danger text, not muted. */
const removeIcon = btnIcon
  .replace('text-fg-muted', 'text-danger')
  .replace('hover:text-fg', 'hover:bg-danger/10')

export interface StepStackEditorProps {
  graph: FlowGraph
  palette: StepTypeInfo[]
  /** The installed, enabled step plugins — the plugin kind's select. */
  plugins?: Plugin[]
  /** Server field errors keyed by step id (or "graph"). */
  errors: Record<string, string>
  onChange: (graph: FlowGraph) => void
}

export default function StepStackEditor({
  graph,
  palette,
  plugins,
  errors,
  onChange,
}: StepStackEditorProps) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [addType, setAddType] = useState<string>('')

  const steps = orderedSteps(graph)

  const setSteps = (next: FlowStep[]) =>
    onChange({ schema: graph.schema, nodes: next, edges: chainEdges(next) })

  const move = (from: number, to: number) => {
    if (to < 0 || to >= steps.length || from === to) return
    const next = [...steps]
    const [row] = next.splice(from, 1)
    next.splice(to, 0, row)
    setSteps(next)
  }

  const remove = (id: string) => setSteps(steps.filter((s) => s.id !== id))

  const add = (type: string) => {
    const info = paletteFor(palette, type)
    if (!info) return
    setSteps([...steps, stepFromPalette(info, steps)])
  }

  const updateParams = (id: string, name: string, value: unknown) => {
    setSteps(
      steps.map((s) =>
        s.id === id
          ? { ...s, params: { ...(s.params ?? {}), [name]: value } }
          : s,
      ),
    )
  }

  return (
    <div className="space-y-3" data-testid="step-stack">
      {steps.map((step, i) => {
        const info = paletteFor(palette, step.type)
        const gatedBy = gatesFor(steps, i)
        const cond = conditionText(step, gatedBy, info)
        const rowError = errors[step.id]
        return (
          <div
            key={step.id}
            draggable
            onDragStart={() => setDragIndex(i)}
            onDragOver={(e) => e.preventDefault()}
            onDrop={() => {
              if (dragIndex !== null) move(dragIndex, i)
              setDragIndex(null)
            }}
            data-testid={`step-row-${step.id}`}
            className={`rounded-card bg-canvas p-4 ring-1 ${
              rowError ? 'ring-danger/60' : 'ring-line'
            }`}
          >
            <div className="flex items-center gap-2">
              <span
                className="flex h-11 w-6 shrink-0 cursor-grab select-none items-center justify-center text-fg-subtle"
                title="Drag to reorder"
                data-testid={`step-drag-${step.id}`}
              >
                ⠿
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-baseline gap-2">
                  <span className="font-semibold text-fg">
                    {info?.label ?? step.type}
                  </span>
                  <span className="truncate font-mono text-caption text-fg-muted">
                    {step.id}
                  </span>
                </div>
                {cond && (
                  <p
                    className="text-caption text-fg-muted"
                    data-testid={`step-condition-${step.id}`}
                  >
                    {cond}
                  </p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  onClick={() => move(i, i - 1)}
                  className={btnIcon}
                  aria-label={`Move ${step.id} up`}
                  data-testid={`step-up-${step.id}`}
                >
                  ↑
                </button>
                <button
                  type="button"
                  onClick={() => move(i, i + 1)}
                  className={btnIcon}
                  aria-label={`Move ${step.id} down`}
                  data-testid={`step-down-${step.id}`}
                >
                  ↓
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setExpanded(expanded === step.id ? null : step.id)
                  }
                  className={btnIcon}
                  aria-label={`Edit ${step.id}`}
                  aria-expanded={expanded === step.id}
                  data-testid={`step-edit-${step.id}`}
                >
                  ✎
                </button>
                <button
                  type="button"
                  onClick={() => remove(step.id)}
                  className={removeIcon}
                  aria-label={`Remove ${step.id}`}
                  data-testid={`step-remove-${step.id}`}
                >
                  ✕
                </button>
              </div>
            </div>

            {info && info.params.length > 0 && (
              <div
                className="mt-2 flex flex-wrap gap-2"
                data-testid={`step-chips-${step.id}`}
              >
                {chipsFor(step, info).map((chip, ci) => (
                  <Badge key={ci}>{chip}</Badge>
                ))}
              </div>
            )}

            {rowError && (
              <p
                className="mt-2 text-caption text-danger"
                data-testid={`step-error-${step.id}`}
              >
                {rowError}
              </p>
            )}

            {expanded === step.id && info && (
              <div
                className="mt-3 grid gap-4 border-t border-line pt-3"
                data-testid={`step-params-${step.id}`}
              >
                {info.params.map((f) => (
                  <ParamField
                    key={f.name}
                    field={f}
                    step={step}
                    plugins={plugins}
                    testId={`step-param-${f.name}`}
                    onChange={(name, value) =>
                      updateParams(step.id, name, value)
                    }
                  />
                ))}
              </div>
            )}
          </div>
        )
      })}

      <div className="flex flex-wrap items-center gap-2" data-testid="step-add">
        <select
          value={addType}
          onChange={(e) => setAddType(e.target.value)}
          className={`${select} max-w-xs`}
          aria-label="Step type to add"
          data-testid="step-add-type"
        >
          <option value="">Choose a step…</option>
          {palette.map((p) => (
            <option key={p.type} value={p.type}>
              {p.label}
            </option>
          ))}
        </select>
        <button
          type="button"
          disabled={!addType}
          onClick={() => {
            add(addType)
            setAddType('')
          }}
          className={btn}
          data-testid="step-add-button"
        >
          Add step
        </button>
      </div>

      {errors['graph'] && (
        <p
          className="text-caption text-danger"
          data-testid="step-stack-graph-error"
        >
          {errors['graph']}
        </p>
      )}
    </div>
  )
}
