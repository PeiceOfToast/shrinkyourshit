// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV111DatabaseBootsAndServesLibrariesUnchanged is the task 76 boot test. A
// database in the v1.11 state (every library follows a profile, and the six
// legacy rule columns still hold values that disagree with the profile) must
// boot with no error and no legacy warning, and serve the same library shape as
// before the boot. A row the v1.11 boot never assigned gets the default profile.
func TestV111DatabaseBootsAndServesLibrariesUnchanged(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	root := t.TempDir()

	// The v1.11 state, written through the store, then the legacy columns are
	// set to values the profile does not match.
	st, err := store.New(dbPath)
	require.NoError(t, err)
	g, err := flow.TargetToFlow(flow.Default())
	require.NoError(t, err)
	profileID := graphProfile(t, st, g, store.ResultPolicyKeepBoth)
	require.NoError(t, st.CreateLibrary(ctx, &store.Library{ID: "films", Name: "Films", Path: filepath.Join(root, "films"), ProfileID: profileID}))
	require.NoError(t, st.CreateLibrary(ctx, &store.Library{ID: "stray", Name: "Stray", Path: filepath.Join(root, "stray")}))
	require.NoError(t, st.Close())

	raw, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	const legacy = `UPDATE libraries SET target = ?, easy = ?, flow_json = ?, flow_source = ?, rule_tier = ?, result_policy = ? WHERE id = ?`
	_, err = raw.ExecContext(ctx, legacy, `{"video":{"codec":"h264","quality":3},"container":"mkv"}`, `{"quality":5}`,
		`{"schema":1,"nodes":[],"edges":[]}`, "steps", store.RuleTierBasic, store.ResultPolicyReplace, "films")
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, legacy, `{not json`, ``, `{not json`, "advanced", store.RuleTierAdvanced, store.ResultPolicyKeepBoth, "stray")
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	// The new release opens the database and serves the libraries.
	st, err = store.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	films, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	before := NewLibraryDTO(films)
	require.Equal(t, profileID, before.ProfileID)
	require.Equal(t, flow.CodecHEVC, before.Target.Video.Codec, "the profile's target, not the legacy h264")

	var logs bytes.Buffer
	svc := New(Config{
		Store:             st,
		DataDir:           dir,
		Log:               slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Prober:            newFakeProber(hevcSatisfied).Probe,
		Debounce:          200 * time.Millisecond,
		StableWindow:      300 * time.Millisecond,
		SweepInterval:     50 * time.Millisecond,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
	})
	svc.Libraries().SetScanHook(func(string) {})
	svc.Start(ctx)
	t.Cleanup(svc.Stop)

	// No boot error and no legacy warning: the removed steps (backfillFlows and
	// logLegacyColumns) would have logged one of these.
	assert.NotContains(t, logs.String(), "level=ERROR", "boot logged an error:\n%s", logs.String())
	assert.NotContains(t, logs.String(), "legacy", "boot logged the legacy warning:\n%s", logs.String())

	after, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	assert.Equal(t, before, NewLibraryDTO(after), "a v1.11 library is served unchanged")

	stray, err := st.GetLibrary(ctx, "stray")
	require.NoError(t, err)
	assert.Equal(t, profileID, stray.ProfileID, "an unassigned row gets the default profile")
	assert.Equal(t, store.ResultPolicyKeepBoth, NewLibraryDTO(stray).ResultPolicy, "the default profile's policy")

	profiles, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 1, "no profile is created for the unassigned row")
}
