// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The rule tiers as the UI names them (task 62, spec 6 and 7). Basic is the
 * sliders, Moderate the rule tree, Advanced the visual canvas. A tier whose
 * editor is handed a graph it cannot represent renders read only: the
 * unrepresentable list from the server is the lock.
 */
import type { FlowFieldError, RuleTier } from './api'

export const TIER_OPTIONS: Array<{ id: RuleTier; label: string }> = [
  { id: 'basic', label: 'Basic' },
  { id: 'moderate', label: 'Moderate' },
  { id: 'advanced', label: 'Advanced' },
]

/** The plain word for a tier, as the library page and the editors show it. */
export function tierLabel(tier: RuleTier): string {
  return TIER_OPTIONS.find((t) => t.id === tier)?.label ?? tier
}

/**
 * The step id a representability error names: the server writes it into the
 * message as `step "<id>"`. Empty for a graph-level error (for example the
 * basic tier's missing guard), which the caller shows as its message.
 */
export function stepOf(err: FlowFieldError): string {
  const m = /step "([^"]+)"/.exec(err.message)
  return m ? m[1] : ''
}

/**
 * The lowest tier that can edit a step, read from the server's message
 * (`... the moderate tier can edit it`). Empty when the message does not say.
 */
export function editableIn(err: FlowFieldError): RuleTier | '' {
  const m = /the (basic|moderate|advanced) tier can edit it/.exec(err.message)
  return m ? (m[1] as RuleTier) : ''
}

/** True when the list locks an editor (any step the tier cannot express). */
export function isLocked(
  unrepresentable: FlowFieldError[] | undefined,
): boolean {
  return (unrepresentable?.length ?? 0) > 0
}
