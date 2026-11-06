// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Profile endpoints (task 71, spec 5.3): an encode profile is a named, reusable
// graph with a tier and a result policy. Libraries reference one (task 73
// wires the library side). Usage counts come from the libraries table; the
// delete guard is the store's, so a referenced profile is refused atomically.

// profileBody is the POST and PUT request body. PUT replaces every field.
type profileBody struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Tier         string     `json:"tier"`
	ResultPolicy string     `json:"result_policy"`
	Graph        flow.Graph `json:"graph"`
}

// profileList is the GET /api/v1/profiles body.
type profileList struct {
	Items []server.ProfileDTO `json:"items"`
	Total int                 `json:"total"`
}

// profileUsers maps each profile id to the names of the libraries that
// reference it, in library order. One read of the libraries table serves every
// profile in a request.
func (a *API) profileUsers(ctx context.Context) (map[string][]string, error) {
	libs, err := a.st.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	users := map[string][]string{}
	for _, lib := range libs {
		if lib.ProfileID != "" {
			users[lib.ProfileID] = append(users[lib.ProfileID], lib.Name)
		}
	}
	return users, nil
}

// profileNameTaken reports whether another profile (not exceptID) has name.
func (a *API) profileNameTaken(ctx context.Context, name, exceptID string) (bool, error) {
	list, err := a.st.ListProfiles(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range list {
		if p.Name == name && p.ID != exceptID {
			return true, nil
		}
	}
	return false, nil
}

// profileFromBody validates a profile body and returns the store row it
// describes (ID and CreatedAt left for the caller). It writes the 400 or 409
// reply itself and reports false when the body is refused. Graph problems are
// the editor's field errors (400 invalid_flow), the same shape the library flow
// endpoint answers.
func (a *API) profileFromBody(w http.ResponseWriter, r *http.Request, body profileBody, exceptID string) (*store.Profile, bool) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "a profile needs a name")
		return nil, false
	}
	tier, err := flow.ParseTier(body.Tier)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return nil, false
	}
	if !validResultPolicy(body.ResultPolicy) {
		writeError(w, http.StatusBadRequest, "invalid_request",
			fmt.Sprintf("invalid result_policy %q: want one of %v", body.ResultPolicy, store.ResultPolicyValues()))
		return nil, false
	}
	errs, ok := a.flowErrors(w, r, body.Graph)
	if !ok {
		return nil, false
	}
	if len(errs) > 0 {
		writeFlowErrors(w, errs)
		return nil, false
	}
	taken, err := a.profileNameTaken(r.Context(), name, exceptID)
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	if taken {
		writeError(w, http.StatusConflict, "name_taken", fmt.Sprintf("a profile named %q already exists", name))
		return nil, false
	}
	raw, err := json.Marshal(body.Graph)
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return &store.Profile{
		Name:         name,
		Description:  body.Description,
		Tier:         string(tier),
		Graph:        string(raw),
		ResultPolicy: body.ResultPolicy,
	}, true
}

// validResultPolicy reports whether v is one of the stored result policies.
func validResultPolicy(v string) bool {
	for _, ok := range store.ResultPolicyValues() {
		if v == ok {
			return true
		}
	}
	return false
}

// profileReply writes p as a profile DTO with the given status.
func (a *API) profileReply(w http.ResponseWriter, r *http.Request, status int, p *store.Profile) {
	users, err := a.profileUsers(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	dto, err := server.NewProfileDTO(p, users[p.ID])
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, status, dto)
}

// listProfiles answers GET /api/v1/profiles: every profile, oldest first.
func (a *API) listProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.ListProfiles(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	users, err := a.profileUsers(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]server.ProfileDTO, 0, len(list))
	for _, p := range list {
		dto, err := server.NewProfileDTO(p, users[p.ID])
		if err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, dto)
	}
	writeJSON(w, http.StatusOK, profileList{Items: items, Total: len(items)})
}

// getProfile answers GET /api/v1/profiles/{id}. With ?tier= the graph is judged
// at that tier (task 72, the same as GET /libraries/{id}/flow?tier=) so each
// editor can show its own lock; without it the profile's own tier is used.
func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := a.profile(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("tier")
	if q == "" {
		a.profileReply(w, r, http.StatusOK, p)
		return
	}
	tier, err := flow.ParseTier(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	users, err := a.profileUsers(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	dto, err := server.NewProfileDTO(p, users[p.ID])
	if err != nil {
		a.internal(w, err)
		return
	}
	dto.Unrepresentable = flow.RepresentableAt(dto.Graph, tier)
	if dto.Unrepresentable == nil {
		dto.Unrepresentable = []flow.FieldError{}
	}
	writeJSON(w, http.StatusOK, dto)
}

// createProfile answers POST /api/v1/profiles: 201 with the new profile.
func (a *API) createProfile(w http.ResponseWriter, r *http.Request) {
	body, err := decode[profileBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	p, ok := a.profileFromBody(w, r, body, "")
	if !ok {
		return
	}
	if err := a.st.CreateProfile(r.Context(), p); err != nil {
		a.internal(w, err)
		return
	}
	a.profileReply(w, r, http.StatusCreated, p)
}

// updateProfile answers PUT /api/v1/profiles/{id}: a full replace. Libraries
// using the profile see the new behaviour at once, which is the point of a
// shared profile (spec 5.3); the reply's used_by_names says who that is.
func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.profile(w, r)
	if !ok {
		return
	}
	body, err := decode[profileBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	p, ok := a.profileFromBody(w, r, body, existing.ID)
	if !ok {
		return
	}
	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt
	switch err := a.st.UpdateProfile(r.Context(), p); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such profile")
		return
	case err != nil:
		a.internal(w, err)
		return
	}
	a.profileReply(w, r, http.StatusOK, p)
}

// deleteProfile answers DELETE /api/v1/profiles/{id}: 204, or 409 naming the
// libraries that still use it. A referenced profile is never removed.
func (a *API) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := a.st.DeleteProfile(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such profile")
		return
	case errors.Is(err, store.ErrProfileInUse):
		users, uerr := a.profileUsers(r.Context())
		if uerr != nil {
			a.internal(w, uerr)
			return
		}
		names := users[id]
		writeError(w, http.StatusConflict, "profile_in_use",
			fmt.Sprintf("profile is used by %d libraries: %s", len(names), strings.Join(names, ", ")))
		return
	case err != nil:
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// duplicateProfile answers POST /api/v1/profiles/{id}/duplicate: a new profile
// with the same graph, tier, policy and description, named "<name> (copy)" or
// the next free "<name> (copy N)". It starts with no libraries, so editing the
// copy leaves the original's behaviour alone.
func (a *API) duplicateProfile(w http.ResponseWriter, r *http.Request) {
	src, ok := a.profile(w, r)
	if !ok {
		return
	}
	list, err := a.st.ListProfiles(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	taken := map[string]bool{}
	for _, p := range list {
		taken[p.Name] = true
	}
	name := src.Name + " (copy)"
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s (copy %d)", src.Name, n)
	}
	dup := &store.Profile{
		Name:         name,
		Description:  src.Description,
		Tier:         src.Tier,
		Graph:        src.Graph,
		ResultPolicy: src.ResultPolicy,
	}
	if err := a.st.CreateProfile(r.Context(), dup); err != nil {
		a.internal(w, err)
		return
	}
	a.profileReply(w, r, http.StatusCreated, dup)
}

// profile loads the profile named by the path id, or writes the 404 and
// reports false.
func (a *API) profile(w http.ResponseWriter, r *http.Request) (*store.Profile, bool) {
	p, err := a.st.GetProfile(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such profile")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return p, true
}
