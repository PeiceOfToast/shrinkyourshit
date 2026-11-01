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

	// A finished scan stamps the library's last scan time (task 19).
	fresh, err := h.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	assert.False(t, fresh.LastScanAt.IsZero(), "the scan stamps last_scan_at")

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

	// Swap the rule to h264 directly in the store (the manager path is
	// tested separately): since task 14 the flow is the rule, so the test
	// stores the matching flow graph beside the target. The scan must
	// notice via status==new... it does not — satisfied files with
	// unchanged fingerprints stay put. The update-triggered re-evaluation
	// is the mechanism (library_test).
	target := flow.Default()
	target.Video.Codec = flow.CodecH264
	graph, err := flow.TargetToFlow(target)
	require.NoError(t, err)
	raw, err := json.Marshal(graph)
	require.NoError(t, err)
	rawTarget, err := json.Marshal(target)
	require.NoError(t, err)
	lib.TargetJSON = string(rawTarget)
	lib.FlowJSON = string(raw)
	lib.FlowSource = flow.FlowSourceAdvanced
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

// TestScannerImageLibrarySkipsOtherKinds (task 34): a library that looks
// for images scans a folder of images without probing anything, satisfies
// them with the not-media reason, and counts files of other kinds as
// skipped — not queued, not failed, and no row for a never-seen file.
func TestScannerImageLibrarySkipsOtherKinds(t *testing.T) {
	h := newHarness(t)
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name: "Photos", Path: h.root, TargetJSON: target, FileKinds: []string{"image"},
	})
	require.NoError(t, err)

	writeFixture(t, h.root, "photo.jpg", 512)
	writeFixture(t, h.root, "pic.png", 512)
	writeFixture(t, h.root, "movie.mkv", 512) // not wanted by this library
	writeFixture(t, h.root, "notes.txt", 64)  // not wanted either

	res := h.scan(lib.ID)
	assert.Equal(t, 2, res.Seen, "only the images the library looks for")
	assert.Equal(t, 2, res.SkippedKinds, "the video and the text file are counted separately")
	assert.Equal(t, 0, res.Probed, "nothing probes a non-media file")
	assert.Equal(t, 0, res.Queued, "a default video flow queues nothing on images")
	assert.Equal(t, 2, res.Satisfied)
	assert.Equal(t, 0, res.Errors, "a not-wanted kind is never a failure")

	fs := h.files(lib.ID)
	require.Len(t, fs, 2)
	for _, name := range []string{"photo.jpg", "pic.png"} {
		assert.Equal(t, store.FileStatusSatisfied, fs[name].Status)
		assert.Equal(t, flow.NotMediaReason, fs[name].SatisfiedReason)
	}
}

// TestScannerKindSkippedRowRecorded (task 34): a file that was scanned
// while the library wanted its kind keeps an honest row when the library's
// kinds change — satisfied with the reason instead of marked missing, since
// the file is still on disk.
func TestScannerKindSkippedRowRecorded(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(false)
	path := writeFixture(t, h.root, "movie.mkv", 2048)
	h.scan(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, h.files(lib.ID)["movie.mkv"].Status)

	// The library stops looking for video files.
	updated, err := h.svc.Libraries().Update(context.Background(), lib.ID, LibraryParams{
		Name: lib.Name, Path: lib.Path, TargetJSON: []byte(lib.TargetJSON),
		ResultPolicy: lib.ResultPolicy, FileKinds: []string{"image"},
	})
	require.NoError(t, err)
	h.scan(updated.ID)

	f := h.files(lib.ID)["movie.mkv"]
	assert.Equal(t, store.FileStatusSatisfied, f.Status, "the file is on disk, not missing")
	assert.Equal(t, kindSkippedReason, f.SatisfiedReason)
	_ = path
}

// TestScannerNonMediaKindQueuesKindAgnosticWork (task 34): a rename+move
// flow queues images; a media-only flow does not. The flow change arrives
// through UpdateFlow, the path that re-evaluates every file.
func TestScannerNonMediaKindQueuesKindAgnosticWork(t *testing.T) {
	h := newHarness(t)
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name: "Photos", Path: h.root, TargetJSON: target, FileKinds: []string{"image"},
	})
	require.NoError(t, err)
	writeFixture(t, h.root, "photo.jpg", 512)

	res := h.scan(lib.ID)
	assert.Equal(t, 0, res.Queued, "a media-only flow queues nothing on images")

	// Attach rename + move; the image now has kind-agnostic work to do.
	dest := filepath.Join(t.TempDir(), "out")
	g := flow.Graph{Schema: flow.FlowSchema, Nodes: []flow.Step{
		{ID: "probe", Type: flow.StepProbe},
		{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		{ID: "rename", Type: flow.StepRename, Params: json.RawMessage(`{"template":"{name}.moved"}`)},
		{ID: "move", Type: flow.StepMove, Params: json.RawMessage(`{"destination":"` + dest + `"}`)},
	}, Edges: []flow.Edge{
		{From: "probe", To: "remux"}, {From: "remux", To: "rename"}, {From: "rename", To: "move"},
	}}
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	graph, err := flow.ParseGraph(raw)
	require.NoError(t, err)
	_, err = h.svc.Libraries().UpdateFlow(context.Background(), lib.ID, flow.FlowSourceAdvanced, graph)
	require.NoError(t, err)

	fs := h.files(lib.ID)
	assert.Equal(t, store.FileStatusQueued, fs["photo.jpg"].Status)
	assert.Contains(t, fs["photo.jpg"].SatisfiedReason, "the flow renames the result")

	// A rescan sees an unchanged fingerprint and leaves the queued job be.
	res = h.scan(lib.ID)
	assert.Equal(t, 0, res.Queued)
	assert.Equal(t, 0, res.Satisfied)
}
