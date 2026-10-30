// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The step palette (task 15): built entirely from the server's
 * step-types response (ARCHITECTURE.md — the UI holds no second copy of
 * the step list). Click or keyboard adds a step; drag is never the only
 * path to any action.
 */
import type { StepTypeInfo } from './api'

function PaletteEntry({
  info,
  onAdd,
}: {
  info: StepTypeInfo
  onAdd: (type: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onAdd(info.type)}
      className="w-full rounded-xl bg-slate-900 px-3 py-2 text-left ring-1 ring-slate-800 hover:bg-slate-800"
      data-testid={`palette-add-${info.type}`}
      aria-label={`Add ${info.label}`}
    >
      <span className="block text-sm font-semibold text-slate-200">
        {info.label}
      </span>
      {info.description && (
        <span className="mt-0.5 block text-xs text-slate-500">
          {info.description}
        </span>
      )}
    </button>
  )
}

export default function FlowPalette({
  palette,
  onAdd,
}: {
  palette: StepTypeInfo[]
  onAdd: (type: string) => void
}) {
  const conditions = palette.filter((p) => p.condition)
  const actions = palette.filter((p) => !p.condition)
  return (
    <aside
      className="space-y-4 rounded-2xl bg-slate-950/60 p-3 ring-1 ring-slate-800"
      data-testid="flow-palette"
      aria-label="Step palette"
    >
      <h2 className="px-1 text-xs font-semibold uppercase tracking-wide text-slate-500">
        Add a step
      </h2>
      {conditions.length > 0 && (
        <section className="space-y-2">
          <h3 className="px-1 text-xs text-slate-500">Conditions</h3>
          {conditions.map((info) => (
            <PaletteEntry key={info.type} info={info} onAdd={onAdd} />
          ))}
        </section>
      )}
      {actions.length > 0 && (
        <section className="space-y-2">
          <h3 className="px-1 text-xs text-slate-500">Steps</h3>
          {actions.map((info) => (
            <PaletteEntry key={info.type} info={info} onAdd={onAdd} />
          ))}
        </section>
      )}
    </aside>
  )
}
