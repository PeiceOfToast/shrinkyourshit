// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
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
