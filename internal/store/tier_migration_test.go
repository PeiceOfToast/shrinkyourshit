// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRuleTierMigrationMatchesFlowSource builds a database as the release before
// 0016 leaves it (every migration through 0015 applied, legacy rows of each
// shape), then applies 0016 and reads rule_tier raw. Every library must end with
// the tier its flow_source implies, and legacy rows follow LibraryFlow's
// inference. Migration 0019 drops rule_tier later, so this checks 0016 alone.
func TestRuleTierMigrationMatchesFlowSource(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")
	migrateTo(t, path, "0015_library_last_scan.sql")

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	const insert = `INSERT INTO libraries (id, name, path, target, easy, flow_json, flow_source, watch, schedule, result_policy, created_at, updated_at, file_kinds)
		VALUES (?, ?, ?, '{}', ?, ?, ?, 0, '', 'replace_if_smaller', '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z', '["video"]')`
	rows := []struct {
		id, source, easy, flow string
		wantTier               string
	}{
		{"easy", "easy", `{"quality":3}`, `{"schema":1}`, RuleTierBasic},
		{"steps", "steps", ``, `{"schema":1}`, RuleTierModerate},
		{"advanced", "advanced", ``, `{"schema":1}`, RuleTierAdvanced},
		{"legacy-easy", "", `{"quality":3}`, ``, RuleTierBasic},
		{"legacy-target", "", ``, ``, RuleTierAdvanced},
		{"legacy-flow", "", ``, `{"schema":1}`, RuleTierAdvanced},
	}
	for _, r := range rows {
		_, err := db.ExecContext(ctx, insert, r.id, r.id, "/media/"+r.id, r.easy, r.flow, r.source)
		require.NoError(t, err, r.id)
	}
	require.NoError(t, db.Close())

	// Apply 0016 on the same file and read the column raw.
	migrateTo(t, path, "0016_library_tier.sql")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	defer func() { _ = raw.Close() }()
	got := map[string]string{}
	rowsOut, err := raw.QueryContext(ctx, `SELECT id, rule_tier FROM libraries`)
	require.NoError(t, err)
	for rowsOut.Next() {
		var id, tier string
		require.NoError(t, rowsOut.Scan(&id, &tier))
		got[id] = tier
	}
	require.NoError(t, rowsOut.Err())
	require.NoError(t, rowsOut.Close())
	require.Len(t, got, len(rows))
	for _, r := range rows {
		assert.Equal(t, r.wantTier, got[r.id], "library %s (flow_source %q)", r.id, r.source)
	}
}

// TestLibraryWritesRoundTripProfileOnly: a library created and then updated
// through the store round trips with its profile only. The rule comes from the
// profile (task 74); the legacy columns are gone after 0019 (task 78).
func TestLibraryWritesRoundTripProfileOnly(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	p := &Profile{Name: "Standard", Tier: RuleTierModerate, Graph: graphA, ResultPolicy: ResultPolicyKeepBoth}
	require.NoError(t, st.CreateProfile(ctx, p))

	lib := validLibrary()
	lib.ProfileID = p.ID
	require.NoError(t, st.CreateLibrary(ctx, lib))
	got, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ProfileID)
	assert.Equal(t, graphA, got.ProfileGraph)
	assert.Equal(t, RuleTierModerate, got.ProfileTier)
	assert.Equal(t, ResultPolicyKeepBoth, got.ProfileResultPolicy)

	got.Name = "Renamed"
	require.NoError(t, st.UpdateLibrary(ctx, got))
	again, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", again.Name)
	assert.Equal(t, p.ID, again.ProfileID)
}
