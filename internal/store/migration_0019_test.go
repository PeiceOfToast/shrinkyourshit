// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrateTo applies, on a raw database file, every embedded migration whose
// name is at most last and that is not yet recorded in schema_migrations. It
// rebuilds the state a previous release leaves behind, so a test can insert
// rows into that schema and then let New apply the rest.
func migrateTo(t *testing.T, path, last string) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`)
	require.NoError(t, err)
	migrations, err := loadMigrations()
	require.NoError(t, err)
	for _, m := range migrations {
		if m.name > last {
			continue
		}
		var applied int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, m.name).Scan(&applied))
		if applied > 0 {
			continue
		}
		_, err := db.ExecContext(ctx, m.sql)
		require.NoError(t, err, m.name)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.name, "2026-11-01T00:00:00Z")
		require.NoError(t, err, m.name)
	}
}

// droppedLibraryColumns are the six columns migration 0019 removes.
var droppedLibraryColumns = []string{"flow_json", "flow_source", "target", "easy", "rule_tier", "result_policy"}

// libraryColumnNames reads PRAGMA table_info(libraries) through the store.
func libraryColumnNames(t *testing.T, st *SQLite) map[string]bool {
	t.Helper()
	rows, err := st.db.QueryContext(context.Background(), `SELECT name FROM pragma_table_info('libraries')`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		cols[name] = true
	}
	require.NoError(t, rows.Err())
	return cols
}

// TestMigration0019DropsLegacyLibraryColumns opens a v1.11 database (every
// migration through 0018, real rows with the legacy columns filled) with the
// new release. The six columns are gone, every library row survives, and every
// library still carries its profile_id column.
func TestMigration0019DropsLegacyLibraryColumns(t *testing.T) {
	ctx := context.Background()
	path := buildV111DB(t, v111Libraries)

	st, err := New(path)
	require.NoError(t, err)
	defer func() { _ = st.Close() }()

	cols := libraryColumnNames(t, st)
	for _, c := range droppedLibraryColumns {
		assert.False(t, cols[c], "libraries.%s must be dropped", c)
	}
	assert.True(t, cols["profile_id"], "profile_id stays: it is the rule now")
	assert.True(t, cols["id"] && cols["name"] && cols["path"], "identity columns stay")

	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	assert.Len(t, libs, len(v111Libraries), "library count unchanged")

	var applied int
	require.NoError(t, st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, "0019_drop_library_rule_columns.sql").Scan(&applied))
	assert.Equal(t, 1, applied, "0019 recorded exactly once")
	require.NoError(t, st.Close())

	// A second open must not re-run the drops (they would fail on a missing column).
	again, err := New(path)
	require.NoError(t, err, "a second open is a no-op")
	defer func() { _ = again.Close() }()
	require.NoError(t, again.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, "0019_drop_library_rule_columns.sql").Scan(&applied))
	assert.Equal(t, 1, applied, "still recorded exactly once after the second open")
}

// TestMigration0019BackfillAfterDrop: the boot backfill still assigns the
// default profile to every v1.11 row once the columns are gone.
func TestMigration0019BackfillAfterDrop(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries)
	def := defaultProfileFor(t, st)

	report, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)
	require.Len(t, report, len(v111Libraries))

	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	for _, l := range libs {
		assert.Equal(t, def.ID, l.ProfileID, "library %s has a profile", l.ID)
	}
}

// TestMigration0019FreshInstall: an empty database gets the full schema without
// the six columns, and a library can be created with no target column at all.
func TestMigration0019FreshInstall(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	cols := libraryColumnNames(t, st)
	for _, c := range droppedLibraryColumns {
		assert.False(t, cols[c], "fresh libraries.%s must not exist", c)
	}

	p := &Profile{Name: "Default (basic)", Tier: RuleTierBasic, Graph: graphA, ResultPolicy: ResultPolicyReplaceIfSmaller}
	require.NoError(t, st.CreateProfile(ctx, p))
	lib := validLibrary()
	lib.ProfileID = p.ID
	require.NoError(t, st.CreateLibrary(ctx, lib))

	got, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ProfileID)
}
