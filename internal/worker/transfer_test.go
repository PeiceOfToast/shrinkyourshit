// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// transferSpec builds a transfer-mode spec over a real file whose content
// matches what the fake source endpoint serves.
func transferSpec(t *testing.T, dir, jobID string, data []byte) (*fakeAPI, *server.JobSpec) {
	t.Helper()
	api := newFakeAPI(t, "key")
	api.setSource(jobID, data)
	src := writeSource(t, dir, "movie.mp4", string(data))
	spec := &server.JobSpec{
		ID: jobID, FileID: "file-1", LibraryID: "lib-1",
		Target:       flow.Default(),
		SourcePath:   src.Path,
		SourceSize:   src.Size,
		SourceMtime:  src.Mtime,
		Fingerprint:  src.Fingerprint,
		AccessMode:   store.NodeAccessTransfer,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
		BytesBefore:  src.Size,
	}
	api.enqueue(jobID, spec)
	return api, spec
}

func TestTransferRunnerDownloadsEncodesAndUploads(t *testing.T) {
	dir := t.TempDir()
	data := strings.Repeat("SOURCE-", 20_000) // 140k
	api, spec := transferSpec(t, dir, "job1", []byte(data))

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("SMALLER-OUTPUT", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, store.NodeAccessTransfer, in.AccessMode)
	require.Empty(t, in.OutputPath, "the server applies the result policy from the upload")
	require.Empty(t, in.SkippedReason)
	require.Equal(t, int64(len("SMALLER-OUTPUT")), in.BytesAfter)

	// The upload arrived byte-for-byte.
	up, ok := api.uploadOf("job1")
	require.True(t, ok)
	require.Equal(t, "SMALLER-OUTPUT", string(up))

	// The reported progress named the transfer mode.
	p, ok := api.progressOf("job1")
	require.True(t, ok)
	require.Equal(t, store.NodeAccessTransfer, p.AccessMode)

	// Scratch is clean.
	_, err := os.Stat(filepath.Join(dir, "scratch", "job-job1"))
	require.True(t, os.IsNotExist(err))
}

func TestTransferRunnerSkipsUploadWhenNotSmaller(t *testing.T) {
	dir := t.TempDir()
	data := strings.Repeat("S", 500)
	api, spec := transferSpec(t, dir, "job1", []byte(data))
	// The fake encoder produces something bigger than the source.
	big := strings.Repeat("X", 600)
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun(big, nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, store.NodeAccessTransfer, in.AccessMode)
	require.Contains(t, in.SkippedReason, "not smaller")
	_, uploaded := api.uploadOf("job1")
	require.False(t, uploaded, "a skip must not upload the output")
}

func TestTransferRunnerFailsOnChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	api, spec := transferSpec(t, dir, "job1", []byte(strings.Repeat("S", 100)))
	api.rejectChecksum = true // the final chunk fails verification (422)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "upload result")
	require.Contains(t, rep.Error, "checksum_mismatch")
}

func TestTransferRunnerRestartsUploadOnGap(t *testing.T) {
	dir := t.TempDir()
	api, spec := transferSpec(t, dir, "job1", []byte(strings.Repeat("S", 100)))
	// The 2nd chunk answers 409 once: the client restarts from byte 0.
	api.failUploadNth("job1", 2)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT-DATA", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	in := api.completeOf("job1")
	require.Equal(t, store.NodeAccessTransfer, in.AccessMode)
	up, ok := api.uploadOf("job1")
	require.True(t, ok)
	require.Equal(t, "OUTPUT-DATA", string(up))
}

func TestTransferDownloadResumesAfterDrop(t *testing.T) {
	dir := t.TempDir()
	data := []byte(strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 4000)) // 104k
	api, spec := transferSpec(t, dir, "job1", data)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, store.NodeAccessTransfer, in.AccessMode)
	// The runner verified the fingerprint of the reassembled download
	// before encoding (reaching complete proves it), and the test really
	// exercised the mid-transfer abort.
	require.True(t, api.wasDropped("job1"))
}

func TestTransferSkipsExtractCommands(t *testing.T) {
	dir := t.TempDir()
	api, spec := transferSpec(t, dir, "job1", []byte(strings.Repeat("S", 100)))
	spec.Target.Subtitles.Mode = "remove"
	spec.Target.Subtitles.ExtractRemoved = true

	seen := &[]string{}
	// The output probe must reflect the strip (no subtitles left) or the
	// runner would fail verification before uploading.
	clean := outInfoF()
	clean.Subtitle = nil
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		byBase: map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": clean},
		run:    writeRun("OUTPUT", seen),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	// Only the encode ran: no extract command was executed in transfer
	// mode (there is no extract upload channel yet).
	require.Len(t, *seen, 1)
	in := api.completeOf("job1")
	require.Equal(t, store.NodeAccessTransfer, in.AccessMode)
}

func TestAccessModeAutoSharedWhenSampleMatches(t *testing.T) {
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	spec.AccessMode = store.NodeAccessAuto
	api := newFakeAPI(t, "key")
	// The challenge points at the real source: the node reads it through
	// its (identity) mapping and the sample matches.
	sum := sha256.Sum256([]byte("SOURCE-CONTENT"))
	api.setChallenge("job1", &AccessChallenge{
		Path: src.Path, Size: src.Size, SampleOffset: 0, SampleSHA256: hex.EncodeToString(sum[:]),
	})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.NotEmpty(t, in.OutputPath, "shared mode replaces the file locally")
	p, ok := api.progressOf("job1")
	require.True(t, ok)
	require.Equal(t, store.NodeAccessShared, p.AccessMode)
}

func TestAccessModeAutoTransferWhenSampleDiffers(t *testing.T) {
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	spec.AccessMode = store.NodeAccessAuto
	api := newFakeAPI(t, "key")
	// The challenge describes a file of the same size but different bytes
	// at a path the mapping resolves to (same-size decoy, acceptance 4).
	api.setChallenge("job1", &AccessChallenge{
		Path: spec.SourcePath, Size: spec.SourceSize, SampleOffset: 0,
		SampleSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	// Transfer mode then needs a source to download.
	api.setSource("job1", []byte("SOURCE-CONTENT"))
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	p, ok := api.progressOf("job1")
	require.True(t, ok)
	require.Equal(t, store.NodeAccessTransfer, p.AccessMode)
	_, uploaded := api.uploadOf("job1")
	require.True(t, uploaded, "the job ran in transfer mode")
}

func TestAccessCacheReusesTheDecisionPerLibrary(t *testing.T) {
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	spec.AccessMode = store.NodeAccessAuto
	api := newFakeAPI(t, "key")
	sum := sha256.Sum256([]byte("SOURCE-CONTENT"))
	api.setChallenge("job1", &AccessChallenge{
		Path: src.Path, Size: src.Size, SampleOffset: 0, SampleSHA256: hex.EncodeToString(sum[:]),
	})

	cache := newAccessCache()
	client := NewClient(api.srv.URL, api.key)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	mode, err := resolveAccessMode(context.Background(), client, spec, cache, log)
	require.NoError(t, err)
	require.Equal(t, store.NodeAccessShared, mode)

	// A second job of the same library resolves the same way even though
	// the fake has no challenge for it (the endpoint is never called).
	spec2 := *spec
	spec2.ID = "job2"
	mode, err = resolveAccessMode(context.Background(), client, &spec2, cache, log)
	require.NoError(t, err)
	require.Equal(t, store.NodeAccessShared, mode)
}

func TestAccessCacheExpires(t *testing.T) {
	cache := newAccessCache()
	cache.put("lib1", store.NodeAccessShared)
	mode, ok := cache.get("lib1")
	require.True(t, ok)
	require.Equal(t, store.NodeAccessShared, mode)

	// Force expiry.
	cache.mu.Lock()
	cache.byLib["lib1"] = cacheEntry{mode: store.NodeAccessShared, at: time.Now().Add(-AccessCacheTTL)}
	cache.mu.Unlock()
	_, ok = cache.get("lib1")
	require.False(t, ok)
}

func TestPacedReaderLimitsThroughput(t *testing.T) {
	// 250 KB at 100 KB/s must take at least ~1.5s (upper bound generous
	// for slow CI machines).
	data := strings.Repeat("x", 250_000)
	start := time.Now()
	got, err := io.ReadAll(newPacedReader(strings.NewReader(data), 100_000, context.Background()))
	require.NoError(t, err)
	elapsed := time.Since(start)
	require.Equal(t, data, string(got))
	require.Greater(t, elapsed, 1500*time.Millisecond, "the paced reader must slow the stream down")

	// Unlimited passes through instantly.
	start = time.Now()
	_, err = io.ReadAll(newPacedReader(strings.NewReader(data), 0, context.Background()))
	require.NoError(t, err)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestPacedReaderStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data := strings.Repeat("x", 1_000_000)
	r := newPacedReader(strings.NewReader(data), 50_000, ctx)
	buf := make([]byte, 8192)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	var total int64
	for {
		n, err := r.Read(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Less(t, total, int64(len(data)), "the cancel must have stopped the paced read early")
}

func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))
	sum, err := fileSHA256(path)
	require.NoError(t, err)
	want := sha256.Sum256([]byte("hello"))
	require.Equal(t, hex.EncodeToString(want[:]), sum)
}
