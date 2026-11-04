// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One condensed live line for the file a node is working on (task 18):
 * the file name (truncated, full path on hover), a percent bar, the
 * encoding speed, the ETA and the original size against the growing
 * output, plus a Log action (task 27) that opens the structured log
 * viewer. One line at `sm` and up, two lines below it.
 */
import { useState } from 'react'
import type { Job } from './api'
import { formatBytes, formatEta } from './easyModel'
import JobLogDialog from './JobLogDialog'
import { btnGhost } from './uiControls'

function progressWidth(percent: number): string {
  return `${Math.max(0, Math.min(100, percent))}%`
}

export default function NodeJobLine({ job }: { job: Job }) {
  const name = job.file_name || job.file_id
  const eta = formatEta(job.eta_seconds)
  const [logOpen, setLogOpen] = useState(false)
  return (
    <div
      className="mt-2 text-xs text-fg-muted"
      data-testid={`node-job-line-${job.id}`}
    >
      <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
        {/* First line: what is being worked on, and how far along. */}
        <span className="flex min-w-0 items-center gap-2">
          <span
            className="truncate font-medium text-fg"
            title={job.file_path || name}
          >
            {name}
          </span>
          <span className="flex-none tabular-nums text-fg">
            {Math.round(job.percent)}%
          </span>
        </span>
        {/* Second line under `sm`, same line at `sm` and up. */}
        <span className="flex flex-none items-center gap-2 whitespace-nowrap tabular-nums">
          {job.speed > 0 && (
            <span data-testid="node-job-line-speed">
              {job.speed.toFixed(1)}x
            </span>
          )}
          {eta && <span data-testid="node-job-line-eta">{eta} left</span>}
          <span data-testid="node-job-line-size">
            {formatBytes(job.bytes_before)} → {formatBytes(job.bytes_so_far)}
          </span>
          <button
            type="button"
            onClick={() => setLogOpen(true)}
            className={btnGhost}
            data-testid="node-job-line-log"
          >
            Log
          </button>
        </span>
      </div>
      <div
        className="mt-1 h-1.5 overflow-hidden rounded-full bg-raised"
        role="progressbar"
        aria-valuenow={Math.round(job.percent)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`Progress of ${name}`}
        data-testid="node-job-line-bar"
      >
        <div
          className="h-full rounded-full bg-info transition-all"
          style={{ width: progressWidth(job.percent) }}
        />
      </div>
      {logOpen && <JobLogDialog job={job} onClose={() => setLogOpen(false)} />}
    </div>
  )
}
