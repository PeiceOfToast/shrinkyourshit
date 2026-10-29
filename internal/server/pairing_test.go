// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// makeToken inserts a pairing token row directly, with a chosen expiry.
func makeToken(t *testing.T, h *nodeHarness, expires time.Time, used bool) (raw string) {
	t.Helper()
	raw = "raw-pairing-token"
	tok := &store.PairingToken{
		TokenHash: HashToken(raw),
		ExpiresAt: expires,
	}
	if used {
		tok.UsedAt = time.Now().UTC()
	}
	require.NoError(t, h.st.CreatePairingToken(context.Background(), tok))
	return raw
}

// makeNode inserts a node in a chosen state and returns it.
func makeNode(t *testing.T, h *nodeHarness, name, status string, enabled bool) *store.Node {
	t.Helper()
	n := &store.Node{
		Name: name, AccessMode: store.NodeAccessShared, PathMappings: "[]",
		Status: status, Enabled: enabled,
	}
	if status == store.NodeStatusApproved {
		n.KeyHash = HashKey("key-" + name)
	}
	require.NoError(t, h.st.CreateNode(context.Background(), n))
	return n
}

func TestCreateTokenStoresHashOnly(t *testing.T) {
	h := newNodeHarness(t)
	raw, expires, err := h.reg.CreateToken(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, HashToken(raw), raw) // raw never doubles as the hash
	require.WithinDuration(t, time.Now().Add(PairingTokenTTL), expires, time.Minute)

	stored, err := h.st.GetPairingTokenByHash(context.Background(), HashToken(raw))
	require.NoError(t, err)
	require.Equal(t, HashToken(raw), stored.TokenHash)
	require.True(t, stored.UsedAt.IsZero())
}

func TestPairWithTokenApprovesImmediately(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	raw, _, err := h.reg.CreateToken(ctx)
	require.NoError(t, err)

	node, key, err := h.reg.Pair(ctx, PairInput{
		Token: raw, Name: "worker-1", OS: "linux", Arch: "amd64",
		Capabilities: `{"encoders":[]}`,
	})
	require.NoError(t, err)
	require.Equal(t, store.NodeStatusApproved, node.Status)
	require.True(t, node.Enabled)
	require.Equal(t, store.NodeAccessAuto, node.AccessMode)
	require.Equal(t, HashKey(key), node.KeyHash)
	require.Empty(t, node.KeyOnce) // delivered in the response, never stored raw
	require.Equal(t, `{"encoders":[]}`, node.Capabilities)

	// The token is used up: the exact same request fails.
	_, _, err = h.reg.Pair(ctx, PairInput{Token: raw, Name: "worker-1"})
	require.ErrorIs(t, err, ErrInvalidToken)

	// The node authenticates with its key.
	got, err := h.reg.Authenticate(ctx, key)
	require.NoError(t, err)
	require.Equal(t, node.ID, got.ID)
}

func TestPairExpiredOrUnknownToken(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	_, _, err := h.reg.Pair(ctx, PairInput{Token: "no-such-token", Name: "w"})
	require.ErrorIs(t, err, ErrInvalidToken)

	raw := makeToken(t, h, time.Now().UTC().Add(-time.Minute), false)
	_, _, err = h.reg.Pair(ctx, PairInput{Token: raw, Name: "w"})
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestPairWithoutTokenIsPending(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()

	node, key, err := h.reg.Pair(ctx, PairInput{Name: "worker-2", OS: "linux"})
	require.NoError(t, err)
	require.Empty(t, key)
	require.Equal(t, store.NodeStatusPending, node.Status)
	require.Empty(t, node.KeyHash)
	require.Empty(t, node.KeyOnce)

	// A pending node cannot authenticate.
	_, err = h.reg.Authenticate(ctx, "anything")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestPairRequiresName(t *testing.T) {
	h := newNodeHarness(t)
	_, _, err := h.reg.Pair(context.Background(), PairInput{Name: "  "})
	require.ErrorIs(t, err, ErrPairInput)
}

func TestApproveDeliversKeyOnce(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node, _, err := h.reg.Pair(ctx, PairInput{Name: "worker-3"})
	require.NoError(t, err)

	approved, err := h.reg.Approve(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStatusApproved, approved.Status)
	require.NotEmpty(t, approved.KeyHash)
	require.NotEmpty(t, approved.KeyOnce)

	// The raw key is delivered exactly once; afterwards only the hash
	// remains (acceptance item 6).
	got, key, err := h.reg.PollPair(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStatusApproved, got.Status)
	require.NotEmpty(t, key)
	require.Equal(t, HashKey(key), approved.KeyHash)

	got2, key2, err := h.reg.PollPair(ctx, node.ID)
	require.NoError(t, err)
	require.Empty(t, key2)
	require.Equal(t, store.NodeStatusApproved, got2.Status)

	stored, err := h.st.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.Empty(t, stored.KeyOnce)
	require.NotEmpty(t, stored.KeyHash)

	// The key authenticates.
	authed, err := h.reg.Authenticate(ctx, key)
	require.NoError(t, err)
	require.Equal(t, node.ID, authed.ID)
}

func TestApproveTwiceFails(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node, _, err := h.reg.Pair(ctx, PairInput{Name: "worker-4"})
	require.NoError(t, err)
	_, err = h.reg.Approve(ctx, node.ID)
	require.NoError(t, err)
	_, err = h.reg.Approve(ctx, node.ID)
	require.ErrorIs(t, err, ErrNotPending)
}

// seedJobFor creates a library, a file and a job for nodeID. The file sits
// in processing, as it would after a real claim.
func seedJobFor(t *testing.T, h *nodeHarness, nodeID string, status string, attempt int) *store.Job {
	t.Helper()
	ctx := context.Background()
	lib := h.createLibrary(false)
	f := &store.File{LibraryID: lib.ID, Path: fmt.Sprintf("%s/a-%s.mkv", h.root, nodeID[:4]), Status: store.FileStatusProcessing}
	require.NoError(t, h.st.UpsertFile(ctx, f))
	job := &store.Job{ID: store.NewID(), FileID: f.ID, LibraryID: lib.ID,
		NodeID: nodeID, Status: status, Attempt: attempt, HeartbeatAt: time.Now().UTC()}
	require.NoError(t, h.st.CreateJob(ctx, job))
	return job
}

func TestRevokeClearsKeysAndRequeuesJobs(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node := makeNode(t, h, "worker-5", store.NodeStatusApproved, true)
	node.KeyOnce = "raw-key-not-yet-retrieved"
	require.NoError(t, h.st.UpdateNode(ctx, node))
	job := seedJobFor(t, h, node.ID, store.JobStatusRunning, 0)
	other := seedJobFor(t, h, "other-node", store.JobStatusRunning, 0)

	_, err := h.reg.Revoke(ctx, node.ID)
	require.NoError(t, err)

	stored, err := h.st.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeStatusRevoked, stored.Status)
	require.Empty(t, stored.KeyHash) // revoking deletes the key hash
	require.Empty(t, stored.KeyOnce)

	// The node's own job is back in the queue with a fresh attempt; the
	// other node's job is untouched.
	moved, err := h.st.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, moved.Status)
	require.Equal(t, 1, moved.Attempt)
	require.Empty(t, moved.NodeID)
	untouched, err := h.st.GetJob(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusRunning, untouched.Status)
	require.Equal(t, "other-node", untouched.NodeID)

	// The node's key no longer authenticates (acceptance item 4).
	_, err = h.reg.Authenticate(ctx, "key-worker-5")
	require.ErrorIs(t, err, ErrUnauthorized)

	// Revoking twice is fine.
	_, err = h.reg.Revoke(ctx, node.ID)
	require.NoError(t, err)
}

func TestRevokeFailsJobAtAttemptCap(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node := makeNode(t, h, "worker-6", store.NodeStatusApproved, true)
	job := seedJobFor(t, h, node.ID, store.JobStatusRunning, MaxJobAttempts-1)

	_, err := h.reg.Revoke(ctx, node.ID)
	require.NoError(t, err)

	failed, err := h.st.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusFailed, failed.Status)
	require.Contains(t, failed.Error, "revoked")

	f, err := h.st.GetFile(ctx, job.FileID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusFailed, f.Status)
}

func TestDeleteNodeRemovesAndRequeues(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node, _, err := h.reg.Pair(ctx, PairInput{Name: "worker-7"})
	require.NoError(t, err)
	job := seedJobFor(t, h, node.ID, store.JobStatusClaimed, 0)

	require.NoError(t, h.reg.Delete(ctx, node.ID))
	_, err = h.st.GetNode(ctx, node.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	moved, err := h.st.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, moved.Status)
	require.Empty(t, moved.NodeID)

	// Deleting again is not found.
	require.ErrorIs(t, h.reg.Delete(ctx, node.ID), store.ErrNotFound)
}

func TestUpdateNodeParams(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node, _, err := h.reg.Pair(ctx, PairInput{Name: "worker-8"})
	require.NoError(t, err)
	_, err = h.reg.Approve(ctx, node.ID)
	require.NoError(t, err)

	enabled := false
	mode := store.NodeAccessTransfer
	mappings := `[{"server_path": "/media", "node_path": "/mnt/nas"}]`
	updated, err := h.reg.Update(ctx, node.ID, NodeUpdateParams{
		Name: strPtr("renamed"), Enabled: &enabled,
		AccessMode: &mode, PathMappings: &mappings,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.False(t, updated.Enabled)
	require.Equal(t, store.NodeAccessTransfer, updated.AccessMode)
	require.Equal(t, mappings, updated.PathMappings)

	// Invalid values are rejected, state unchanged.
	badMode := "teleport"
	_, err = h.reg.Update(ctx, node.ID, NodeUpdateParams{AccessMode: &badMode})
	require.ErrorIs(t, err, ErrPairInput)
	empty := " "
	_, err = h.reg.Update(ctx, node.ID, NodeUpdateParams{Name: &empty})
	require.ErrorIs(t, err, ErrPairInput)
	broken := `{`
	_, err = h.reg.Update(ctx, node.ID, NodeUpdateParams{PathMappings: &broken})
	require.ErrorIs(t, err, ErrPairInput)
}

func strPtr(s string) *string { return &s }

func TestValidatePathMappings(t *testing.T) {
	require.NoError(t, ValidatePathMappings(""))
	require.NoError(t, ValidatePathMappings("   "))
	require.NoError(t, ValidatePathMappings(`[{"server_path": "/media", "node_path": "/mnt/nas"}]`))
	require.NoError(t, ValidatePathMappings(`[]`))

	require.ErrorIs(t, ValidatePathMappings(`{`), ErrPairInput)
	require.ErrorIs(t, ValidatePathMappings(`[{"server_path": "/media"}]`), ErrPairInput)
	require.ErrorIs(t, ValidatePathMappings(`[{"server_path": "/media", "node_path": " "}]`), ErrPairInput)
	require.ErrorIs(t, ValidatePathMappings(`{"server_path": "/media"}`), ErrPairInput)
}

func TestConsumeKeyOnceIsOnce(t *testing.T) {
	h := newNodeHarness(t)
	ctx := context.Background()
	node := makeNode(t, h, "worker-9", store.NodeStatusApproved, true)
	node.KeyOnce = "one-time-raw-key"
	require.NoError(t, h.st.UpdateNode(ctx, node))

	key, err := h.st.ConsumeNodeKeyOnce(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, "one-time-raw-key", key)

	key, err = h.st.ConsumeNodeKeyOnce(ctx, node.ID)
	require.NoError(t, err)
	require.Empty(t, key)

	// Unknown node is not found.
	_, err = h.st.ConsumeNodeKeyOnce(ctx, "no-such-node")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// syncBuffer is a concurrency-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestPairingNeverLogsSecrets walks the whole pairing flow with a capture
// logger and asserts the raw key and the raw token never appear in the
// log output (acceptance item 6).
func TestPairingNeverLogsSecrets(t *testing.T) {
	ctx := context.Background()
	var logs syncBuffer
	capture := slog.New(slog.NewTextHandler(&logs, nil))

	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	reg := NewNodeRegistry(st, nil, capture)

	rawToken, _, err := reg.CreateToken(ctx)
	require.NoError(t, err)

	node, key, err := reg.Pair(ctx, PairInput{Token: rawToken, Name: "quiet-worker"})
	require.NoError(t, err)
	require.NotEmpty(t, key)

	pending, _, err := reg.Pair(ctx, PairInput{Name: "pending-worker"})
	require.NoError(t, err)
	_, err = reg.Approve(ctx, pending.ID)
	require.NoError(t, err)
	_, pollKey, err := reg.PollPair(ctx, pending.ID)
	require.NoError(t, err)
	require.NotEmpty(t, pollKey)
	_, err = reg.Revoke(ctx, pending.ID)
	require.NoError(t, err)

	out := logs.String()
	require.NotContains(t, out, rawToken)
	require.NotContains(t, out, key)
	require.NotContains(t, out, pollKey)
}
