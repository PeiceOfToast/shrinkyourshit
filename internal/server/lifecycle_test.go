// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lifecycle service tests (task 11): the registry transitions, the
// pause-all snapshot semantics and the heartbeat-driven drain completion.

func TestNodeLifecycleTransitions(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	key, node, err := h.reg.EnsureBuiltinNode(ctx, h.dataDir, BuiltinNodeName("host"))
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, node.DesiredState,
		"a fresh install starts with a stopped built-in node")

	started, err := h.reg.Start(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateRunning, started.DesiredState)
	require.Equal(t, "running", EffectiveState(started))

	draining, err := h.reg.Stop(ctx, node.ID, true)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateDraining, draining.DesiredState)

	now, err := h.reg.Stop(ctx, node.ID, false)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, now.DesiredState)

	// Restarting keeps the operator's intent: EnsureBuiltinNode returns
	// the stored node untouched.
	_, again, err := h.reg.EnsureBuiltinNode(ctx, h.dataDir, BuiltinNodeName("host"))
	require.NoError(t, err)
	require.Equal(t, now.DesiredState, again.DesiredState)
	require.Equal(t, HashKey(key), again.KeyHash)
}

func TestEffectiveState(t *testing.T) {
	cases := []struct {
		desired string
		err     string
		want    string
	}{
		{store.NodeStateStopped, "", "stopped"},
		{store.NodeStateRunning, "", "running"},
		{store.NodeStateDraining, "", "draining"},
		{store.NodeStateRunning, "pause-all failed", "error"},
		{"", "", "stopped"},
	}
	for _, tc := range cases {
		n := &store.Node{DesiredState: tc.desired, StateError: tc.err}
		assert.Equal(t, tc.want, EffectiveState(n), "desired=%q err=%q", tc.desired, tc.err)
	}
}

func TestHeartbeatCompletesDrainWhenWorkerReportsStopped(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	node := &store.Node{
		Name: "worker", KeyHash: HashKey("k"), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusApproved, Enabled: true,
		DesiredState: store.NodeStateDraining, Concurrency: 2,
	}
	require.NoError(t, h.st.CreateNode(ctx, node))

	// The worker went idle and reports stopped: the drain is complete.
	state, err := h.reg.Heartbeat(ctx, node, "", store.NodeStateStopped, nil)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, state.DesiredState)
	require.Equal(t, "stopped", state.EffectiveState)
	require.Equal(t, 2, state.Concurrency, "the node's own cap rides along")

	// A stopped report from a node the admin wanted running changes
	// nothing (operator intent is not the worker's to overturn).
	node.DesiredState = store.NodeStateRunning
	state, err = h.reg.Heartbeat(ctx, node, "", store.NodeStateStopped, nil)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateRunning, state.DesiredState)

	// No state report (a pre-task-11 worker) changes nothing either.
	node.DesiredState = store.NodeStateDraining
	state, err = h.reg.Heartbeat(ctx, node, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateDraining, state.DesiredState)
}

func TestPauseAllRestoresIntent(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	node := &store.Node{
		Name: "worker", KeyHash: HashKey("k"), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusApproved, Enabled: true, DesiredState: store.NodeStateRunning,
	}
	require.NoError(t, h.st.CreateNode(ctx, node))

	stopped, err := h.reg.PauseAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, stopped)

	after, err := h.st.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, after.DesiredState)

	// A repeat pause-all keeps the first snapshot: nothing new to stop.
	stopped, err = h.reg.PauseAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, stopped)

	// A node that joins while everything is paused is not in the
	// snapshot; resume-all leaves it stopped (a human starts it).
	fresh := &store.Node{
		Name: "late", KeyHash: HashKey("k2"), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusApproved, Enabled: true,
	}
	require.NoError(t, h.st.CreateNode(ctx, fresh))

	resumed, err := h.reg.ResumeAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, resumed)
	after, err = h.st.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateRunning, after.DesiredState)
	late, err := h.st.GetNode(ctx, fresh.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStateStopped, late.DesiredState)

	// Without a snapshot, resume-all is a no-op.
	resumed, err = h.reg.ResumeAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, resumed)
}

func TestCurrentJobIDs(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	lib := h.createLibrary(false)
	f1 := &store.File{LibraryID: lib.ID, Path: h.root + "/a.mkv", Status: store.FileStatusNew}
	require.NoError(t, h.st.UpsertFile(ctx, f1))
	f2 := &store.File{LibraryID: lib.ID, Path: h.root + "/b.mkv", Status: store.FileStatusNew}
	require.NoError(t, h.st.UpsertFile(ctx, f2))

	node := &store.Node{Name: "worker", KeyHash: HashKey("k"), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusApproved, Enabled: true}
	require.NoError(t, h.st.CreateNode(ctx, node))

	old := &store.Job{ID: store.NewID(), FileID: f1.ID, LibraryID: lib.ID, NodeID: node.ID,
		Status: store.JobStatusClaimed, HeartbeatAt: time.Now(),
		CreatedAt: time.Now().Add(-time.Minute)}
	require.NoError(t, h.st.CreateJob(ctx, old))
	new := &store.Job{ID: store.NewID(), FileID: f2.ID, LibraryID: lib.ID, NodeID: node.ID,
		Status: store.JobStatusRunning, HeartbeatAt: time.Now(), CreatedAt: time.Now()}
	require.NoError(t, h.st.CreateJob(ctx, new))
	other := &store.Job{ID: store.NewID(), FileID: f2.ID, LibraryID: lib.ID, NodeID: "another",
		Status: store.JobStatusRunning, HeartbeatAt: time.Now(), CreatedAt: time.Now()}
	require.NoError(t, h.st.CreateJob(ctx, other))

	ids := h.reg.CurrentJobIDs(ctx)
	require.Len(t, ids, 2)
	require.Equal(t, old.ID, ids[node.ID], "the oldest in-flight job is the current one")
	require.Equal(t, other.ID, ids["another"])
}
