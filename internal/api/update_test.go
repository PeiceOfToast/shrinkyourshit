// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHub is a stand-in for the GitHub release API (task 17 endpoint
// tests): a fixture release with the platform archives and checksums.txt,
// served like the real hub does.
type fakeHub struct {
	t      *testing.T
	server *httptest.Server
	tag    string
	bin    string
	// noArchive serves a release whose assets do not cover this platform.
	noArchive bool
	// slow delays the archive download so a test can observe an apply in
	// flight.
	slow     time.Duration
	requests int
}

func newFakeHub(t *testing.T, tag, bin string) *fakeHub {
	t.Helper()
	hub := &fakeHub{t: t, tag: tag, bin: bin}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/PeiceOfToast/shrinkyourshit/releases/latest", hub.latest)
	mux.HandleFunc("GET /repos/PeiceOfToast/shrinkyourshit/releases", hub.list)
	mux.HandleFunc("GET /download/{name}", hub.download)
	hub.server = httptest.NewServer(mux)
	t.Cleanup(hub.server.Close)
	return hub
}

func (h *fakeHub) archiveBytes() []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte(h.bin)
	require.NoError(h.t, tw.WriteHeader(&tar.Header{
		Name: "shrinkyourshit-linux-amd64", Mode: 0o755, Size: int64(len(content)),
	}))
	_, err := tw.Write(content)
	require.NoError(h.t, err)
	require.NoError(h.t, tw.Close())
	require.NoError(h.t, gw.Close())
	return buf.Bytes()
}

func (h *fakeHub) asset(name string) map[string]any {
	return map[string]any{
		"name":                 name,
		"browser_download_url": h.server.URL + "/download/" + name,
		"size":                 1,
	}
}

func (h *fakeHub) release() map[string]any {
	assets := []map[string]any{
		h.asset("checksums.txt"),
		h.asset(update.ArchiveName("linux", "amd64")),
		h.asset(update.ArchiveName("linux", "arm64")),
		h.asset(update.ArchiveName("windows", "amd64")),
	}
	if h.noArchive {
		// Only checksums.txt: nothing matches the running GOOS/GOARCH.
		assets = assets[:1]
	}
	return map[string]any{
		"tag_name":     h.tag,
		"name":         h.tag,
		"html_url":     h.server.URL + "/releases/tag/" + h.tag,
		"body":         "Fixes a lot, adds a little.",
		"prerelease":   false,
		"draft":        false,
		"published_at": "2026-10-30T00:00:00Z",
		"assets":       assets,
	}
}

func (h *fakeHub) latest(w http.ResponseWriter, _ *http.Request) {
	h.requests++
	writeJSON(w, http.StatusOK, h.release())
}

func (h *fakeHub) list(w http.ResponseWriter, _ *http.Request) {
	h.requests++
	rel := h.release()
	older := map[string]any{"tag_name": "v0.9.0", "draft": false}
	writeJSON(w, http.StatusOK, []any{rel, older})
}

func (h *fakeHub) download(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("name") {
	case "checksums.txt":
		sum := sha256.Sum256(h.archiveBytes())
		_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " +
			update.ArchiveName("linux", "amd64") + "\n"))
	case update.ArchiveName("linux", "amd64"):
		if h.slow > 0 {
			time.Sleep(h.slow)
		}
		_, _ = w.Write(h.archiveBytes())
	default:
		http.NotFound(w, r)
	}
}

// newUpdateHarness builds a harness whose update service points at a fake
// release hub and a fake running binary, with container detection and the
// once-per-minute window under test control.
func newUpdateHarness(t *testing.T, hub *fakeHub, opts func(*server.UpdateOptions)) (*apiHarness, string) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "shrinkyourshit")
	require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0o755))
	require.NoError(t, os.WriteFile(exe, []byte("old-binary-content"), 0o755))

	var container bool
	h := newAPIHarness(t, func(cfg *server.Config) {
		cfg.DataDir = filepath.Join(dir, "config")
		cfg.Update = server.UpdateOptions{
			Current: update.Info{Version: "v1.2.0", Commit: "abc1234", Date: "2026-01-02T03:04:05Z"},
			APIBase: hub.server.URL,
			Executable: func() (string, error) {
				return exe, nil
			},
			IsContainer: func() bool { return container },
		}
		if opts != nil {
			opts(&cfg.Update)
		}
	})
	// The container override is read through the closure the config
	// captured, so tests flip it between cases.
	return h, exe
}

// containerFlip wires a settable container override and returns the setter.
func containerFlip() (func(*server.UpdateOptions), *bool) {
	flag := new(bool)
	return func(o *server.UpdateOptions) { o.IsContainer = func() bool { return *flag } }, flag
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) server.UpdateStatus {
	t.Helper()
	var status server.UpdateStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	return status
}

// TestUpdateStatus covers GET /api/v1/update/status (API.md, task 17):
// "not checked yet" before the first check, the fixture release after one,
// and the container's unsupported answer with the fix in the reason.
func TestUpdateStatus(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	flip, inContainer := containerFlip()
	h, _ := newUpdateHarness(t, hub, flip)

	// Before any check: no network, immediate answer, not-checked reason.
	rec := h.do(http.MethodGet, "/api/v1/update/status", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	status := decodeStatus(t, rec)
	assert.Equal(t, "v1.2.0", status.Current)
	assert.False(t, status.UpdateAvailable)
	assert.Equal(t, "not checked yet", status.Reason)
	assert.Empty(t, status.Latest)
	assert.True(t, status.SelfUpdateSupported)

	// A forced check against the fixture release.
	rec = h.do(http.MethodPost, "/api/v1/update/check", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	status = decodeStatus(t, rec)
	assert.Equal(t, "v1.2.3", status.Latest)
	assert.True(t, status.UpdateAvailable)
	assert.Equal(t, hub.server.URL+"/releases/tag/v1.2.3", status.ReleaseURL)
	assert.Equal(t, "2026-10-30T00:00:00Z", status.PublishedAt)
	assert.Contains(t, status.ReleaseNotes, "Fixes a lot")

	// The status endpoint serves the last check without touching the
	// network (the hub saw exactly one request).
	rec = h.do(http.MethodGet, "/api/v1/update/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	status = decodeStatus(t, rec)
	assert.True(t, status.UpdateAvailable)
	assert.Equal(t, "v1.2.3", status.Latest)
	assert.Equal(t, 1, hub.requests)

	// In a container self update is refused and the reason names the fix.
	*inContainer = true
	rec = h.do(http.MethodGet, "/api/v1/update/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	status = decodeStatus(t, rec)
	assert.False(t, status.SelfUpdateSupported)
	assert.Contains(t, status.Reason, "docker compose pull")
	assert.Contains(t, status.Reason, "docker compose up -d")
	assert.Contains(t, status.Reason, "ghcr.io/peiceoftoast/shrinkyourshit")
}

// TestApplyRefusedInContainer covers POST /api/v1/update/apply inside a
// container (API.md, task 17): 409 self_update_unsupported with the exact
// pull instructions, and nothing applied.
func TestApplyRefusedInContainer(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	flip, inContainer := containerFlip()
	h, exe := newUpdateHarness(t, hub, flip)
	*inContainer = true

	rec := h.do(http.MethodPost, "/api/v1/update/apply", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "self_update_unsupported", body.Code)
	assert.Contains(t, body.Error, "docker compose pull")
	assert.Contains(t, body.Error, "docker compose up -d")

	// The running binary is untouched and no apply ever started.
	assert.Equal(t, "old-binary-content", readFileBytes(t, exe))
	assert.False(t, h.svc.Update().Applying())
}

// TestApplyEmitsProgressEvents covers POST /api/v1/update/apply on a
// binary install (API.md, task 17): 202 immediately, the update runs in
// the background and reports update.progress stages, then
// update.succeeded; the fake binary is really replaced.
func TestApplyEmitsProgressEvents(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	flip, _ := containerFlip()
	h, exe := newUpdateHarness(t, hub, flip)

	id, events := h.svc.Events().Subscribe()
	defer h.svc.Events().Unsubscribe(id)

	rec := h.do(http.MethodPost, "/api/v1/update/apply", nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "started")

	type event struct {
		Type string
		Data map[string]string
	}
	var progress []string
	var succeeded bool
	deadline := time.After(5 * time.Second)
	for !(succeeded && progress != nil) {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for update events; got progress=%v succeeded=%v", progress, succeeded)
		case ev, ok := <-events:
			require.True(t, ok, "event channel closed unexpectedly")
			switch ev.Type {
			case server.EventUpdateProgress:
				var data map[string]string
				require.NoError(t, json.Unmarshal(ev.Data, &data))
				progress = append(progress, data["stage"])
			case server.EventUpdateSucceeded:
				var data map[string]string
				require.NoError(t, json.Unmarshal(ev.Data, &data))
				assert.Equal(t, "v1.2.0", data["from"])
				assert.Equal(t, "v1.2.3", data["to"])
				succeeded = true
			case server.EventUpdateFailed:
				t.Fatalf("unexpected update.failed: %s", ev.Data)
			}
		}
	}

	// The stages streamed in apply order and the binary was swapped, with
	// the previous one kept for rollback.
	assert.Contains(t, progress, update.StageChecking)
	assert.Contains(t, progress, update.StageDownloading)
	assert.Contains(t, progress, update.StageVerifying)
	assert.Contains(t, progress, update.StageSwapping)
	assert.Equal(t, "new-binary-content", readFileBytes(t, exe))
	assert.Equal(t, "old-binary-content", readFileBytes(t, exe+".prev"))
}

// TestApplyWhileRunningIsRefused covers the in-progress guard: a second
// apply while the first one still downloads is 409 update_in_progress.
func TestApplyWhileRunningIsRefused(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	hub.slow = 400 * time.Millisecond
	flip, _ := containerFlip()
	h, _ := newUpdateHarness(t, hub, flip)

	id, events := h.svc.Events().Subscribe()
	defer h.svc.Events().Unsubscribe(id)

	rec := h.do(http.MethodPost, "/api/v1/update/apply", nil)
	require.Equal(t, http.StatusAccepted, rec.Code)

	// While the first update downloads, a second one is refused.
	rec = h.do(http.MethodPost, "/api/v1/update/apply", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "update_in_progress")

	// The first update still finishes.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the first apply to finish")
		case ev, ok := <-events:
			require.True(t, ok)
			if ev.Type == server.EventUpdateSucceeded {
				return
			}
		}
	}
}

// TestApplyNoMatchingAssetFailsQuietly covers the third failure mode the
// task names: a release without an asset for this platform produces a
// specific update.failed (progress carries the reason) and the running
// binary survives.
func TestApplyNoMatchingAssetFailsQuietly(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	hub.noArchive = true
	flip, _ := containerFlip()
	h, exe := newUpdateHarness(t, hub, flip)

	id, events := h.svc.Events().Subscribe()
	defer h.svc.Events().Unsubscribe(id)

	rec := h.do(http.MethodPost, "/api/v1/update/apply", nil)
	require.Equal(t, http.StatusAccepted, rec.Code)

	deadline := time.After(5 * time.Second)
	var failed string
outer:
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for update.failed")
		case ev, ok := <-events:
			require.True(t, ok)
			if ev.Type == server.EventUpdateFailed {
				var data map[string]string
				require.NoError(t, json.Unmarshal(ev.Data, &data))
				failed = data["error"]
				break outer
			}
		}
	}
	assert.Contains(t, failed, "no release asset for this platform")
	assert.Equal(t, "old-binary-content", readFileBytes(t, exe))

	// The failure lands in the status as the quiet progress error.
	rec = h.do(http.MethodGet, "/api/v1/update/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	status := decodeStatus(t, rec)
	require.NotNil(t, status.Progress)
	assert.False(t, status.Progress.Active)
	assert.Contains(t, status.Progress.Error, "no release asset for this platform")
}

// TestUpdateCheckRateLimited covers the once-per-minute window on
// POST /api/v1/update/check (API.md, task 17): the second check within a
// minute is 429 rate_limited.
func TestUpdateCheckRateLimited(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	flip, _ := containerFlip()
	h, _ := newUpdateHarness(t, hub, flip)

	rec := h.do(http.MethodPost, "/api/v1/update/check", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = h.do(http.MethodPost, "/api/v1/update/check", nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "rate_limited")

	// The status endpoint keeps answering the last check, rate limit or
	// not; it never checks on its own.
	rec = h.do(http.MethodGet, "/api/v1/update/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "v1.2.3", decodeStatus(t, rec).Latest)
}

// TestOfflineStartup covers the task-17 startup rule: with no network the
// app starts and serves normally, the background check fails quietly into
// the status reason, no update event is published and nothing else
// changes.
func TestOfflineStartup(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	hub.server.Close() // an unreachable hub: no network for the updater

	flip, _ := containerFlip()
	h, _ := newUpdateHarness(t, hub, func(o *server.UpdateOptions) {
		flip(o)
		o.BackgroundDelay = 10 * time.Millisecond
	})

	id, events := h.svc.Events().Subscribe()
	defer h.svc.Events().Unsubscribe(id)

	// The background check runs after start without blocking anything.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.svc.Update().StartBackgroundCheck(ctx)

	// Wait for the quiet failure to land in the status.
	var status server.UpdateStatus
	deadline := time.After(5 * time.Second)
	for {
		rec := h.do(http.MethodGet, "/api/v1/update/status", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		status = decodeStatus(t, rec)
		if status.Reason != "" && status.Reason != "not checked yet" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the background check never reported its failure")
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.False(t, status.UpdateAvailable)
	assert.Contains(t, status.Reason, "Could not check for updates")

	// Nothing else changed: the app serves normally and no update event
	// was published for a failed check.
	rec := h.do(http.MethodGet, "/api/v1/health", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/stats", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	select {
	case ev := <-events:
		t.Fatalf("unexpected event for a failed background check: %s", ev.Type)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestUpdateCheckDisabledSkipsBackground covers the About card's automatic
// checks switch: with update_check off the background check never runs.
func TestUpdateCheckDisabledSkipsBackground(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	flip, _ := containerFlip()
	h, _ := newUpdateHarness(t, hub, func(o *server.UpdateOptions) {
		flip(o)
		o.BackgroundDelay = 10 * time.Millisecond
	})
	require.NoError(t, h.st.SetSetting(context.Background(), store.SettingUpdateCheck, "false"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.svc.Update().StartBackgroundCheck(ctx)
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, 0, hub.requests, "no check may run when automatic checks are off")
	status := decodeStatus(t, h.do(http.MethodGet, "/api/v1/update/status", nil))
	assert.Equal(t, "not checked yet", status.Reason)
}

func readFileBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
