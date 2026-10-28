// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Queue state errors the API maps to HTTP status codes.
var (
	// ErrJobNotActive: cancel on a job that is queued/claimed/running no more.
	ErrJobNotActive = errors.New("queue: job is not active")
	// ErrJobNotRetryable: retry on a job that is neither failed nor cancelled.
	ErrJobNotRetryable = errors.New("queue: job is not retryable")
	// ErrFileSatisfied: retry of a job whose file no longer needs work.
	ErrFileSatisfied = errors.New("queue: file no longer needs work")
)

// MaxJobAttempts is the attempt cap (ARCHITECTURE.md job lifecycle): a job
// that times out this many times is failed instead of requeued.
const MaxJobAttempts = 3

// HeartbeatTimeoutDefault is how long a claimed/running job may go without
// a heartbeat before the sweeper returns it to the queue.
const HeartbeatTimeoutDefault = 120 * time.Second

// Queue owns job state transitions on top of the store: queueing from the
// scanner and library updates, claiming for workers (task 05 wires the API),
// admin cancel/retry, and the heartbeat-timeout sweep. Every transition
// keeps the file status in step and publishes events.
type Queue struct {
	st      store.Store
	events  *EventBus
	log     *slog.Logger
	attempt int
}

// NewQueue builds a Queue. events may be nil (no publishing); log may be nil.
func NewQueue(st store.Store, events *EventBus, log *slog.Logger) *Queue {
	if log == nil {
		log = slog.Default()
	}
	return &Queue{st: st, events: events, log: log, attempt: MaxJobAttempts}
}

// QueueJob creates a queued job for a file that needs work and moves the
// file to status queued with the evaluation reasons. If an active job
// already exists for the file, nothing is created and the existing job is
// returned with queued=false. reasons are the Evaluate reasons.
func (q *Queue) QueueJob(ctx context.Context, lib *store.Library, f *store.File, reasons []string) (*store.Job, bool, error) {
	active, err := q.activeJob(ctx, f.ID)
	if err != nil {
		return nil, false, err
	}
	if active != nil {
		return active, false, nil
	}

	job := &store.Job{
		ID:          store.NewID(),
		FileID:      f.ID,
		LibraryID:   lib.ID,
		Status:      store.JobStatusQueued,
		BytesBefore: f.Size,
	}
	if err := q.st.CreateJob(ctx, job); err != nil {
		return nil, false, fmt.Errorf("queue: create job: %w", err)
	}

	f.Status = store.FileStatusQueued
	f.LastJobID = job.ID
	f.SatisfiedReason = strings.Join(reasons, "; ")
	if err := q.st.UpdateFile(ctx, f); err != nil {
		return nil, false, fmt.Errorf("queue: update file %s: %w", f.ID, err)
	}

	q.log.Debug("job queued", "job", job.ID, "file", f.Path)
	q.publish(EventJobUpdated, NewJobDTO(job))
	q.publish(EventFileUpdated, NewFileDTO(f))
	return job, true, nil
}

// Claim atomically claims the next job for a node and moves its file to
// processing. Returns store.ErrQueueEmpty when nothing is queued.
func (q *Queue) Claim(ctx context.Context, nodeID string) (*store.Job, error) {
	job, err := q.st.ClaimNextJob(ctx, nodeID)
	if err != nil {
		return nil, err // includes ErrQueueEmpty
	}

	if f, err := q.st.GetFile(ctx, job.FileID); err == nil {
		f.Status = store.FileStatusProcessing
		if err := q.st.UpdateFile(ctx, f); err != nil {
			q.log.Error("queue: claim file update", "file", f.ID, "err", err)
		} else {
			q.publish(EventFileUpdated, f)
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		q.log.Error("queue: claim file lookup", "job", job.ID, "err", err)
	}

	q.log.Debug("job claimed", "job", job.ID, "node", nodeID)
	q.publish(EventJobUpdated, NewJobDTO(job))
	return job, nil
}

// Cancel moves an active job (queued/claimed/running) to cancelled and
// resets its file so the next scan re-evaluates it.
func (q *Queue) Cancel(ctx context.Context, jobID string) (*store.Job, error) {
	job, err := q.st.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case store.JobStatusQueued, store.JobStatusClaimed, store.JobStatusRunning:
	default:
		return nil, ErrJobNotActive
	}

	job.Status = store.JobStatusCancelled
	job.FinishedAt = time.Now().UTC()
	if err := q.st.UpdateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("queue: cancel job: %w", err)
	}
	q.resetFileToNew(ctx, job)

	q.log.Info("job cancelled", "job", job.ID)
	q.publish(EventJobUpdated, NewJobDTO(job))
	return job, nil
}

// Retry moves a failed or cancelled job back to queued with a fresh attempt
// count. It refuses when the file no longer needs work (satisfied).
func (q *Queue) Retry(ctx context.Context, jobID string) (*store.Job, error) {
	job, err := q.st.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case store.JobStatusFailed, store.JobStatusCancelled:
	default:
		return nil, ErrJobNotRetryable
	}

	f, err := q.st.GetFile(ctx, job.FileID)
	if err != nil {
		return nil, err
	}
	if f.Status == store.FileStatusSatisfied {
		return nil, ErrFileSatisfied
	}

	job.Status = store.JobStatusQueued
	job.Attempt = 0
	job.NodeID = ""
	job.ClaimedAt = time.Time{}
	job.StartedAt = time.Time{}
	job.FinishedAt = time.Time{}
	job.HeartbeatAt = time.Time{}
	job.Percent = 0
	job.Speed = 0
	job.ETASeconds = 0
	if err := q.st.UpdateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("queue: retry job: %w", err)
	}

	f.Status = store.FileStatusQueued
	if err := q.st.UpdateFile(ctx, f); err != nil {
		return nil, fmt.Errorf("queue: retry file: %w", err)
	}

	q.log.Info("job retried", "job", job.ID)
	q.publish(EventJobUpdated, NewJobDTO(job))
	q.publish(EventFileUpdated, NewFileDTO(f))
	return job, nil
}

// Sweep returns stale claimed/running jobs (heartbeat older than timeout)
// to the queue, failing jobs that have exhausted their attempts. Returns
// the jobs that transitioned.
func (q *Queue) Sweep(ctx context.Context, heartbeatTimeout time.Duration) ([]*store.Job, error) {
	cutoff := time.Now().UTC().Add(-heartbeatTimeout)
	moved, err := q.st.RequeueStaleJobs(ctx, cutoff, q.attempt)
	if err != nil {
		return nil, err
	}
	for _, job := range moved {
		q.log.Warn("job heartbeat timed out", "job", job.ID, "attempt", job.Attempt, "status", job.Status)
		q.publish(EventJobUpdated, NewJobDTO(job))

		if f, err := q.st.GetFile(ctx, job.FileID); err == nil && f.Status == store.FileStatusProcessing {
			// A requeued job hands the file back to the queue; a job
			// that exhausted its attempts fails the file with it.
			if job.Status == store.JobStatusQueued {
				f.Status = store.FileStatusQueued
			} else {
				f.Status = store.FileStatusFailed
			}
			if err := q.st.UpdateFile(ctx, f); err != nil {
				q.log.Error("queue: sweep file update", "file", f.ID, "err", err)
				continue
			}
			q.publish(EventFileUpdated, NewFileDTO(f))
		}
	}
	return moved, nil
}

// activeJob returns the queued/claimed/running job for a file, or nil.
func (q *Queue) activeJob(ctx context.Context, fileID string) (*store.Job, error) {
	jobs, err := q.st.ListJobs(ctx, store.JobFilter{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("queue: list jobs: %w", err)
	}
	for _, j := range jobs {
		switch j.Status {
		case store.JobStatusQueued, store.JobStatusClaimed, store.JobStatusRunning:
			return j, nil
		}
	}
	return nil, nil
}

// resetFileToNew moves the job's file back to new when it was sitting in a
// queue-driven status, so the next scan re-evaluates it.
func (q *Queue) resetFileToNew(ctx context.Context, job *store.Job) {
	f, err := q.st.GetFile(ctx, job.FileID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			q.log.Error("queue: cancel file lookup", "job", job.ID, "err", err)
		}
		return
	}
	if f.Status != store.FileStatusQueued && f.Status != store.FileStatusProcessing {
		return
	}
	f.Status = store.FileStatusNew
	if err := q.st.UpdateFile(ctx, f); err != nil {
		q.log.Error("queue: cancel file update", "file", f.ID, "err", err)
		return
	}
	q.publish(EventFileUpdated, NewFileDTO(f))
}

// publish is a nil-safe event publish.
func (q *Queue) publish(eventType string, payload any) {
	if q.events == nil {
		return
	}
	q.events.Publish(eventType, payload)
}
