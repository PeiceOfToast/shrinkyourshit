// SPDX-License-Identifier: AGPL-3.0-only

/**
 * FileHistory (task 19): one file's job history in a dialog, over
 * GET /files/{id}/jobs. Every job shows its attempt, status, encoder,
 * fallback reasons, bytes before and after, and the full structured log
 * behind a Log action (task 27). Empty and error states follow the task 13
 * conventions.
 */
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { getFileJobs, type FileItem, type Job } from './api'
import Badge, { type BadgeTone } from './Badge'
import { statusLabel } from './easyModel'
import ErrorBox from './ErrorBox'
import { formatBytes } from './format'
import JobLogDialog from './JobLogDialog'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { btn, btnGhost } from './uiControls'

function jobStatusTone(status: string): BadgeTone {
  switch (status) {
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'running':
    case 'claimed':
      return 'info'
    case 'cancelled':
      return 'neutral'
    default:
      return 'warning'
  }
}

/** One job's row in the history: the facts the owner asked for. */
function JobRow({
  job,
  onOpenLog,
}: {
  job: Job
  onOpenLog: (job: Job) => void
}) {
  const hasSizes = job.bytes_before > 0
  return (
    <li className="py-3" data-testid="file-history-job">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-body">
        <Badge tone={jobStatusTone(job.status)}>
          {statusLabel(job.status)}
        </Badge>
        <span className="text-fg-muted" data-testid="file-history-attempt">
          Attempt {job.attempt + 1}
        </span>
        <span className="text-fg-muted" data-testid="file-history-encoder">
          <span className="text-fg-subtle">Encoder: </span>
          {job.encoder_used || '—'}
        </span>
        {hasSizes && (
          <span
            className="tabular-nums text-fg-muted"
            data-testid="file-history-bytes"
          >
            <span className="text-fg-subtle">Size: </span>
            {formatBytes(job.bytes_before)}
            {' → '}
            {job.bytes_after > 0 ? formatBytes(job.bytes_after) : '—'}
          </span>
        )}
        {job.status === 'running' && job.percent > 0 && (
          <span className="text-info">{Math.round(job.percent)}%</span>
        )}
        {job.has_log && (
          <button
            type="button"
            onClick={() => onOpenLog(job)}
            className={btnGhost}
            data-testid={`file-history-log-${job.id}`}
          >
            Log
          </button>
        )}
      </div>
      {job.fallback_reasons.length > 0 && (
        <ul
          className="mt-2 space-y-1 text-body text-warning"
          data-testid="file-history-fallbacks"
        >
          {job.fallback_reasons.map((reason, i) => (
            <li key={i}>Fell back: {reason}</li>
          ))}
        </ul>
      )}
      {job.error && (
        <p
          className="mt-2 break-words text-body text-danger"
          data-testid="file-history-error-text"
        >
          {job.error}
        </p>
      )}
      {job.log && (
        <details className="mt-2" data-testid="file-history-log">
          <summary className="cursor-pointer text-body text-fg-muted hover:text-fg">
            Log
          </summary>
          <pre
            className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-control bg-canvas p-3 text-caption text-fg-muted"
            data-testid="file-history-log-text"
          >
            {job.log}
          </pre>
        </details>
      )}
    </li>
  )
}

export default function FileHistory({
  file,
  onClose,
}: {
  file: FileItem
  onClose: () => void
}) {
  const name = file.path.split('/').pop() ?? file.path
  const [logJob, setLogJob] = useState<Job | null>(null)
  const jobs = useQuery({
    queryKey: ['file-jobs', file.id],
    queryFn: () => getFileJobs(file.id, { limit: 100 }),
  })

  return (
    <ModalDialog
      onClose={onClose}
      label={`Job history for ${name}`}
      testId="file-history"
      size="lg"
    >
      <DialogHeader
        title="Job history"
        titleTestId="file-history-title"
        onClose={onClose}
      />
      <DialogBody>
        <p className="-mt-2 mb-4 truncate text-caption text-fg-muted">{name}</p>
        {jobs.isPending ? (
          <p
            className="text-body text-fg-muted"
            data-testid="file-history-loading"
          >
            Loading the history…
          </p>
        ) : jobs.isError ? (
          <ErrorBox title="Cannot load the job history." error={jobs.error} />
        ) : jobs.data.total === 0 ? (
          <p
            className="text-body text-fg-muted"
            data-testid="file-history-empty"
          >
            No jobs have run for this file yet.
          </p>
        ) : (
          <ul className="divide-y divide-line" data-testid="file-history-jobs">
            {jobs.data.items.map((job) => (
              <JobRow key={job.id} job={job} onOpenLog={setLogJob} />
            ))}
          </ul>
        )}
      </DialogBody>
      <DialogFooter>
        <button
          type="button"
          onClick={onClose}
          className={btn}
          data-testid="file-history-close"
        >
          Close
        </button>
      </DialogFooter>
      {logJob && <JobLogDialog job={logJob} onClose={() => setLogJob(null)} />}
    </ModalDialog>
  )
}
