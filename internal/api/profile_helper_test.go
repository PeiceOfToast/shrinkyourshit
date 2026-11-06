// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// targetProfile stores an encode profile whose graph is the given Target and
// returns its id (task 74). A library runs its profile, so a test that used to
// hand a Target to a library hands it a profile that carries the same graph.
// An empty policy means replace_if_smaller.
func targetProfile(t *testing.T, st store.Store, target []byte, policy string) string {
	t.Helper()
	tg, err := flow.ParseTarget(target)
	require.NoError(t, err)
	g, err := flow.TargetToFlow(tg)
	require.NoError(t, err)
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	if policy == "" {
		policy = store.ResultPolicyReplaceIfSmaller
	}
	p := &store.Profile{
		Name:         "test " + store.NewID(),
		Tier:         store.RuleTierAdvanced,
		Graph:        string(raw),
		ResultPolicy: policy,
	}
	require.NoError(t, st.CreateProfile(context.Background(), p))
	return p.ID
}

// graphProfile stores an encode profile carrying graph g and returns its id
// (task 74). Advanced tier and replace_if_smaller, unless the caller says so.
func graphProfile(t *testing.T, st store.Store, g flow.Graph, tier string) string {
	t.Helper()
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	if tier == "" {
		tier = store.RuleTierAdvanced
	}
	p := &store.Profile{
		Name:         "test " + store.NewID(),
		Tier:         tier,
		Graph:        string(raw),
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	}
	require.NoError(t, st.CreateProfile(context.Background(), p))
	return p.ID
}

// putFlowViaProfile saves a flow body the way the library flow route used to
// (task 74): the graph is saved on the profile the library follows, because
// PUT /libraries/{id}/flow is refused now. The tier follows the body's source
// (easy is basic, steps is moderate, anything else advanced); the profile's
// name and policy are kept.
func (h *apiHarness) putFlowViaProfile(libID string, body map[string]any) *httptest.ResponseRecorder {
	h.t.Helper()
	// An unknown source is refused by the library route, which still checks it.
	if src, _ := body["source"].(string); !slices.Contains(flow.FlowSources(), src) {
		return h.do(http.MethodPut, "/api/v1/libraries/"+libID+"/flow", body)
	}
	prof := h.profileFor(h.libraryDTO(libID).ProfileID)
	tier := store.RuleTierAdvanced
	switch body["source"] {
	case "easy":
		tier = store.RuleTierBasic
	case "steps":
		tier = store.RuleTierModerate
	}
	return h.do(http.MethodPut, "/api/v1/profiles/"+prof.ID, map[string]any{
		"name": prof.Name, "description": prof.Description, "tier": tier,
		"result_policy": prof.ResultPolicy, "graph": body["graph"],
	})
}
