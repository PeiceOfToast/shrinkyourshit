// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// newTransferHarness builds a store + a library with a mkv target + a
// source file, and a Transfer service over a temp scratch dir.
type transferHarness struct {
	t       *testing.T
	st      store.Store
	tr      *Transfer
	lib     *store.Library
	file    *store.File
	job     *store.Job
	srcPath string
	srcData string
}

func newTransferHarness(t *testing.T) *transferHarness {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib := &store.Library{Name: "Movies", Path: "/srv/media", TargetJSON: string(target),
		ResultPolicy: store.ResultPolicyReplaceIfSmaller}
	require.NoError(t, st.CreateLibrary(context.Background(), lib))

	h := &transferHarness{
		t:       t,
		st:      st,
		tr:      NewTransfer(st, t.TempDir()),
		lib:     lib,
		srcData: strings.Repeat("SOURCE-BYTES-", 40_000), // ~520KiB
	}
	h.srcPath = filepath.Join(t.TempDir(), "movie.mkv")
	require.NoError(t, os.WriteFile(h.srcPath, []byte(h.srcData), 0o644))
	st0, err := os.Stat(h.srcPath)
	require.NoError(t, err)

	h.file = &store.File{LibraryID: lib.ID, Path: h.srcPath, Size: st0.Size(),
		Status: store.FileStatusProcessing}
	require.NoError(t, st.UpsertFile(context.Background(), h.file))
	h.job = &store.Job{FileID: h.file.ID, LibraryID: lib.ID, Status: store.JobStatusRunning}
	require.NoError(t, st.CreateJob(context.Background(), h.job))
	return h
}

func (h *transferHarness) checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestChallengeDescribesTheSourceAndHashesTheSample(t *testing.T) {
	h := newTransferHarness(t)
	node := &store.Node{ID: "n1", PathMappings: "[]"}
	ch, err := h.tr.Challenge(node, h.file)
	require.NoError(t, err)

	require.Equal(t, h.srcPath, ch.Path, "identity mapping returns the path unchanged")
	require.Equal(t, int64(len(h.srcData)), ch.Size)
	require.GreaterOrEqual(t, ch.SampleOffset, int64(0))
	require.Less(t, ch.SampleOffset, ch.Size)

	// The digest matches a manual read of the same region.
	want, err := hashFileSample(h.srcPath, ch.SampleOffset, min(SampleSize, ch.Size-ch.SampleOffset))
	require.NoError(t, err)
	require.Equal(t, want, ch.SampleSHA256)
}

func TestChallengeMapsTheNodeSidePath(t *testing.T) {
	h := newTransferHarness(t)
	node := &store.Node{ID: "n1", PathMappings: `[{"server_path": "` + filepath.Dir(h.srcPath) + `", "node_path": "/mnt/nas"}]`}
	ch, err := h.tr.Challenge(node, h.file)
	require.NoError(t, err)
	require.Equal(t, "/mnt/nas/"+filepath.Base(h.srcPath), ch.Path)
}

func TestPutChunkFullFlowVerifiesAndFinalizes(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()

	payload := []byte(strings.Repeat("OUTPUT-", 30_000))
	total := int64(len(payload))
	chunk := int64(8 << 10)
	checksum := h.checksum(payload)

	// Chunk 1 (mid chunk): accepted with 204 semantics (done=false).
	done, err := h.tr.PutChunk(ctx, h.job, 0, chunk-1, total, strings.NewReader(string(payload[:chunk])), "")
	require.NoError(t, err)
	require.False(t, done)

	// Chunk 2 is the final one and carries the checksum.
	done, err = h.tr.PutChunk(ctx, h.job, chunk, total-1, total,
		strings.NewReader(string(payload[chunk:])), checksum)
	require.NoError(t, err)
	require.True(t, done)

	path, size, err := h.tr.VerifiedOutput(ctx, h.job)
	require.NoError(t, err)
	require.Equal(t, total, size)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, got, "the reassembled upload matches byte for byte")
	require.Equal(t, ".mkv", filepath.Ext(path), "default target container is mkv")

	// No .part leftover.
	_, err = os.Stat(path + ".part")
	require.True(t, os.IsNotExist(err))

	// Discard cleans the whole job dir.
	h.tr.Discard(h.job.ID)
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestPutChunkRepeatAndGap(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()
	payload := strings.Repeat("Z", 100)
	total := int64(len(payload))

	// First chunk.
	_, err := h.tr.PutChunk(ctx, h.job, 0, 49, total, strings.NewReader(payload[:50]), "")
	require.NoError(t, err)

	// A repeat of the same range overwrites cleanly (retransmit).
	_, err = h.tr.PutChunk(ctx, h.job, 0, 49, total, strings.NewReader(payload[:50]), "")
	require.NoError(t, err)

	// A chunk starting beyond the received prefix is a gap.
	_, err = h.tr.PutChunk(ctx, h.job, 60, 99, total, strings.NewReader(payload[60:]), "")
	require.ErrorIs(t, err, ErrUploadGap)

	// Finish from where we are.
	_, err = h.tr.PutChunk(ctx, h.job, 50, 99, total, strings.NewReader(payload[50:]), h.checksum([]byte(payload)))
	require.NoError(t, err)
	path, size, verr := h.tr.VerifiedOutput(ctx, h.job)
	require.NoError(t, verr)
	require.Equal(t, total, size)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, string(got))
}

func TestPutChunkRestartFromZeroReplacesStaleState(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()

	// A stale verified output from an earlier attempt exists.
	old := []byte("STALE-OUTPUT")
	total := int64(len(old))
	_, err := h.tr.PutChunk(ctx, h.job, 0, total-1, total, strings.NewReader(string(old)), h.checksum(old))
	require.NoError(t, err)

	// A fresh upload from byte 0 discards the stale verified result.
	fresh := []byte("FRESH")
	newTotal := int64(len(fresh))
	_, err = h.tr.PutChunk(ctx, h.job, 0, newTotal-1, newTotal, strings.NewReader(string(fresh)), h.checksum(fresh))
	require.NoError(t, err)
	path, size, err := h.tr.VerifiedOutput(ctx, h.job)
	require.NoError(t, err)
	require.Equal(t, newTotal, size)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, fresh, got)
}

func TestPutChunkChecksumMismatchDiscards(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()
	payload := strings.Repeat("A", 64)

	// Flip a byte between what is uploaded and what the checksum covers.
	corrupt := strings.Repeat("B", 64)
	_, err := h.tr.PutChunk(ctx, h.job, 0, int64(len(corrupt))-1, int64(len(corrupt)),
		strings.NewReader(corrupt), h.checksum([]byte(payload)))
	require.ErrorIs(t, err, ErrChecksumMismatch)

	// The partial upload is gone and nothing verified exists.
	_, _, verr := h.tr.VerifiedOutput(ctx, h.job)
	require.ErrorIs(t, verr, store.ErrNotFound)
	matches, _ := filepath.Glob(filepath.Join(h.tr.jobUploadDir(h.job.ID), "*"))
	require.Empty(t, matches)
}

func TestPutChunkRequiresChecksumOnFinalChunk(t *testing.T) {
	h := newTransferHarness(t)
	payload := strings.Repeat("A", 16)
	_, err := h.tr.PutChunk(context.Background(), h.job, 0, int64(len(payload))-1,
		int64(len(payload)), strings.NewReader(payload), "")
	require.ErrorContains(t, err, "X-Checksum-SHA256")
}

func TestPutChunkRejectsBadRanges(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()
	_, err := h.tr.PutChunk(ctx, h.job, 5, 4, 100, strings.NewReader("x"), "")
	require.Error(t, err, "end before start")
	_, err = h.tr.PutChunk(ctx, h.job, -1, 4, 100, strings.NewReader("x"), "")
	require.Error(t, err, "negative start")
	_, err = h.tr.PutChunk(ctx, h.job, 0, 99, 0, strings.NewReader("x"), "")
	require.Error(t, err, "zero total")
}

func TestPutChunkEnforcesChunkLimit(t *testing.T) {
	h := newTransferHarness(t)
	total := int64(MaxChunkBytes + 2)
	_, err := h.tr.PutChunk(context.Background(), h.job, 0, total-1, total,
		strings.NewReader(strings.Repeat("x", MaxChunkBytes+2)), "")
	require.ErrorContains(t, err, "exceeds")
}

func TestVerifiedOutputMissingIsNotFound(t *testing.T) {
	h := newTransferHarness(t)
	_, _, err := h.tr.VerifiedOutput(context.Background(), h.job)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestTransferDiscardOnUnknownJob(t *testing.T) {
	h := newTransferHarness(t)
	h.tr.Discard("no-such-job") // must not panic or error
}

func TestOutputExtFollowsTargetContainer(t *testing.T) {
	h := newTransferHarness(t)
	ctx := context.Background()

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	target = []byte(strings.Replace(string(target), `"container":"mkv"`, `"container":"mp4"`, 1))
	require.NoError(t, h.st.UpdateLibrary(ctx, &store.Library{ID: h.lib.ID, Name: "Movies",
		Path: h.lib.Path, TargetJSON: string(target), ResultPolicy: store.ResultPolicyReplaceIfSmaller}))

	payload := strings.Repeat("P", 8)
	_, err = h.tr.PutChunk(ctx, h.job, 0, int64(len(payload))-1, int64(len(payload)),
		strings.NewReader(payload), h.checksum([]byte(payload)))
	require.NoError(t, err)
	path, _, err := h.tr.VerifiedOutput(ctx, h.job)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(path, "output.mp4"), path)
}
