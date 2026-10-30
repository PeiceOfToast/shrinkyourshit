// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One parameter input, rendered from the server's parameter schema
 * (task 14). Shared by the step stack rows and the flow canvas inspector
 * (task 15) so both editors speak the schema the same way. The server owns
 * validation; this only edits values.
 */
import type { FlowStep, StepParamField } from './api'

export interface ParamFieldProps {
  field: StepParamField
  step: FlowStep
  /** The input's full test id. */
  testId: string
  onChange: (name: string, value: unknown) => void
}

export default function ParamField({
  field,
  step,
  testId,
  onChange,
}: ParamFieldProps) {
  const params = (step.params ?? {}) as Record<string, unknown>
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
      {field.help && (
        <span className="text-xs text-slate-500">{field.help}</span>
      )}
    </label>
  )
}
