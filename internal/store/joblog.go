// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Job log levels and sources (task 27, ARCHITECTURE.md).
const (
	JobLogInfo  = "info"
	JobLogWarn  = "warn"
	JobLogError = "error"
)

// JobLogSourceFFmpeg, JobLogSourceSys and JobLogSourcePlugin are the
// values allowed in job_log.source: ffmpeg stderr lines, the worker's own
// sys lines, and the server-side plugin output (task 31).
const (
	JobLogSourceFFmpeg = "ffmpeg"
	JobLogSourceSys    = "sys"
	JobLogSourcePlugin = "plugin"
)

// JobLogMaxLinesPerJob caps one job's log rows, enforced on append, so one
// pathological file cannot flood the database (task 27).
const JobLogMaxLinesPerJob = 2000

// JobLogRetention is how long log rows are kept (task 27): 14 days.
const JobLogRetention = 14 * 24 * time.Hour

// JobLogMaxTotal is the total-row cap the prune keeps, oldest first beyond
// it (task 27).
const JobLogMaxTotal = 500000

// JobLogLine is one structured log line of a job (task 27). ID is the
// store-assigned row id; Seq is the worker's batch order so out-of-order
// arrivals can be sorted; Repeated collapses consecutive identical messages.
// JSON tags serve the worker API (task 27): the worker appends batches in
// this shape.
type JobLogLine struct {
	ID       int64     `json:"id,omitempty"`
	JobID    string    `json:"job_id,omitempty"`
	Seq      int64     `json:"seq"`
	At       time.Time `json:"at,omitempty"`
	Level    string    `json:"level"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
	Repeated int64     `json:"repeated"`
}

// validateJobLogLine rejects unknown levels and sources and empty messages.
// A repeat count below one is normalized to one by AppendJobLog (the
// worker protocol always sends one; only a broken client would send zero).
func validateJobLogLine(l *JobLogLine) error {
	switch l.Level {
	case JobLogInfo, JobLogWarn, JobLogError:
	default:
		return fmt.Errorf("store: invalid job log level %q: want info, warn or error", l.Level)
	}
	switch l.Source {
	case JobLogSourceFFmpeg, JobLogSourceSys, JobLogSourcePlugin:
	default:
		return fmt.Errorf("store: invalid job log source %q: want ffmpeg, sys or plugin", l.Source)
	}
	if l.Message == "" {
		return fmt.Errorf("store: job log message must not be empty")
	}
	return nil
}

// AppendJobLog appends a batch of lines for one job (task 27). The batch's
// lines are inserted in slice order (the worker sends them in seq order).
// The per-job cap of JobLogMaxLinesPerJob rows is enforced here: a batch
// that would exceed it is trimmed from the head, so the earliest lines of
// the batch land and the overflow is dropped.
func (s *SQLite) AppendJobLog(ctx context.Context, jobID string, lines []JobLogLine) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: append job log begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM job_log WHERE job_id = ?`, jobID).Scan(&count); err != nil {
		return fmt.Errorf("store: append job log count: %w", err)
	}
	room := JobLogMaxLinesPerJob - count
	if room <= 0 {
		_ = tx.Rollback()
		return nil // the job is at its cap; nothing to append
	}
	if room < int64(len(lines)) {
		lines = lines[:room]
	}
	now := nowUTC()
	for _, l := range lines {
		line := l
		line.JobID = jobID
		if line.At.IsZero() {
			line.At = now
		}
		if line.Repeated < 1 {
			line.Repeated = 1
		}
		if err := validateJobLogLine(&line); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO job_log (job_id, seq, at, level, source, message, repeated)
			   VALUES (?, ?, ?, ?, ?, ?, ?)`,
			line.JobID, line.Seq, fmtTime(line.At), line.Level, line.Source,
			line.Message, line.Repeated); err != nil {
			return fmt.Errorf("store: append job log line: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: append job log commit: %w", err)
	}
	return nil
}

// ListJobLog returns one job's log rows ordered by id, newest page last.
// afterID returns only rows newer than it (0 = all); level filters by
// level ("" = all); limit caps the page (0 = no limit).
func (s *SQLite) ListJobLog(ctx context.Context, jobID string, afterID int64, level string, limit int) ([]JobLogLine, error) {
	query := `SELECT id, job_id, seq, at, level, source, message, repeated
		   FROM job_log WHERE job_id = ?`
	args := []any{jobID}
	if afterID > 0 {
		query += ` AND id > ?`
		args = append(args, afterID)
	}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, level)
	}
	query += ` ORDER BY id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list job log: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanJobLogRows(rows)
}

// scanJobLogRows reads job_log rows in id order.
func scanJobLogRows(rows *sql.Rows) ([]JobLogLine, error) {
	var out []JobLogLine
	for rows.Next() {
		var l JobLogLine
		var at string
		if err := rows.Scan(&l.ID, &l.JobID, &l.Seq, &at, &l.Level, &l.Source, &l.Message, &l.Repeated); err != nil {
			return nil, err
		}
		l.At = parseTime(at)
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CountJobLog returns how many log rows a job has (task 27).
func (s *SQLite) CountJobLog(ctx context.Context, jobID string) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM job_log WHERE job_id = ?`, jobID).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count job log: %w", err)
	}
	return count, nil
}

// PruneJobLog applies the retention rules (task 27): rows older than
// olderThan go first, then rows are removed oldest-first while the total
// stays above maxTotal. Rows belonging to a queued or running job are never
// deleted. When rows were deleted the freed space is reclaimed with
// PRAGMA incremental_vacuum (the database runs in incremental auto vacuum
// mode; see store.New). Returns the number of rows removed.
func (s *SQLite) PruneJobLog(ctx context.Context, olderThan time.Time, maxTotal int) (int64, error) {
	var removed int64
	// Age rule: rows older than the cutoff, excluding protected jobs.
	res, err := s.db.ExecContext(ctx, `DELETE FROM job_log WHERE at < ?
	    AND job_id NOT IN (SELECT id FROM jobs WHERE status IN (?, ?, ?))`,
		fmtTime(olderThan.UTC()), JobStatusQueued, JobStatusRunning, JobStatusClaimed)
	if err != nil {
		return 0, fmt.Errorf("store: prune job log by age: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		removed += n
	}

	// Count rule: keep only the newest maxTotal unprotected rows (maxTotal
	// is a literal keep cap: 0 keeps nothing unprotected).
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_log
	    WHERE job_id NOT IN (SELECT id FROM jobs WHERE status IN (?, ?, ?))`,
		JobStatusQueued, JobStatusRunning, JobStatusClaimed).Scan(&total); err != nil {
		return removed, fmt.Errorf("store: prune job log count: %w", err)
	}
	if total > int64(maxTotal) {
		excess := total - int64(maxTotal)
		res, err := s.db.ExecContext(ctx, `DELETE FROM job_log WHERE id IN (
		    SELECT id FROM job_log
		    WHERE job_id NOT IN (SELECT id FROM jobs WHERE status IN (?, ?, ?))
		    ORDER BY id LIMIT ?)`,
			JobStatusQueued, JobStatusRunning, JobStatusClaimed, excess)
		if err != nil {
			return removed, fmt.Errorf("store: prune job log by count: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed += n
		}
	}

	if removed > 0 {
		if err := s.incrementalVacuum(ctx); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// DeleteJobLog removes every log row of one job (task 27). Safe when the
// rows are already gone (the FK cascade normally does this first).
func (s *SQLite) DeleteJobLog(ctx context.Context, jobID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM job_log WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("store: delete job log: %w", err)
	}
	return nil
}
