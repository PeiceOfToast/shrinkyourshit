// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFixture writes a fake media file (content irrelevant: the fake
// prober supplies the probe).
func writeFixture(t *testing.T, dir, name string, size int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestScannerClassifiesAndQueues(t *testing.T) {
	h := newHarness(t)
	h.prober.mu.Lock()
	h.prober.byName["needs.mkv"] = h264Needs
	h.prober.mu.Unlock()

	writeFixture(t, h.root, "needs.mkv", 4096)     // h264Needs by name override
	writeFixture(t, h.root, "satisfied.mkv", 4096) // default (hevc satisfied)
	writeFixture(t, h.root, "notes.txt", 64)       // non-media
	writeFixture(t, h.root, ".hidden.mkv", 1024)   // hidden
	require.NoError(t, os.MkdirAll(filepath.Join(h.root, ".AppleDouble"), 0o755))
	writeFixture(t, filepath.Join(h.root, ".AppleDouble"), "junk.mp4", 1024)

	lib := h.createLibrary(false)
	res := h.scan(lib.ID)
	assert.Equal(t, 2, res.Seen, "only non-hidden media files")
	assert.Equal(t, 2, res.Probed)
	assert.Equal(t, 1, res.Queued)
	assert.Equal(t, 1, res.Satisfied)

	fs := h.files(lib.ID)
	require.Len(t, fs, 2)

	needs := fs["needs.mkv"]
	assert.Equal(t, store.FileStatusQueued, needs.Status)
	assert.Contains(t, needs.SatisfiedReason, "video is h264, target is hevc")
	jobs := h.jobsOf(needs.ID)
	require.Len(t, jobs, 1)
	assert.Equal(t, store.JobStatusQueued, jobs[0].Status)
	assert.Equal(t, needs.Size, jobs[0].BytesBefore)

	sat := fs["satisfied.mkv"]
	assert.Equal(t, store.FileStatusSatisfied, sat.Status)
	assert.Empty(t, sat.SatisfiedReason)
	assert.Empty(t, h.jobsOf(sat.ID))
}

func TestScannerFingerprintShortCircuitsProbe(t *testing.T) {
	h := newHarness(t)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	path := writeFixture(t, h.root, "movie.mkv", 2048)
	lib := h.createLibrary(false)

	h.scan(lib.ID)
	require.Equal(t, 1, h.prober.callsOf(path))

	// Unchanged file: no new probe, no duplicate job.
	h.scan(lib.ID)
	assert.Equal(t, 1, h.prober.callsOf(path), "fingerprint unchanged, no re-probe")
	fs := h.files(lib.ID)
	require.Len(t, h.jobsOf(fs["movie.mkv"].ID), 1)

	// Changed file (size): probe again, still one job (the old one is
	// still active, so no duplicate is created).
	require.NoError(t, os.WriteFile(path, []byte("changed content"), 0o600))
	h.scan(lib.ID)
	assert.Equal(t, 2, h.prober.callsOf(path))
	assert.Len(t, h.jobsOf(fs["movie.mkv"].ID), 1)
}

func TestScannerCancelledFileReevaluatesFromCache(t *testing.T) {
	h := newHarness(t)
	path := writeFixture(t, h.root, "movie.mkv", 2048)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	lib := h.createLibrary(false)

	h.scan(lib.ID)
	file := h.files(lib.ID)["movie.mkv"]
	require.Len(t, h.jobsOf(file.ID), 1)
	job := h.jobsOf(file.ID)[0]

	// Cancel the job; the file goes back to new with its probe cached.
	_, err := h.svc.Queue().Cancel(context.Background(), job.ID)
	require.NoError(t, err)
	file, err = h.st.GetFile(context.Background(), file.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FileStatusNew, file.Status)

	// Next scan: fingerprint unchanged, cached probe re-evaluated, job
	// re-queued, and the prober never called again for it.
	h.scan(lib.ID)
	assert.Equal(t, 1, h.prober.callsOf(path))
	fs := h.files(lib.ID)
	assert.Equal(t, store.FileStatusQueued, fs["movie.mkv"].Status)
	assert.Len(t, h.jobsOf(fs["movie.mkv"].ID), 2, "a fresh job after cancellation")
}

func TestScannerMarksMissing(t *testing.T) {
	h := newHarness(t)
	writeFixture(t, h.root, "kept.mkv", 2048)
	gone := writeFixture(t, h.root, "gone.mkv", 2048)
	lib := h.createLibrary(false)
	h.scan(lib.ID)

	require.NoError(t, os.Remove(gone))
	h.scan(lib.ID)

	fs := h.files(lib.ID)
	assert.Equal(t, store.FileStatusMissing, fs["gone.mkv"].Status)
	assert.Equal(t, store.FileStatusSatisfied, fs["kept.mkv"].Status)
}

func TestScannerProbeFailureIsTolerated(t *testing.T) {
	h := newHarness(t)
	writeFixture(t, h.root, "broken.mkv", 2048)
	lib := h.createLibrary(false)
	h.svc.scanner.prober = func(context.Context, string, string) (probe.MediaInfo, error) {
		return probe.MediaInfo{}, errors.New("boom")
	}

	res := h.scan(lib.ID)
	assert.Equal(t, 1, res.Seen)
	assert.Equal(t, 0, res.Queued)
	assert.Equal(t, 1, res.Errors, "probe failures surface in the scan summary")
	assert.Empty(t, h.files(lib.ID), "failed probes leave no row behind")
}

func TestScannerNewTargetQueuesFromCacheOnScan(t *testing.T) {
	h := newHarness(t)
	writeFixture(t, h.root, "movie.mkv", 2048)
	lib := h.createLibrary(false)
	h.scan(lib.ID)

	fs := h.files(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, fs["movie.mkv"].Status)

	// Swap the Target to h264 directly in the store (the manager path is
	// tested separately): the scan must notice via status==new... it does
	// not — satisfied files with unchanged fingerprints stay put. The
	// update-triggered re-evaluation is the mechanism (library_test).
	target := flow.Default()
	target.Video.Codec = flow.CodecH264
	raw, err := json.Marshal(target)
	require.NoError(t, err)
	lib.TargetJSON = string(raw)
	require.NoError(t, h.st.UpdateLibrary(context.Background(), lib))

	// Force the file back to new to simulate a fresh classification need.
	f := fs["movie.mkv"]
	f.Status = store.FileStatusNew
	require.NoError(t, h.st.UpdateFile(context.Background(), f))

	res := h.scan(lib.ID)
	assert.Equal(t, 1, res.Reused, "re-evaluated from cached probe")
	assert.Equal(t, 0, res.Probed, "no ffprobe call")
	assert.Equal(t, 1, res.Queued)
}

func TestScannerConcurrencyLimit(t *testing.T) {
	h := newHarness(t)
	h.svc.scanner.procs = make(chan struct{}, 1) // force serialization
	lib := h.createLibrary(false)
	for i := range 6 {
		writeFixture(t, h.root, fmt.Sprintf("movie%d.mkv", i), 1024)
	}
	res := h.scan(lib.ID)
	assert.Equal(t, 6, res.Seen)
	assert.Equal(t, 6, res.Probed)
}
