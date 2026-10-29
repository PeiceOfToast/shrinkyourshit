// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/discovery"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonUnmarshal decodes a recorded JSON body.
func jsonUnmarshal(rec *httptest.ResponseRecorder, v any) error {
	return json.Unmarshal(rec.Body.Bytes(), v)
}

// doReq performs a prepared request against the handler.
func (h *apiHarness) doReq(req *http.Request) *httptest.ResponseRecorder {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// cookieOf extracts the sys_session cookie from a login response.
func cookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sys_session" {
			return c
		}
	}
	t.Fatalf("no sys_session cookie in response: %s", rec.Body.String())
	return nil
}

// withCookie clones a request with the session cookie attached.
func withCookie(r *http.Request, c *http.Cookie) *http.Request {
	r.AddCookie(c)
	return r
}

func TestSetupFlowWithLoginEnabled(t *testing.T) {
	h := newAPIHarness(t)

	// Fresh data dir: setup incomplete.
	rec := h.do(http.MethodGet, "/api/v1/setup/status", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var status server.Status
	require.NoError(t, jsonUnmarshal(rec, &status))
	assert.False(t, status.SetupComplete)
	assert.False(t, status.LoginEnabled)

	// Complete setup with login enabled.
	rec = h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &status))
	assert.True(t, status.SetupComplete)
	assert.True(t, status.LoginEnabled)
	assert.Equal(t, "admin", status.Username)

	// Admin API is closed without a session.
	rec = h.do(http.MethodGet, "/api/v1/libraries", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var errBody struct {
		Code string `json:"code"`
	}
	require.NoError(t, jsonUnmarshal(rec, &errBody))
	assert.Equal(t, "unauthorized", errBody.Code)

	// Login sets the cookie and the API opens.
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	sess := cookieOf(t, rec)
	assert.True(t, sess.HttpOnly, "cookie must be HttpOnly")
	assert.Equal(t, http.SameSiteLaxMode, sess.SameSite, "cookie must be SameSite=Lax")
	assert.Greater(t, sess.MaxAge, 0, "30-day Max-Age")

	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil), sess))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// /auth/me identifies the account.
	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), sess))
	require.Equal(t, http.StatusOK, rec.Code)
	var me server.Status
	require.NoError(t, jsonUnmarshal(rec, &me))
	assert.Equal(t, "admin", me.Username)
	assert.True(t, me.LoginEnabled)

	// POST /setup after completion conflicts.
	rec = h.do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "server"})
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, jsonUnmarshal(rec, &errBody))
	assert.Equal(t, "setup_complete", errBody.Code)

	// Wrong password on login → 401 with a generic message.
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "wrong",
	})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, jsonUnmarshal(rec, &errBody))
	assert.Equal(t, "invalid_credentials", errBody.Code)
}

func TestSetupFlowWithLoginDisabled(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": false,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// The admin API is open without any cookie.
	rec = h.do(http.MethodGet, "/api/v1/libraries", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// /auth/me reports login disabled.
	rec = h.do(http.MethodGet, "/api/v1/auth/me", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var me server.Status
	require.NoError(t, jsonUnmarshal(rec, &me))
	assert.False(t, me.LoginEnabled)
	assert.Empty(t, me.Username)
}

func TestLoginRateLimit(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Five wrong attempts return 401; the sixth within a minute gets 429.
	for i := 1; i <= 5; i++ {
		rec := h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": "admin", "password": "wrong",
		})
		require.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d", i)
	}
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "wrong",
	})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	var errBody struct {
		Code string `json:"code"`
	}
	require.NoError(t, jsonUnmarshal(rec, &errBody))
	assert.Equal(t, "too_many_attempts", errBody.Code)

	// A correct password is also blocked while the window is full.
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestLogoutInvalidatesSession(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	sess := cookieOf(t, rec)

	// Logout clears the cookie and kills the session server-side.
	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil), sess))
	require.Equal(t, http.StatusNoContent, rec.Code)
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sys_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	assert.True(t, cleared, "logout must clear the cookie")

	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), sess))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestExpiredSessionRejected(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	sess := cookieOf(t, rec)

	// Age the session past its expiry directly in the store.
	ctx := context.Background()
	hash := server.HashToken(sess.Value)
	row, err := h.st.GetSessionByHash(ctx, hash)
	require.NoError(t, err)
	require.NoError(t, h.st.DeleteSession(ctx, hash))
	past := &store.Session{TokenHash: hash, CreatedAt: row.CreatedAt, ExpiresAt: time.Now().Add(-time.Minute)}
	require.NoError(t, h.st.CreateSession(ctx, past))

	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), sess))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPasswordHashNeverExposed(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// The stored hash is argon2id.
	v, ok, err := h.st.GetSetting(context.Background(), store.SettingAdminPasswordHash)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(v, server.Argon2Prefix), "stored hash must be argon2id")

	// Log in; no admin response carries the hash.
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	sess := cookieOf(t, rec)

	for _, spec := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/setup/status"},
		{http.MethodGet, "/api/v1/settings"},
		{http.MethodGet, "/api/v1/auth/me"},
	} {
		rec := h.doReq(withCookie(httptest.NewRequest(spec.method, spec.path, nil), sess))
		require.Equal(t, http.StatusOK, rec.Code, "%s %s", spec.method, spec.path)
		assert.NotContains(t, rec.Body.String(), "argon2id", "%s leaks the hash", spec.path)
		assert.NotContains(t, rec.Body.String(), "admin_password_hash", "%s leaks the hash key", spec.path)
	}
}

func TestSettingsEndpoints(t *testing.T) {
	h := newAPIHarness(t)

	// Defaults on a fresh store (login disabled, so no cookie needed).
	rec := h.do(http.MethodGet, "/api/v1/settings", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var vals struct {
		WorkerConcurrency   int      `json:"worker_concurrency"`
		ScratchDir          string   `json:"scratch_dir"`
		DefaultResultPolicy string   `json:"default_result_policy"`
		LoginEnabled        bool     `json:"login_enabled"`
		PreferredLanguages  []string `json:"preferred_languages"`
		DefaultVideoCodec   string   `json:"default_video_codec"`
	}
	require.NoError(t, jsonUnmarshal(rec, &vals))
	assert.Equal(t, 1, vals.WorkerConcurrency)
	assert.Equal(t, "replace_if_smaller", vals.DefaultResultPolicy)
	assert.Equal(t, []string{"eng"}, vals.PreferredLanguages)
	assert.Equal(t, "hevc", vals.DefaultVideoCodec)

	// PUT updates the plain fields.
	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{
		"worker_concurrency":    3,
		"scratch_dir":           "/scratch/fast",
		"default_result_policy": "keep_both",
		"preferred_languages":   []string{"deu", "eng"},
		"default_video_codec":   "av1",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &vals))
	assert.Equal(t, 3, vals.WorkerConcurrency)
	assert.Equal(t, "/scratch/fast", vals.ScratchDir)
	assert.Equal(t, "keep_both", vals.DefaultResultPolicy)
	assert.Equal(t, []string{"deu", "eng"}, vals.PreferredLanguages)
	assert.Equal(t, "av1", vals.DefaultVideoCodec)

	// Invalid values are refused.
	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"worker_concurrency": 0})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_video_codec": "mpeg2"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSettingsEnableLoginRequiresPassword(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": false,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Login on without a password in the same request → 400.
	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{
		"login_enabled": true, "admin_username": "admin",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// With a password it works and the account can log in.
	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{
		"login_enabled": true, "admin_username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	sess := cookieOf(t, rec)
	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil), sess))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestSettingsPasswordChangeRequiresCurrent(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "old",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "old",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	sess := cookieOf(t, rec)

	// Change without the current password → 401.
	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodPut, "/api/v1/settings",
		strings.NewReader(`{"password":"new"}`)), sess))
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())

	// With the current password it succeeds.
	rec = h.doReq(withCookie(httptest.NewRequest(http.MethodPut, "/api/v1/settings",
		strings.NewReader(`{"password":"new","current_password":"old"}`)), sess))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "admin", "password": "new"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestSetupModeJoin(t *testing.T) {
	h := newAPIHarness(t)

	// Joining needs a server URL.
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "join"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	rec = h.do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "join", "server": "not a url"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Without a wired callback (e.g. an embedded build that did not opt
	// in) joining is a 500, not a silent half-join.
	rec = h.do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "join", "server": "http://10.0.0.5:8765"})
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	// With the callback wired the join records the decision and completes
	// setup (task 08).
	var joined []string
	joinMux := New("v0.0.0", testStatic(), h.svc, WithJoin(func(serverURL string) error {
		joined = append(joined, serverURL)
		return nil
	}))
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		rec := httptest.NewRecorder()
		joinMux.ServeHTTP(rec, req)
		return rec
	}
	rec = do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "join", "server": "http://10.0.0.5:8765"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, []string{"http://10.0.0.5:8765"}, joined)
	var status server.Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.True(t, status.SetupComplete)
	require.Equal(t, server.SetupModeJoin, status.Mode)
	require.Equal(t, "http://10.0.0.5:8765", status.Server)

	// Setup is complete now: a second join (or a server-mode setup) is 409.
	rec = do(http.MethodPost, "/api/v1/setup", map[string]any{"mode": "join", "server": "http://10.0.0.5:8765"})
	require.Equal(t, http.StatusConflict, rec.Code)
}

// setupServersWith wires a fake Discover so the endpoint is testable
// without a real LAN.
func setupServersWith(t *testing.T, found []discovery.ServerInfo) (http.Handler, *bool) {
	h := newAPIHarness(t)
	called := false
	mux := New("v0.0.0", testStatic(), h.svc)
	h.svc.SetDiscover(func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo {
		called = true
		return found
	}, "self-node-id")
	return mux, &called
}

func TestSetupServersListsDiscoveredUntilSetupComplete(t *testing.T) {
	found := []discovery.ServerInfo{
		{ID: "self-node-id", Name: "me", URL: "http://127.0.0.1:8765"},
		{ID: "other", Name: "media-box", URL: "http://10.0.0.5:8765"},
	}
	mux, called := setupServersWith(t, found)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/setup/servers", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, *called)
	var resp struct {
		Servers []server.DiscoveredServer `json:"servers"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// The server's own announcement is hidden from the list.
	require.Len(t, resp.Servers, 1)
	require.Equal(t, "media-box", resp.Servers[0].Name)
	require.Equal(t, "http://10.0.0.5:8765", resp.Servers[0].URL)

	// After setup the wizard no longer scans.
	h2 := newAPIHarness(t)
	require.NoError(t, h2.svc.Auth().CompleteSetup(context.Background(), server.SetupParams{Mode: server.SetupModeServer}))
	rec = h2.do(http.MethodGet, "/api/v1/setup/servers", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Servers)
}
