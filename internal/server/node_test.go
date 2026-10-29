// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// nodeHarness is a server harness plus a registry handle.
type nodeHarness struct {
	*harness
	reg    *NodeRegistry
	dataDir string
}

func newNodeHarness(t *testing.T) *nodeHarness {
	t.Helper()
	h := &nodeHarness{harness: newHarness(t), dataDir: t.TempDir()}
	h.reg = h.svc.Nodes()
	return h
}

func TestAuthenticateEmptyKey(t *testing.T) {
	h := newNodeHarness(t)
	_, err := h.reg.Authenticate(context.Background(), "")
	require.ErrorIs(t, err, ErrUnauthorized)
	_, err = h.reg.Authenticate(context.Background(), "   ")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestAuthenticateUnknownKey(t *testing.T) {
	h := newNodeHarness(t)
	_, err := h.reg.Authenticate(context.Background(), "no-such-key")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestAuthenticateNodeStates(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	key := "secret-key"
	node := &store.Node{
		Name: "worker-1", KeyHash: HashKey(key), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusPending, Enabled: true,
	}
	require.NoError(t, h.st.CreateNode(ctx, node))

	// Pending nodes may not use the worker API.
	_, err := h.reg.Authenticate(ctx, key)
	require.ErrorIs(t, err, ErrUnauthorized)

	node.Status = store.NodeStatusApproved
	node.Enabled = false
	require.NoError(t, h.st.UpdateNode(ctx, node))
	_, err = h.reg.Authenticate(ctx, key)
	require.ErrorIs(t, err, ErrUnauthorized) // disabled

	node.Enabled = true
	require.NoError(t, h.st.UpdateNode(ctx, node))
	got, err := h.reg.Authenticate(ctx, key)
	require.NoError(t, err)
	require.Equal(t, node.ID, got.ID)

	node.Status = store.NodeStatusRevoked
	require.NoError(t, h.st.UpdateNode(ctx, node))
	_, err = h.reg.Authenticate(ctx, key)
	require.ErrorIs(t, err, ErrUnauthorized) // revoked
}

func TestEnsureBuiltinNodeCreatesAndReuses(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	name := BuiltinNodeName("testhost")

	key1, node1, err := h.reg.EnsureBuiltinNode(ctx, h.dataDir, name)
	require.NoError(t, err)
	require.Equal(t, name, node1.Name)
	require.Equal(t, store.NodeStatusApproved, node1.Status)
	require.True(t, node1.Enabled)
	require.Equal(t, store.NodeAccessShared, node1.AccessMode)
	require.Equal(t, HashKey(key1), node1.KeyHash)

	raw, err := os.ReadFile(filepath.Join(h.dataDir, NodeKeyFile))
	require.NoError(t, err)
	require.Equal(t, key1, trimSpace(string(raw)))

	// A restart with the same key file returns the same node.
	key2, node2, err := h.reg.EnsureBuiltinNode(ctx, h.dataDir, name)
	require.NoError(t, err)
	require.Equal(t, key1, key2)
	require.Equal(t, node1.ID, node2.ID)

	nodes, err := h.st.ListNodes(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
}

func TestEnsureBuiltinNodeRekeysLostKeyFile(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	name := BuiltinNodeName("testhost")

	_, node1, err := h.reg.EnsureBuiltinNode(ctx, h.dataDir, name)
	require.NoError(t, err)

	// The key file is lost (new data dir, same DB): the existing node is
	// re-keyed instead of spawning a duplicate.
	newDir := t.TempDir()
	key2, node2, err := h.reg.EnsureBuiltinNode(ctx, newDir, name)
	require.NoError(t, err)
	require.NotEqual(t, node1.KeyHash, HashKey(key2))
	require.Equal(t, node1.ID, node2.ID)

	nodes, err := h.st.ListNodes(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, HashKey(key2), nodes[0].KeyHash)
}

func TestConcurrencySetting(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	n, err := h.reg.Concurrency(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n) // default

	require.NoError(t, h.st.SetSetting(ctx, store.SettingWorkerConcurrency, "4"))
	n, err = h.reg.Concurrency(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, n)

	require.NoError(t, h.st.SetSetting(ctx, store.SettingWorkerConcurrency, "not-a-number"))
	n, err = h.reg.Concurrency(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n) // invalid falls back

	require.NoError(t, h.st.SetSetting(ctx, store.SettingWorkerConcurrency, "0"))
	n, err = h.reg.Concurrency(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n) // minimum 1
}

func TestHeartbeatRefreshesNodeAndJobs(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	node := &store.Node{
		Name: "worker-1", KeyHash: HashKey("k"), AccessMode: store.NodeAccessShared,
		Status: store.NodeStatusApproved, Enabled: true,
	}
	require.NoError(t, h.st.CreateNode(ctx, node))

	// Real library and file rows: jobs carry foreign keys.
	lib := h.createLibrary(false)
	f := &store.File{LibraryID: lib.ID, Path: filepath.Join(h.root, "a.mkv"), Status: store.FileStatusNew}
	require.NoError(t, h.st.UpsertFile(ctx, f))

	// An old running job of this node and someone else's done job.
	job := &store.Job{ID: store.NewID(), FileID: f.ID, LibraryID: lib.ID,
		NodeID: node.ID, Status: store.JobStatusRunning, HeartbeatAt: time.Now().Add(-2 * time.Minute)}
	require.NoError(t, h.st.CreateJob(ctx, job))
	other := &store.Job{ID: store.NewID(), FileID: f.ID, LibraryID: lib.ID,
		NodeID: "other-node", Status: store.JobStatusRunning, HeartbeatAt: time.Now().Add(-2 * time.Minute)}
	require.NoError(t, h.st.CreateJob(ctx, other))

	concurrency, err := h.reg.Heartbeat(ctx, node, `{"encoders":[]}`, "", []string{job.ID, other.ID, "ghost"})
	require.NoError(t, err)
	require.Equal(t, 1, concurrency.Concurrency)
	require.Equal(t, store.NodeStateStopped, concurrency.DesiredState,
		"a node the admin never started is stopped, even mid-heartbeat")

	updated, err := h.st.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), updated.HeartbeatAt, 5*time.Second) // refreshed

	untouched, err := h.st.GetJob(ctx, other.ID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(-2*time.Minute), untouched.HeartbeatAt, 5*time.Second) // not ours

	after, err := h.st.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), after.LastHeartbeatAt, 5*time.Second)
	require.JSONEq(t, `{"encoders":[]}`, after.Capabilities)
}

func TestResolveNodePath(t *testing.T) {
	identity := &store.Node{PathMappings: "[]"}
	require.Equal(t, "/media/Movies/a.mkv", ResolveNodePath(identity, "/media/Movies/a.mkv"))

	mapped := &store.Node{PathMappings: `[{"server_path": "/media", "node_path": "/mnt/nas"}]`}
	require.Equal(t, "/mnt/nas/Movies/a.mkv", ResolveNodePath(mapped, "/media/Movies/a.mkv"))
	// Path-boundary aware: /media2 is not under /media.
	require.Equal(t, "/media2/x.mkv", ResolveNodePath(mapped, "/media2/x.mkv"))
	require.Equal(t, "/mnt/nas", ResolveNodePath(mapped, "/media"))

	// Longest prefix wins.
	nested := &store.Node{PathMappings: `[
		{"server_path": "/media", "node_path": "/mnt/nas"},
		{"server_path": "/media/kids", "node_path": "/srv/kids"}
	]`}
	require.Equal(t, "/srv/kids/cartoons/a.mkv", ResolveNodePath(nested, "/media/kids/cartoons/a.mkv"))
	require.Equal(t, "/mnt/nas/adults/a.mkv", ResolveNodePath(nested, "/media/adults/a.mkv"))

	// Broken JSON is identity, never a mangled path.
	broken := &store.Node{PathMappings: "not json"}
	require.Equal(t, "/media/a.mkv", ResolveNodePath(broken, "/media/a.mkv"))
}

func TestResolveServerPathIsTheInverse(t *testing.T) {
	mapped := &store.Node{PathMappings: `[{"server_path": "/media", "node_path": "/mnt/nas"}]`}
	require.Equal(t, "/media/Movies/a.mkv", ResolveServerPath(mapped, "/mnt/nas/Movies/a.mkv"))
	// The node side may report its sidecar next to the source (keep_both).
	require.Equal(t, "/media/Movies/a.shrunk.mkv", ResolveServerPath(mapped, "/mnt/nas/Movies/a.shrunk.mkv"))
	// Unmapped paths pass through unchanged.
	require.Equal(t, "/somewhere/else.mkv", ResolveServerPath(mapped, "/somewhere/else.mkv"))
	// No mappings: identity (the built-in node).
	identity := &store.Node{PathMappings: "[]"}
	require.Equal(t, "/media/Movies/a.mkv", ResolveServerPath(identity, "/media/Movies/a.mkv"))
	// Longest node-side prefix wins, symmetric to the forward mapping.
	nested := &store.Node{PathMappings: `[
		{"server_path": "/media", "node_path": "/mnt/nas"},
		{"server_path": "/media/kids", "node_path": "/srv/kids"}
	]`}
	require.Equal(t, "/media/kids/cartoons/a.mkv", ResolveServerPath(nested, "/srv/kids/cartoons/a.mkv"))
	require.Equal(t, "/media/adults/a.mkv", ResolveServerPath(nested, "/mnt/nas/adults/a.mkv"))
}

func TestHashKeyIsSha256Hex(t *testing.T) {
	// A stable, verifiable hash so keys cannot collide across formats.
	require.Equal(t, HashKey("abc"), HashKey("abc"))
	require.NotEqual(t, HashKey("abc"), HashKey("abd"))
	require.Len(t, HashKey("abc"), 64)
	require.Equal(t, HashKey("abc"), hashRef("abc"))
}

// hashRef is an independent SHA-256 reference implementation check.
func hashRef(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// trimSpace is a local helper so the test does not import strings twice.
func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
