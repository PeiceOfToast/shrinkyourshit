// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webhookHarness builds an api harness with a login-enabled admin session
// attached, so webhook endpoint tests exercise the admin auth path. The
// returned cookie must be attached to every request.
func webhookHarness(t *testing.T) (*apiHarness, *http.Cookie) {
	t.Helper()
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return h, cookieOf(t, rec)
}

// doWebhook performs a webhook request with the admin cookie attached.
func doWebhook(t *testing.T, h *apiHarness, sess *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	}
	req.AddCookie(sess)
	return h.doReq(req)
}

// validBody is a webhook create body the endpoint accepts.
func validBody() map[string]any {
	return map[string]any{
		"name":    "Discord",
		"url":     "https://discord.com/api/webhooks/1/abc",
		"events":  []string{"job.done", "job.failed"},
		"enabled": true,
	}
}

// TestWebhookEndpointsCRUD covers acceptance items 5 and 6: the full CRUD
// surface, and the secret returned exactly once and never again.
func TestWebhookEndpointsCRUD(t *testing.T) {
	h, sess := webhookHarness(t)

	// Create returns 201 with the secret, exactly once.
	rec := doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", validBody())
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		URL     string   `json:"url"`
		Events  []string `json:"events"`
		Enabled bool     `json:"enabled"`
		Secret  string   `json:"secret"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))
	require.NotEmpty(t, created.ID)
	require.Len(t, created.Secret, 64, "a 32 byte hex secret")
	assert.Equal(t, []string{"job.done", "job.failed"}, created.Events)
	assert.Equal(t, "https://discord.com/api/webhooks/1/abc", created.URL)

	// The list carries no secret at all — not even a null "secret" field.
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"secret"`, "the secret must never appear again")
	var list struct {
		Items []struct {
			ID        string   `json:"id"`
			SecretSet bool     `json:"secret_set"`
			Secret    string   `json:"secret"`
			Events    []string `json:"events"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, jsonUnmarshal(rec, &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, created.ID, list.Items[0].ID)
	assert.True(t, list.Items[0].SecretSet)
	assert.Empty(t, list.Items[0].Secret)

	// GET one: still no secret.
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/"+created.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"secret"`)

	// PUT with an empty secret keeps the stored one and the hook still
	// delivers. Events are replaced.
	rec = doWebhook(t, h, sess, http.MethodPut, "/api/v1/webhooks/"+created.ID, map[string]any{
		"name": "Discord #media", "url": "https://discord.com/api/webhooks/1/abc",
		"events": []string{"library.scanned"}, "enabled": false,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"secret"`)
	var updated struct {
		Name      string   `json:"name"`
		Events    []string `json:"events"`
		Enabled   bool     `json:"enabled"`
		SecretSet bool     `json:"secret_set"`
	}
	require.NoError(t, jsonUnmarshal(rec, &updated))
	assert.Equal(t, "Discord #media", updated.Name)
	assert.Equal(t, []string{"library.scanned"}, updated.Events)
	assert.False(t, updated.Enabled)
	assert.True(t, updated.SecretSet, "the stored secret survives the update")

	// The stored secret is really unchanged: a direct store read.
	row, err := h.st.GetWebhook(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Secret, row.Secret)

	// PUT with an unknown event name is a 400.
	rec = doWebhook(t, h, sess, http.MethodPut, "/api/v1/webhooks/"+created.ID, map[string]any{
		"name": "x", "url": "https://discord.com/api/webhooks/1/abc",
		"events": []string{"job.finished"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// POST with a bad URL is a 400.
	rec = doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"name": "x", "url": "no-scheme", "events": []string{"job.done"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// DELETE returns 204 and the hook is gone.
	rec = doWebhook(t, h, sess, http.MethodDelete, "/api/v1/webhooks/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/"+created.ID, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	// Delete of an unknown id is a 404.
	rec = doWebhook(t, h, sess, http.MethodDelete, "/api/v1/webhooks/nope", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// Webhooks are admin only: without a session every endpoint answers 401.
func TestWebhookEndpointsRequireAdmin(t *testing.T) {
	h, _ := webhookHarness(t)
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/webhooks"},
		{http.MethodPost, "/api/v1/webhooks"},
		{http.MethodGet, "/api/v1/webhooks/x"},
		{http.MethodPut, "/api/v1/webhooks/x"},
		{http.MethodDelete, "/api/v1/webhooks/x"},
		{http.MethodPost, "/api/v1/webhooks/x/test"},
		{http.MethodGet, "/api/v1/webhooks/x/deliveries"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.method == http.MethodPost || tc.method == http.MethodPut {
			raw, _ := json.Marshal(validBody())
			req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader(raw))
		}
		rec := h.doReq(req)
		require.Equal(t, http.StatusUnauthorized, rec.Code, tc.method+" "+tc.path)
	}
}

// TestWebhookTestEndpoint covers acceptance item 7: the Test button fires
// one delivery with the sample payload against the configured URL, a
// single attempt, and reports the remote's answer.
func TestWebhookTestEndpoint(t *testing.T) {
	h, sess := webhookHarness(t)

	var mu sync.Mutex
	var bodies []string
	var headers http.Header
	var count int
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		headers = r.Header.Clone()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	rec := doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"name": "Discord", "url": remote.URL, "events": []string{"job.done", "job.failed"},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))

	rec = doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks/"+created.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result struct {
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	require.NoError(t, jsonUnmarshal(rec, &result))
	assert.True(t, result.OK)
	assert.Equal(t, http.StatusNoContent, result.Status)
	assert.Empty(t, result.Error)

	// One attempt, the fixed sample payload, signed with the hook's secret.
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, count, "the test call never retries")
	require.Len(t, bodies, 1)
	assert.Contains(t, bodies[0], `"event":"job.done"`)
	assert.Contains(t, bodies[0], `"file_name":"The Matrix (1999).mkv"`)
	assert.Equal(t, "job.done", headers.Get("X-SYS-Event"))

	// The signature verifies with the create-time secret.
	mac := hmac.New(sha256.New, []byte(created.Secret))
	mac.Write([]byte(bodies[0]))
	assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), headers.Get("X-SYS-Signature"))
}

// A failing remote is reported through ok=false and the response keeps 200.
func TestWebhookTestEndpointFailure(t *testing.T) {
	h, sess := webhookHarness(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(remote.Close)

	rec := doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"name": "hook", "url": remote.URL, "events": []string{"job.done"},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))

	rec = doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks/"+created.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result struct {
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	require.NoError(t, jsonUnmarshal(rec, &result))
	assert.False(t, result.OK)
	assert.Equal(t, http.StatusInternalServerError, result.Status)
	assert.Contains(t, result.Error, "unexpected status")

	// The recorded delivery row carries the remote's body head.
	row, err := h.st.GetWebhook(context.Background(), created.ID)
	require.NoError(t, err)
	dels, err := h.st.ListWebhookDeliveries(context.Background(), row.ID, 0)
	require.NoError(t, err)
	require.Len(t, dels, 1)
	assert.Contains(t, dels[0].Error, "boom")
	assert.Equal(t, http.StatusInternalServerError, dels[0].Status)

	// A transport failure is reported the same way.
	rec = doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"name": "hook", "url": "http://127.0.0.1:1", "events": []string{"job.done"},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &created))
	rec = doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks/"+created.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &result))
	assert.False(t, result.OK)
	assert.Equal(t, 0, result.Status)
	assert.NotEmpty(t, result.Error)
}

// TestWebhookDeliveriesEndpoint covers acceptance item 8: the deliveries
// endpoint lists a hook's recorded outcomes, newest first.
func TestWebhookDeliveriesEndpoint(t *testing.T) {
	h, sess := webhookHarness(t)

	// Create a hook and record two deliveries directly in the store (the
	// dispatcher's own recording is covered by the server tests).
	rec := doWebhook(t, h, sess, http.MethodPost, "/api/v1/webhooks", validBody())
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))

	d1 := &store.WebhookDelivery{WebhookID: created.ID, Event: store.WebhookEventJobDone, Status: 500, Error: "boom"}
	require.NoError(t, h.st.AddWebhookDelivery(context.Background(), d1))
	d2 := &store.WebhookDelivery{WebhookID: created.ID, Event: store.WebhookEventJobFailed, Status: 204}
	require.NoError(t, h.st.AddWebhookDelivery(context.Background(), d2))

	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/"+created.ID+"/deliveries", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page struct {
		Items []struct {
			Event  string `json:"event"`
			Status int    `json:"status"`
			Error  string `json:"error,omitempty"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, jsonUnmarshal(rec, &page))
	require.Len(t, page.Items, 2)
	assert.Equal(t, store.WebhookEventJobFailed, page.Items[0].Event, "newest first")
	assert.Equal(t, 204, page.Items[0].Status)
	assert.Equal(t, store.WebhookEventJobDone, page.Items[1].Event)
	assert.Equal(t, 500, page.Items[1].Status)
	assert.Equal(t, "boom", page.Items[1].Error)

	// limit trims the page.
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/"+created.ID+"/deliveries?limit=1", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, store.WebhookEventJobFailed, page.Items[0].Event)

	// An out-of-range limit is a 400.
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/"+created.ID+"/deliveries?limit=99", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// Unknown hook is a 404.
	rec = doWebhook(t, h, sess, http.MethodGet, "/api/v1/webhooks/nope/deliveries", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
