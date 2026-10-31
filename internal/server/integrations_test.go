// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureBuffer is a slog sink that is safe for the refresher's goroutines.
type captureBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *captureBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *captureBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// countingRemote is an httptest server that counts requests and records
// what the last one looked like.
type countingRemote struct {
	srv *httptest.Server

	mu     sync.Mutex
	count  int
	method string
	path   string
	query  string
	header http.Header
	body   string
}

func newCountingRemote(t *testing.T, status int) *countingRemote {
	t.Helper()
	c := &countingRemote{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := ""
		if r.Body != nil {
			if rawBytes, err := io.ReadAll(r.Body); err == nil {
				raw = string(rawBytes)
			}
		}
		c.mu.Lock()
		c.count++
		c.method = r.Method
		c.path = r.URL.Path
		c.query = r.URL.RawQuery
		c.header = r.Header.Clone()
		c.body = raw
		c.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *countingRemote) requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func (c *countingRemote) last() (method, path, query, body string, header http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.method, c.path, c.query, c.body, c.header
}

// waitForCount polls until the remote saw at least n requests (the calls
// run in refresher goroutines) or fails the test.
func waitForCount(t *testing.T, c *countingRemote, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if c.requests() >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("remote saw %d requests, want at least %d", c.requests(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func validateOK(in Integration) error {
	return ValidateIntegration(&in)
}

func TestValidateIntegration(t *testing.T) {
	base := Integration{Kind: "jellyfin", Name: " JR ", BaseURL: "http://media.lan:8096/", APIKey: " key "}

	// The happy path normalizes in place: trailing slash and surrounding
	// whitespace gone, kind and fields kept.
	in := base
	require.NoError(t, ValidateIntegration(&in))
	assert.Equal(t, "jellyfin", in.Kind)
	assert.Equal(t, "JR", in.Name)
	assert.Equal(t, "http://media.lan:8096", in.BaseURL)
	assert.Equal(t, "key", in.APIKey)

	// Unknown kind.
	bad := base
	bad.Kind = "kodi"
	require.ErrorContains(t, validateOK(bad), "unknown kind")

	// base_url that does not parse.
	bad = base
	bad.BaseURL = "http://med ia:8096"
	require.Error(t, validateOK(bad))

	// base_url without http/https.
	bad = base
	bad.BaseURL = "ftp://media.lan:8096"
	require.ErrorContains(t, validateOK(bad), "http or https")
	bad.BaseURL = "media.lan:8096"
	require.Error(t, validateOK(bad))

	// Empty base_url or api_key.
	bad = base
	bad.BaseURL = "///"
	require.Error(t, validateOK(bad))
	bad = base
	bad.BaseURL = ""
	require.ErrorContains(t, validateOK(bad), "base_url is required")
	bad = base
	bad.APIKey = ""
	require.ErrorContains(t, validateOK(bad), "api_key is required")
}

// TestCallIntegrationRequestShapes pins the exact request per kind
// (acceptance item 3) against real httptest remotes.
func TestCallIntegrationRequestShapes(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		kind       string
		method     string
		path       string
		queryToken string
		header     string
		body       string
	}{
		{
			kind:   store.IntegrationKindJellyfin,
			method: http.MethodPost, path: "/Library/Refresh", header: "X-Emby-Token",
		},
		{
			kind:   store.IntegrationKindEmby,
			method: http.MethodPost, path: "/Library/Refresh", header: "X-Emby-Token",
		},
		{
			kind:   store.IntegrationKindPlex,
			method: http.MethodGet, path: "/library/sections/all/refresh",
			queryToken: "X-Plex-Token",
		},
		{
			kind:   store.IntegrationKindSonarr,
			method: http.MethodPost, path: "/api/v3/command", header: "X-Api-Key",
			body: `{"name":"RescanSeries"}`,
		},
		{
			kind:   store.IntegrationKindRadarr,
			method: http.MethodPost, path: "/api/v3/command", header: "X-Api-Key",
			body: `{"name":"RescanMovie"}`,
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			remote := newCountingRemote(t, http.StatusNoContent)
			in := Integration{
				Kind: tc.kind, Name: "remote " + tc.kind,
				BaseURL: remote.srv.URL, APIKey: "distinctive-key-value",
			}
			status, err := CallIntegration(ctx, remote.srv.Client(), in)
			require.NoError(t, err)
			require.Equal(t, http.StatusNoContent, status)
			waitForCount(t, remote, 1)

			method, path, query, body, header := remote.last()
			assert.Equal(t, tc.method, method)
			assert.Equal(t, tc.path, path)
			if tc.queryToken != "" {
				assert.Equal(t, tc.queryToken+"=distinctive-key-value", query)
			}
			assert.Equal(t, tc.body, body)
			if tc.header != "" {
				assert.Equal(t, "distinctive-key-value", header.Get(tc.header))
			}
		})
	}
}

// A non-2xx is an error carrying the remote's status; the error text never
// carries the key or the base URL.
func TestCallIntegrationNon2xxAndTransportFailure(t *testing.T) {
	ctx := context.Background()

	remote := newCountingRemote(t, http.StatusNotFound)
	status, err := CallIntegration(ctx, remote.srv.Client(), Integration{
		Kind: store.IntegrationKindSonarr, Name: "tv", BaseURL: remote.srv.URL, APIKey: "k",
	})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	assert.NotContains(t, err.Error(), "k", "the key must not appear in errors")
	assert.NotContains(t, err.Error(), remote.srv.URL, "the base URL must not appear in errors")

	// Transport failure: status 0, error without the URL (a Plex URL
	// carries the token in its query).
	status, err = CallIntegration(ctx, remote.srv.Client(), Integration{
		Kind: store.IntegrationKindPlex, Name: "plex",
		BaseURL: "http://127.0.0.1:1", APIKey: "plex-token-value",
	})
	require.Error(t, err)
	assert.Zero(t, status)
	assert.NotContains(t, err.Error(), "plex-token-value")
	assert.NotContains(t, err.Error(), "http://127.0.0.1:1")
}

func testIntegration(t *testing.T, kind, baseURL string) *store.Integration {
	t.Helper()
	return &store.Integration{
		Kind: kind, Name: kind + " remote", BaseURL: baseURL, APIKey: "key-" + kind,
		Enabled: true, RefreshOnDone: true,
	}
}

// One in-flight call per id and a floor between calls for the same id:
// a Notify inside the window is dropped, not queued (task 23). With a
// shrunk gap the next Notify after the window goes through.
func TestRefresherCoalescing(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	require.NoError(t, h.st.CreateIntegration(context.Background(), testIntegration(t, store.IntegrationKindJellyfin, remote.srv.URL)))

	r := h.svc.Integrations()
	r.gap = 250 * time.Millisecond
	ctx := context.Background()

	r.Notify(ctx)
	waitForCount(t, remote, 1)
	r.Notify(ctx) // inside the window: dropped
	r.Notify(ctx) // still dropped
	time.Sleep(300 * time.Millisecond)
	r.Notify(ctx)
	waitForCount(t, remote, 2)
	assert.Equal(t, 2, remote.requests())
}

// The acceptance scenario at production settings: two job completions one
// second apart produce exactly one remote request per integration.
func TestQueueTwoCompletionsOneRemoteRequest(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	require.NoError(t, h.st.CreateIntegration(context.Background(), testIntegration(t, store.IntegrationKindRadarr, remote.srv.URL)))

	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return inspectedOutput, nil
	}, "")

	complete := func() *store.Job {
		_, _, job := claimJob(t, h, "node-1")
		got, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
			OutputPath: h.root + "/movie.mkv", BytesAfter: 100,
		})
		require.NoError(t, err)
		require.Equal(t, store.JobStatusDone, got.Status)
		return got
	}

	first := complete()
	waitForCount(t, remote, 1)
	time.Sleep(time.Second) // one second between the two completions
	second := complete()
	time.Sleep(300 * time.Millisecond) // give a wrongly-fired call time to land
	assert.Equal(t, 1, remote.requests(),
		"the second completion one second apart must be coalesced away")
	require.Equal(t, store.JobStatusDone, first.Status)
	require.Equal(t, store.JobStatusDone, second.Status)
}

// A remote 500 or a dead host logs a warning and the job still ends done
// (acceptance item 5).
func TestQueueRemoteFailureWarnsAndJobStillDone(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	failing := newCountingRemote(t, http.StatusInternalServerError)
	dead := "http://127.0.0.1:1"
	require.NoError(t, h.st.CreateIntegration(ctx, testIntegration(t, store.IntegrationKindJellyfin, failing.srv.URL)))
	require.NoError(t, h.st.CreateIntegration(ctx, testIntegration(t, store.IntegrationKindPlex, dead)))

	logs := &captureBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	q := NewQueue(h.st, nil, logger)
	q.SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return inspectedOutput, nil
	}, "")
	q.SetNotifier(NewRefresher(h.st, logger))

	complete := func() *store.Job {
		_, _, job := claimJob(t, h, "node-1")
		got, err := q.Complete(ctx, job.ID, "node-1", CompleteParams{
			OutputPath: h.root + "/movie.mkv", BytesAfter: 100,
		})
		require.NoError(t, err)
		return got
	}

	got := complete()
	require.Equal(t, store.JobStatusDone, got.Status)
	waitForCount(t, failing, 1)

	deadline := time.Now().Add(5 * time.Second)
	for {
		text := logs.text()
		jellyfin := strings.Contains(text, `integration="jellyfin remote"`) &&
			strings.Contains(text, "status=500") &&
			strings.Contains(text, "integration refresh failed")
		plex := strings.Contains(text, `integration="plex remote"`) &&
			strings.Contains(text, "integration refresh failed")
		if jellyfin && plex {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("warnings missing for both failures; log:\n%s", logs.text())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The dead-host warning names the integration and never the key.
	assert.Contains(t, logs.text(), "request failed")
	assert.NotContains(t, logs.text(), "key-plex")
}

// Disabled integrations, and enabled ones without refresh_on_done, stay
// quiet; the one-shot Test call works either way.
func TestRefresherSkipsQuietIntegrations(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	ctx := context.Background()

	off := testIntegration(t, store.IntegrationKindEmby, remote.srv.URL)
	off.Enabled = false
	require.NoError(t, h.st.CreateIntegration(ctx, off))
	noRefresh := testIntegration(t, store.IntegrationKindPlex, remote.srv.URL)
	noRefresh.RefreshOnDone = false
	require.NoError(t, h.st.CreateIntegration(ctx, noRefresh))

	r := h.svc.Integrations()
	r.Notify(ctx)
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, remote.requests())

	// The manual test call reaches the remote regardless.
	status, err := r.Test(ctx, IntegrationFromRow(off))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, status)
	waitForCount(t, remote, 1)
}
