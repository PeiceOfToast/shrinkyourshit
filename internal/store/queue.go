// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// ClaimNextJob atomically claims the highest-priority oldest queued job for
// a node: one transaction picks the row, moves it to claimed and stamps the
// claim fields, so two workers can never receive the same job. The UPDATE
// re-checks status so the claim stays correct even with more than one
// database connection (or a future Postgres backend). Attempt is not
// touched here; the stale-job sweeper counts interrupted attempts.
//
// Returns ErrQueueEmpty when no queued job exists.
func (s *SQLite) ClaimNextJob(ctx context.Context, nodeID string) (*Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: claim begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM jobs WHERE status = ? ORDER BY priority DESC, created_at ASC, id ASC LIMIT 1`,
		JobStatusQueued).Scan(&id)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrQueueEmpty
		}
		return nil, fmt.Errorf("store: claim select: %w", err)
	}

	now := nowUTC()
	res, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status = ?, node_id = ?, claimed_at = ?, heartbeat_at = ? WHERE id = ? AND status = ?`,
		JobStatusClaimed, nodeID, fmtTime(now), fmtTime(now), id, JobStatusQueued)
	if err != nil {
		return nil, fmt.Errorf("store: claim update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Another claimant won the row between SELECT and UPDATE; the
		// caller retries on its next poll.
		return nil, ErrQueueEmpty
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: claim commit: %w", err)
	}
	return s.GetJob(ctx, id)
}

// RequeueStaleJobs returns claimed/running jobs whose heartbeat_at is older
// than cutoff to queued with attempt+1, clearing the claim fields. A job
// whose attempt would reach maxAttempts is marked failed instead of
// requeued. Staleness is decided in Go (RFC3339Nano strings do not compare
// lexically) and the UPDATE guards on the previous status and heartbeat, so
// a worker that heartbeats between SELECT and UPDATE keeps its job.
//
// Returns the jobs that were actually transitioned (in their new state).
func (s *SQLite) RequeueStaleJobs(ctx context.Context, cutoff time.Time, maxAttempts int) ([]*Job, error) {
	if maxAttempts < 1 {
		return nil, fmt.Errorf("store: maxAttempts must be >= 1, got %d", maxAttempts)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: requeue begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT `+jobColumns+` FROM jobs WHERE status IN (?, ?)`,
		JobStatusClaimed, JobStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("store: requeue select: %w", err)
	}
	var stale []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: requeue scan: %w", err)
		}
		stale = append(stale, j)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("store: requeue rows: %w", err)
	}
	_ = rows.Close()

	var moved []*Job
	for _, j := range stale {
		// A job with a zero heartbeat was claimed by a broken claim path;
		// treat it as stale so it cannot wedge the queue.
		if !j.HeartbeatAt.IsZero() && j.HeartbeatAt.After(cutoff) {
			continue // heartbeat is fresh
		}

		attempt := j.Attempt + 1
		var updated *Job
		if attempt >= maxAttempts {
			updated = j.clone()
			updated.Status = JobStatusFailed
			updated.Attempt = attempt
			updated.Error = fmt.Sprintf("heartbeat timeout after %d attempt(s)", attempt)
			updated.FinishedAt = nowUTC()
		} else {
			updated = j.clone()
			updated.Status = JobStatusQueued
			updated.Attempt = attempt
			updated.NodeID = ""
			updated.ClaimedAt = time.Time{}
			updated.HeartbeatAt = time.Time{}
			updated.Percent = 0
			updated.Speed = 0
			updated.ETASeconds = 0
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE jobs SET status = ?, attempt = ?, node_id = ?, claimed_at = ?, heartbeat_at = ?,
			   percent = ?, speed = ?, eta_seconds = ?, error = ?, finished_at = ? WHERE id = ? AND status = ? AND heartbeat_at = ?`,
			updated.Status, updated.Attempt, updated.NodeID, fmtTime(updated.ClaimedAt), fmtTime(updated.HeartbeatAt),
			updated.Percent, updated.Speed, updated.ETASeconds, updated.Error, fmtTime(updated.FinishedAt),
			j.ID, j.Status, fmtTime(j.HeartbeatAt))
		if err != nil {
			return nil, fmt.Errorf("store: requeue update %s: %w", j.ID, err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			moved = append(moved, updated)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: requeue commit: %w", err)
	}
	return moved, nil
}

// clone copies the job so guarded updates can change fields without
// aliasing the scan result.
func (j *Job) clone() *Job {
	c := *j
	return &c
}

// UpdateJobIfActive applies j's fields only while the job is still
// claimed or running. A worker report that arrives after the job was
// cancelled (admin), failed (attempt cap) or finished elsewhere must not
// resurrect it. Returns false when the update did not apply.
func (s *SQLite) UpdateJobIfActive(ctx context.Context, j *Job) (bool, error) {
	if err := validateJobStatus(j.Status); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET file_id = ?, library_id = ?, node_id = ?, status = ?, priority = ?, attempt = ?, encoder_used = ?, fallback_reasons = ?, percent = ?, speed = ?, eta_seconds = ?, bytes_before = ?, bytes_after = ?, log = ?, error = ?, access_mode = ?, claimed_at = ?, started_at = ?, finished_at = ?, heartbeat_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		j.FileID, j.LibraryID, j.NodeID, j.Status, j.Priority, j.Attempt, j.EncoderUsed, j.FallbackReasons, j.Percent, j.Speed, j.ETASeconds, j.BytesBefore, j.BytesAfter, j.Log, j.Error, j.AccessMode,
		fmtTime(j.ClaimedAt), fmtTime(j.StartedAt), fmtTime(j.FinishedAt), fmtTime(j.HeartbeatAt),
		j.ID, JobStatusClaimed, JobStatusRunning)
	if err != nil {
		return false, fmt.Errorf("store: guarded job update: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
