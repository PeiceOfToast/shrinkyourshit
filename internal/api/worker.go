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
// the node's detected capabilities, the ids of its claimed/running jobs
// and — task 11 — the worker's own lifecycle state. A `stopped` report
// while the node is draining completes the drain.
type heartbeatBody struct {
	Capabilities  json.RawMessage `json:"capabilities"`
	RunningJobIDs []string        `json:"running_job_ids"`
	State         string          `json:"state,omitempty"`
}

// workerHeartbeat records the node heartbeat and returns the instruction
// the claim loop honours: the node's desired_state and concurrency cap.
func (a *API) workerHeartbeat(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[heartbeatBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	caps := strings.TrimSpace(string(body.Capabilities))
	state, err := a.svc.Nodes().Heartbeat(workerCtx(r), node, caps, body.State, body.RunningJobIDs)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// workerClaim returns the next queued job for this node, or 204. A claim
// needs an approved, enabled, started node (API.md, task 11): nodeAuth
// enforces approved and enabled, the desired_state check is here.
func (a *API) workerClaim(w http.ResponseWriter, r *http.Request, node *store.Node) {
	if node.DesiredState != store.NodeStateRunning {
		writeError(w, http.StatusConflict, "node_not_running",
			"node is not started; it claims jobs only while its desired_state is running")
		return
	}
	job, err := a.svc.Queue().Claim(workerCtx(r), node.ID)
	if errors.Is(err, store.ErrQueueEmpty) || errors.Is(err, server.ErrPreflightRejected) {
		// An empty queue, or a queue whose freshly claimed job was just
		// rejected by a preflight plugin (task 31): the job is failed
		// already and the node polls again later. Same 204 either way.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.jobDTO(workerCtx(r), job))
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
	graph, source, err := server.LibraryFlow(lib)
	if err != nil {
		a.internal(w, err) // stored flows are validated on write
		return
	}
	plan, err := flow.Compile(graph)
	if err != nil {
		a.internal(w, err) // stored flows are validated on write
		return
	}
	// The plugin steps are resolved at dispatch (task 32): the spec
	// carries the manifest facts and merged settings. A flow that names a
	// plugin that meanwhile disappeared, was disabled or is not a step
	// plugin fails the job here instead of silently skipping the step;
	// the node answers the worker with the same 422, which drops the job.
	plugins, err := a.svc.Plugins().ResolveSteps(r.Context(), plan)
	if err != nil {
		msg := err.Error()
		if _, ferr := a.svc.Queue().Fail(workerCtx(r), job.ID, node.ID, server.FailParams{Error: msg}); ferr != nil {
			a.internal(w, ferr)
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "plugin_unavailable", msg)
		return
	}
	// The move destination (task 24) is given to the worker in its own path
	// view, mapped like the source; the folder layout it should rebuild
	// under it is computed here, where both the file and the library root
	// are known.
	moveTo := plan.MoveTo
	if moveTo != "" {
		moveTo = server.ResolveNodePath(node, moveTo)
	}
	writeJSON(w, http.StatusOK, server.JobSpec{
		ID:                 job.ID,
		FileID:             f.ID,
		LibraryID:          lib.ID,
		Attempt:            job.Attempt,
		Target:             target,
		Flow:               &graph,
		FlowSource:         source,
		SourcePath:         server.ResolveNodePath(node, f.Path),
		SourceSize:         f.Size,
		SourceMtime:        f.Mtime,
		Fingerprint:        f.Fingerprint,
		AccessMode:         effectiveAccessMode(node),
		ResultPolicy:       lib.ResultPolicy,
		BytesBefore:        job.BytesBefore,
		ThroughputLimitMBs: node.ThroughputLimit,
		MoveTo:             moveTo,
		MoveKeepTree:       plan.MoveKeepTree,
		MoveRelDir:         server.RelDirUnder(lib.Path, f.Path),
		Plugins:            plugins,
	})
}

// completeBody is the POST .../complete request (API.md). output_path is
// the final path of the produced file in the library. skipped_reason is set
// when the result policy kept the original (replace_if_smaller with a
// not-smaller output); log_tail is stored for the UI. access_mode (task 09)
// is the mode the node ran in: when it is "transfer" the server applies the
// result policy itself from the verified upload before recording.
type completeBody struct {
	OutputPath      string   `json:"output_path"`
	BytesAfter      int64    `json:"bytes_after"`
	Encoder         string   `json:"encoder"`
	FallbackReasons []string `json:"fallback_reasons"`
	SkippedReason   string   `json:"skipped_reason"`
	LogTail         string   `json:"log_tail"`
	AccessMode      string   `json:"access_mode"`
}

// workerComplete records a successful job. The worker reports the output
// path in its own view (it replaced the file where it was told the source
// was); the server maps it back through the node's path mappings so the
// library row and the output inspection always use the library-side path.
// In transfer mode the worker uploads the output instead (task 09): the
// server takes the verified upload, applies the result policy itself, and
// records the final library path.
func (a *API) workerComplete(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[completeBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	params := server.CompleteParams{
		OutputPath:      server.ResolveServerPath(node, body.OutputPath),
		BytesAfter:      body.BytesAfter,
		Encoder:         body.Encoder,
		FallbackReasons: body.FallbackReasons,
		SkippedReason:   body.SkippedReason,
		LogTail:         body.LogTail,
		AccessMode:      body.AccessMode,
	}
	if body.AccessMode != "" && body.AccessMode != store.NodeAccessShared && body.AccessMode != store.NodeAccessTransfer {
		writeError(w, http.StatusBadRequest, "invalid_request", "access_mode must be shared or transfer")
		return
	}
	if body.AccessMode == store.NodeAccessTransfer {
		transferOut, ok := a.transferComplete(w, r, node)
		if !ok {
			return
		}
		params.OutputPath = transferOut.OutputPath
		params.SkippedReason = transferOut.SkipReason
		params.BytesAfter = transferOut.BytesAfter
		// The upload has served its purpose: the result was applied to
		// the library above and Complete below only records state. Drop
		// the scratch before the job is recorded done, so a done job
		// never owns upload scratch (if the job left the active state
		// meanwhile, the cancel path discards it anyway).
		a.svc.Transfer().Discard(r.PathValue("id"))
	}
	job, err := a.svc.Queue().Complete(workerCtx(r), r.PathValue("id"), node.ID, params)
	if a.queueError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.jobDTO(workerCtx(r), job))
}

// progressBody is the POST .../progress request (API.md). access_mode
// (task 09) is the mode the node resolved for this job; the server stores
// it on the job when present. bytes_so_far (task 18) is the scratch
// output's size at report time — the worker stats its output once per
// tick; the server keeps the last known positive value.
type progressBody struct {
	Percent    float64 `json:"percent"`
	Speed      float64 `json:"speed"`
	ETASeconds int64   `json:"eta_seconds"`
	Encoder    string  `json:"encoder"`
	AccessMode string  `json:"access_mode"`
	BytesSoFar int64   `json:"bytes_so_far"`
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
	if body.AccessMode != "" && body.AccessMode != store.NodeAccessShared && body.AccessMode != store.NodeAccessTransfer {
		writeError(w, http.StatusBadRequest, "invalid_request", "access_mode must be shared or transfer")
		return
	}
	job, err := a.svc.Queue().Progress(workerCtx(r), r.PathValue("id"), node.ID, server.ProgressParams{
		Percent:    body.Percent,
		Speed:      body.Speed,
		ETASeconds: body.ETASeconds,
		Encoder:    body.Encoder,
		AccessMode: body.AccessMode,
		BytesSoFar: body.BytesSoFar,
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

// failBody is the POST .../fail request (API.md).
type failBody struct {
	Error   string `json:"error"`
	LogTail string `json:"log_tail"`
}

// workerFail records a failed job. Any partial transfer upload (task 09)
// goes away with it.
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
	a.svc.Transfer().Discard(r.PathValue("id"))
	writeJSON(w, http.StatusOK, a.jobDTO(workerCtx(r), job))
}

// workerJobLog appends one batch of log lines from the node that owns the
// job (task 27). Ownership is checked the same way complete and fail do;
// the active-status check is dropped so the final flush of a finished job
// still lands. The store enforces the per-job line cap.
type jobLogBatchBody struct {
	Seq   int64              `json:"seq"`
	Lines []store.JobLogLine `json:"lines"`
}

func (a *API) workerJobLog(w http.ResponseWriter, r *http.Request, node *store.Node) {
	body, err := decode[jobLogBatchBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(body.Lines) == 0 {
		writeJSON(w, http.StatusOK, map[string]int{"appended": 0})
		return
	}
	if err := a.svc.Queue().AppendJobLog(workerCtx(r), r.PathValue("id"), node.ID, body.Lines); err != nil {
		if a.queueError(w, err) {
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"appended": len(body.Lines)})
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

// effectiveAccessMode resolves the node's configured access mode for job
// specs (task 09): shared and transfer pass through; auto stays auto — the
// worker runs the shared-storage check and reports the mode it chose.
func effectiveAccessMode(n *store.Node) string {
	switch n.AccessMode {
	case store.NodeAccessShared, store.NodeAccessTransfer:
		return n.AccessMode
	default:
		return store.NodeAccessAuto
	}
}
