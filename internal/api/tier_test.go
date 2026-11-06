// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/stretchr/testify/require"
)

// advancedTierGraph is an advanced-only graph: a condition, a scale and a
// raw_args escape hatch on top of an easy-shaped encode. Basic cannot edit it,
// moderate cannot edit the raw_args step, advanced can edit all of it.
func advancedTierGraph() flow.Graph {
	return flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "if_bitrate_above", Type: flow.StepIfBitrateAbove, Params: rawParams(`{"mbps":8}`)},
			{ID: "transcode_video", Type: flow.StepTranscodeVideo, Params: rawParams(`{"codec":"hevc","quality":3,"preset":"slow"}`)},
			{ID: "scale", Type: flow.StepScale, Params: rawParams(`{"height":1080}`)},
			{ID: "raw", Type: flow.StepRawArgs, Params: rawParams(`{"stage":"output","args":"-threads 2"}`)},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: rawParams(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "if_bitrate_above"},
			{From: "if_bitrate_above", To: "transcode_video"},
			{From: "transcode_video", To: "scale"},
			{From: "scale", To: "raw"},
			{From: "raw", To: "remux_container"},
		},
	}
}

// profileWith stores a profile carrying g at tier and returns its DTO.
func (h *apiHarness) profileWith(name, tier string, g flow.Graph) server.ProfileDTO {
	h.t.Helper()
	return h.createProfileOK(name, tier, g)
}

// libraryOn creates a library that follows the given profile.
func (h *apiHarness) libraryOn(profileID string) server.LibraryDTO {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root, "profile_id": profileID,
	})
	require.Equal(h.t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &dto))
	return dto
}

// putProfileTier saves the profile at a new tier with the same graph and
// policy, the only edit the tier tests make.
func (h *apiHarness) putProfileTier(p server.ProfileDTO, tier string) *httptest.ResponseRecorder {
	return h.do(http.MethodPut, "/api/v1/profiles/"+p.ID, profileReq(p.Name, tier, p.ResultPolicy, p.Graph))
}

// TestProfileTierDowngradeKeepsGraphByteIdentical is the spec's proof for
// decision 4 under profiles: an advanced graph stays exactly as stored through
// every tier change, downgrades included. Only the tier moves.
func TestProfileTierDowngradeKeepsGraphByteIdentical(t *testing.T) {
	h := newAPIHarness(t)
	p := h.profileWith("Shrink", "advanced", advancedTierGraph())
	lib := h.libraryOn(p.ID)
	stored := string(mustJSON(t, h.libraryDTO(lib.ID).Flow))

	for _, tier := range []string{"basic", "moderate", "advanced"} {
		rec := h.putProfileTier(p, tier)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, stored, string(mustJSON(t, h.libraryDTO(lib.ID).Flow)), "tier %s rewrote the graph", tier)
		require.Equal(t, tier, h.libraryDTO(lib.ID).RuleTier)
	}

	// Reads never write either.
	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow?tier=basic", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, string(mustJSON(t, h.libraryDTO(lib.ID).Flow)))
}

// TestProfileTierChangeReachesTheLibraryFlow: the profile's downgrade shows on
// the library: its own tier is the profile's, and the flow reply judges the
// same graph at that tier, listing the steps the tier cannot express.
func TestProfileTierChangeReachesTheLibraryFlow(t *testing.T) {
	h := newAPIHarness(t)
	p := h.profileWith("Shrink", "advanced", advancedTierGraph())
	lib := h.libraryOn(p.ID)

	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var own flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &own))
	require.Equal(t, "advanced", own.Tier)
	require.Empty(t, own.Unrepresentable)

	require.Equal(t, http.StatusOK, h.putProfileTier(p, "moderate").Code)
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var moderate flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &moderate))
	require.Equal(t, "moderate", moderate.Tier)
	require.Len(t, moderate.Unrepresentable, 1)
	require.Equal(t, "nodes[4].type", moderate.Unrepresentable[0].Path)
	require.Contains(t, moderate.Unrepresentable[0].Message, `step "raw"`)
	require.Equal(t, "moderate", h.libraryDTO(lib.ID).RuleTier)
}

// TestGetFlowTierQuery: ?tier= judges the graph at that tier; without it the
// profile's tier is used; a bad value is 400.
func TestGetFlowTierQuery(t *testing.T) {
	h := newAPIHarness(t)
	p := h.profileWith("Shrink", "advanced", advancedTierGraph())
	lib := h.libraryOn(p.ID)

	// Own tier (advanced): nothing is unrepresentable.
	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var own flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &own))
	require.Equal(t, "advanced", own.Tier)
	require.NotNil(t, own.Unrepresentable)
	require.Empty(t, own.Unrepresentable)

	// The same graph judged at basic lists what basic cannot edit.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow?tier=basic", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var basic flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &basic))
	require.Equal(t, "basic", basic.Tier)
	require.NotEmpty(t, basic.Unrepresentable)
	require.Equal(t, "advanced", h.libraryDTO(lib.ID).RuleTier, "a read does not change the tier")
	require.JSONEq(t, string(mustJSON(t, own.Graph)), string(mustJSON(t, basic.Graph)), "the graph is the same whatever tier judges it")

	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow?tier=expert", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

// TestNewLibraryFollowsFirstProfileTier (task 74): a library saved without a
// profile follows the first profile, and its tier is that profile's. The
// default_rule_tier setting only seeds a fresh install's first profile; a later
// settings change leaves existing libraries alone.
func TestNewLibraryFollowsFirstProfileTier(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/settings", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"default_rule_tier":"basic"`)

	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_rule_tier": "expert"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	first := h.createLibrary(false)
	list := h.do(http.MethodGet, "/api/v1/profiles", nil)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var profiles profileList
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &profiles))
	require.NotEmpty(t, profiles.Items)
	require.Equal(t, profiles.Items[0].ID, first.ProfileID, "no profile named: the first profile is followed")
	require.Equal(t, profiles.Items[0].Tier, first.RuleTier)

	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_rule_tier": "advanced"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, first.RuleTier, h.libraryDTO(first.ID).RuleTier, "a settings change must not move existing libraries")

	// A save that renames the library keeps its profile and tier.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+first.ID, map[string]any{
		"name": "Films", "path": h.root,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	after := h.libraryDTO(first.ID)
	require.Equal(t, first.ProfileID, after.ProfileID)
	require.Equal(t, first.RuleTier, after.RuleTier)
}

func (h *apiHarness) libraryDTO(id string) server.LibraryDTO {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/api/v1/libraries/"+id, nil)
	require.Equal(h.t, http.StatusOK, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &dto))
	return dto
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}
