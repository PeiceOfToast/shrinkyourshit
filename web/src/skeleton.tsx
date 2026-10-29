// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Skeletons (task 13): loading placeholders for tables and stat cards, so
 * a slow server looks busy rather than broken. Pure shapes — the pages
 * compose them with their own layout.
 */
export function Skeleton({ className = '' }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={`animate-pulse rounded bg-slate-800 ${className}`}
    />
  )
}

/** One pulsing card, the shape of a stat tile or a list row. */
export function SkeletonCard({ className = 'h-16' }: { className?: string }) {
  return (
    <div
      role="status"
      aria-label="Loading"
      className={`rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800 ${className}`}
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
      className="space-y-2 rounded-2xl bg-slate-900/50 p-4 ring-1 ring-slate-800"
      data-testid="rows-skeleton"
    >
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-5 w-full" />
      ))}
    </div>
  )
}
