// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIntegrationHarness builds a harness with real ffprobe and the real
// ffmpeg fixture set.
func newIntegrationHarness(t *testing.T) (*harness, testutil.Fixtures) {
	t.Helper()
	ffmpeg, _ := testutil.RequireFFmpeg(t)
	h := &harness{
		t:          t,
		dbPath:     filepath.Join(t.TempDir(), "data.db"),
		root:       filepath.Join(t.TempDir(), "library"),
		realProber: true,
	}
	require.NoError(t, os.MkdirAll(h.root, 0o755))
	h.construct(nil) // realProber → real ffprobe
	t.Cleanup(h.stop)
	fixtures := testutil.GenerateFixtures(t, ffmpeg, t.TempDir())
	return h, fixtures
}

// TestIntegrationScanQueuesExactlyWhatNeedsWork covers acceptance item 1:
// three fixture files (1 needs work, 2 satisfied) queue exactly 1 job and
// the satisfied filter returns 2.
func TestIntegrationScanQueuesExactlyWhatNeedsWork(t *testing.T) {
	h, fx := newIntegrationHarness(t)

	// Satisfied: hevc + aac(eng) + srt(eng) in mkv — and a copy of it as
	// the second satisfied file.
	satisfiedCopy := filepath.Join(t.TempDir(), "satisfied-copy.mkv")
	data, err := os.ReadFile(fx.HEVCAudioSubtitleMKV)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(satisfiedCopy, data, 0o600))
	// Both copies must sit inside the library folder.
	lib := h.createLibrary(false)
	dst := filepath.Join(h.root, "satisfied-copy.mkv")
	require.NoError(t, os.Rename(satisfiedCopy, dst))
	// Move the generated fixtures in as well.
	require.NoError(t, os.Rename(fx.HEVCAudioSubtitleMKV, filepath.Join(h.root, "satisfied.mkv")))
	require.NoError(t, os.Rename(fx.H264TwoAudioSubtitleMKV, filepath.Join(h.root, "needs-work.mkv")))

	res := h.scan(lib.ID)
	assert.Equal(t, 3, res.Seen)
	assert.Equal(t, 1, res.Queued, "exactly one file needs work")

	files := h.files(lib.ID)
	assert.Equal(t, store.FileStatusQueued, files["needs-work.mkv"].Status)
	assert.Equal(t, store.FileStatusSatisfied, files["satisfied.mkv"].Status)
	assert.Equal(t, store.FileStatusSatisfied, files["satisfied-copy.mkv"].Status)

	// The satisfied filter via the store mirrors the API filter contract.
	sat, err := h.st.ListFilesByStatus(context.Background(), lib.ID, store.FileStatusSatisfied)
	require.NoError(t, err)
	assert.Len(t, sat, 2)
}

// TestIntegrationWatchedSlowCopyReal covers acceptance item 2 with the real
// fsnotify + ffprobe path: a file copied slowly queues a job only after the
// copy completes.
func TestIntegrationWatchedSlowCopyReal(t *testing.T) {
	h, fx := newIntegrationHarness(t)
	lib := h.createLibrary(true)

	// A pause between chunks must never reach the stability window, or a
	// part-written file could be probed: 250ms gaps vs a 1s window.
	h.svc.scanner.stable = 1 * time.Second

	src, err := os.ReadFile(fx.H264TwoAudioSubtitleMKV)
	require.NoError(t, err)
	dst := filepath.Join(h.root, "slow.mkv")

	// Copy in chunks over ~2.75s while the watcher fires.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f, err := os.Create(dst)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = f.Close() }()
		const chunk = 4096
		for off := 0; off < len(src); off += chunk {
			end := min(off+chunk, len(src))
			if _, err := f.Write(src[off:end]); err != nil {
				t.Error(err)
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
	}()

	// No job may appear while the copy is in flight (at 1.5s the copy is
	// roughly halfway).
	time.Sleep(1500 * time.Millisecond)
	for _, f := range h.files(lib.ID) {
		assert.NotEqual(t, store.FileStatusQueued, f.Status, "no job while copying")
	}

	wg.Wait()

	// Debounce (200ms) + stability (1s) + probe + slack: the job appears
	// only once, for the complete file.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the queued job after copy completion")
		}
		files := h.files(lib.ID)
		if f := files["slow.mkv"]; f != nil && f.Status == store.FileStatusQueued && f.Size == int64(len(src)) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Exactly one job for the file.
	file := h.files(lib.ID)["slow.mkv"]
	jobs := h.jobsOf(file.ID)
	require.Len(t, jobs, 1)
	assert.Equal(t, int64(len(src)), file.Size)
}

// TestIntegrationDeleteLibraryKeepsFiles covers acceptance item 6 with real
// fixture files on disk.
func TestIntegrationDeleteLibraryKeepsFiles(t *testing.T) {
	h, fx := newIntegrationHarness(t)
	require.NoError(t, os.Rename(fx.HEVCAudioSubtitleMKV, filepath.Join(h.root, "a.mkv")))
	require.NoError(t, os.Rename(fx.H264AudioMP4, filepath.Join(h.root, "b.mp4")))
	lib := h.createLibrary(false)
	h.scan(lib.ID)
	require.Len(t, h.files(lib.ID), 2)

	require.NoError(t, h.svc.Libraries().Delete(context.Background(), lib.ID))
	assert.Empty(t, h.files(lib.ID))

	// The fixture files still exist.
	for _, name := range []string{"a.mkv", "b.mp4"} {
		_, err := os.Stat(filepath.Join(h.root, name))
		require.NoError(t, err, name+" must survive the library delete")
	}
}
