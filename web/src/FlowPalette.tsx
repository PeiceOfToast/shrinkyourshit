// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The step palette (task 15): built entirely from the server's
 * step-types response (ARCHITECTURE.md — the UI holds no second copy of
 * the step list). Click or keyboard adds a step; drag is never the only
 * path to any action.
 */
import type { StepTypeInfo } from './api'
import { card, cardPad } from './uiControls'

// The entry and the plugin hint share one look: a 44 px row on the canvas
// surface, with the label and a caption description.
const entry =
  'flex min-h-11 w-full flex-col items-start rounded-control bg-canvas px-3 py-2 text-left ring-1 ring-line transition-colors duration-fast ease-standard hover:bg-raised'

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
      className={entry}
      data-testid={`palette-add-${info.type}`}
      aria-label={`Add ${info.label}`}
    >
      <span className="block text-body font-semibold text-fg">
        {info.label}
      </span>
      {info.description && (
        <span className="mt-0.5 block text-caption text-fg-muted">
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
      className={`flex flex-col gap-4 ${card} ${cardPad}`}
      data-testid="flow-palette"
      aria-label="Step palette"
    >
      <h2 className="text-heading text-fg">Add a step</h2>
      {conditions.length > 0 && (
        <section className="flex flex-col gap-2">
          <h3 className="text-caption font-medium uppercase tracking-wider text-fg-muted">
            Conditions
          </h3>
          {conditions.map((info) => (
            <PaletteEntry key={info.type} info={info} onAdd={onAdd} />
          ))}
        </section>
      )}
      {actions.length > 0 && (
        <section className="flex flex-col gap-2">
          <h3 className="text-caption font-medium uppercase tracking-wider text-fg-muted">
            Steps
          </h3>
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
      className={entry}
      data-testid="palette-plugin-hint"
    >
      <span className="block text-body font-semibold text-fg">
        {info.label}
      </span>
      <span className="mt-0.5 block text-caption text-fg-muted">
        No step plugin is installed and enabled yet. Install one on the
        Integrations page's plugins tab, then come back here.
      </span>
    </a>
  )
}
