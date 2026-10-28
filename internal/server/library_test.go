// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryCreateValidation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	m := h.svc.Libraries()

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)

	// Missing target falls back to the default.
	lib, err := m.Create(ctx, LibraryParams{
		Name: "Movies", Path: h.root, Watch: true,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	assert.Equal(t, flow.CodecHEVC, mustTarget(t, lib.TargetJSON).Video.Codec)

	// Bad target rejected.
	_, err = m.Create(ctx, LibraryParams{Name: "X", Path: h.root, TargetJSON: []byte(`{"video":{"codec":"mpeg5"}}`)})
	assert.ErrorContains(t, err, "codec")

	// Relative path rejected.
	_, err = m.Create(ctx, LibraryParams{Name: "X", Path: "relative/path", TargetJSON: target})
	assert.Error(t, err)

	// Empty name rejected.
	_, err = m.Create(ctx, LibraryParams{Name: "", Path: h.root})
	assert.Error(t, err)

	// Bad cron rejected.
	_, err = m.Create(ctx, LibraryParams{Name: "X", Path: h.root, Schedule: "99 * * * *"})
	assert.Error(t, err)

	// Bad result policy rejected.
	_, err = m.Create(ctx, LibraryParams{Name: "X", Path: h.root, ResultPolicy: "delete_everything"})
	assert.Error(t, err)

	// Good schedule accepted.
	lib2, err := m.Create(ctx, LibraryParams{Name: "Series", Path: h.root, Schedule: "0 3 * * *"})
	require.NoError(t, err)
	assert.Equal(t, "0 3 * * *", lib2.Schedule)
}

// mustTarget parses a stored Target JSON, failing the test on error.
func mustTarget(t *testing.T, raw string) flow.Target {
	t.Helper()
	target, err := flow.ParseTarget([]byte(raw))
	require.NoError(t, err)
	return target
}

func TestLibraryUpdateReevaluates(t *testing.T) {
	h := newHarness(t)
	writeFixture(t, h.root, "hevc.mkv", 2048) // default prober: satisfied
	h.prober.mu.Lock()
	h.prober.byName["h264.mkv"] = h264Needs
	h.prober.byName["other.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "h264.mkv", 2048)  // needs work under hevc target
	writeFixture(t, h.root, "other.mkv", 2048) // needs work under both targets
	lib := h.createLibrary(false)
	h.scan(lib.ID)

	fs := h.files(lib.ID)
	assert.Equal(t, store.FileStatusSatisfied, fs["hevc.mkv"].Status)
	assert.Equal(t, store.FileStatusQueued, fs["h264.mkv"].Status)
	assert.Equal(t, store.FileStatusQueued, fs["other.mkv"].Status)
	require.Len(t, h.jobsOf(fs["h264.mkv"].ID), 1)

	// Cancel other.mkv's job so a later cancel-on-update has something to
	// work with, then flip the target to h264.
	_, err := h.svc.Queue().Cancel(context.Background(), h.jobsOf(fs["other.mkv"].ID)[0].ID)
	require.NoError(t, err)

	target := flow.Default()
	target.Video.Codec = flow.CodecH264
	raw, err := json.Marshal(target)
	require.NoError(t, err)

	updated, err := h.svc.Libraries().Update(context.Background(), lib.ID, LibraryParams{
		Name: "Movies", Path: h.root, TargetJSON: raw,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	assert.Equal(t, flow.CodecH264, mustTarget(t, updated.TargetJSON).Video.Codec)

	fs = h.files(lib.ID)
	// hevc.mkv now needs work (hevc file, h264 target): queued.
	assert.Equal(t, store.FileStatusQueued, fs["hevc.mkv"].Status)
	require.Len(t, h.jobsOf(fs["hevc.mkv"].ID), 1)
	// h264.mkv now satisfies: its queued job was cancelled, file satisfied.
	assert.Equal(t, store.FileStatusSatisfied, fs["h264.mkv"].Status)
	job := h.jobsOf(fs["h264.mkv"].ID)[0]
	assert.Equal(t, store.JobStatusCancelled, job.Status)
	// other.mkv (h264 content) also satisfies under the new target: its
	// job was already cancelled by hand, so no active job remained.
	assert.Equal(t, store.FileStatusSatisfied, fs["other.mkv"].Status)
}

func TestLibraryUpdateCancelsJobsForSatisfiedFiles(t *testing.T) {
	h := newHarness(t)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "movie.mkv", 2048)
	lib := h.createLibrary(false)
	h.scan(lib.ID)

	file := h.files(lib.ID)["movie.mkv"]
	require.Len(t, h.jobsOf(file.ID), 1)

	// Flip to an h264 target: the file (h264 video, keep-all audio and
	// subs, mkv) now satisfies, so its queued job is cancelled.
	target := flow.Default()
	target.Video.Codec = flow.CodecH264
	raw, err := json.Marshal(target)
	require.NoError(t, err)
	_, err = h.svc.Libraries().Update(context.Background(), lib.ID, LibraryParams{
		Name: "Movies", Path: h.root, TargetJSON: raw,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)

	after := h.files(lib.ID)["movie.mkv"]
	assert.Equal(t, store.FileStatusSatisfied, after.Status)
	assert.Equal(t, store.JobStatusCancelled, h.jobsOf(file.ID)[0].Status)
}

func TestLibraryDeleteLeavesFilesOnDisk(t *testing.T) {
	h := newHarness(t)
	p1 := writeFixture(t, h.root, "a.mkv", 2048)
	p2 := writeFixture(t, h.root, "b.mkv", 2048)
	lib := h.createLibrary(false)
	h.scan(lib.ID)
	require.Len(t, h.files(lib.ID), 2)

	require.NoError(t, h.svc.Libraries().Delete(context.Background(), lib.ID))

	// DB records gone...
	_, err := h.st.GetLibrary(context.Background(), lib.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Empty(t, h.files(lib.ID))
	jobs, err := h.st.ListJobs(context.Background(), store.JobFilter{})
	require.NoError(t, err)
	assert.Empty(t, jobs)

	// ...files untouched (acceptance item).
	for _, p := range []string{p1, p2} {
		_, err := os.Stat(p)
		require.NoError(t, err, "library file must survive the delete")
	}
}

func TestLibraryRestartDoesNotDuplicateJobs(t *testing.T) {
	h := newHarness(t)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "movie.mkv", 2048)
	lib := h.createLibrary(false)
	h.scan(lib.ID)

	before := h.jobsOf(h.files(lib.ID)["movie.mkv"].ID)
	require.Len(t, before, 1)

	// Restart the whole server against the same DB and folder, then scan
	// again: the queued job survives and is not duplicated (acceptance
	// item).
	h.restart()
	h.scan(lib.ID)

	after := h.jobsOf(h.files(lib.ID)["movie.mkv"].ID)
	require.Len(t, after, 1)
	assert.Equal(t, before[0].ID, after[0].ID)
	assert.Equal(t, store.JobStatusQueued, after[0].Status)
}

func TestLibraryWatcherFilesIgnoredByPathChange(t *testing.T) {
	h := newHarness(t)
	// A library whose folder is replaced: old files vanish → missing.
	oldDir := filepath.Join(h.root, "old")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	writeFixture(t, oldDir, "movie.mkv", 2048)
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name: "Moves", Path: oldDir, TargetJSON: target,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	h.scan(lib.ID)
	require.Len(t, h.files(lib.ID), 1)

	newDir := filepath.Join(h.root, "new")
	require.NoError(t, os.MkdirAll(newDir, 0o755))
	_, err = h.svc.Libraries().Update(context.Background(), lib.ID, LibraryParams{
		Name: "Moves", Path: newDir, TargetJSON: target,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	h.scan(lib.ID)
	assert.Equal(t, store.FileStatusMissing, h.files(lib.ID)["movie.mkv"].Status)
}

func TestLibraryCreateTriggersInitialScan(t *testing.T) {
	h := newHarness(t)
	// Re-install the real hook: creating a library must scan its folder
	// without any manual trigger.
	h.svc.Libraries().SetScanHook(h.svc.Scanner().ScanAsync)

	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "movie.mkv", 2048)

	lib := h.createLibrary(false)
	waitFor(t, 10*time.Second, "initial scan to queue the file", func() bool {
		for _, f := range h.files(lib.ID) {
			if f.Status == store.FileStatusQueued {
				return true
			}
		}
		return false
	})
}
