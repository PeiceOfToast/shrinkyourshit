// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactNode } from 'react'
import Icon, { type IconName } from './Icon'
import { card } from './uiControls'

type Props = {
  testid: string
  label: string
  value: string | number
  /** A named icon from Icon.tsx, shown beside the label. */
  icon: IconName
  /** Colours the value: `good` for a success, `warning`/`danger` for a problem. */
  tone?: 'default' | 'good' | 'warning' | 'danger'
  /** A caption under the value, such as "from 3 nodes". */
  sub?: ReactNode
}

const VALUE_TONE: Record<NonNullable<Props['tone']>, string> = {
  default: 'text-fg',
  good: 'text-accent-text',
  warning: 'text-warning',
  danger: 'text-danger',
}

/**
 * One dashboard statistic (task 21, restyled in task 39). The icon and the
 * value scale with the viewport via clamp(), so a 320 px phone and a 4K
 * monitor both look deliberate rather than zoomed. Labels are overline
 * text; the value is a semibold tabular-number.
 */
export default function StatCard({
  testid,
  label,
  value,
  icon,
  tone = 'default',
  sub,
}: Props) {
  return (
    <div className={`${card} p-3 sm:p-4`} data-testid={testid}>
      <div className="flex items-center gap-2">
        <Icon
          name={icon}
          className="h-[clamp(16px,4vw,22px)] w-[clamp(16px,4vw,22px)] shrink-0 text-fg-subtle"
        />
        <p className="text-caption font-medium uppercase tracking-wider text-fg-muted">
          {label}
        </p>
      </div>
      <p
        className={`mt-1 text-[clamp(1.5rem,3.2vw,1.875rem)] font-semibold leading-tight tabular-nums ${VALUE_TONE[tone]}`}
      >
        {value}
      </p>
      {sub && <p className="mt-1 text-caption text-fg-muted">{sub}</p>}
    </div>
  )
}
