// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

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
}

func TestEmptyStaticServesIndexFallback(t *testing.T) {
	h := New("v0.0.0", fstest.MapFS{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	// Missing index.html yields a 404 from ServeFileFS; the important part is
	// that the handler does not panic and keeps the SPA contract.
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
