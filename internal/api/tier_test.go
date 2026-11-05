// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
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

// storedFlowText is the library's flow_json exactly as the store holds it.
func (h *apiHarness) storedFlowText(id string) string {
	h.t.Helper()
	lib, err := h.st.GetLibrary(context.Background(), id)
	require.NoError(h.t, err)
	return lib.FlowJSON
}

func (h *apiHarness) putTier(id, tier string) *httptest.ResponseRecorder {
	return h.do(http.MethodPut, "/api/v1/libraries/"+id+"/tier", map[string]any{"tier": tier})
}

// TestTierDowngradeKeepsStoredGraphByteIdentical is the spec's proof for
// decision 4: an advanced graph stays exactly as stored through every tier
// change, downgrades included. Only the tier column moves.
func TestTierDowngradeKeepsStoredGraphByteIdentical(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "advanced", "graph": advancedTierGraph(),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored := h.storedFlowText(lib.ID)
	require.NotEmpty(t, stored)

	rec = h.putTier(lib.ID, "advanced")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, h.storedFlowText(lib.ID))

	rec = h.putTier(lib.ID, "basic")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, h.storedFlowText(lib.ID), "downgrade to basic rewrote the stored graph")

	rec = h.putTier(lib.ID, "moderate")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, h.storedFlowText(lib.ID), "tier change to moderate rewrote the stored graph")

	// Reads never write either.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow?tier=basic", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, h.storedFlowText(lib.ID))
}

// TestPutTierReportsUnrepresentableSteps: a downgrade with steps the new tier
// cannot express is still 200, names each step, and changes the tier.
func TestPutTierReportsUnrepresentableSteps(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "advanced", "graph": advancedTierGraph(),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, http.StatusOK, h.putTier(lib.ID, "advanced").Code)

	rec = h.putTier(lib.ID, "moderate")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct {
		Tier            string            `json:"tier"`
		Unrepresentable []flow.FieldError `json:"unrepresentable"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "moderate", got.Tier)
	require.Len(t, got.Unrepresentable, 1)
	require.Equal(t, "nodes[4].type", got.Unrepresentable[0].Path)
	require.Contains(t, got.Unrepresentable[0].Message, `step "raw"`)
	require.Contains(t, got.Unrepresentable[0].Message, "the advanced tier can edit it")

	rec = h.putTier(lib.ID, "basic")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "basic", got.Tier)
	require.Greater(t, len(got.Unrepresentable), 1)

	// The library reports the tier it now has.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.Equal(t, "basic", dto.RuleTier)
}

// TestPutTierEasyShapedIsEmptyList: an easy-shaped graph downgrades to basic
// with an empty (not null) unrepresentable list.
func TestPutTierEasyShapedIsEmptyList(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	rec := h.putTier(lib.ID, "basic")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"tier":"basic","unrepresentable":[]}`, rec.Body.String())
}

// TestPutTierValidation: an unknown tier is 400, an unknown library is 404.
func TestPutTierValidation(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	rec := h.putTier(lib.ID, "expert")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")

	rec = h.putTier(lib.ID, "")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = h.putTier("no-such-library", "basic")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "not_found")
}

// TestGetFlowTierQuery: ?tier= judges the graph at that tier; without it the
// library's own tier is used; a bad value is 400.
func TestGetFlowTierQuery(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "advanced", "graph": advancedTierGraph(),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, http.StatusOK, h.putTier(lib.ID, "advanced").Code)

	// Own tier (advanced): nothing is unrepresentable.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
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
	require.Equal(t, "advanced", own.Tier, "the library's own tier is unchanged by a read")
	require.JSONEq(t, string(mustJSON(t, own.Graph)), string(mustJSON(t, basic.Graph)), "the graph is the same whatever tier judges it")

	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow?tier=expert", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

// TestNewLibraryInheritsDefaultRuleTier: the default_rule_tier setting is the
// tier of a new library; changing it later leaves existing libraries alone,
// and a library save keeps the tier it has.
func TestNewLibraryInheritsDefaultRuleTier(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/settings", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"default_rule_tier":"basic"`)

	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_rule_tier": "moderate"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"default_rule_tier":"moderate"`)

	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_rule_tier": "expert"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	inherited := h.createLibrary(false)
	require.Equal(t, "moderate", inherited.RuleTier)

	rec = h.do(http.MethodPut, "/api/v1/settings", map[string]any{"default_rule_tier": "advanced"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "moderate", h.libraryDTO(inherited.ID).RuleTier, "a settings change must not move existing libraries")

	// A save that renames the library keeps its tier.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+inherited.ID, map[string]any{
		"name": "Films", "path": h.root, "target": flow.Default(), "result_policy": store.ResultPolicyReplaceIfSmaller,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "moderate", h.libraryDTO(inherited.ID).RuleTier)

	fresh := h.createLibrary(false)
	require.Equal(t, "advanced", fresh.RuleTier)
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
