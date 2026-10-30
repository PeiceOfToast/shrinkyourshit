// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/stretchr/testify/require"
)

// validFlow is a small valid graph used by the round trip test.
func validFlow() flow.Graph {
	return flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "transcode_video", Type: flow.StepTranscodeVideo, Params: rawParams(`{"codec":"hevc","quality":3}`)},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: rawParams(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "transcode_video"},
			{From: "transcode_video", To: "remux_container"},
		},
	}
}

func rawParams(s string) []byte { return []byte(s) }

// TestFlowRoundTrip: write a flow via the API, read it back byte
// identical, and the library reports source correctly (task 14 acceptance).
func TestFlowRoundTrip(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	sent := validFlow()
	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "steps", "graph": sent,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Read it back: the graph must round trip byte identical and the
	// source must be what was stored.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "steps", got.Source)
	sentJSON, err := json.Marshal(sent)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(got.Graph)
	require.NoError(t, err)
	require.JSONEq(t, string(sentJSON), string(gotJSON))

	// The library DTO reports the same flow and source.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.NotNil(t, dto.Flow)
	require.Equal(t, "steps", dto.FlowSource)

	// PUT with an invalid source is a 400.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "wat", "graph": sent,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")

	// PUT with an invalid graph never saves: 400 invalid_flow with errors.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "steps", "graph": flow.Graph{Schema: flow.FlowSchema},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_flow")

	// The stored flow survived the failed PUT.
	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "steps", got.Source)
}

// TestFlowValidate: 200 for a valid graph; 400 invalid_flow with a field
// error for a cycle, a dangling edge, an unknown step type and a missing
// required parameter (task 14 acceptance).
func TestFlowValidate(t *testing.T) {
	h := newAPIHarness(t)

	post := func(graph flow.Graph) (int, string) {
		rec := h.do(http.MethodPost, "/api/v1/flows/validate", map[string]any{"graph": graph})
		return rec.Code, rec.Body.String()
	}

	code, body := post(validFlow())
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, `"valid":true`)

	// A cycle that keeps an entry: probe -> transcode -> remux -> back ->
	// transcode loops back onto an existing step.
	cycle := validFlow()
	cycle.Nodes = append(cycle.Nodes, flow.Step{ID: "back", Type: flow.StepSkipIfSmaller})
	cycle.Edges = append(cycle.Edges, flow.Edge{From: "remux_container", To: "back"}, flow.Edge{From: "back", To: "transcode_video"})
	code, body = post(cycle)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "invalid_flow")
	require.Contains(t, body, "loop")

	// A dangling edge.
	dangling := validFlow()
	dangling.Edges = append(dangling.Edges, flow.Edge{From: "probe", To: "ghost"})
	code, body = post(dangling)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "edges[2].to")
	require.Contains(t, body, "ghost")

	// An unknown step type.
	unknown := validFlow()
	unknown.Nodes = append(unknown.Nodes, flow.Step{ID: "wat", Type: "wat"})
	unknown.Edges = append(unknown.Edges, flow.Edge{From: "remux_container", To: "wat"})
	code, body = post(unknown)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "unknown step type")

	// A missing required parameter.
	missing := validFlow()
	missing.Nodes[1].Params = rawParams(`{"quality":3}`)
	code, body = post(missing)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "params.codec")
}

// TestFlowStepTypesAndPreview covers the palette and the preview endpoint
// the easy screen's "what this does" panel and the step stack UI use.
func TestFlowStepTypesAndPreview(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/flows/step-types", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var types server.ListResponse[flow.StepTypeInfo]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &types))
	require.Positive(t, types.Total)
	require.Equal(t, types.Total, len(types.Items))
	for _, st := range types.Items {
		require.NotEmpty(t, st.Type)
		require.NotEmpty(t, st.Label)
	}

	// Preview from the easy input: the server compiles it with the
	// current settings.
	rec = h.do(http.MethodPost, "/api/v1/flows/preview", map[string]any{
		"easy": map[string]any{"quality": 3, "remove_subtitles": false, "audio": 1},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var preview previewResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.NotEmpty(t, preview.Steps)
	require.Equal(t, flow.FlowSchema, preview.Graph.Schema)
	for _, s := range preview.Steps {
		require.NotEmpty(t, s.Plain)
	}

	// Preview of a graph: lines, no save.
	graph := validFlow()
	rec = h.do(http.MethodPost, "/api/v1/flows/preview", map[string]any{"graph": graph})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.Len(t, preview.Steps, 3)

	// Both easy and graph is a 400.
	rec = h.do(http.MethodPost, "/api/v1/flows/preview", map[string]any{
		"easy":  map[string]any{"quality": 3, "remove_subtitles": false, "audio": 1},
		"graph": graph,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Neither is a 400.
	rec = h.do(http.MethodPost, "/api/v1/flows/preview", map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestLegacyLibraryFlow: a library row written before the flow columns
// (target only, empty flow) still answers GET /flow with the compiled
// graph and the inferred source.
func TestLegacyLibraryFlow(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	// Strip the flow: a legacy row.
	got, err := h.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	got.FlowJSON = ""
	got.FlowSource = ""
	require.NoError(t, h.st.UpdateLibrary(context.Background(), got))

	rec := h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var fr flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fr))
	require.Equal(t, flow.FlowSchema, fr.Graph.Schema)
	require.NotEmpty(t, fr.Graph.Nodes)
	require.Equal(t, flow.FlowSourceAdvanced, fr.Source)
}
