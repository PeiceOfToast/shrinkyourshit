// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Banner (task 38, spec 3.7): a page-level notice that is not an error (the
 * errors use ErrorBox). One shape with four tones replaces the five amber and
 * sky styles. `danger` is announced at once (role="alert"); the others are
 * status updates (role="status").
 */
import type { ReactNode } from 'react'
import Icon, { type IconName } from './Icon'

type Tone = 'info' | 'success' | 'warning' | 'danger'

// Full class strings, not interpolated: Tailwind has to see every one.
const TONES: Record<
  Tone,
  { box: string; icon: string; defaultIcon: IconName }
> = {
  info: {
    box: 'bg-info/10 ring-info/30',
    icon: 'text-info',
    defaultIcon: 'info',
  },
  success: {
    box: 'bg-success/10 ring-success/30',
    icon: 'text-success',
    defaultIcon: 'check',
  },
  warning: {
    box: 'bg-warning/10 ring-warning/30',
    icon: 'text-warning',
    defaultIcon: 'alert',
  },
  danger: {
    box: 'bg-danger/10 ring-danger/30',
    icon: 'text-danger',
    defaultIcon: 'alert',
  },
}

export default function Banner({
  tone,
  icon,
  title,
  children,
  action,
  testId,
}: {
  tone: Tone
  icon?: IconName
  title?: ReactNode
  children: ReactNode
  action?: ReactNode
  testId?: string
}) {
  const style = TONES[tone]
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      data-testid={testId}
      className={`flex items-start gap-3 rounded-card p-4 ring-1 ring-inset ${style.box}`}
    >
      <Icon
        name={icon ?? style.defaultIcon}
        className={`mt-0.5 h-5 w-5 shrink-0 ${style.icon}`}
      />
      <div className="min-w-0 flex-1 text-body text-fg">
        {title && <p className="font-semibold">{title}</p>}
        <div className={title ? 'mt-1' : undefined}>{children}</div>
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
