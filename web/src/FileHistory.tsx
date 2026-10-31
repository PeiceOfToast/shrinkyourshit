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
import { formatBytes, statusLabel } from './easyModel'
import ErrorBox from './ErrorBox'
import JobLogDialog from './JobLogDialog'
import ModalDialog from './dialog'
import { btnGhost } from './uiControls'

function jobStatusColor(status: string): string {
  switch (status) {
    case 'done':
      return 'text-emerald-400'
    case 'failed':
      return 'text-red-400'
    case 'running':
    case 'claimed':
      return 'text-sky-400'
    case 'cancelled':
      return 'text-slate-400'
    default:
      return 'text-amber-400'
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
    <li
      className="rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
      data-testid="file-history-job"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
        <span className={`font-medium ${jobStatusColor(job.status)}`}>
          {statusLabel(job.status)}
        </span>
        <span className="text-slate-400" data-testid="file-history-attempt">
          Attempt {job.attempt + 1}
        </span>
        <span className="text-slate-400" data-testid="file-history-encoder">
          <span className="text-slate-600">Encoder: </span>
          {job.encoder_used || '—'}
        </span>
        {hasSizes && (
          <span className="text-slate-400" data-testid="file-history-bytes">
            <span className="text-slate-600">Size: </span>
            {formatBytes(job.bytes_before)}
            {' → '}
            {job.bytes_after > 0 ? formatBytes(job.bytes_after) : '—'}
          </span>
        )}
        {job.status === 'running' && job.percent > 0 && (
          <span className="text-sky-300">{Math.round(job.percent)}%</span>
        )}
        {job.has_log && (
          <button
            type="button"
            onClick={() => onOpenLog(job)}
            className={`${btnGhost} !min-h-11 !px-3 !text-xs`}
            data-testid={`file-history-log-${job.id}`}
          >
            Log
          </button>
        )}
      </div>
      {job.fallback_reasons.length > 0 && (
        <ul
          className="mt-2 space-y-1 text-sm text-amber-300"
          data-testid="file-history-fallbacks"
        >
          {job.fallback_reasons.map((reason, i) => (
            <li key={i}>Fell back: {reason}</li>
          ))}
        </ul>
      )}
      {job.error && (
        <p
          className="mt-2 break-words text-sm text-red-400"
          data-testid="file-history-error-text"
        >
          {job.error}
        </p>
      )}
      {job.log && (
        <details className="mt-2" data-testid="file-history-log">
          <summary className="cursor-pointer text-sm text-slate-400 hover:text-slate-200">
            Log
          </summary>
          <pre
            className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-xl bg-slate-950 p-3 text-xs text-slate-400"
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
      className="m-auto w-full max-w-2xl rounded-2xl bg-slate-950 p-6 text-slate-100 shadow-xl ring-1 ring-slate-800"
    >
      <h2 className="text-lg font-bold" data-testid="file-history-title">
        Job history
      </h2>
      <p className="mt-0.5 truncate text-sm text-slate-500">{name}</p>

      <div className="mt-4">
        {jobs.isPending ? (
          <p
            className="text-sm text-slate-500"
            data-testid="file-history-loading"
          >
            Loading the history…
          </p>
        ) : jobs.isError ? (
          <ErrorBox title="Cannot load the job history." error={jobs.error} />
        ) : jobs.data.total === 0 ? (
          <p
            className="text-sm text-slate-400"
            data-testid="file-history-empty"
          >
            No jobs have run for this file yet.
          </p>
        ) : (
          <ul className="space-y-2" data-testid="file-history-jobs">
            {jobs.data.items.map((job) => (
              <JobRow key={job.id} job={job} onOpenLog={setLogJob} />
            ))}
          </ul>
        )}
      </div>

      <button
        type="button"
        onClick={onClose}
        className="mt-5 rounded-lg bg-slate-800 px-4 py-2 text-sm hover:bg-slate-700"
        data-testid="file-history-close"
      >
        Close
      </button>
      {logJob && <JobLogDialog job={logJob} onClose={() => setLogJob(null)} />}
    </ModalDialog>
  )
}
