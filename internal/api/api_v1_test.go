// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// apiFakeProber mirrors the server test fake: canned probes by base name.
type apiFakeProber struct {
	mu       sync.Mutex
	byName   map[string]probe.MediaInfo
	fallback probe.MediaInfo
}

func (p *apiFakeProber) Probe(_ context.Context, _ string, path string) (probe.MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	info, ok := p.byName[filepath.Base(path)]
	if !ok {
		info = p.fallback
	}
	return info, nil
}

var apiSatisfied = probe.MediaInfo{
	Container: flow.ContainerMKV,
	Video:     []probe.VideoStream{{Codec: flow.CodecHEVC}},
	Audio:     []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
}

var apiNeeds = probe.MediaInfo{
	Container: flow.ContainerMKV,
	Video:     []probe.VideoStream{{Codec: flow.CodecH264}},
	Audio:     []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
}

// apiHarness is a full stack (store + server services + API handler) for
// endpoint tests. It does NOT start background scanning: the library
// create-scan hook is disabled and tests scan via the API.
type apiHarness struct {
	t      *testing.T
	svc    *server.Server
	st     store.Store
	stated *apiFakeProber
	root   string
	mux    http.Handler
}

func newAPIHarness(t *testing.T) *apiHarness {
	t.Helper()
	h := &apiHarness{
		t:      t,
		stated: &apiFakeProber{byName: map[string]probe.MediaInfo{}, fallback: apiSatisfied},
		root:   filepath.Join(t.TempDir(), "library"),
	}
	require.NoError(t, os.MkdirAll(h.root, 0o755))

	st, err := store.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	h.st = st

	h.svc = server.New(server.Config{
		Store:             st,
		Prober:            h.stated.Probe,
		Debounce:          200 * time.Millisecond,
		StableWindow:      300 * time.Millisecond,
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
		ScratchDir:        filepath.Join(t.TempDir(), "scratch"),
	})
	h.svc.Libraries().SetScanHook(func(string) {})
	h.svc.Start(context.Background())
	t.Cleanup(h.svc.Stop)

	h.mux = New("v0.0.0", testStatic(), h.svc)
	return h
}

// do performs a request against the handler and returns the recorder.
func (h *apiHarness) do(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var req *http.Request
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// createLibrary POSTs a library and returns its DTO.
func (h *apiHarness) createLibrary(watch bool) server.LibraryDTO {
	h.t.Helper()
	target := flow.Default()
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root, "watch": watch,
		"target": target, "result_policy": store.ResultPolicyReplaceIfSmaller,
	})
	require.Equal(h.t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &dto))
	return dto
}

// scan calls the scan endpoint and waits for it to finish by polling the
// job count... actually scans are synchronous in ScanLibrary; the endpoint
// is async. Tests that need settled state scan directly through the server.
func (h *apiHarness) scanSync(libID string) {
	h.t.Helper()
	_, err := h.svc.Scanner().ScanLibrary(context.Background(), libID)
	require.NoError(h.t, err)
}

// writeFixture writes a fake media file.
func (h *apiHarness) writeFixture(name string, size int) string {
	h.t.Helper()
	path := filepath.Join(h.root, name)
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	require.NoError(h.t, os.WriteFile(path, data, 0o600))
	return path
}

func TestLibraryEndpointsCRUD(t *testing.T) {
	h := newAPIHarness(t)
	target := flow.Default()

	// Create.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root, "watch": true, "target": target,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, "Movies", created.Name)
	require.Equal(t, flow.CodecHEVC, created.Target.Video.Codec)
	require.True(t, created.Watch)

	// Get.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+created.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var got server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, created.ID, got.ID)

	// List.
	rec = h.do(http.MethodGet, "/api/v1/libraries", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []server.LibraryDTO `json:"items"`
		Total int                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	require.Equal(t, 1, list.Total)

	// Update with a changed target.
	target.Video.Codec = flow.CodecH264
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+created.ID, map[string]any{
		"name": "Movies", "path": h.root, "watch": true, "target": target,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, flow.CodecH264, updated.Target.Video.Codec)

	// Delete.
	rec = h.do(http.MethodDelete, "/api/v1/libraries/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+created.ID, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// Error shapes follow API.md conventions. An empty name is no longer
	// an error (it defaults to the folder's base name for the easy
	// screen); an invalid target is still a 400.
	rec = h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "", "path": h.root,
		"target": flow.Target{
			Video:     flow.VideoTarget{Codec: "nope", Quality: 3},
			Audio:     flow.AudioTarget{Mode: flow.AudioKeepAll},
			Subtitles: flow.SubtitleTarget{Mode: flow.SubsKeepAll},
			Container: flow.ContainerMKV,
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var errBody struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	require.Equal(t, "invalid_request", errBody.Code)
	require.NotEmpty(t, errBody.Error)
}

// TestLibraryLastScanAtExposed covers the DTO field (task 19): a library
// that has never been scanned omits last_scan_at; a finished scan stamps
// it and the API exposes it.
func TestLibraryLastScanAtExposed(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.NotContains(t, raw, "last_scan_at", "never scanned: the field is omitted")

	h.scanSync(lib.ID)

	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	raw = map[string]any{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	stamped, ok := raw["last_scan_at"].(string)
	require.True(t, ok, "last_scan_at is a string after a scan")
	_, err := time.Parse(time.RFC3339, stamped)
	require.NoError(t, err, "last_scan_at is RFC 3339: %q", stamped)
}

func TestFilesEndpointStatusFilterAndPagination(t *testing.T) {
	h := newAPIHarness(t)
	h.stated.mu.Lock()
	h.stated.byName["needs.mkv"] = apiNeeds
	h.stated.mu.Unlock()
	h.writeFixture("needs.mkv", 2048)
	h.writeFixture("sat1.mkv", 2048)
	h.writeFixture("sat2.mkv", 2048)

	lib := h.createLibrary(false)
	h.scanSync(lib.ID)

	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/files?status=satisfied", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page struct {
		Items []server.FileDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Equal(t, 2, page.Total, "exactly the two satisfied files")

	// Pagination + probe payload.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/files?limit=1&offset=1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var paged struct {
		Items []server.FileDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &paged))
	require.Len(t, paged.Items, 1)
	require.Equal(t, 3, paged.Total)
	require.NotNil(t, paged.Items[0].Probe, "probe JSON is exposed")

	// Bad status filter.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/files?status=bogus", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJobsEndpointsListGetCancelRetry(t *testing.T) {
	h := newAPIHarness(t)
	h.stated.mu.Lock()
	h.stated.byName["movie.mkv"] = apiNeeds
	h.stated.mu.Unlock()
	h.writeFixture("movie.mkv", 2048)
	lib := h.createLibrary(false)
	h.scanSync(lib.ID)

	// List (filter).
	rec := h.do(http.MethodGet, "/api/v1/jobs?status=queued", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []server.JobDTO `json:"items"`
		Total int             `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	job := list.Items[0]

	// Get includes the log tail field.
	rec = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var got server.JobDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, job.ID, got.ID)
	require.Equal(t, store.JobStatusQueued, got.Status)

	// Cancel, then get.
	rec = h.do(http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil)
	require.Equal(t, http.StatusNoContent, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, store.JobStatusCancelled, got.Status)

	// Cancel again conflicts.
	rec = h.do(http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil)
	require.Equal(t, http.StatusConflict, rec.Code)

	// Retry returns it to queued.
	rec = h.do(http.MethodPost, "/api/v1/jobs/"+job.ID+"/retry", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, store.JobStatusQueued, got.Status)
	require.Equal(t, 0, got.Attempt)

	// Unknown job → 404.
	rec = h.do(http.MethodGet, "/api/v1/jobs/does-not-exist", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestScanEndpointAccepted(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	rec := h.do(http.MethodPost, "/api/v1/libraries/"+lib.ID+"/scan", nil)
	require.Equal(t, http.StatusAccepted, rec.Code)
	var body map[string]bool
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body["started"])

	rec = h.do(http.MethodPost, "/api/v1/libraries/nope/scan", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFsBrowse(t *testing.T) {
	h := newAPIHarness(t)
	sub := filepath.Join(h.root, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	h.writeFixture("movie.mkv", 64)

	rec := h.do(http.MethodGet, "/api/v1/fs/browse?path="+h.root, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Entries []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, h.root, body.Path)
	seen := map[string]bool{}
	dirs := map[string]bool{}
	for _, e := range body.Entries {
		seen[e.Name] = true
		if e.IsDir {
			dirs[e.Name] = true
		}
	}
	require.True(t, seen["subdir"] && dirs["subdir"], "directory listed")
	require.True(t, seen["movie.mkv"] && !dirs["movie.mkv"], "file name listed (no contents)")

	// Missing directory → 404.
	rec = h.do(http.MethodGet, "/api/v1/fs/browse?path=/nonexistent-xyz", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// Relative path → 400.
	rec = h.do(http.MethodGet, "/api/v1/fs/browse?path=relative", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEventsStreamDeliversScanEvents(t *testing.T) {
	h := newAPIHarness(t)
	h.stated.mu.Lock()
	h.stated.byName["movie.mkv"] = apiNeeds
	h.stated.mu.Unlock()
	h.writeFixture("movie.mkv", 2048)
	lib := h.createLibrary(false)

	// Open the SSE stream and wait for the subscription (retry line) before
	// triggering the scan, so no event is missed.
	srv := httptest.NewServer(h.mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/events", nil)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	events := make(chan string, 32)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				select {
				case events <- string(buf[:n]):
				default:
				}
			}
			if err != nil {
				close(events)
				return
			}
		}
	}()

	// Wait until subscribed.
	subscribed := false
	deadline := time.Now().Add(5 * time.Second)
	for !subscribed && time.Now().Before(deadline) {
		select {
		case chunk, ok := <-events:
			if !ok {
				t.Fatal("stream closed before subscription")
			}
			subscribed = subscribed || strings.Contains(chunk, "retry:")
		case <-time.After(100 * time.Millisecond):
		}
	}
	require.True(t, subscribed, "SSE subscription established")

	// Trigger the scan: job.updated + file.updated must arrive.
	h.scanSync(lib.ID)

	want := map[string]bool{"event: job.updated": false, "event: file.updated": false}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case chunk, ok := <-events:
			if !ok {
				t.Fatal("stream closed early")
			}
			for k := range want {
				if strings.Contains(chunk, k) {
					want[k] = true
				}
			}
		case <-time.After(100 * time.Millisecond):
		}
		done := true
		for _, seen := range want {
			done = done && seen
		}
		if done {
			return
		}
	}
	t.Fatalf("missing events: %v", want)
}
