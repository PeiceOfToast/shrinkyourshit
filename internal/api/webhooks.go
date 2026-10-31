// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// webhookDTO is the API shape of one webhook (task 25). The secret never
// leaves the server after the create response: every other response
// carries secret_set instead, and a PUT with an empty secret keeps the
// stored one.
type webhookDTO struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	SecretSet bool     `json:"secret_set"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// toWebhookDTO maps a stored row to the keyless shape.
func toWebhookDTO(row *store.Webhook) webhookDTO {
	in := server.WebhookFromRow(row)
	return webhookDTO{
		ID:        in.ID,
		Name:      in.Name,
		URL:       in.URL,
		Events:    in.Events,
		Enabled:   in.Enabled,
		SecretSet: in.Secret != "",
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// webhookResponse is the POST /webhooks body: the normal DTO plus the
// secret, exactly once. Later responses never carry it again.
type webhookResponse struct {
	webhookDTO
	Secret string `json:"secret"`
}

// webhookBody is the create/update request. On update an empty secret
// means "keep the stored secret" (the form shows (unchanged) in its field).
type webhookBody struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// listWebhooks returns every configured webhook, keyless.
func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.st.ListWebhooks(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]webhookDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toWebhookDTO(row))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[webhookDTO]{Items: items, Total: len(items)})
}

// getWebhook returns one webhook, keyless.
func (a *API) getWebhook(w http.ResponseWriter, r *http.Request) {
	row, ok := a.storedWebhook(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toWebhookDTO(row))
}

// createWebhook validates and stores a new webhook. The server generates
// the signing secret — the create response is the one and only time the
// client sees it.
func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := decode[webhookBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in := server.Webhook{
		Name:    body.Name,
		URL:     body.URL,
		Events:  body.Events,
		Enabled: body.Enabled,
	}
	if err := server.ValidateWebhook(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	secret, err := server.NewWebhookSecret()
	if err != nil {
		a.internal(w, err)
		return
	}
	in.Secret = secret
	row := in.Row()
	if err := a.st.CreateWebhook(r.Context(), row); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, webhookResponse{webhookDTO: toWebhookDTO(row), Secret: secret})
}

// storedWebhook loads one webhook row or writes the 404.
func (a *API) storedWebhook(w http.ResponseWriter, r *http.Request) (*store.Webhook, bool) {
	row, err := a.st.GetWebhook(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such webhook")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return row, true
}

// updateWebhook replaces a webhook. An empty secret keeps the stored one,
// so the form never round-trips the secret.
func (a *API) updateWebhook(w http.ResponseWriter, r *http.Request) {
	row, ok := a.storedWebhook(w, r)
	if !ok {
		return
	}
	body, err := decode[webhookBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in := server.Webhook{
		ID:      row.ID,
		Name:    body.Name,
		URL:     body.URL,
		Secret:  row.Secret,
		Events:  body.Events,
		Enabled: body.Enabled,
	}
	if err := server.ValidateWebhook(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated := in.Row()
	if err := a.st.UpdateWebhook(r.Context(), updated); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookDTO(updated))
}

// deleteWebhook removes the webhook and its delivery history.
func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.storedWebhook(w, r); !ok {
		return
	}
	if err := a.st.DeleteWebhook(r.Context(), r.PathValue("id")); err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testWebhook performs that one webhook's one-shot call with a fixed
// sample job.done payload. A single attempt (an explicit diagnostic); the
// remote's answer decides ok/error; this endpoint always answers 200.
func (a *API) testWebhook(w http.ResponseWriter, r *http.Request) {
	row, ok := a.storedWebhook(w, r)
	if !ok {
		return
	}
	status, err := a.svc.Webhooks().Test(r.Context(), server.WebhookFromRow(row), server.SampleEvent())
	resp := testWebhookResponse{Status: status, OK: err == nil && status >= 200 && status < 300}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// testWebhookResponse is the POST /{id}/test body.
type testWebhookResponse struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

// webhookDeliveries returns a hook's recorded delivery outcomes, newest
// first, at most limit rows (default 50, the prune bound). The endpoint is
// paginated by limit alone: delivery history is bounded anyway.
func (a *API) webhookDeliveries(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.storedWebhook(w, r); !ok {
		return
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > store.WebhookDeliveriesKeep {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 50")
			return
		}
		limit = n
	}
	rows, err := a.st.ListWebhookDeliveries(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]deliveryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDeliveryDTO(row))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[deliveryDTO]{Items: items, Total: len(items)})
}

// deliveryDTO is the API shape of one recorded delivery outcome (task 25):
// status 0 means the request never got an answer, and error carries the
// head of the remote's body or the transport failure text.
type deliveryDTO struct {
	ID     string `json:"id"`
	Event  string `json:"event"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
	At     string `json:"at"`
}

func toDeliveryDTO(row *store.WebhookDelivery) deliveryDTO {
	return deliveryDTO{
		ID:     row.ID,
		Event:  row.Event,
		Status: row.Status,
		Error:  row.Error,
		At:     row.CreatedAt.UTC().Format(time.RFC3339),
	}
}
