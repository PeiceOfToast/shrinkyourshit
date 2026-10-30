// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStatic() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html><body>index</body></html>\n")},
		"assets/app.js":  {Data: []byte("console.log('app')\n")},
		"assets/app.css": {Data: []byte("body{}\n")},
	}
}

func TestHealth(t *testing.T) {
	h := New("v1.2.3", testStatic(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ok", body.Status)
	assert.Equal(t, "v1.2.3", body.Version)
}

// TestVersionEndpoint covers /api/v1/version (API.md, task 16): the full
// build identity plus the runtime, open like /health (no admin auth,
// works without a wired server).
func TestVersionEndpoint(t *testing.T) {
	h := New("v1.2.3", testStatic(), nil, WithBuild("abc1234", "2026-01-02T03:04:05Z"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body versionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "v1.2.3", body.Version)
	assert.Equal(t, "abc1234", body.Commit)
	assert.Equal(t, "2026-01-02T03:04:05Z", body.Date)
	// The test VM is not a container; pin the runtime without guessing
	// what environment a future containerized CI may add.
	if update.IsContainer() {
		assert.Equal(t, update.RuntimeContainer, body.Runtime)
	} else {
		assert.Equal(t, update.RuntimeBinary, body.Runtime)
	}
}

func TestServesIndexAtRoot(t *testing.T) {
	h := New("v0.0.0", testStatic(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	got, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Contains(t, string(got), "index")
}

func TestUnknownPathFallsBackToIndex(t *testing.T) {
	h := New("v0.0.0", testStatic(), nil)
	for _, p := range []string{"/libraries", "/settings/display", "/deep/nested/path"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		require.Equal(t, http.StatusOK, rec.Code, p)
		got, err := io.ReadAll(rec.Body)
		require.NoError(t, err)
		assert.Contains(t, string(got), "index", p)
	}
}

func TestServesAsset(t *testing.T) {
	h := New("v0.0.0", testStatic(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	got, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, "console.log('app')\n", string(got))
}

func TestUnknownAPIPathReturnsJSON404(t *testing.T) {
	h := New("v0.0.0", testStatic(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body.Code)
	assert.NotEmpty(t, body.Error)
}

func TestNonGetRootIs405(t *testing.T) {
	h := New("v0.0.0", testStatic(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	// Task 13: every 4xx/5xx path carries a stable code field, so the UI
	// can show it instead of a generic sentence.
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "method_not_allowed", body.Code)
}

func TestEmptyStaticServesIndexFallback(t *testing.T) {
	h := New("v0.0.0", fstest.MapFS{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	// Missing index.html yields a 404 from ServeFileFS; the important part is
	// that the handler does not panic and keeps the SPA contract.
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
