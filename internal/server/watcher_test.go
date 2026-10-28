// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFor polls until the condition holds or the timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWatcherSlowCopyQueuesOnlyAfterCompletion(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(true) // watch=true

	// A file that needs work once fully written.
	h.prober.mu.Lock()
	h.prober.byName["slow.mkv"] = h264Needs
	h.prober.mu.Unlock()

	// Copy "slowly": grow the file in chunks over ~1.2s while the watcher
	// fires. The job must appear only after the last chunk (acceptance
	// item, unit-level with a fake prober).
	final := make([]byte, 8192)
	for i := range final {
		final[i] = byte(i % 251)
	}
	path := filepath.Join(h.root, "slow.mkv")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f, err := os.Create(path)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = f.Close() }()
		for off := 0; off < len(final); off += 1024 {
			if _, err := f.Write(final[off:min(off+1024, len(final))]); err != nil {
				t.Error(err)
				return
			}
			time.Sleep(150 * time.Millisecond) // 8 chunks ≈ 1.2s
		}
	}()

	// While the copy runs, no job may be queued: writes keep resetting the
	// debounce, and stability (mtime age) is never reached mid-copy.
	time.Sleep(700 * time.Millisecond)
	queuedCount := func() int {
		n := 0
		for _, f := range h.files(lib.ID) {
			if f.Status == store.FileStatusQueued {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 0, queuedCount(), "no job while the file is still copying")

	wg.Wait()

	// Debounce (200ms) + stability (300ms) + slack: the job must appear,
	// for the complete file and exactly once.
	waitFor(t, 10*time.Second, "queued job after copy completed", func() bool {
		for _, f := range h.files(lib.ID) {
			if f.Status == store.FileStatusQueued && f.Size == int64(len(final)) {
				return true
			}
		}
		return false
	})
	fs := h.files(lib.ID)
	require.Len(t, fs, 1)
	assert.Equal(t, store.FileStatusQueued, fs["slow.mkv"].Status)
	require.Len(t, h.jobsOf(fs["slow.mkv"].ID), 1)
}

func TestWatcherNonMediaFilesDoNotQueue(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(true)
	writeFixture(t, h.root, "base.mkv", 1024) // satisfied via default prober
	h.scan(lib.ID)
	require.Len(t, h.files(lib.ID), 1)

	// A non-media file dropped into the folder must never produce work.
	require.NoError(t, os.WriteFile(filepath.Join(h.root, "poster.jpg"), []byte("x"), 0o600))
	time.Sleep(800 * time.Millisecond) // debounce + stability + slack

	for _, f := range h.files(lib.ID) {
		assert.NotEqual(t, store.FileStatusQueued, f.Status)
	}
	assert.Len(t, h.files(lib.ID), 1, "non-media files stay out of the library")
}

func TestWatcherNewSubdirectoryIsWatched(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(true)

	// A subdirectory created after the watch started: its files are picked
	// up because the watcher adds new directories recursively.
	sub := filepath.Join(h.root, "season1")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	h.prober.mu.Lock()
	h.prober.byName["ep.mkv"] = h264Needs
	h.prober.mu.Unlock()
	require.NoError(t, os.WriteFile(filepath.Join(sub, "ep.mkv"), make([]byte, 1024), 0o600))

	waitFor(t, 10*time.Second, "file in new subdirectory queued", func() bool {
		for _, f := range h.files(lib.ID) {
			if f.Status == store.FileStatusQueued {
				return true
			}
		}
		return false
	})
}

func TestWatcherPollFallbackWhenPathUnwatchable(t *testing.T) {
	h := newHarness(t)
	// A path that exists but cannot carry an inotify watch on Linux.
	unwatchable := "/proc/self"
	if _, err := os.Stat(unwatchable); err != nil {
		t.Skip("no /proc/self to test the poll fallback")
	}
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name: "Proc", Path: unwatchable, TargetJSON: target, Watch: true,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)

	// The watcher falls back to polling; with a 1h poll interval it never
	// fires during the test — assert the watch registered without dying.
	time.Sleep(100 * time.Millisecond)
	assert.True(t, h.svc.watcher.Watching(lib.ID), "poll-fallback watch stays registered")
}

func TestWatcherRemoveStopsWatching(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(true)
	require.True(t, h.svc.watcher.Watching(lib.ID))

	h.svc.Libraries().Delete(context.Background(), lib.ID)
	assert.False(t, h.svc.watcher.Watching(lib.ID), "delete removes the watch")
}
