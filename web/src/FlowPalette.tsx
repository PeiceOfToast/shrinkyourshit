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

/** The enabled step plugins a palette entry offers, when it is the plugin
 * step (its `plugin` param's options; task 32). */
function pluginOptions(info: StepTypeInfo): string[] {
  const param = info.params.find((f) => f.kind === 'plugin')
  return param?.options ?? []
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
  const pluginInfo = actions.find((p) => p.type === 'plugin')
  const hasStepPlugins =
    pluginInfo !== undefined && pluginOptions(pluginInfo).length > 0
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
          {actions.map((info) =>
            info.type === 'plugin' && !hasStepPlugins ? (
              <PalettePluginHint key={info.type} info={info} />
            ) : (
              <PaletteEntry key={info.type} info={info} onAdd={onAdd} />
            ),
          )}
        </section>
      )}
    </aside>
  )
}

/** PalettePluginHint replaces the "Run a plugin" entry while no enabled
 * step plugin exists: it explains how to install one and links to the
 * plugins tab. */
function PalettePluginHint({ info }: { info: StepTypeInfo }) {
  return (
    <a
      href="/integrations?tab=plugins"
      className="block w-full rounded-xl bg-slate-900 px-3 py-2 text-left ring-1 ring-slate-800 hover:bg-slate-800"
      data-testid="palette-plugin-hint"
    >
      <span className="block text-sm font-semibold text-slate-200">
        {info.label}
      </span>
      <span className="mt-0.5 block text-xs text-slate-500">
        No step plugin is installed and enabled yet. Install one on the
        Integrations page's plugins tab, then come back here.
      </span>
    </a>
  )
}
