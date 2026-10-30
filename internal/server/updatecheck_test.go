// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUpdateService builds an update-check service over a throwaway store,
// with the overrides a test asks for.
func newUpdateService(t *testing.T, mutate func(*UpdateOptions)) (*UpdateCheck, store.Store) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	opts := UpdateOptions{
		DataDir:    t.TempDir(),
		ScratchDir: t.TempDir(),
		Current:    update.Info{Version: "v1.2.0", Commit: "abc", Date: "2026-01-02"},
	}
	if mutate != nil {
		mutate(&opts)
	}
	return NewUpdateCheck(NewEventBus(nil), NewSettings(st, nil), st, opts, nil), st
}

// TestUpdateSnapshotBeforeCheck pins the never-blocking contract: before
// any check the status answers immediately with the current version and
// the not-checked reason, and self update support follows the runtime.
func TestUpdateSnapshotBeforeCheck(t *testing.T) {
	svc, _ := newUpdateService(t, nil)
	status := svc.Snapshot()
	assert.Equal(t, "v1.2.0", status.Current)
	assert.False(t, status.UpdateAvailable)
	assert.Equal(t, "not checked yet", status.Reason)
	assert.Empty(t, status.Latest)
	assert.True(t, status.SelfUpdateSupported, "a plain binary install can self update")
	assert.Nil(t, status.Progress)
}

// TestUpdateSupportReasons pins every "cannot self update" answer: the
// container reason names the fix, a dev build explains itself, and a
// read-only install folder is refused instead of failing mid-apply.
func TestUpdateSupportReasons(t *testing.T) {
	t.Run("container", func(t *testing.T) {
		svc, _ := newUpdateService(t, func(o *UpdateOptions) {
			o.IsContainer = func() bool { return true }
		})
		status := svc.Snapshot()
		assert.Equal(t, update.RuntimeContainer, svc.Runtime())
		assert.False(t, status.SelfUpdateSupported)
		assert.Contains(t, status.Reason, "docker compose pull")
		assert.Contains(t, status.Reason, "docker compose up -d")
		assert.Contains(t, status.Reason, "ghcr.io/peiceoftoast/shrinkyourshit")
	})

	t.Run("dev build", func(t *testing.T) {
		svc, _ := newUpdateService(t, func(o *UpdateOptions) {
			o.Current.Version = "v0.0.0"
		})
		status := svc.Snapshot()
		assert.False(t, status.SelfUpdateSupported)
		assert.Contains(t, status.Reason, "release information")
	})

	t.Run("read-only install folder", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "shrinkyourshit")
		require.NoError(t, os.WriteFile(exe, []byte("bin"), 0o755))
		require.NoError(t, os.Chmod(dir, 0o555)) // read-only: no staging file possible
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		svc, _ := newUpdateService(t, func(o *UpdateOptions) {
			o.Executable = func() (string, error) { return exe, nil }
		})
		status := svc.Snapshot()
		assert.False(t, status.SelfUpdateSupported)
		assert.Contains(t, status.Reason, "not writable")
	})
}

// updateHub serves a single fixture release, enough for check-window
// tests.
func updateHub(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/PeiceOfToast/shrinkyourshit/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		writeRelease(t, w, tag)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeRelease(t *testing.T, w http.ResponseWriter, tag string) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"tag_name":     tag,
		"html_url":     "https://example.com/releases/tag/" + tag,
		"published_at": "2026-10-30T00:00:00Z",
	}))
}

// TestUpdateCheckWindow pins the once-per-minute window on the service:
// the first refresh goes out, a refresh inside the window is refused with
// ErrUpdateRateLimited, and one past the window goes out again.
func TestUpdateCheckWindow(t *testing.T) {
	hub := updateHub(t, "v1.2.3")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	svc, _ := newUpdateService(t, func(o *UpdateOptions) {
		o.APIBase = hub.URL
		o.CheckWindow = time.Minute
		o.Now = func() time.Time { return now }
	})
	ctx := context.Background()

	_, err := svc.Check(ctx)
	require.NoError(t, err)

	// Inside the window: refused.
	_, err = svc.Check(ctx)
	require.ErrorIs(t, err, ErrUpdateRateLimited)

	// Past the window: goes out again.
	now = now.Add(time.Minute + time.Second)
	_, err = svc.Check(ctx)
	require.NoError(t, err)

	status := svc.Snapshot()
	assert.Equal(t, "v1.2.3", status.Latest)
	assert.True(t, status.UpdateAvailable)
}

// TestUpdateCheckFailureIsQuiet pins that a failed network check is not an
// error to the caller: it lands in the status reason (the About card's
// quiet message) and the next check may try again.
func TestUpdateCheckFailureIsQuiet(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	svc, _ := newUpdateService(t, func(o *UpdateOptions) {
		o.APIBase = "http://127.0.0.1:1" // nothing listens there
		o.Now = func() time.Time { return now }
	})
	ctx := context.Background()

	status, err := svc.Check(ctx)
	require.NoError(t, err)
	assert.False(t, status.UpdateAvailable)
	assert.Contains(t, status.Reason, "Could not check for updates")

	// The snapshot keeps the quiet failure.
	snap := svc.Snapshot()
	assert.Contains(t, snap.Reason, "Could not check for updates")

	// A later check that succeeds replaces the failure with the release.
	now = now.Add(2 * time.Minute)
	svc.opts.APIBase = updateHub(t, "v1.2.3").URL
	status, err = svc.Check(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", status.Latest)
	assert.True(t, status.UpdateAvailable)
	assert.Empty(t, status.Reason)
}
