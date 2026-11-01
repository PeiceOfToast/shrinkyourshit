// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One parameter input, rendered from the server's parameter schema
 * (task 14). Shared by the step stack rows and the flow canvas inspector
 * (task 15) so both editors speak the schema the same way. The server owns
 * validation; this only edits values.
 *
 * Kind `plugin` (task 32) renders a select of installed, enabled step
 * plugins plus a settings form derived from the selected plugin's
 * manifest: one input per setting key, per-step values overriding the
 * stored ones, and a "Use installed value" reset that clears the override.
 */
import type { FlowStep, Plugin, StepParamField } from './api'

export interface ParamFieldProps {
  field: StepParamField
  step: FlowStep
  /** The input's full test id. */
  testId: string
  /** The installed, enabled step plugins — only for kind `plugin`. */
  plugins?: Plugin[]
  onChange: (name: string, value: unknown) => void
}

/** stepParams reads the step's params as a record. */
function stepParams(step: FlowStep): Record<string, unknown> {
  return (step.params ?? {}) as Record<string, unknown>
}

export default function ParamField({
  field,
  step,
  testId,
  plugins,
  onChange,
}: ParamFieldProps) {
  const params = stepParams(step)
  return (
    <label className="grid gap-1">
      <span className="text-xs font-medium text-slate-400">
        {field.label}
        {field.required ? ' *' : ''}
      </span>
      {field.kind === 'select' && (
        <select
          value={String(params[field.name] ?? field.default ?? '')}
          onChange={(e) => onChange(field.name, e.target.value)}
          className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
          data-testid={testId}
        >
          {(field.options ?? []).map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      )}
      {field.kind === 'number' && (
        <input
          type="number"
          min={field.min}
          max={field.max}
          value={Number(params[field.name] ?? field.default ?? 0)}
          onChange={(e) => onChange(field.name, Number(e.target.value))}
          className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
          data-testid={testId}
        />
      )}
      {field.kind === 'text' && (
        <input
          type="text"
          value={String(params[field.name] ?? '')}
          onChange={(e) => onChange(field.name, e.target.value)}
          className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
          data-testid={testId}
        />
      )}
      {field.kind === 'languages' && (
        <input
          type="text"
          placeholder="eng, ger"
          value={((params[field.name] as string[]) ?? []).join(', ')}
          onChange={(e) =>
            onChange(
              field.name,
              e.target.value
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean),
            )
          }
          className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
          data-testid={testId}
        />
      )}
      {field.kind === 'toggle' && (
        <input
          type="checkbox"
          checked={Boolean(params[field.name])}
          onChange={(e) => onChange(field.name, e.target.checked)}
          className="h-4 w-4 accent-emerald-500"
          data-testid={testId}
        />
      )}
      {field.kind === 'plugin' && (
        <PluginParam
          params={params}
          plugins={plugins ?? []}
          testId={testId}
          onChange={onChange}
        />
      )}
      {field.help && (
        <span className="text-xs text-slate-500">{field.help}</span>
      )}
    </label>
  )
}

/** PluginParam is the `plugin` kind: the plugin select plus the settings
 * form derived from the selected plugin's manifest. */
function PluginParam({
  params,
  plugins,
  testId,
  onChange,
}: {
  params: Record<string, unknown>
  plugins: Plugin[]
  testId: string
  onChange: (name: string, value: unknown) => void
}) {
  const selected = plugins.find(
    (p) => p.name === String(params['plugin'] ?? ''),
  )
  const settings = (params['settings'] ?? {}) as Record<string, string>
  const setSettings = (key: string, value: string) => {
    const next = { ...settings }
    if (value === '') delete next[key]
    else next[key] = value
    onChange('settings', next)
  }
  return (
    <div className="grid gap-2" data-testid={`${testId}-plugin`}>
      <select
        value={String(params['plugin'] ?? '')}
        onChange={(e) => onChange('plugin', e.target.value)}
        className="w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
        data-testid={testId}
      >
        {plugins.length === 0 && (
          <option value="">No step plugin installed</option>
        )}
        {plugins.map((p) => (
          <option key={p.id} value={p.name}>
            {p.name}
          </option>
        ))}
      </select>
      {selected?.settings?.length ? (
        <div className="grid gap-2 rounded-lg bg-slate-900/60 p-2">
          {selected.settings.map((s) => (
            <label key={s.key} className="grid gap-1">
              <span className="flex items-center justify-between gap-2">
                <span className="text-xs text-slate-400">
                  {s.label}
                  {s.required ? ' *' : ''}
                </span>
                {settings[s.key] !== undefined && (
                  <button
                    type="button"
                    onClick={() => setSettings(s.key, '')}
                    className="rounded px-1.5 py-0.5 text-[11px] text-slate-400 hover:bg-slate-800 hover:text-slate-200"
                    data-testid={`${testId}-reset-${s.key}`}
                  >
                    Use installed value
                  </button>
                )}
              </span>
              <input
                type="text"
                value={settings[s.key] ?? ''}
                placeholder={
                  selected.values?.[s.key] !== undefined
                    ? 'installed value'
                    : 'unset'
                }
                onChange={(e) => setSettings(s.key, e.target.value)}
                className="w-full rounded-lg bg-slate-800 px-2 py-1.5 text-sm"
                data-testid={`${testId}-setting-${s.key}`}
              />
            </label>
          ))}
          <p className="text-xs text-slate-500">
            Per-step values override the plugin's stored settings for this step
            only.
          </p>
        </div>
      ) : null}
    </div>
  )
}
