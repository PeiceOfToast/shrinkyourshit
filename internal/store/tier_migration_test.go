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

// TestRuleTierMigrationMatchesFlowSource builds a database as the current
// release leaves it (every migration before 0016 applied, legacy rows of each
// shape), then lets the new release open it. Every library must end with the
// tier its flow_source implies, and legacy rows follow LibraryFlow's inference.
func TestRuleTierMigrationMatchesFlowSource(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`)
	require.NoError(t, err)
	migrations, err := loadMigrations()
	require.NoError(t, err)
	before := 0
	for _, m := range migrations {
		if m.name >= "0016_library_tier.sql" {
			continue
		}
		before++
		_, err := db.ExecContext(ctx, m.sql)
		require.NoError(t, err, m.name)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.name, "2026-11-01T00:00:00Z")
		require.NoError(t, err, m.name)
	}
	require.Equal(t, 15, before, "the current release runs to 0015")

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

	// The new release applies 0016 on open and every row gets its tier. The
	// column is read raw: the Library struct no longer carries it (task 76).
	st, err := New(path)
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	got := map[string]string{}
	rowsOut, err := st.db.QueryContext(ctx, `SELECT id, rule_tier FROM libraries`)
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

// TestLibraryWritesLeaveLegacyColumnsEmpty: a library created and then updated
// through the store round trips with its profile only. The six legacy rule
// columns are no longer written, so they keep their column defaults (task 76).
func TestLibraryWritesLeaveLegacyColumnsEmpty(t *testing.T) {
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

	var flow, target, tier, policy string
	require.NoError(t, st.db.QueryRowContext(ctx, `SELECT flow_json, target, rule_tier, result_policy FROM libraries WHERE id = ?`, lib.ID).Scan(&flow, &target, &tier, &policy))
	assert.Empty(t, flow)
	assert.Empty(t, target)
	assert.Empty(t, tier)
	// result_policy has a schema default; the profile says keep_both, the
	// column keeps the default because the library write no longer sets it.
	assert.Equal(t, "replace_if_smaller", policy)
}
