// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// nodeAuth guards the worker API: Authorization: Bearer <node-key>, hashed
// and looked up in the nodes table; the node must be approved and enabled
// (API.md). Node endpoints never accept session cookies and admin
// endpoints never accept node keys. The resolved node is passed to the
// handler.
func (a *API) nodeAuth(next func(w http.ResponseWriter, r *http.Request, node *store.Node)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		node, err := a.svc.Nodes().Authenticate(r.Context(), key)
		if errors.Is(err, server.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing, unknown, revoked or disabled node key")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		next(w, r, node)
	}
}

// workerCtx detaches a worker request from its HTTP context. These calls
// record the outcome of work that already happened (a claim, a completed
// encode); they must persist even when the worker disconnects mid-request,
// or a cancelled completion would leave a file stuck in processing.
func workerCtx(r *http.Request) context.Context {
	return context.WithoutCancel(r.Context())
}

// heartbeatBody is the POST /api/v1/worker/heartbeat request (API.md):
// the node's detected capabilities and the ids of its claimed/running jobs.
type heartbeatBody struct {
	Capabilities  json.RawMessage `json:"capabilities"`
	RunningJobIDs []string        `json:"running_job_ids"`
}

// workerHeartbeat records the node heartbeat and returns the concurrency
// the claim loop should use (worker_concurrency from settings).
func (a *API) workerHeartbeat(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[heartbeatBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	caps := strings.TrimSpace(string(body.Capabilities))
	concurrency, err := a.svc.Nodes().Heartbeat(workerCtx(r), node, caps, body.RunningJobIDs)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"concurrency": concurrency})
}

// workerClaim returns the next queued job for this node, or 204.
func (a *API) workerClaim(w http.ResponseWriter, r *http.Request, node *store.Node) {
	job, err := a.svc.Queue().Claim(workerCtx(r), node.ID)
	if errors.Is(err, store.ErrQueueEmpty) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewJobDTO(job))
}

// JobSpec lives in server.JobSpec (single source of truth for the API.md
// task-05 shape; the worker client parses the same struct).

// workerJobSpec hands a claimed job to its owning node.
func (a *API) workerJobSpec(w http.ResponseWriter, r *http.Request, node *store.Node) {
	job, ok := a.ownedJob(w, r, node)
	if !ok {
		return
	}
	f, err := a.st.GetFile(r.Context(), job.FileID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such file")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	lib, err := a.svc.Libraries().Get(r.Context(), job.LibraryID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	target, err := flow.ParseTarget([]byte(lib.TargetJSON))
	if err != nil {
		a.internal(w, err) // stored targets are validated on write
		return
	}
	writeJSON(w, http.StatusOK, server.JobSpec{
		ID:           job.ID,
		FileID:       f.ID,
		LibraryID:    lib.ID,
		Attempt:      job.Attempt,
		Target:       target,
		SourcePath:   server.ResolveNodePath(node, f.Path),
		SourceSize:   f.Size,
		SourceMtime:  f.Mtime,
		Fingerprint:  f.Fingerprint,
		AccessMode:   effectiveAccessMode(node),
		ResultPolicy: lib.ResultPolicy,
		BytesBefore:  job.BytesBefore,
	})
}

// progressBody is the POST .../progress request (API.md).
type progressBody struct {
	Percent    float64 `json:"percent"`
	Speed      float64 `json:"speed"`
	ETASeconds int64   `json:"eta_seconds"`
	Encoder    string  `json:"encoder"`
}

// workerProgress records encode progress. The response carries the job's
// current status so the worker can detect a cancel — including a job that
// left the active state while it encoded (cancelled, failed, done): those
// return 200 with the terminal status instead of 409, or the worker would
// never learn it should stop.
func (a *API) workerProgress(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[progressBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	job, err := a.svc.Queue().Progress(workerCtx(r), r.PathValue("id"), node.ID, server.ProgressParams{
		Percent:    body.Percent,
		Speed:      body.Speed,
		ETASeconds: body.ETASeconds,
		Encoder:    body.Encoder,
	})
	if err != nil && errors.Is(err, server.ErrJobNotActive) {
		if j, gerr := a.st.GetJob(workerCtx(r), r.PathValue("id")); gerr == nil && j.NodeID == node.ID {
			writeJSON(w, http.StatusOK, map[string]string{"status": j.Status})
			return
		}
	}
	if a.queueError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": job.Status})
}

// completeBody is the POST .../complete request (API.md). output_path is
// the final path of the produced file in the library. skipped_reason is set
// when the result policy kept the original (replace_if_smaller with a
// not-smaller output); log_tail is stored for the UI.
type completeBody struct {
	OutputPath      string   `json:"output_path"`
	BytesAfter      int64    `json:"bytes_after"`
	Encoder         string   `json:"encoder"`
	FallbackReasons []string `json:"fallback_reasons"`
	SkippedReason   string   `json:"skipped_reason"`
	LogTail         string   `json:"log_tail"`
}

// workerComplete records a successful job.
func (a *API) workerComplete(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[completeBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	job, err := a.svc.Queue().Complete(workerCtx(r), r.PathValue("id"), node.ID, server.CompleteParams{
		OutputPath:      body.OutputPath,
		BytesAfter:      body.BytesAfter,
		Encoder:         body.Encoder,
		FallbackReasons: body.FallbackReasons,
		SkippedReason:   body.SkippedReason,
		LogTail:         body.LogTail,
	})
	if a.queueError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, server.NewJobDTO(job))
}

// failBody is the POST .../fail request (API.md).
type failBody struct {
	Error   string `json:"error"`
	LogTail string `json:"log_tail"`
}

// workerFail records a failed job.
func (a *API) workerFail(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[failBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	job, err := a.svc.Queue().Fail(workerCtx(r), r.PathValue("id"), node.ID, server.FailParams{
		Error:   body.Error,
		LogTail: body.LogTail,
	})
	if a.queueError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, server.NewJobDTO(job))
}

// ownedJob loads the job and enforces that it is active and owned by this
// node (the status check comes first: a queued job has no owner yet).
func (a *API) ownedJob(w http.ResponseWriter, r *http.Request, node *store.Node) (*store.Job, bool) {
	job, err := a.st.GetJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	if job.Status != store.JobStatusClaimed && job.Status != store.JobStatusRunning {
		writeError(w, http.StatusConflict, "job_not_active", "job is not claimed or running")
		return nil, false
	}
	if job.NodeID != node.ID {
		writeError(w, http.StatusForbidden, "not_job_owner", "this node does not own the job")
		return nil, false
	}
	return job, true
}

// queueError writes the HTTP error for a queue transition result and
// returns true when it handled the error (err was not nil).
func (a *API) queueError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such job")
	case errors.Is(err, server.ErrJobNotClaimed):
		writeError(w, http.StatusForbidden, "not_job_owner", "this node does not own the job")
	case errors.Is(err, server.ErrJobNotActive):
		writeError(w, http.StatusConflict, "job_not_active", "job is not claimed or running")
	default:
		a.internal(w, err)
	}
	return true
}

// effectiveAccessMode resolves the node's access mode for job specs. auto
// means shared for now; the shared-storage check and transfer mode arrive
// with task 09.
func effectiveAccessMode(n *store.Node) string {
	if n.AccessMode == store.NodeAccessAuto || n.AccessMode == "" {
		return store.NodeAccessShared
	}
	return n.AccessMode
}
