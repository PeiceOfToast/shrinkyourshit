// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The distinctive key this suite plants everywhere: no response body may
// ever echo it back (acceptance item 2).
const secretIntegrationKey = "distinctive-secret-key-9f3a"

// createIntegrationViaAPI POSTs an integration and returns its DTO.
func (h *apiHarness) createIntegrationViaAPI(t *testing.T, body map[string]any) integrationDTO {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/integrations", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto integrationDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	return dto
}

// doWith performs a request with an explicit session cookie.
func (h *apiHarness) doWith(t *testing.T, sess *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	}
	return h.doReq(withCookie(req, sess))
}

func integrationCreateBody() map[string]any {
	return map[string]any{
		"kind": "jellyfin", "name": "Living room",
		"base_url": "http://media.lan:8096/", "api_key": secretIntegrationKey,
		"enabled": true, "refresh_on_done": true,
	}
}

func TestIntegrationsCRUDViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	created := h.createIntegrationViaAPI(t, integrationCreateBody())
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "jellyfin", created.Kind)
	assert.Equal(t, "Living room", created.Name)
	assert.Equal(t, "http://media.lan:8096", created.BaseURL, "the trailing slash is trimmed")
	assert.True(t, created.APIKeySet)
	assert.True(t, created.Enabled)
	assert.True(t, created.RefreshOnDone)

	// Every response body is checked against the distinctive key: the key
	// never comes back out of the API (the task defines no GET single, so
	// the list is the only read).
	seen := 0
	checkNoKey := func(t *testing.T, body string) {
		t.Helper()
		require.NotContains(t, body, secretIntegrationKey)
		require.NotContains(t, body, `"api_key"`)
		seen++
	}

	rec := h.do(http.MethodGet, "/api/v1/integrations", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []integrationDTO `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	require.Len(t, list.Items, 1)
	checkNoKey(t, rec.Body.String())

	// PUT with an empty api_key keeps the stored key.
	rec = h.do(http.MethodPut, "/api/v1/integrations/"+created.ID, map[string]any{
		"kind": "jellyfin", "name": "Living room TV",
		"base_url": "http://media.lan:8096", "api_key": "",
		"enabled": false, "refresh_on_done": true,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	checkNoKey(t, rec.Body.String())
	var updated integrationDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Living room TV", updated.Name)
	assert.False(t, updated.Enabled)
	assert.True(t, updated.APIKeySet, "an empty api_key on PUT keeps the stored key")

	// The refresher still holds the stored key for the outbound call.
	row, err := h.st.GetIntegration(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, secretIntegrationKey, row.APIKey)

	// The key also survives an update that changes the key to a new one.
	rec = h.do(http.MethodPut, "/api/v1/integrations/"+created.ID, map[string]any{
		"kind": "emby", "name": "Living room TV",
		"base_url": "http://media.lan:8096", "api_key": "replacement-key",
		"enabled": true, "refresh_on_done": false,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	row, err = h.st.GetIntegration(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "replacement-key", row.APIKey)
	assert.False(t, row.RefreshOnDone)

	// Delete answers 204; unknown id answers 404 in the existing shape.
	rec = h.do(http.MethodDelete, "/api/v1/integrations/"+created.ID, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	rec = h.do(http.MethodGet, "/api/v1/integrations/"+created.ID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not_found")
	rec = h.do(http.MethodDelete, "/api/v1/integrations/does-not-exist", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not_found")

	assert.Positive(t, seen)
}

func TestIntegrationsValidationViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	for _, bad := range []map[string]any{
		{"kind": "kodi", "name": "x", "base_url": "http://m.lan", "api_key": "k"},
		{"kind": "jellyfin", "name": "x", "base_url": "", "api_key": "k"},
		{"kind": "jellyfin", "name": "x", "base_url": "ftp://m.lan", "api_key": "k"},
		{"kind": "jellyfin", "name": "x", "base_url": "not a url", "api_key": "k"},
		{"kind": "jellyfin", "name": "x", "base_url": "http://m.lan", "api_key": ""},
	} {
		rec := h.do(http.MethodPost, "/api/v1/integrations", bad)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "invalid_request")
	}

	// Nothing was stored.
	rec := h.do(http.MethodGet, "/api/v1/integrations", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"total":0`)
}

// The test endpoint performs that one integration's call once and answers
// 200 even when the remote refuses.
func TestIntegrationsTestEndpointViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != secretIntegrationKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	created := h.createIntegrationViaAPI(t, map[string]any{
		"kind": "jellyfin", "name": "Living room",
		"base_url": remote.URL, "api_key": secretIntegrationKey,
		"enabled": true, "refresh_on_done": true,
	})

	rec := h.do(http.MethodPost, "/api/v1/integrations/"+created.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	assert.Equal(t, http.StatusNoContent, resp.Status)
	assert.Empty(t, resp.Error)

	// An integration whose remote is down still answers 200, with the
	// error text.
	dead := h.createIntegrationViaAPI(t, map[string]any{
		"kind": "plex", "name": "Plex",
		"base_url": "http://127.0.0.1:1", "api_key": "plex-token",
		"enabled": true, "refresh_on_done": true,
	})
	rec = h.do(http.MethodPost, "/api/v1/integrations/"+dead.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.OK)
	assert.NotEmpty(t, resp.Error)
}

// All five endpoints are admin only (task 23). The harness runs with login
// disabled, so both tests first complete setup with login enabled.
func TestIntegrationsRequireAdmin(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	hits := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/integrations"},
		{http.MethodPost, "/api/v1/integrations"},
		{http.MethodPut, "/api/v1/integrations/some-id"},
		{http.MethodDelete, "/api/v1/integrations/some-id"},
		{http.MethodPost, "/api/v1/integrations/some-id/test"},
	}
	for _, hit := range hits {
		rec := h.do(hit.method, hit.path, map[string]any{"kind": "jellyfin", "name": "x", "base_url": "http://m.lan", "api_key": secretIntegrationKey})
		require.Equal(t, http.StatusUnauthorized, rec.Code,
			"%s %s must demand a session", hit.method, hit.path)
		require.NotContains(t, rec.Body.String(), secretIntegrationKey)
	}

	// Login sets the cookie and the endpoints open.
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	sess := cookieOf(t, rec)

	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/integrations", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// With login enabled and no session the API is closed; the list answer
// is the empty-shape body, not the integrations.
func TestIntegrationsNotInWorkerSurface(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodGet, "/api/v1/integrations", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NotContains(t, rec.Body.String(), `"items"`)
}
