// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Skeletons (task 13, restyled in task 39): loading placeholders for tables
 * and stat cards, so a slow server looks busy rather than broken. Pure
 * shapes; the pages compose them with their own layout. The pulse stops
 * under reduced motion (spec 2.7), and the colour is the raised surface.
 */
import { card, cardPad } from './uiControls'

export function Skeleton({ className = '' }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={`animate-pulse rounded bg-raised motion-reduce:animate-none ${className}`}
    />
  )
}

/** One pulsing card, the shape of a stat tile or a list row. */
export function SkeletonCard({ className = 'h-16' }: { className?: string }) {
  return (
    <div
      role="status"
      aria-label="Loading"
      className={`${card} p-4 ${className}`}
    >
      <Skeleton className="h-4 w-1/3" />
      <Skeleton className="mt-3 h-6 w-1/2" />
    </div>
  )
}

/** Four stat tiles, the dashboard's grid. */
export function SkeletonStatCards() {
  return (
    <div
      className="grid grid-cols-2 gap-4 sm:grid-cols-4"
      data-testid="stat-skeleton"
    >
      {[0, 1, 2, 3].map((i) => (
        <SkeletonCard key={i} />
      ))}
    </div>
  )
}

/** Rows of a table or list, the file and library shapes. */
export function SkeletonRows({ rows = 4 }: { rows?: number }) {
  return (
    <div
      role="status"
      aria-label="Loading"
      className={`${card} ${cardPad}`}
      data-testid="rows-skeleton"
    >
      <div className="flex flex-col gap-2">
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} className="h-5 w-full" />
        ))}
      </div>
    </div>
  )
}
