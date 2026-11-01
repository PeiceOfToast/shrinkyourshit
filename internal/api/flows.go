// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Flow endpoints (API.md, task 14): the rule graph of a library — its
// single source of truth for what the worker runs — plus the shared
// validation and palette services the UI never reimplements.

// flowResponse is the GET /libraries/{id}/flow body and the PUT reply.
type flowResponse struct {
	LibraryID string     `json:"library_id"`
	Source    string     `json:"source"`
	Graph     flow.Graph `json:"graph"`
}

// getLibraryFlow answers GET /api/v1/libraries/{id}/flow: the stored graph
// plus its source. Legacy libraries (no stored flow) get the flow compiled
// from their Target, so the response never lies about what runs.
func (a *API) getLibraryFlow(w http.ResponseWriter, r *http.Request) {
	lib, ok := a.library(w, r)
	if !ok {
		return
	}
	graph, source, err := server.LibraryFlow(lib)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, flowResponse{
		LibraryID: lib.ID,
		Source:    source,
		Graph:     graph,
	})
}

// putFlowBody is the PUT /api/v1/libraries/{id}/flow request body: the
// graph plus the tier that produced it (easy, steps or advanced).
type putFlowBody struct {
	Source string     `json:"source"`
	Graph  flow.Graph `json:"graph"`
}

// putLibraryFlow validates and stores a graph, then re-evaluates the
// library's files against it. An invalid graph never saves: the reply is
// 400 invalid_flow with the field errors, so the UI can attach them to rows.
func (a *API) putLibraryFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	body, err := decode[putFlowBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	lib, err := a.svc.Libraries().UpdateFlow(r.Context(), id, body.Source, body.Graph)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	case server.IsInvalidFlow(err):
		flowErr := &server.ErrInvalidFlow{}
		_ = errors.As(err, &flowErr)
		writeFlowErrors(w, flowErr.Errors)
		return
	case server.IsPluginStepError(err):
		// A plugin step names a plugin that is not installed, is disabled
		// or is not a step plugin (task 32): 422 naming the plugin and the
		// reason, distinct from a structural flow error (400 invalid_flow).
		pe := &server.ErrPluginStep{}
		_ = errors.As(err, &pe)
		writeError(w, http.StatusUnprocessableEntity, "plugin_unavailable", pe.Error())
		return
	case err != nil:
		if strings.Contains(err.Error(), "invalid flow source") {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		a.internal(w, err)
		return
	}
	graph, source, gerr := server.LibraryFlow(lib)
	if gerr != nil {
		a.internal(w, gerr)
		return
	}
	writeJSON(w, http.StatusOK, flowResponse{
		LibraryID: lib.ID,
		Source:    source,
		Graph:     graph,
	})
}

// validateFlowBody is the POST /api/v1/flows/validate request.
type validateFlowBody struct {
	Graph flow.Graph `json:"graph"`
}

// validateFlow answers POST /api/v1/flows/validate: 200 {"valid": true} or
// 400 invalid_flow with field errors. It never saves anything.
func (a *API) validateFlow(w http.ResponseWriter, r *http.Request) {
	body, err := decode[validateFlowBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if errs := flow.Validate(body.Graph); len(errs) > 0 {
		writeFlowErrors(w, errs)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

// stepTypes answers GET /api/v1/flows/step-types: the palette with
// parameter schemas, so the UI never hardcodes the step list twice.
func (a *API) stepTypes(w http.ResponseWriter, r *http.Request) {
	types := a.stepTypesFor(r.Context())
	writeJSON(w, http.StatusOK, server.ListResponse[flow.StepTypeInfo]{
		Items: types,
		Total: len(types),
	})
}

// stepTypesFor builds the palette and fills the plugin step's select with
// the installed, enabled step plugins (task 32). The flow package stays
// pure — the store lives here. An install-store failure degrades to an
// empty option list: the editor then explains how to install a plugin
// instead of listing one.
func (a *API) stepTypesFor(ctx context.Context) []flow.StepTypeInfo {
	types := flow.StepTypes()
	rows, err := a.svc.Plugins().List(ctx)
	if err != nil {
		return types
	}
	var names []string
	for _, row := range rows {
		if row.Enabled && row.Type == store.PluginTypeStep {
			names = append(names, row.Name)
		}
	}
	for i := range types {
		if types[i].Type != flow.StepPlugin {
			continue
		}
		for j := range types[i].Params {
			if types[i].Params[j].Kind == "plugin" {
				types[i].Params[j].Options = names
			}
		}
	}
	return types
}

// previewBody is the POST /api/v1/flows/preview request: either the easy
// input (compiled with the current settings) or a graph.
type previewBody struct {
	Easy   *flow.EasyModeInput `json:"easy"`
	Graph  *flow.Graph         `json:"graph"`
}

// previewResponse is the POST /api/v1/flows/preview reply: the canonical
// graph and one plain-language line per compiled step, the teaching
// surface for the step stack.
type previewResponse struct {
	Graph flow.Graph          `json:"graph"`
	Steps []previewStep       `json:"steps"`
}

type previewStep struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Plain string `json:"plain"`
}

// previewFlow compiles an easy input or a graph and returns the canonical
// graph plus plain-language lines — one per step — without saving anything.
// The server owns the compilation so the UI never duplicates it.
func (a *API) previewFlow(w http.ResponseWriter, r *http.Request) {
	body, err := decode[previewBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var graph flow.Graph
	switch {
	case body.Easy != nil && body.Graph != nil:
		writeError(w, http.StatusBadRequest, "invalid_request", "provide either easy or graph, not both")
		return
	case body.Easy != nil:
		settings, err := a.svc.Settings().Get(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		target, err := flow.CompileEasyMode(*body.Easy, flow.EasySettings{
			Codec:              settings.DefaultVideoCodec,
			PreferredLanguages: settings.PreferredLanguages,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		graph, err = flow.TargetToFlow(target)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	case body.Graph != nil:
		graph = *body.Graph
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "provide easy or graph")
		return
	}
	if errs := flow.Validate(graph); len(errs) > 0 {
		writeFlowErrors(w, errs)
		return
	}
	plan, err := flow.Compile(graph)
	if err != nil {
		writeFlowErrors(w, []flow.FieldError{{Path: "graph", Message: err.Error()}})
		return
	}
	lines := plan.Describe()
	steps := make([]previewStep, 0, len(lines))
	for i, line := range lines {
		step := previewStep{Plain: line}
		if i < len(plan.Steps) {
			step.ID = plan.Steps[i].ID
			step.Type = plan.Steps[i].Type
		}
		steps = append(steps, step)
	}
	writeJSON(w, http.StatusOK, previewResponse{Graph: graph, Steps: steps})
}

// writeFlowErrors writes the 400 invalid_flow body.
func writeFlowErrors(w http.ResponseWriter, errs []flow.FieldError) {
	raw, _ := json.Marshal(errs)
	_ = raw
	writeJSON(w, http.StatusBadRequest, struct {
		Error  string            `json:"error"`
		Code   string            `json:"code"`
		Errors []flow.FieldError `json:"errors"`
	}{Error: "the flow has errors", Code: "invalid_flow", Errors: errs})
}
