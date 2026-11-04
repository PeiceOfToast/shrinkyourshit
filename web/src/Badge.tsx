// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Badge (task 39, spec 4.1): the one small pill for a status, a tag or a
 * count. It replaces the node pills, encoder chips, level chips and flow
 * chips that each had their own classes. Tones follow the tint recipe in
 * spec 2.2: `bg-{tone}/10 text-{tone} ring-1 ring-inset ring-{tone}/30`.
 * Neutral is a raised surface with muted text.
 *
 * The tone classes are written out in full below so Tailwind can see them.
 * Do not build them with string templates.
 */
import type { ReactNode } from 'react'

export type BadgeTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

const TONE: Record<BadgeTone, string> = {
  neutral: 'bg-raised text-fg-muted ring-1 ring-inset ring-line',
  info: 'bg-info/10 text-info ring-1 ring-inset ring-info/30',
  success: 'bg-success/10 text-success ring-1 ring-inset ring-success/30',
  warning: 'bg-warning/10 text-warning ring-1 ring-inset ring-warning/30',
  danger: 'bg-danger/10 text-danger ring-1 ring-inset ring-danger/30',
}

const DOT: Record<BadgeTone, string> = {
  neutral: 'bg-fg-muted',
  info: 'bg-info',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
}

export default function Badge({
  tone = 'neutral',
  dot = false,
  pulse = false,
  children,
  testId,
}: {
  tone?: BadgeTone
  /** A leading dot in the tone colour (a running or working state). */
  dot?: boolean
  /** Pulses the dot. Disabled under reduced motion by the global CSS rule. */
  pulse?: boolean
  children: ReactNode
  testId?: string
}) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-caption font-medium ${TONE[tone]}`}
      data-testid={testId}
    >
      {dot && (
        <span
          aria-hidden="true"
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${DOT[tone]} ${
            pulse ? 'animate-pulse' : ''
          }`}
        />
      )}
      {children}
    </span>
  )
}
