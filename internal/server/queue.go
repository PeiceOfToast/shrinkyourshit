// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
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
	// ErrJobNotClaimed: a worker touching a job another node owns.
	ErrJobNotClaimed = errors.New("queue: job is not claimed by this node")
)

// MaxLogTail is the size cap of the ffmpeg stderr tail kept on the job
// record (ARCHITECTURE.md: log capped at 64KiB tail).
const MaxLogTail = 64 << 10

// MaxJobAttempts is the attempt cap (ARCHITECTURE.md job lifecycle): a job
// that times out this many times is failed instead of requeued.
const MaxJobAttempts = 3

// HeartbeatTimeoutDefault is how long a claimed/running job may go without
// a heartbeat before the sweeper returns it to the queue.
const HeartbeatTimeoutDefault = 120 * time.Second

// Queue owns job state transitions on top of the store: queueing from the
// scanner and library updates, claiming and worker updates (progress,
// complete, fail), admin cancel/retry, and the heartbeat-timeout sweep.
// Every transition keeps the file status in step and publishes events.
type Queue struct {
	st      store.Store
	events  *EventBus
	log     *slog.Logger
	attempt int
	inspect ResultInspector
	ffprobe string
}

// NewQueue builds a Queue. events may be nil (no publishing); log may be nil.
func NewQueue(st store.Store, events *EventBus, log *slog.Logger) *Queue {
	if log == nil {
		log = slog.Default()
	}
	return &Queue{st: st, events: events, log: log, attempt: MaxJobAttempts}
}

// SetInspector wires output inspection used by Complete (fingerprint + size
// + probe of the produced file). Nil selects the default (stat + fingerprint
// + ffprobe with the given ffprobePath).
func (q *Queue) SetInspector(inspect ResultInspector, ffprobePath string) {
	if inspect == nil {
		inspect = defaultInspector
	}
	q.inspect = inspect
	q.ffprobe = ffprobePath
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

// ProgressParams are the fields a worker reports while running a job.
type ProgressParams struct {
	Percent    float64
	Speed      float64
	ETASeconds int64
	Encoder    string
}

// Progress records a running worker's progress. The first report moves the
// job from claimed to running and stamps started_at; every report refreshes
// the job heartbeat, so the sweeper only requeues genuinely dead workers.
// Only the claiming node may report. The write is guarded: a report racing
// a cancel cannot resurrect the cancelled job — the caller then gets the
// fresh job back with ErrJobNotActive.
func (q *Queue) Progress(ctx context.Context, jobID, nodeID string, p ProgressParams) (*store.Job, error) {
	job, err := q.ownedActiveJob(ctx, jobID, nodeID)
	if err != nil {
		return nil, err
	}

	job.Percent = clampPercent(p.Percent)
	job.Speed = p.Speed
	job.ETASeconds = max(p.ETASeconds, 0)
	if p.Encoder != "" {
		job.EncoderUsed = p.Encoder
	}
	now := time.Now().UTC()
	job.HeartbeatAt = now
	if job.Status == store.JobStatusClaimed {
		job.Status = store.JobStatusRunning
		job.StartedAt = now
	}
	applied, err := q.st.UpdateJobIfActive(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("queue: progress job: %w", err)
	}
	if !applied {
		return q.refreshed(ctx, job)
	}

	q.publish(EventJobUpdated, NewJobDTO(job))
	return job, nil
}

// refreshed re-reads a job after a guarded write did not apply, so callers
// can see the terminal status that beat them.
func (q *Queue) refreshed(ctx context.Context, job *store.Job) (*store.Job, error) {
	if fresh, err := q.st.GetJob(ctx, job.ID); err == nil {
		return fresh, ErrJobNotActive
	}
	return nil, ErrJobNotActive
}

// CompleteParams are the fields a worker reports on success. OutputPath is
// the final path of the produced file in the library (empty with a
// SkippedReason when the result policy kept the original). SkippedReason
// lands on the file's satisfied_reason for the UI.
type CompleteParams struct {
	OutputPath      string
	BytesAfter      int64
	Encoder         string
	FallbackReasons []string
	SkippedReason   string
	LogTail         string
}

// Complete finishes a claimed/running job owned by nodeID: the job becomes
// done and the file reflects the result policy outcome:
//
//   - output applied (replace / keep_both): the file is satisfied. When the
//     file on disk changed identity (replacement with a new extension), the
//     row's path, size, fingerprint and probe are refreshed from the output;
//     for keep_both the untouched original row keeps its fingerprint.
//   - skipped (replace_if_smaller with a not-smaller output): the file is
//     done with bytes_saved 0 and the skip reason.
//
// The worker applies the policy to the filesystem before reporting, so this
// only records what is already true on disk.
func (q *Queue) Complete(ctx context.Context, jobID, nodeID string, p CompleteParams) (*store.Job, error) {
	job, err := q.ownedActiveJob(ctx, jobID, nodeID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job.Status = store.JobStatusDone
	job.BytesAfter = p.BytesAfter
	job.EncoderUsed = p.Encoder
	if raw, err := json.Marshal(p.FallbackReasons); err == nil {
		job.FallbackReasons = string(raw)
	}
	job.Log = capLogTail(p.LogTail)
	job.Percent = 100
	job.ETASeconds = 0
	job.FinishedAt = now
	job.HeartbeatAt = now
	applied, err := q.st.UpdateJobIfActive(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("queue: complete job: %w", err)
	}
	if !applied {
		// The job left the active state while we were finishing (a cancel
		// raced the last second). Do not touch the file: disk state
		// self-heals on the next scan.
		return q.refreshed(ctx, job)
	}

	f, err := q.st.GetFile(ctx, job.FileID)
	if err != nil {
		return job, fmt.Errorf("queue: complete file lookup: %w", err)
	}
	f.LastJobID = job.ID
	f.BytesSaved = f.Size - p.BytesAfter

	// The result policy decides what completion means for the file row.
	// keep_both wrote a sidecar beside the untouched original; the row
	// keeps pointing at the original (its fingerprint and probe are still
	// valid, so rescans skip it instead of reprocessing forever).
	policy := store.ResultPolicyReplaceIfSmaller
	if lib, err := q.st.GetLibrary(ctx, job.LibraryID); err != nil {
		q.log.Error("queue: complete policy lookup", "library", job.LibraryID, "err", err)
	} else if lib.ResultPolicy != "" {
		policy = lib.ResultPolicy
	}
	keepBoth := policy == store.ResultPolicyKeepBoth && p.SkippedReason == ""

	if p.SkippedReason != "" {
		// The original was kept: nothing on disk changed, so the
		// fingerprint and probe stay valid and the next scan skips it.
		f.BytesSaved = 0
		f.Status = store.FileStatusDone
		f.SatisfiedReason = p.SkippedReason
		if err := q.st.UpdateFile(ctx, f); err != nil {
			return job, fmt.Errorf("queue: complete skipped file: %w", err)
		}
		q.finishEvents(job, f)
		return job, nil
	}

	// The output becomes the file's new identity — except for keep_both,
	// whose output is a sidecar beside the original.
	if !keepBoth {
		// Drop any row a racing scan created for the new path first, so
		// (library_id, path) stays unique.
		if err := q.dropDuplicateFiles(ctx, f.LibraryID, p.OutputPath, f.ID); err != nil {
			q.log.Error("queue: complete dedupe", "file", f.ID, "err", err)
		}

		res, inspectErr := q.inspect(ctx, q.ffprobe, p.OutputPath)
		if inspectErr != nil {
			// The output could not be inspected (deleted mid-job, or a
			// future transfer-mode node). Record the failure on the file;
			// the next scan re-evaluates from disk.
			q.log.Error("queue: complete inspect", "path", p.OutputPath, "err", inspectErr)
			f.Status = store.FileStatusFailed
			f.SatisfiedReason = "output could not be verified: " + inspectErr.Error()
			if err := q.st.UpdateFile(ctx, f); err != nil {
				return job, fmt.Errorf("queue: complete failed file: %w", err)
			}
			q.finishEvents(job, f)
			return job, nil
		}

		f.Path = p.OutputPath
		f.Size = res.Size
		f.Mtime = res.Mtime
		f.Fingerprint = res.Fingerprint
		if raw, err := json.Marshal(res.Info); err == nil {
			f.ProbeJSON = string(raw)
		}
	} else {
		f.BytesSaved = 0 // disk usage did not shrink; the original is still there
	}
	f.Status = store.FileStatusSatisfied
	f.SatisfiedReason = ""
	if err := q.st.UpdateFile(ctx, f); err != nil {
		return job, fmt.Errorf("queue: complete file: %w", err)
	}

	q.log.Info("job completed", "job", job.ID, "file", f.Path,
		"bytes_saved", f.BytesSaved, "encoder", job.EncoderUsed)
	q.finishEvents(job, f)
	return job, nil
}

// FailParams are the fields a worker reports on failure.
type FailParams struct {
	Error   string
	LogTail string
}

// Fail marks a claimed/running job owned by nodeID as failed with its last
// ffmpeg stderr tail. A worker-reported failure is definitive (attempts
// only count heartbeat timeouts); the file goes to failed so a rescan or a
// manual retry picks it back up.
func (q *Queue) Fail(ctx context.Context, jobID, nodeID string, p FailParams) (*store.Job, error) {
	job, err := q.ownedActiveJob(ctx, jobID, nodeID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job.Status = store.JobStatusFailed
	job.Error = p.Error
	job.Log = capLogTail(p.LogTail)
	job.FinishedAt = now
	job.HeartbeatAt = now
	applied, err := q.st.UpdateJobIfActive(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("queue: fail job: %w", err)
	}
	if !applied {
		return q.refreshed(ctx, job)
	}

	if f, ferr := q.st.GetFile(ctx, job.FileID); ferr == nil {
		if f.Status == store.FileStatusProcessing {
			f.Status = store.FileStatusFailed
			if uerr := q.st.UpdateFile(ctx, f); uerr != nil {
				q.log.Error("queue: fail file update", "file", f.ID, "err", uerr)
			} else {
				q.publish(EventFileUpdated, NewFileDTO(f))
			}
		}
	} else if !errors.Is(ferr, store.ErrNotFound) {
		q.log.Error("queue: fail file lookup", "job", job.ID, "err", ferr)
	}

	q.log.Warn("job failed", "job", job.ID, "error", p.Error)
	q.publish(EventJobUpdated, NewJobDTO(job))
	return job, nil
}

// ownedActiveJob returns the job when it is claimed/running and owned by
// nodeID; otherwise the caller gets a typed error to map to HTTP.
func (q *Queue) ownedActiveJob(ctx context.Context, jobID, nodeID string) (*store.Job, error) {
	job, err := q.st.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case store.JobStatusClaimed, store.JobStatusRunning:
	default:
		return nil, ErrJobNotActive
	}
	if nodeID != "" && job.NodeID != nodeID {
		return nil, ErrJobNotClaimed
	}
	return job, nil
}

// dropDuplicateFiles removes file rows of a library that already use path,
// except keepID (a replacement may share its path with a row a racing scan
// created while the file row still had the old path).
func (q *Queue) dropDuplicateFiles(ctx context.Context, libraryID, path, keepID string) error {
	files, err := q.st.ListFiles(ctx, libraryID)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.Path == path && f.ID != keepID {
			if err := q.st.DeleteFile(ctx, f.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// finishEvents publishes the job and file after a completion.
func (q *Queue) finishEvents(job *store.Job, f *store.File) {
	q.publish(EventJobUpdated, NewJobDTO(job))
	q.publish(EventFileUpdated, NewFileDTO(f))
}

// capLogTail keeps only the last MaxLogTail bytes of a log.
func capLogTail(s string) string {
	if len(s) <= MaxLogTail {
		return s
	}
	return s[len(s)-MaxLogTail:]
}

// clampPercent bounds a progress percentage to 0..100.
func clampPercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
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

// RequeueNodeJobs returns every claimed/running job of nodeID to the queue
// (attempt+1, claim fields cleared; at the attempt cap the job fails).
// Used when a node is revoked or deleted (task 08). Returns the jobs that
// transitioned. A report racing the revoke cannot resurrect its job:
// UpdateJobIfActive only applies while the job is still claimed/running.
func (q *Queue) RequeueNodeJobs(ctx context.Context, nodeID string) ([]*store.Job, error) {
	jobs, err := q.st.ListJobs(ctx, store.JobFilter{NodeID: nodeID})
	if err != nil {
		return nil, fmt.Errorf("queue: list node jobs: %w", err)
	}
	var moved []*store.Job
	for _, job := range jobs {
		switch job.Status {
		case store.JobStatusClaimed, store.JobStatusRunning:
		default:
			continue
		}

		now := time.Now().UTC()
		attempt := job.Attempt + 1
		if attempt >= q.attempt {
			job.Status = store.JobStatusFailed
			job.Attempt = attempt
			job.Error = fmt.Sprintf("node revoked or removed after %d attempt(s)", attempt)
			job.FinishedAt = now
			job.HeartbeatAt = now
		} else {
			job.Status = store.JobStatusQueued
			job.Attempt = attempt
			job.NodeID = ""
			job.ClaimedAt = time.Time{}
			job.StartedAt = time.Time{}
			job.HeartbeatAt = time.Time{}
			job.Percent = 0
			job.Speed = 0
			job.ETASeconds = 0
		}
		applied, err := q.st.UpdateJobIfActive(ctx, job)
		if err != nil {
			q.log.Error("queue: requeue node job", "job", job.ID, "err", err)
			continue
		}
		if !applied {
			continue // finished or cancelled while we looked at it
		}
		moved = append(moved, job)
		q.publish(EventJobUpdated, NewJobDTO(job))

		if f, ferr := q.st.GetFile(ctx, job.FileID); ferr == nil && f.Status == store.FileStatusProcessing {
			if job.Status == store.JobStatusQueued {
				f.Status = store.FileStatusQueued
			} else {
				f.Status = store.FileStatusFailed
			}
			if uerr := q.st.UpdateFile(ctx, f); uerr != nil {
				q.log.Error("queue: requeue node file update", "file", f.ID, "err", uerr)
				continue
			}
			q.publish(EventFileUpdated, NewFileDTO(f))
		}
	}
	return moved, nil
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
