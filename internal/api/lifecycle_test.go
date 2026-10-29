// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// Lifecycle endpoint tests (task 11): start, stop (drain and now),
// drain, pause-all/resume-all, the concurrency validation on PUT, the
// claim gate, and the setup wizard's explicit start choice.

// lifecycleHarness is an apiHarness with the built-in node registered.
type lifecycleHarness struct {
	*apiHarness
	builtinKey string
	builtinID  string
}

func newLifecycleHarness(t *testing.T) *lifecycleHarness {
	h := &lifecycleHarness{apiHarness: newAPIHarness(t)}
	key, node := h.addNode(t, "built-in", store.NodeStatusApproved, true)
	h.builtinKey, h.builtinID = key, node.ID
	return h
}

// pairNode registers a fresh node through the pairing endpoint (so it
// starts stopped, as every new node does) and returns its id and key.
func (h *lifecycleHarness) pairNode(t *testing.T, name string) (string, string) {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/nodes/tokens", nil)
	var tok struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	rec = h.do(http.MethodPost, "/api/v1/pair", map[string]any{
		"token": tok.Token, "name": name, "os": "linux", "arch": "amd64",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp server.PairResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.NodeKey)
	return resp.NodeID, resp.NodeKey
}

func (h *apiHarness) listNodes(t *testing.T) []server.NodeDTO {
	t.Helper()
	rec := h.do(http.MethodGet, "/api/v1/nodes", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []server.NodeDTO `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	return list.Items
}

// doOn runs a request against a handler built with extra options.
func (h *apiHarness) doOn(mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var req *http.Request
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeNode(t *testing.T, rec *httptest.ResponseRecorder) server.NodeDTO {
	t.Helper()
	var node server.NodeDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &node))
	return node
}

func TestNodeLifecycleViaAPI(t *testing.T) {
	h := newLifecycleHarness(t)

	// A freshly paired node is stopped with concurrency 1 (every new
	// node waits for a human).
	id, key := h.pairNode(t, "worker-a")
	nodes := h.listNodes(t)
	require.Len(t, nodes, 2)
	require.Equal(t, store.NodeStateStopped, nodes[1].DesiredState)
	require.Equal(t, "stopped", nodes[1].EffectiveState)
	require.Equal(t, 1, nodes[1].Concurrency)

	// Start: running.
	rec := h.do(http.MethodPost, "/api/v1/nodes/"+id+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	started := decodeNode(t, rec)
	require.Equal(t, store.NodeStateRunning, started.DesiredState)
	require.Equal(t, "running", started.EffectiveState)

	// Stop with drain: draining.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+id+"/stop", map[string]any{"drain": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	draining := decodeNode(t, rec)
	require.Equal(t, store.NodeStateDraining, draining.DesiredState)
	require.Equal(t, "draining", draining.EffectiveState)

	// The dedicated drain endpoint does the same.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+id+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+id+"/drain", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, store.NodeStateDraining, decodeNode(t, rec).DesiredState)

	// Stop now (a bodyless call defaults to drain=false): stopped.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+id+"/stop", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stopped := decodeNode(t, rec)
	require.Equal(t, store.NodeStateStopped, stopped.DesiredState)
	require.Equal(t, "stopped", stopped.EffectiveState)

	// The worker learns the instruction from the heartbeat.
	hb := h.doNode(http.MethodPost, "/api/v1/worker/heartbeat", map[string]any{}, key)
	var state struct {
		Concurrency    int    `json:"concurrency"`
		DesiredState   string `json:"desired_state"`
		EffectiveState string `json:"effective_state"`
	}
	require.NoError(t, json.Unmarshal(hb.Body.Bytes(), &state))
	require.Equal(t, store.NodeStateStopped, state.DesiredState)
	require.Equal(t, "stopped", state.EffectiveState)
	require.Equal(t, 1, state.Concurrency)
}

func TestNodeStartClearsStateError(t *testing.T) {
	h := newLifecycleHarness(t)
	ctx := context.Background()

	// A recorded transition failure shows as error with its reason…
	node, err := h.st.GetNode(ctx, h.builtinID)
	require.NoError(t, err)
	node.StateError = "pause-all: disk went away"
	require.NoError(t, h.st.UpdateNode(ctx, node))
	nodes := h.listNodes(t)
	require.Len(t, nodes, 1)
	broken := nodes[0]
	require.Equal(t, "error", broken.EffectiveState)
	require.Equal(t, "pause-all: disk went away", broken.StateError)

	// …and a successful start clears it.
	rec := h.do(http.MethodPost, "/api/v1/nodes/"+node.ID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	started := decodeNode(t, rec)
	require.Equal(t, "running", started.EffectiveState)
	require.Empty(t, started.StateError)
}

func TestPauseAllRestoresIntent(t *testing.T) {
	h := newLifecycleHarness(t)

	// The built-in node is running; a second node is left draining.
	otherID, _ := h.pairNode(t, "worker-b")
	rec := h.do(http.MethodPost, "/api/v1/nodes/"+otherID+"/drain", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Pause all: both stop.
	rec = h.do(http.MethodPost, "/api/v1/nodes/pause-all", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var paused struct {
		Paused int `json:"paused"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &paused))
	require.Equal(t, 2, paused.Paused)
	for _, n := range h.listNodes(t) {
		require.Equal(t, store.NodeStateStopped, n.DesiredState)
	}

	// A repeat pause-all must not overwrite the remembered intent.
	rec = h.do(http.MethodPost, "/api/v1/nodes/pause-all", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Resume all: the built-in node runs again, the other drains again.
	rec = h.do(http.MethodPost, "/api/v1/nodes/resume-all", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resumed struct {
		Resumed int `json:"resumed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resumed))
	require.Equal(t, 2, resumed.Resumed)
	byID := map[string]server.NodeDTO{}
	for _, n := range h.listNodes(t) {
		byID[n.ID] = n
	}
	require.Equal(t, store.NodeStateRunning, byID[h.builtinID].DesiredState)
	require.Equal(t, store.NodeStateDraining, byID[otherID].DesiredState)

	// Resume without a snapshot is a no-op.
	rec = h.do(http.MethodPost, "/api/v1/nodes/resume-all", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resumed))
	require.Equal(t, 0, resumed.Resumed)
}

func TestNodeConcurrencyValidation(t *testing.T) {
	h := newLifecycleHarness(t)
	id := h.builtinID

	for _, bad := range []int{0, -1, 17, 100} {
		rec := h.do(http.MethodPut, "/api/v1/nodes/"+id, map[string]any{"concurrency": bad})
		require.Equal(t, http.StatusBadRequest, rec.Code, "concurrency %d must be 400", bad)
		var errBody struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
		require.Equal(t, "invalid_request", errBody.Code)
	}

	rec := h.do(http.MethodPut, "/api/v1/nodes/"+id, map[string]any{"concurrency": 16})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 16, decodeNode(t, rec).Concurrency)
}

func TestClaimRefusedWhenNodeNotRunning(t *testing.T) {
	h := newLifecycleHarness(t)
	_, _, _ = h.queueOne(t)

	// A freshly paired node is stopped: it may heartbeat but not claim.
	id, key := h.pairNode(t, "worker-a")
	rec := h.doNode(http.MethodPost, "/api/v1/worker/heartbeat", map[string]any{}, key)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusConflict, rec.Code)
	var errBody struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	require.Equal(t, "node_not_running", errBody.Code)

	// Started, the claim goes through.
	rec = h.do(http.MethodPost, "/api/v1/nodes/"+id+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestSetupStartNodeChoice(t *testing.T) {
	h := newLifecycleHarness(t)
	// The API main builds wires the built-in node's id; the wizard's
	// explicit start choice acts on it.
	mux := New("v0.0.0", testStatic(), h.svc, WithBuiltinNode(h.builtinID))

	// Start from stopped, the way a fresh install is.
	rec := h.do(http.MethodPost, "/api/v1/nodes/"+h.builtinID+"/stop", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Default: the wizard sends no start choice → the node stays stopped.
	rec = h.doOn(mux, http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": false,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	node, err := h.st.GetNode(context.Background(), h.builtinID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, node.DesiredState,
		"a completed wizard with no start choice leaves the node stopped")

	// An explicit "start now" flips it to running. Re-open setup first.
	require.NoError(t, h.st.SetSetting(context.Background(), store.SettingSetupComplete, ""))
	rec = h.doOn(mux, http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": false, "start_node": true,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	node, err = h.st.GetNode(context.Background(), h.builtinID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateRunning, node.DesiredState)
}
