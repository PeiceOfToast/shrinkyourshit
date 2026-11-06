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
// Tier is the editor the answer is judged against (the ?tier= parameter, or
// the library's own tier) and Unrepresentable lists the steps of the graph
// that tier cannot express (task 59). Nothing is rewritten by either.
type flowResponse struct {
	LibraryID       string            `json:"library_id"`
	Source          string            `json:"source"`
	Tier            string            `json:"tier"`
	Graph           flow.Graph        `json:"graph"`
	Unrepresentable []flow.FieldError `json:"unrepresentable"`
}

// flowReply builds the shared flow body for a graph judged at tier.
func flowReply(lib *store.Library, graph flow.Graph, source string, tier flow.Tier) flowResponse {
	unrepresentable := flow.RepresentableAt(graph, tier)
	if unrepresentable == nil {
		unrepresentable = []flow.FieldError{}
	}
	return flowResponse{
		LibraryID:       lib.ID,
		Source:          source,
		Tier:            string(tier),
		Graph:           graph,
		Unrepresentable: unrepresentable,
	}
}

// libraryTier parses the tier of the library's profile (task 74: the tier
// belongs to the profile). A profile-less library has none and is an error.
func libraryTier(lib *store.Library) (flow.Tier, error) {
	return flow.ParseTier(server.LibraryTier(lib))
}

// getLibraryFlow answers GET /api/v1/libraries/{id}/flow: the stored graph
// plus its source. Legacy libraries (no stored flow) get the flow compiled
// from their Target, so the response never lies about what runs. The optional
// ?tier=basic|moderate|advanced judges the graph at that tier; without it the
// library's own tier is used (task 59).
func (a *API) getLibraryFlow(w http.ResponseWriter, r *http.Request) {
	lib, ok := a.library(w, r)
	if !ok {
		return
	}
	graph, source, err := server.LibraryFlow(lib)
	if errors.Is(err, server.ErrNoProfile) {
		writeError(w, http.StatusConflict, "no_profile", err.Error())
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	tier, err := libraryTier(lib)
	if err != nil {
		a.internal(w, err)
		return
	}
	if q := r.URL.Query().Get("tier"); q != "" {
		tier, err = flow.ParseTier(q)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, flowReply(lib, graph, source, tier))
}

// putTierBody is the PUT /api/v1/libraries/{id}/tier request body.
type putTierBody struct {
	Tier string `json:"tier"`
}

// putLibraryTier answers PUT /api/v1/libraries/{id}/tier with 409
// follows_profile. The tier is the profile's (task 74): changing it on the
// library would change nothing that runs, so the request is refused rather
// than silently ignored. The body is still validated first.
func (a *API) putLibraryTier(w http.ResponseWriter, r *http.Request) {
	body, err := decode[putTierBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, err := flow.ParseTier(body.Tier); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, ok := a.library(w, r); !ok {
		return
	}
	writeError(w, http.StatusConflict, "follows_profile", "the tier belongs to the encode profile this library follows; change it with PUT /api/v1/profiles/{id}")
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
	case errors.Is(err, server.ErrFollowsProfile):
		writeError(w, http.StatusConflict, "follows_profile", err.Error())
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
	tier, terr := libraryTier(lib)
	if terr != nil {
		a.internal(w, terr)
		return
	}
	writeJSON(w, http.StatusOK, flowReply(lib, graph, source, tier))
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
	errs, ok := a.flowErrors(w, r, body.Graph)
	if !ok {
		return
	}
	if len(errs) > 0 {
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
	Easy  *flow.EasyModeInput `json:"easy"`
	Graph *flow.Graph         `json:"graph"`
}

// previewResponse is the POST /api/v1/flows/preview reply: the canonical
// graph and one plain-language line per compiled step, the teaching
// surface for the step stack.
type previewResponse struct {
	Graph flow.Graph    `json:"graph"`
	Steps []previewStep `json:"steps"`
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
	errs, ok := a.flowErrors(w, r, graph)
	if !ok {
		return
	}
	if len(errs) > 0 {
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

// flowErrors is the validation the validate and preview endpoints share:
// the structural rules, plus the raw_args refusals while the allow_raw_args
// switch is off (task 58). ok is false when the settings could not be read;
// the reply is already written.
func (a *API) flowErrors(w http.ResponseWriter, r *http.Request, g flow.Graph) ([]flow.FieldError, bool) {
	refused, err := a.svc.Settings().RefusedRawArgs(r.Context(), g)
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return append(flow.Validate(g), refused...), true
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
