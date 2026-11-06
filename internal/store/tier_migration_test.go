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

	// The new release applies 0016 on open and every row gets its tier.
	st, err := New(path)
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	got := map[string]string{}
	for _, l := range libs {
		got[l.ID] = l.RuleTier
	}
	require.Len(t, got, len(rows))
	for _, r := range rows {
		assert.Equal(t, r.wantTier, got[r.id], "library %s (flow_source %q)", r.id, r.source)
	}
}

// TestCreateLibraryDefaultsRuleTier: a library created without a tier is
// stored as basic, and a save made from a fresh read keeps the tier the
// library already has.
func TestCreateLibraryDefaultsRuleTier(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	lib := validLibrary()
	require.NoError(t, st.CreateLibrary(ctx, lib))
	got, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, RuleTierBasic, got.RuleTier)

	// The tier is written through the library row (task 74: the tier is the
	// profile's; the column is a legacy snapshot, so the write is direct).
	fresh, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	fresh.RuleTier = RuleTierModerate
	fresh.Name = "Renamed"
	require.NoError(t, st.UpdateLibrary(ctx, fresh))
	again, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, RuleTierModerate, again.RuleTier)
}
