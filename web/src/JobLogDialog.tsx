// SPDX-License-Identifier: AGPL-3.0-only

/**
 * JobLogDialog (task 27): the structured job log in a dialog. Rows carry
 * the time, a level chip and the message in monospace. While the job is
 * still running or queued the viewer polls GET /jobs/{id}/log with
 * after_id once a second and appends, so a live job can be followed
 * without re-reading the whole log. Level filter chips (all, warnings,
 * errors), an auto-scroll toggle, Copy and Download. The Download button
 * and the log endpoint are admin routes, so this viewer lives behind the
 * admin session.
 */
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  getJobLog,
  jobLogDownloadURL,
  type Job,
  type JobLogLine,
  type JobLogResponse,
} from './api'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { btn, btnGhost } from './uiControls'

const JOB_LOG_PAGE = 500

/** A job that is still going: the viewer keeps polling until this is false. */
function isActive(status: string): boolean {
  return status === 'running' || status === 'queued' || status === 'claimed'
}

function levelChipClass(level: JobLogLine['level']): string {
  switch (level) {
    case 'error':
      return 'bg-danger/10 text-danger ring-danger/30'
    case 'warn':
      return 'bg-warning/10 text-warning ring-warning/30'
    default:
      return 'bg-raised text-fg-muted ring-line'
  }
}

function levelChipLabel(level: JobLogLine['level'], repeated: number): string {
  const name = level === 'error' ? 'error' : level === 'warn' ? 'warn' : 'info'
  return repeated > 1 ? `${name} ×${repeated}` : name
}

function timeLabel(at: string): string {
  const date = new Date(at)
  if (Number.isNaN(date.getTime())) return at
  return date.toISOString().replace('T', ' ').replace('Z', 'Z')
}

/** The Copy/Download payload: one row per line as `<at> <level> <source> <message>`. */
function lineToText(line: JobLogLine): string {
  const repeat = line.repeated > 1 ? ` (×${line.repeated})` : ''
  return `${line.at} ${line.level} ${line.source} ${line.message}${repeat}`
}

export default function JobLogDialog({
  job,
  onClose,
}: {
  job: Job
  onClose: () => void
}) {
  const [lines, setLines] = useState<JobLogLine[]>([])
  const [total, setTotal] = useState(0)
  const [filter, setFilter] = useState<'all' | 'warn' | 'error'>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  // The job's live status: every log response carries it, so the viewer
  // stops polling once the job goes terminal without a second request.
  const [status, setStatus] = useState(job.status)
  const logBoxRef = useRef<HTMLDivElement | null>(null)
  const lastIdRef = useRef(0)
  const autoScrollRef = useRef(autoScroll)
  autoScrollRef.current = autoScroll
  const activeRef = useRef(isActive(status))
  activeRef.current = isActive(status)

  const name = job.file_name || job.file_id

  // The initial page — paged through to the end — and, while the job runs,
  // one poll per second with after_id. Polling stops at a terminal status
  // and when the dialog closes (the effect's cleanup clears the timer).
  useEffect(() => {
    let cancelled = false
    const append = (res: JobLogResponse) => {
      if (res.lines.length > 0) {
        lastIdRef.current = res.lines[res.lines.length - 1].id
        setLines((prev) => [...prev, ...res.lines])
      }
      setTotal(res.total)
      if (res.status) setStatus(res.status)
    }
    const load = async () => {
      try {
        // Page through what exists already, newest first order.
        let after = 0
        for (let page = 0; page < 5; page++) {
          const res = await getJobLog(job.id, {
            after_id: after,
            limit: JOB_LOG_PAGE,
          })
          if (cancelled) return
          setError(null)
          append(res)
          if (!res.has_more || res.lines.length === 0) break
          after = lastIdRef.current
        }
      } catch (err) {
        if (!cancelled)
          setError(err instanceof Error ? err.message : String(err))
      }
    }
    const follow = async () => {
      try {
        const res = await getJobLog(job.id, { after_id: lastIdRef.current })
        if (cancelled) return
        setError(null)
        append(res)
      } catch (err) {
        if (!cancelled)
          setError(err instanceof Error ? err.message : String(err))
      }
    }
    void load()
    const timer = window.setInterval(() => {
      if (!activeRef.current) return
      void follow()
    }, 1000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [job.id])

  // Auto scroll follows the appended rows while it is on.
  useEffect(() => {
    if (autoScrollRef.current && logBoxRef.current) {
      logBoxRef.current.scrollTop = logBoxRef.current.scrollHeight
    }
  }, [lines])

  const visible = useMemo(() => {
    if (filter === 'all') return lines
    return lines.filter((line) => line.level === filter)
  }, [lines, filter])

  const copyAll = () => {
    const text = visible.map(lineToText).join('\n')
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }

  const downloadURL = jobLogDownloadURL(job.id)

  return (
    <ModalDialog
      onClose={onClose}
      label={`Log for ${name}`}
      testId="job-log-dialog"
      size="lg"
    >
      <DialogHeader
        title="Job log"
        titleTestId="job-log-title"
        onClose={onClose}
      />
      <DialogBody>
        <p className="-mt-2 mb-3 truncate text-caption text-fg-muted">{name}</p>

        <div
          className="flex flex-wrap items-center gap-2"
          data-testid="job-log-filters"
        >
          {(
            [
              ['all', 'All'],
              ['warn', 'Warnings'],
              ['error', 'Errors'],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              onClick={() => setFilter(value)}
              aria-pressed={filter === value}
              className={`min-h-11 rounded-control px-3 text-body font-medium ${
                filter === value
                  ? 'bg-raised-hover text-fg'
                  : 'text-fg-muted hover:bg-raised hover:text-fg'
              }`}
              data-testid={`job-log-filter-${value}`}
            >
              {label}
            </button>
          ))}
          <label className="ml-auto flex min-h-11 items-center gap-2 text-body text-fg-muted">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(event) => setAutoScroll(event.target.checked)}
              className="h-11 w-11 accent-accent"
              data-testid="job-log-autoscroll"
            />
            Auto scroll
          </label>
        </div>

        <div
          ref={logBoxRef}
          className="mt-3 h-80 overflow-y-auto rounded-control bg-canvas p-3 font-mono text-caption"
          data-testid="job-log-rows"
        >
          {error ? (
            <p className="text-danger" data-testid="job-log-error">
              {error}
            </p>
          ) : visible.length === 0 ? (
            <p className="text-fg-muted" data-testid="job-log-empty">
              {lines.length === 0
                ? isActive(status)
                  ? 'Waiting for the first log lines…'
                  : 'This job has no log lines.'
                : `No ${filter} lines in the log.`}
            </p>
          ) : (
            <ul className="space-y-0.5">
              {visible.map((line) => (
                <li
                  key={line.id}
                  className="flex gap-2 whitespace-pre-wrap break-all"
                  data-testid="job-log-row"
                >
                  <span className="flex-none text-fg-subtle">
                    {timeLabel(line.at)}
                  </span>
                  <span
                    className={`flex-none rounded px-1.5 ring-1 ring-inset ${levelChipClass(line.level)}`}
                  >
                    {levelChipLabel(line.level, line.repeated)}
                  </span>
                  <span
                    className={`min-w-0 ${
                      line.level === 'error'
                        ? 'text-danger'
                        : line.level === 'warn'
                          ? 'text-warning'
                          : 'text-fg'
                    }`}
                  >
                    {line.message}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <p
          className="mt-1 text-caption text-fg-muted"
          data-testid="job-log-count"
        >
          {lines.length === 0
            ? '0'
            : `${lines.length} of ${Math.max(total, lines.length)} lines`}
          {isActive(status) ? ' · following' : ''}
        </p>
      </DialogBody>

      <DialogFooter>
        <button
          type="button"
          onClick={copyAll}
          className={btnGhost}
          data-testid="job-log-copy"
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
        <a
          href={downloadURL}
          download
          className={btn}
          data-testid="job-log-download"
        >
          Download
        </a>
        <button
          type="button"
          onClick={onClose}
          className={btn}
          data-testid="job-log-close"
        >
          Close
        </button>
      </DialogFooter>
    </ModalDialog>
  )
}
