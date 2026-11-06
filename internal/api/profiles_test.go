// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// profileDefaultGraph is the graph the boot seed uses: the default Target
// compiled to a flow. It is basic-representable.
func profileDefaultGraph(t *testing.T) flow.Graph {
	t.Helper()
	g, err := flow.TargetToFlow(flow.Default())
	require.NoError(t, err)
	return g
}

// profileModerateGraph is advancedTierGraph without its raw_args step: it
// needs the moderate tier (scale, rate-free encode with a preset) and no
// escape hatch, so it saves with the raw args switch off.
func profileModerateGraph() flow.Graph {
	return flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "transcode_video", Type: flow.StepTranscodeVideo, Params: rawParams(`{"codec":"hevc","quality":3,"preset":"slow"}`)},
			{ID: "scale", Type: flow.StepScale, Params: rawParams(`{"height":1080}`)},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: rawParams(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "transcode_video"},
			{From: "transcode_video", To: "scale"},
			{From: "scale", To: "remux_container"},
		},
	}
}

// profileBody builds a POST or PUT body.
func profileReq(name, tier, policy string, g flow.Graph) map[string]any {
	return map[string]any{"name": name, "description": "desc " + name, "tier": tier, "result_policy": policy, "graph": g}
}

// decodeProfileDTO reads a profile response body.
func decodeProfileDTO(t *testing.T, rec *httptest.ResponseRecorder) server.ProfileDTO {
	t.Helper()
	var dto server.ProfileDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto), rec.Body.String())
	return dto
}

// createProfileOK POSTs a profile and requires 201.
func (h *apiHarness) createProfileOK(name, tier string, g flow.Graph) server.ProfileDTO {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/profiles", profileReq(name, tier, store.ResultPolicyReplace, g))
	require.Equal(h.t, http.StatusCreated, rec.Code, rec.Body.String())
	return decodeProfileDTO(h.t, rec)
}

// createNamedLibrary POSTs a library with its own folder, so tests can hold
// more than one library.
func (h *apiHarness) createNamedLibrary(name string) server.LibraryDTO {
	h.t.Helper()
	dir := filepath.Join(h.t.TempDir(), name)
	require.NoError(h.t, os.MkdirAll(dir, 0o755))
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": name, "path": dir, "watch": false,
		"target": flow.Default(), "result_policy": store.ResultPolicyReplaceIfSmaller,
	})
	require.Equal(h.t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &dto))
	return dto
}

// profileFor reads one profile through the API.
func (h *apiHarness) profileFor(id string) server.ProfileDTO {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/api/v1/profiles/"+id, nil)
	require.Equal(h.t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeProfileDTO(h.t, rec)
}

// TestFreshInstallSeedsOneDefaultProfile covers spec 5.2's fresh install step:
// the boot seed gives the library form one profile, named for the default tier,
// and the seed graph is representable at that tier.
func TestFreshInstallSeedsOneDefaultProfile(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/profiles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list struct {
		Items []server.ProfileDTO `json:"items"`
		Total int                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	require.Len(t, list.Items, 1)

	seed := list.Items[0]
	assert.Equal(t, "Default (basic)", seed.Name)
	assert.Equal(t, "basic", seed.Tier)
	assert.Equal(t, store.ResultPolicyReplaceIfSmaller, seed.ResultPolicy)
	assert.Equal(t, 0, seed.UsedBy)
	assert.Equal(t, []string{}, seed.UsedByNames)
	assert.Empty(t, seed.Unrepresentable, "the seed graph must be editable in the basic tier")
	assert.NotEmpty(t, seed.Graph.Nodes)
}

// TestProfileCRUDAndValidation covers create, read, list, replace and the
// refusals: bad tier, bad policy, duplicate name, bad graph, unknown id.
func TestProfileCRUDAndValidation(t *testing.T) {
	h := newAPIHarness(t)
	created := h.createProfileOK("Films", "moderate", profileModerateGraph())
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "moderate", created.Tier)
	assert.Equal(t, store.ResultPolicyReplace, created.ResultPolicy)
	assert.Equal(t, "desc Films", created.Description)
	assert.Equal(t, 0, created.UsedBy)
	assert.Equal(t, []string{}, created.UsedByNames)
	assert.Equal(t, []flow.FieldError{}, created.Unrepresentable)

	got := h.profileFor(created.ID)
	assert.Equal(t, created.Name, got.Name)
	assert.Equal(t, created.Graph, got.Graph)

	rec := h.do(http.MethodPut, "/api/v1/profiles/"+created.ID,
		profileReq("Films HD", "advanced", store.ResultPolicyKeepBoth, profileModerateGraph()))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	updated := decodeProfileDTO(t, rec)
	assert.Equal(t, "Films HD", updated.Name)
	assert.Equal(t, "advanced", updated.Tier)
	assert.Equal(t, store.ResultPolicyKeepBoth, updated.ResultPolicy)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt, "replace keeps created_at")

	rec = h.do(http.MethodGet, "/api/v1/profiles", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, 2, list.Total, "seed plus the created profile")

	// Refusals.
	rec = h.do(http.MethodPost, "/api/v1/profiles", profileReq("Bad tier", "expert", store.ResultPolicyReplace, profileModerateGraph()))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), `"invalid_request"`)

	rec = h.do(http.MethodPost, "/api/v1/profiles", profileReq("Bad policy", "basic", "move", profileModerateGraph()))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), `"invalid_request"`)

	rec = h.do(http.MethodPost, "/api/v1/profiles", profileReq("", "basic", store.ResultPolicyReplace, profileModerateGraph()))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = h.do(http.MethodPost, "/api/v1/profiles", profileReq("Films HD", "basic", store.ResultPolicyReplace, profileModerateGraph()))
	assert.Equal(t, http.StatusConflict, rec.Code, "a taken name is a conflict")
	assert.Contains(t, rec.Body.String(), `"name_taken"`)

	// Renaming to its own name is not a conflict.
	rec = h.do(http.MethodPut, "/api/v1/profiles/"+created.ID,
		profileReq("Films HD", "moderate", store.ResultPolicyReplace, profileModerateGraph()))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	bad := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes:  []flow.Step{{ID: "probe", Type: flow.StepProbe}},
		Edges:  []flow.Edge{{From: "probe", To: "missing"}},
	}
	rec = h.do(http.MethodPost, "/api/v1/profiles", profileReq("Broken", "advanced", store.ResultPolicyReplace, bad))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"invalid_flow"`)
	assert.Contains(t, rec.Body.String(), `"errors"`)

	rec = h.do(http.MethodGet, "/api/v1/profiles/nope", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = h.do(http.MethodPut, "/api/v1/profiles/nope", profileReq("X", "basic", store.ResultPolicyReplace, profileModerateGraph()))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = h.do(http.MethodDelete, "/api/v1/profiles/nope", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = h.do(http.MethodPost, "/api/v1/profiles/nope/duplicate", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDeleteReferencedProfileReturns409AndChangesNothing is the first
// acceptance item: a referenced profile is refused with 409 and neither the
// profile nor the library reference moves. Once unreferenced it can go.
func TestDeleteReferencedProfileReturns409AndChangesNothing(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	p := h.createProfileOK("Films", "basic", profileDefaultGraph(t))
	require.NoError(t, h.st.SetLibraryProfile(context.Background(), lib.ID, p.ID))

	rec := h.do(http.MethodDelete, "/api/v1/profiles/"+p.ID, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"profile_in_use"`)
	assert.Contains(t, rec.Body.String(), lib.Name, "the refusal names the library")

	after := h.profileFor(p.ID)
	assert.Equal(t, p.Name, after.Name)
	assert.Equal(t, 1, after.UsedBy)
	stored, err := h.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, stored.ProfileID, "the refused delete must not unassign the library")

	require.NoError(t, h.st.SetLibraryProfile(context.Background(), lib.ID, ""))
	rec = h.do(http.MethodDelete, "/api/v1/profiles/"+p.ID, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = h.do(http.MethodGet, "/api/v1/profiles/"+p.ID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestUsedByAccurateAfterCreateRepointAndDelete is the second acceptance item:
// used_by and used_by_names follow a library being assigned, repointed and
// deleted.
func TestUsedByAccurateAfterCreateRepointAndDelete(t *testing.T) {
	h := newAPIHarness(t)
	ctx := context.Background()
	p1 := h.createProfileOK("Films", "basic", profileDefaultGraph(t))
	p2 := h.createProfileOK("Shows", "moderate", profileModerateGraph())

	one := h.createNamedLibrary("One")
	two := h.createNamedLibrary("Two")
	require.NoError(t, h.st.SetLibraryProfile(ctx, one.ID, p1.ID))
	require.NoError(t, h.st.SetLibraryProfile(ctx, two.ID, p1.ID))

	got := h.profileFor(p1.ID)
	assert.Equal(t, 2, got.UsedBy)
	assert.ElementsMatch(t, []string{"One", "Two"}, got.UsedByNames)
	assert.Equal(t, 0, h.profileFor(p2.ID).UsedBy)

	require.NoError(t, h.st.SetLibraryProfile(ctx, two.ID, p2.ID))
	got = h.profileFor(p1.ID)
	assert.Equal(t, 1, got.UsedBy)
	assert.Equal(t, []string{"One"}, got.UsedByNames)
	got = h.profileFor(p2.ID)
	assert.Equal(t, 1, got.UsedBy)
	assert.Equal(t, []string{"Two"}, got.UsedByNames)

	rec := h.do(http.MethodDelete, "/api/v1/libraries/"+one.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	got = h.profileFor(p1.ID)
	assert.Equal(t, 0, got.UsedBy)
	assert.Equal(t, []string{}, got.UsedByNames)

	// A PUT answers with the same counts the GET does.
	rec = h.do(http.MethodPut, "/api/v1/profiles/"+p2.ID,
		profileReq("Shows", "moderate", store.ResultPolicyReplace, profileModerateGraph()))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, decodeProfileDTO(t, rec).UsedBy)
}

// TestDuplicateGetsNewIDAndSameGraph is the third acceptance item: a copy has
// its own id and name, the same graph, tier, policy and description, and no
// libraries. Repeated copies number themselves.
func TestDuplicateGetsNewIDAndSameGraph(t *testing.T) {
	h := newAPIHarness(t)
	src := h.createProfileOK("Films", "moderate", profileModerateGraph())

	rec := h.do(http.MethodPost, "/api/v1/profiles/"+src.ID+"/duplicate", nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	dup := decodeProfileDTO(t, rec)

	assert.NotEqual(t, src.ID, dup.ID)
	assert.Equal(t, "Films (copy)", dup.Name)
	assert.Equal(t, src.Tier, dup.Tier)
	assert.Equal(t, src.ResultPolicy, dup.ResultPolicy)
	assert.Equal(t, src.Description, dup.Description)
	assert.Equal(t, src.Graph, dup.Graph, "the copy has the same graph")
	assert.Equal(t, 0, dup.UsedBy)

	rec = h.do(http.MethodPost, "/api/v1/profiles/"+src.ID+"/duplicate", nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "Films (copy 2)", decodeProfileDTO(t, rec).Name)
}

// TestUnrepresentableIsJudgedAtTheProfilesTier: a profile may be saved in a
// tier that cannot express its graph. The reply lists the steps that tier
// cannot express, and moving the profile up a tier clears the list.
func TestUnrepresentableIsJudgedAtTheProfilesTier(t *testing.T) {
	h := newAPIHarness(t)
	p := h.createProfileOK("Films", "basic", profileModerateGraph())
	require.NotEmpty(t, p.Unrepresentable, "a scale step is not basic")
	// nodes[2] is the scale step: a moderate-only step, named by its path.
	var paths []string
	for _, fe := range p.Unrepresentable {
		paths = append(paths, fe.Path)
	}
	assert.Contains(t, paths, "nodes[2].type")

	rec := h.do(http.MethodPut, "/api/v1/profiles/"+p.ID,
		profileReq("Films", "moderate", store.ResultPolicyReplace, profileModerateGraph()))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, decodeProfileDTO(t, rec).Unrepresentable)
}
