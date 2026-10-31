// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// integrationDTO is the API shape of one integration (task 23). The API
// key never leaves the server: a response carries api_key_set instead,
// and a PUT with an empty api_key keeps the stored key.
type integrationDTO struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	APIKeySet     bool   `json:"api_key_set"`
	Enabled       bool   `json:"enabled"`
	RefreshOnDone bool   `json:"refresh_on_done"`
}

func toIntegrationDTO(in server.Integration) integrationDTO {
	return integrationDTO{
		ID:            in.ID,
		Kind:          in.Kind,
		Name:          in.Name,
		BaseURL:       in.BaseURL,
		APIKeySet:     in.APIKey != "",
		Enabled:       in.Enabled,
		RefreshOnDone: in.RefreshOnDone,
	}
}

// integrationBody is the create/update request. On update an empty api_key
// means "keep the stored key" (the form shows (unchanged) in its field).
type integrationBody struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Enabled       bool   `json:"enabled"`
	RefreshOnDone bool   `json:"refresh_on_done"`
}

// listIntegrations returns every configured integration, keyless.
func (a *API) listIntegrations(w http.ResponseWriter, r *http.Request) {
	rows, err := a.svc.Integrations().List(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]integrationDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toIntegrationDTO(row))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[integrationDTO]{Items: items, Total: len(items)})
}

// createIntegration validates and stores a new integration. Validate trims
// the fields in place, so the normalized base_url is what gets stored.
func (a *API) createIntegration(w http.ResponseWriter, r *http.Request) {
	body, err := decode[integrationBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in := server.Integration{
		Kind:          body.Kind,
		Name:          body.Name,
		BaseURL:       body.BaseURL,
		APIKey:        body.APIKey,
		Enabled:       body.Enabled,
		RefreshOnDone: body.RefreshOnDone,
	}
	if err := server.ValidateIntegration(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	row := in.Row()
	if err := a.st.CreateIntegration(r.Context(), row); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toIntegrationDTO(server.IntegrationFromRow(row)))
}

// storedIntegration loads one integration row or writes the 404.
func (a *API) storedIntegration(w http.ResponseWriter, r *http.Request) (*store.Integration, bool) {
	row, err := a.st.GetIntegration(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such integration")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return row, true
}

// updateIntegration replaces an integration. An empty api_key keeps the
// stored key, so the form never round-trips the secret.
func (a *API) updateIntegration(w http.ResponseWriter, r *http.Request) {
	row, ok := a.storedIntegration(w, r)
	if !ok {
		return
	}
	body, err := decode[integrationBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in := server.Integration{
		ID:            row.ID,
		Kind:          body.Kind,
		Name:          body.Name,
		BaseURL:       body.BaseURL,
		APIKey:        body.APIKey,
		Enabled:       body.Enabled,
		RefreshOnDone: body.RefreshOnDone,
	}
	if strings.TrimSpace(in.APIKey) == "" {
		in.APIKey = row.APIKey // "unchanged": the form's placeholder
	}
	if err := server.ValidateIntegration(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated := in.Row()
	if err := a.st.UpdateIntegration(r.Context(), updated); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationDTO(server.IntegrationFromRow(updated)))
}

// deleteIntegration removes the integration and its key.
func (a *API) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.storedIntegration(w, r); !ok {
		return
	}
	if err := a.st.DeleteIntegration(r.Context(), r.PathValue("id")); err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testIntegrationResponse is the POST /{id}/test body: the one-shot call's
// outcome, 200 even when the remote refused.
type testIntegrationResponse struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

// testIntegration performs that one integration's call once. The remote's
// answer decides ok/error; this endpoint always answers 200.
func (a *API) testIntegration(w http.ResponseWriter, r *http.Request) {
	row, ok := a.storedIntegration(w, r)
	if !ok {
		return
	}
	status, err := a.svc.Integrations().Test(r.Context(), server.IntegrationFromRow(row))
	resp := testIntegrationResponse{Status: status, OK: err == nil && status >= 200 && status < 300}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}
