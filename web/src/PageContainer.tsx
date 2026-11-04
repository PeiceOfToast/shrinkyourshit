// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The one content container every page uses (task 10): centered, capped
 * width, responsive gutters. Pages must not add their own `max-w-*` on
 * top of it — the shared grid is the point.
 *
 * Task 38 (spec 3.2): the cap is `max-w-6xl` on every route, Flow included,
 * and the inner layout is a single `flex flex-col gap-6`.
 */
import type { ComponentProps } from 'react'

export default function PageContainer({
  className = '',
  ...rest
}: ComponentProps<'div'>) {
  return (
    <div
      className={`mx-auto flex w-full max-w-6xl flex-col gap-6 px-4 sm:px-6 lg:px-8 ${className}`}
      {...rest}
    />
  )
}
