// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// nodeKey is the raw key a test node authenticates with; only its hash is
// stored.
func (h *apiHarness) addNode(t *testing.T, name string, status string, enabled bool) (key string, node *store.Node) {
	t.Helper()
	key = "node-key-" + name
	node = &store.Node{
		Name: name, KeyHash: server.HashKey(key), AccessMode: store.NodeAccessShared,
		Status: status, Enabled: enabled,
	}
	require.NoError(t, h.st.CreateNode(context.Background(), node))
	return key, node
}

// doNode performs a request with a node bearer key.
func (h *apiHarness) doNode(method, path string, body any, key string) *httptest.ResponseRecorder {
	h.t.Helper()
	var req *http.Request
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// queueOne creates a library+file and queues a job for it.
func (h *apiHarness) queueOne(t *testing.T) (*store.Library, *store.File, *store.Job) {
	t.Helper()
	ctx := context.Background()
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(ctx, server.LibraryParams{
		Name: "Movies", Path: h.root, TargetJSON: target,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	f := &store.File{
		LibraryID: lib.ID, Path: h.root + "/movie.mp4", Size: 500,
		Status: store.FileStatusNew, ProbeJSON: `{"container":"mp4"}`,
		Fingerprint: "500:1700000000:abcdef",
	}
	require.NoError(t, h.st.UpsertFile(ctx, f))
	job, queued, err := h.svc.Queue().QueueJob(ctx, lib, f, []string{"video is h264"})
	require.NoError(t, err)
	require.True(t, queued)
	return lib, f, job
}

func TestWorkerAuthRejectsBadKeys(t *testing.T) {
	h := newAPIHarness(t)

	cases := []struct {
		name string
		key  string
	}{
		{"no header", ""},
		{"unknown key", "does-not-exist"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, tc.key)
			require.Equal(t, http.StatusUnauthorized, resp.Code)
			var body struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
			require.Equal(t, "unauthorized", body.Code)
		})
	}

	// A disabled node gets 401.
	key, node := h.addNode(t, "disabled", store.NodeStatusApproved, false)
	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusUnauthorized, resp.Code)

	// So does a revoked one.
	node.Status = store.NodeStatusRevoked
	node.Enabled = true
	require.NoError(t, h.st.UpdateNode(context.Background(), node))
	resp = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestWorkerClaimEmptyQueue(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusNoContent, resp.Code)
	require.Empty(t, resp.Body.Bytes())
}

func TestWorkerClaimAndSpec(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, f, _ := h.queueOne(t)

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))
	require.Equal(t, store.JobStatusClaimed, job.Status)

	// The claim moved the file to processing.
	file, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusProcessing, file.Status)

	// Claim again: the queue is empty now.
	resp = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusNoContent, resp.Code)

	// The spec carries everything the worker needs; the built-in-style node
	// has no mappings, so the source path is the library path unchanged.
	resp = h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+job.ID, nil, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var spec server.JobSpec
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &spec))
	require.Equal(t, job.ID, spec.ID)
	require.Equal(t, f.ID, spec.FileID)
	require.Equal(t, flow.Default().Video.Codec, spec.Target.Video.Codec)
	require.Equal(t, f.Path, spec.SourcePath)
	require.Equal(t, store.NodeAccessShared, spec.AccessMode)
	require.Equal(t, store.ResultPolicyReplaceIfSmaller, spec.ResultPolicy)
	require.Equal(t, int64(500), spec.BytesBefore)
	require.Equal(t, "500:1700000000:abcdef", spec.Fingerprint)
}

func TestWorkerSpecRejectsUnclaimedJob(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, _, job := h.queueOne(t) // queued, not claimed

	resp := h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+job.ID, nil, key)
	require.Equal(t, http.StatusConflict, resp.Code)
}

func TestWorkerProgressCompleteFail(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, f, _ := h.queueOne(t)

	// Completion inspection is faked: no ffprobe in unit tests.
	h.svc.Queue().SetInspector(func(_ context.Context, _, path string) (server.ResultInfo, error) {
		require.Equal(t, h.root+"/movie.mkv", path)
		return server.ResultInfo{
			Fingerprint: "100:1700000001:ff00",
			Size:        100, Mtime: 1700000001,
			Info: probe.MediaInfo{Container: flow.ContainerMKV},
		}, nil
	}, "")

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))

	// Progress flips the job to running and echoes the status.
	resp = h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/progress",
		map[string]any{"percent": 50.0, "speed": 2.5, "eta_seconds": 10, "encoder": "cpu"}, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var status struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &status))
	require.Equal(t, store.JobStatusRunning, status.Status)

	// Complete records the result and updates the file.
	resp = h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/complete", map[string]any{
		"output_path": h.root + "/movie.mkv", "bytes_after": 100, "encoder": "cpu",
		"fallback_reasons": []string{"nvenc: no device"}, "log_tail": "ffmpeg stderr",
	}, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var done server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &done))
	require.Equal(t, store.JobStatusDone, done.Status)
	require.Equal(t, "cpu", done.EncoderUsed)
	require.Equal(t, []string{"nvenc: no device"}, done.FallbackReasons)
	require.Equal(t, "ffmpeg stderr", done.Log)

	file, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusSatisfied, file.Status)
	require.Equal(t, int64(400), file.BytesSaved)
	require.Equal(t, h.root+"/movie.mkv", file.Path)
}

func TestWorkerFailEndpoint(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, f, _ := h.queueOne(t)

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))

	resp = h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/fail",
		map[string]any{"error": "encode exploded", "log_tail": "boom"}, key)
	require.Equal(t, http.StatusOK, resp.Code)

	got, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusFailed, got.Status)
	require.Equal(t, "encode exploded", got.Error)
	require.Equal(t, "boom", got.Log)

	file, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusFailed, file.Status)
}

func TestWorkerProgressOnCancelledJobStillReportsStatus(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, _, _ = h.queueOne(t)

	rec := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &job))

	// An admin cancels the job while the worker encodes.
	_, err := h.svc.Queue().Cancel(context.Background(), job.ID)
	require.NoError(t, err)

	// The next progress poll must answer 200 with the terminal status, not
	// 409 — otherwise the worker never learns to stop the encode.
	rec = h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/progress",
		map[string]any{"percent": 50.0}, key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, store.JobStatusCancelled, out.Status)
	// The late progress must not resurrect the cancelled job.
	after, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusCancelled, after.Status)
}

func TestWorkerOwnershipEnforced(t *testing.T) {
	h := newAPIHarness(t)
	keyA, _ := h.addNode(t, "wA", store.NodeStatusApproved, true)
	keyB, _ := h.addNode(t, "wB", store.NodeStatusApproved, true)
	_, _, _ = h.queueOne(t)

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, keyA)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))

	// Node B cannot see the spec, progress, complete or fail node A's job.
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/worker/jobs/" + job.ID, nil},
		{http.MethodPost, "/api/v1/worker/jobs/" + job.ID + "/progress", map[string]any{}},
		{http.MethodPost, "/api/v1/worker/jobs/" + job.ID + "/complete", map[string]any{}},
		{http.MethodPost, "/api/v1/worker/jobs/" + job.ID + "/fail", map[string]any{}},
	} {
		resp := h.doNode(tc.method, tc.path, tc.body, keyB)
		require.Equal(t, http.StatusForbidden, resp.Code, tc.path)
		var body struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		require.Equal(t, "not_job_owner", body.Code)
	}
}

func TestWorkerHeartbeat(t *testing.T) {
	h := newAPIHarness(t)
	key, node := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, _, _ = h.queueOne(t)

	// Claim a job so the heartbeat has something to refresh.
	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))
	stale, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.NoError(t, h.st.UpdateJob(context.Background(), func() *store.Job {
		j := stale
		j.HeartbeatAt = time.Now().Add(-time.Minute)
		return j
	}()))

	resp = h.doNode(http.MethodPost, "/api/v1/worker/heartbeat", map[string]any{
		"capabilities":    map[string]any{"encoders": []any{}, "cpu_cores": 4},
		"running_job_ids": []string{job.ID, "ghost"},
	}, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var out struct {
		Concurrency int `json:"concurrency"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Equal(t, 1, out.Concurrency) // default setting

	// The claimed job's heartbeat was refreshed.
	after, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), after.HeartbeatAt, 5*time.Second)

	// The node's capabilities were stored.
	updated, err := h.st.GetNode(context.Background(), node.ID)
	require.NoError(t, err)
	require.Contains(t, updated.Capabilities, "cpu_cores")
}

func TestWorkerConcurrencyFromSettings(t *testing.T) {
	h := newAPIHarness(t)
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	require.NoError(t, h.st.SetSetting(context.Background(), store.SettingWorkerConcurrency, "3"))

	resp := h.doNode(http.MethodPost, "/api/v1/worker/heartbeat", map[string]any{}, key)
	var out struct {
		Concurrency int `json:"concurrency"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Equal(t, 3, out.Concurrency)
}
