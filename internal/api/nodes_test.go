// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// apiSyncBuffer is a concurrency-safe log sink.
type apiSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *apiSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *apiSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newAPIHarnessWithLogger is newAPIHarness with a capture logger wired
// into the server (secret-handling tests).
func newAPIHarnessWithLogger(t *testing.T, sink io.Writer) *apiHarness {
	t.Helper()
	h := &apiHarness{
		t:      t,
		stated: &apiFakeProber{byName: map[string]probe.MediaInfo{}, fallback: apiSatisfied},
		root:   filepath.Join(t.TempDir(), "library"),
	}
	require.NoError(t, os.MkdirAll(h.root, 0o755))
	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	h.st = st
	h.svc = server.New(server.Config{
		Store:             st,
		Log:               slog.New(slog.NewTextHandler(sink, nil)),
		Prober:            h.stated.Probe,
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
	})
	h.svc.Libraries().SetScanHook(func(string) {})
	h.svc.Start(context.Background())
	t.Cleanup(h.svc.Stop)
	h.mux = New("v0.0.0", testStatic(), h.svc)
	return h
}

// pairAs POSTs /pair and decodes the reply.
func pairAs(t *testing.T, h *apiHarness, body map[string]any) (int, server.PairResponse) {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/pair", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp server.PairResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return rec.Code, resp
}

// pollPair GETs /pair/{id} and decodes the reply.
func pollPair(t *testing.T, h *apiHarness, id string) (int, server.PairResponse) {
	t.Helper()
	rec := h.do(http.MethodGet, "/api/v1/pair/"+id, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp server.PairResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return rec.Code, resp
}

func TestPairWithTokenViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/nodes/tokens", nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var tok struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
		Commands  struct {
			Docker     string `json:"docker"`
			Shell      string `json:"shell"`
			PowerShell string `json:"powershell"`
		} `json:"commands"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	require.NotEmpty(t, tok.Token)
	require.NotEmpty(t, tok.ExpiresAt)

	// The install commands carry the real server URL (the test request's
	// host) and the token (acceptance item 7, server side).
	for _, cmd := range []string{tok.Commands.Docker, tok.Commands.Shell, tok.Commands.PowerShell} {
		require.Contains(t, cmd, "example.com")
		require.Contains(t, cmd, tok.Token)
	}

	// Pair with the token: approved immediately with the key.
	_, resp := pairAs(t, h, map[string]any{
		"token": tok.Token, "name": "worker-t", "os": "linux", "arch": "amd64",
		"capabilities": map[string]any{"encoders": []any{}},
	})
	require.NotEmpty(t, resp.NodeID)
	require.Equal(t, "approved", resp.Status)
	require.NotEmpty(t, resp.NodeKey)

	// The node works right away: heartbeat answers 200.
	rec = h.doNode(http.MethodPost, "/api/v1/worker/heartbeat",
		map[string]any{"capabilities": map[string]any{}, "running_job_ids": []string{}}, resp.NodeKey)
	require.Equal(t, http.StatusOK, rec.Code)

	// The token is used up: the same request is 401 (acceptance item 2).
	rec = h.do(http.MethodPost, "/api/v1/pair", map[string]any{
		"token": tok.Token, "name": "worker-t",
	})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var errBody struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	require.Equal(t, "invalid_token", errBody.Code)

	// An unknown token is 401 too.
	rec = h.do(http.MethodPost, "/api/v1/pair", map[string]any{"token": "garbage", "name": "w"})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPairExpiredTokenViaAPI(t *testing.T) {
	h := newAPIHarness(t)
	raw := "expired-token"
	tok := &store.PairingToken{
		TokenHash: server.HashToken(raw),
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, h.st.CreatePairingToken(context.Background(), tok))

	rec := h.do(http.MethodPost, "/api/v1/pair", map[string]any{"token": raw, "name": "w"})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPairApprovalFlowViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	// Pair without a token: pending, no key.
	_, resp := pairAs(t, h, map[string]any{"name": "worker-p", "os": "linux"})
	require.Equal(t, "pending", resp.Status)
	require.Empty(t, resp.NodeKey)

	// Pending nodes cannot claim (acceptance item 3, server side).
	rec := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, "not-a-key")
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// The poll says pending…
	_, resp = pollPair(t, h, resp.NodeID)
	require.Equal(t, "pending", resp.Status)

	// …until the admin approves, and the first poll returns the key.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+resp.NodeID+"/approve", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, resp = pollPair(t, h, resp.NodeID)
	require.Equal(t, "approved", resp.Status)
	require.NotEmpty(t, resp.NodeKey)

	// The key authenticates a claim once the admin starts the node
	// (task 11: nodes are created stopped) — empty queue → 204.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+resp.NodeID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, resp.NodeKey)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// The second poll returns no key (acceptance item 6).
	_, again := pollPair(t, h, resp.NodeID)
	require.Equal(t, "approved", again.Status)
	require.Empty(t, again.NodeKey)

	// Approving again is a 409; the node list shows it approved.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+resp.NodeID+"/approve", nil)
	require.Equal(t, http.StatusConflict, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/nodes", nil)
	var list struct {
		Items []server.NodeDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	require.Equal(t, store.NodeStatusApproved, list.Items[0].Status)
}

func TestPairPollUnknownNode(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/pair/no-such-id", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRevokeViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	// Token-paired node; then revoke it.
	rec := h.do(http.MethodPost, "/api/v1/nodes/tokens", nil)
	var tok struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	_, resp := pairAs(t, h, map[string]any{"token": tok.Token, "name": "worker-r"})
	key := resp.NodeKey

	// The node claims a job so there is something in flight. Starting it
	// first is the admin's job now (task 11: nodes are created stopped).
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+resp.NodeID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, f, job := h.queueOne(t)
	rec = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = h.do(http.MethodPost, "/api/v1/nodes/"+resp.NodeID+"/revoke", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// The next worker request is 401 (acceptance item 4)…
	rec = h.doNode(http.MethodPost, "/api/v1/worker/heartbeat",
		map[string]any{"capabilities": map[string]any{}}, key)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// …and the claimed job went back to queued.
	fresh, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, fresh.Status)
	require.Empty(t, fresh.NodeID)
	row, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusQueued, row.Status)
}

func TestNodeUpdateAndDeleteViaAPI(t *testing.T) {
	h := newAPIHarness(t)
	_, resp := pairAs(t, h, map[string]any{"name": "worker-u"})
	id := resp.NodeID

	mappings := `[{"server_path": "/media", "node_path": "/mnt/nas"}]`
	rec := h.do(http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
		"name": "renamed", "enabled": false, "access_mode": "shared",
		"path_mappings": mappings,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var node server.NodeDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &node))
	require.Equal(t, "renamed", node.Name)
	require.False(t, node.Enabled)
	require.Equal(t, "shared", node.AccessMode)
	require.JSONEq(t, mappings, string(node.PathMappings))

	// Bad values are 400.
	rec = h.do(http.MethodPut, "/api/v1/nodes/"+id, map[string]any{"access_mode": "teleport"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	rec = h.do(http.MethodPut, "/api/v1/nodes/"+id, map[string]any{"path_mappings": "{"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Reject (delete) removes the node.
	rec = h.do(http.MethodDelete, "/api/v1/nodes/"+id, nil)
	require.Equal(t, http.StatusNoContent, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/nodes", nil)
	var list struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, 0, list.Total)
	rec = h.do(http.MethodDelete, "/api/v1/nodes/"+id, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPairRequiresName(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/pair", map[string]any{"name": " "})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestPairAndKeyNeverLogged walks pair → approve → poll over HTTP with a
// capture logger and checks the raw key and token never land in the log
// (acceptance item 6, HTTP side).
func TestPairAndKeyNeverLogged(t *testing.T) {
	var logs apiSyncBuffer
	h := newAPIHarnessWithLogger(t, &logs)

	rec := h.do(http.MethodPost, "/api/v1/nodes/tokens", nil)
	var tok struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	_, resp := pairAs(t, h, map[string]any{"token": tok.Token, "name": "quiet"})

	_, pending := pairAs(t, h, map[string]any{"name": "quiet-2"})
	h.do(http.MethodPost, "/api/v1/nodes/"+pending.NodeID+"/approve", nil)
	_, approved := pollPair(t, h, pending.NodeID)
	require.NotEmpty(t, approved.NodeKey)

	out := logs.String()
	require.NotContains(t, out, tok.Token)
	require.NotContains(t, out, resp.NodeKey)
	require.NotContains(t, out, approved.NodeKey)
	// The SSE/event payloads also never carry the key: a node.updated
	// subscriber sees the DTO, which has no key field at all.
	require.False(t, strings.Contains(out, "\"node_key\""))
}

// statsCountingStore wraps the real store and counts NodeJobStats calls,
// so the aggregate test can prove the handler folds all nodes into one
// grouped query instead of asking per node (task 18 acceptance).
type statsCountingStore struct {
	store.Store
	mu         sync.Mutex
	statsCalls int
}

func (s *statsCountingStore) NodeJobStats(ctx context.Context) (map[string]store.NodeStat, error) {
	s.mu.Lock()
	s.statsCalls++
	s.mu.Unlock()
	return s.Store.NodeJobStats(ctx)
}

func (s *statsCountingStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsCalls
}

// TestNodeStatsAggregate covers the per node totals (task 18): GET /nodes
// returns files_done and bytes_saved per node — done jobs only, positive
// savings only — a node with no finished jobs reports zeros, and the whole
// answer is folded with a single stats query.
func TestNodeStatsAggregate(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	counting := &statsCountingStore{Store: st}
	require.NoError(t, os.MkdirAll(filepath.Join(t.TempDir(), "library"), 0o755))

	svc := server.New(server.Config{
		Store:             counting,
		Prober:            (&apiFakeProber{byName: map[string]probe.MediaInfo{}, fallback: apiSatisfied}).Probe,
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
	})
	svc.Libraries().SetScanHook(func(string) {})
	svc.Start(context.Background())
	t.Cleanup(svc.Stop)
	mux := New("v0.0.0", testStatic(), svc)

	ctx := context.Background()
	lib := &store.Library{ID: "lib-1", Name: "Movies", Path: "/media/movies",
		TargetJSON: `{"video":{"codec":"hevc","quality":3,"reencode_if_same_codec":false},"audio":{"mode":"keep_all","preferred_languages":["eng"],"compact_codec":"aac"},"subtitles":{"mode":"keep_all","preferred_languages":["eng"],"extract_removed":false},"container":"mkv"}`,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller}
	require.NoError(t, counting.CreateLibrary(ctx, lib))
	file := &store.File{ID: "file-1", LibraryID: lib.ID, Path: "/media/movies/movie.mkv", Status: store.FileStatusSatisfied}
	require.NoError(t, counting.UpsertFile(ctx, file))

	// worker-a: two done jobs (one with negative savings that must not
	// count) plus one claimed job that must not count either.
	nodeA := &store.Node{ID: "node-a", Name: "worker-a", KeyHash: "h1", Status: store.NodeStatusApproved,
		Enabled: true, AccessMode: store.NodeAccessShared, DesiredState: store.NodeStateRunning, Concurrency: 1}
	require.NoError(t, counting.CreateNode(ctx, nodeA))
	for _, j := range []store.Job{
		{ID: "job-1", FileID: file.ID, LibraryID: lib.ID, NodeID: nodeA.ID, Status: store.JobStatusDone, BytesBefore: 100, BytesAfter: 40},
		{ID: "job-2", FileID: file.ID, LibraryID: lib.ID, NodeID: nodeA.ID, Status: store.JobStatusDone, BytesBefore: 100, BytesAfter: 120},
		{ID: "job-3", FileID: file.ID, LibraryID: lib.ID, NodeID: nodeA.ID, Status: store.JobStatusClaimed, BytesBefore: 999},
	} {
		require.NoError(t, counting.CreateJob(ctx, &j))
	}
	// worker-b: approved, one queued job, nothing ever finished.
	nodeB := &store.Node{ID: "node-b", Name: "worker-b", KeyHash: "h2", Status: store.NodeStatusApproved,
		Enabled: true, AccessMode: store.NodeAccessShared, DesiredState: store.NodeStateStopped, Concurrency: 1}
	require.NoError(t, counting.CreateNode(ctx, nodeB))
	require.NoError(t, counting.CreateJob(ctx, &store.Job{ID: "job-4", FileID: file.ID, LibraryID: lib.ID, NodeID: nodeB.ID, Status: store.JobStatusQueued}))

	rec := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, rec)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Items []server.NodeDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Len(t, body.Items, 2)
	byID := map[string]server.NodeDTO{}
	for _, n := range body.Items {
		byID[n.ID] = n
	}
	require.NotNil(t, byID["node-a"].Stats, "stats present even when zeroed below")
	require.Equal(t, int64(2), byID["node-a"].Stats.FilesDone)
	require.Equal(t, int64(60), byID["node-a"].Stats.BytesSaved, "only positive savings count")
	require.NotNil(t, byID["node-b"].Stats, "a node with no finished jobs still reports stats")
	require.Equal(t, int64(0), byID["node-b"].Stats.FilesDone)
	require.Equal(t, int64(0), byID["node-b"].Stats.BytesSaved)

	// One grouped query for the whole registry, never one per node.
	require.Equal(t, 1, counting.calls())
}
