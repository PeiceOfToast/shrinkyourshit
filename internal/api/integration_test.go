// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestIntegrationScanViaAPI covers acceptance item 1 end to end over HTTP:
// three fixture files (1 needs work, 2 satisfied) queue exactly 1 job and
// GET /libraries/{id}/files?status=satisfied returns 2.
func TestIntegrationScanViaAPI(t *testing.T) {
	ffmpeg, _ := testutil.RequireFFmpeg(t)
	h := newAPIHarnessReal(t)
	fixtures := testutil.GenerateFixtures(t, ffmpeg, t.TempDir())

	// Satisfied: the hevc fixture, plus a copy of it.
	copyPath := filepath.Join(h.root, "satisfied-copy.mkv")
	data, err := os.ReadFile(fixtures.HEVCAudioSubtitleMKV)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(copyPath, data, 0o600))
	require.NoError(t, os.Rename(fixtures.HEVCAudioSubtitleMKV, filepath.Join(h.root, "satisfied.mkv")))
	// Needs work: h264 + two audio languages + subtitle in mkv.
	require.NoError(t, os.Rename(fixtures.H264TwoAudioSubtitleMKV, filepath.Join(h.root, "needs-work.mkv")))

	// Create the library and scan via the API.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root,
		"target": flow.Default(), "watch": false,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))

	rec = h.do(http.MethodPost, "/api/v1/libraries/"+lib.ID+"/scan", nil)
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Wait for the async scan to settle.
	deadline := time.Now().Add(20 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("scan did not settle in time")
		}
		rec = h.do(http.MethodGet, "/api/v1/jobs", nil)
		var jobs struct {
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
		if jobs.Total == 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// The satisfied filter returns exactly the two satisfied files.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/files?status=satisfied", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page struct {
		Items []server.FileDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Items, 2)

	// And the jobs list shows the one queued job.
	rec = h.do(http.MethodGet, "/api/v1/jobs?status="+store.JobStatusQueued, nil)
	var jobs struct {
		Items []server.JobDTO `json:"items"`
		Total int             `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
	require.Equal(t, 1, jobs.Total)
}

// newAPIHarnessReal is the API harness with real ffprobe (no fake).
func newAPIHarnessReal(t *testing.T) *apiHarness {
	t.Helper()
	h := &apiHarness{t: t, root: filepath.Join(t.TempDir(), "library")}
	require.NoError(t, os.MkdirAll(h.root, 0o755))

	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	h.st = st

	h.svc = server.New(server.Config{
		Store:             st,
		StableWindow:      300 * time.Millisecond,
		Debounce:          200 * time.Millisecond,
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
	})
	h.svc.Libraries().SetScanHook(func(string) {})
	h.svc.Start(context.Background())
	t.Cleanup(h.svc.Stop)
	h.mux = New("v0.0.0", testStatic(), h.svc)
	return h
}
