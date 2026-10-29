// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The one content container every page uses (task 10): centered, capped
 * width, responsive gutters. Pages must not add their own `max-w-*` on
 * top of it — the shared grid is the point.
 */
import type { ComponentProps } from 'react'

export default function PageContainer({
  className = '',
  ...rest
}: ComponentProps<'div'>) {
  return (
    <div
      className={`mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8 ${className}`}
      {...rest}
    />
  )
}
