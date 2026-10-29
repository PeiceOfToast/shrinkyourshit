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

// approvedNode is a valid node row for lifecycle tests.
func approvedNode(name string) *Node {
	return &Node{
		Name:         name,
		KeyHash:      "hash-" + name,
		AccessMode:   NodeAccessShared,
		PathMappings: "[]",
		Status:       NodeStatusApproved,
		Enabled:      true,
	}
}

func TestCreateNodeDefaultsToStopped(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	// No DesiredState/Concurrency set: the store fills the task-11
	// defaults (stopped, 1) — a fresh install claims nothing.
	n := approvedNode("fresh")
	require.NoError(t, s.CreateNode(ctx, n))
	got, err := s.GetNode(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, NodeStateStopped, got.DesiredState)
	assert.Equal(t, ConcurrencyMin, got.Concurrency)
	assert.Empty(t, got.StateError)
}

func TestNodeLifecycleRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	n := approvedNode("worker")
	n.DesiredState = NodeStateRunning
	n.Concurrency = 4
	n.StateError = ""
	require.NoError(t, s.CreateNode(ctx, n))

	n.DesiredState = NodeStateDraining
	n.StateError = "pause-all: write failed"
	require.NoError(t, s.UpdateNode(ctx, n))

	got, err := s.GetNode(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, NodeStateDraining, got.DesiredState)
	assert.Equal(t, 4, got.Concurrency)
	assert.Equal(t, "pause-all: write failed", got.StateError)
}

func TestNodeLifecycleValidation(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	bad := approvedNode("bad-state")
	bad.DesiredState = "paused"
	require.ErrorContains(t, s.CreateNode(ctx, bad), "desired_state")

	loaded, err := s.GetNode(ctx, mustCreate(t, s, ctx, approvedNode("bad-conc")))
	require.NoError(t, err)
	loaded.Concurrency = ConcurrencyMax + 1
	require.ErrorContains(t, s.UpdateNode(ctx, loaded), "concurrency")
	loaded.Concurrency = 0
	require.ErrorContains(t, s.UpdateNode(ctx, loaded), "concurrency")

	ok := approvedNode("edge")
	ok.Concurrency = ConcurrencyMax
	ok.DesiredState = NodeStateRunning
	require.NoError(t, s.CreateNode(ctx, ok))
}

// mustCreate inserts n and returns its id (tests that need a row to mutate).
func mustCreate(t *testing.T, s *SQLite, ctx context.Context, n *Node) string {
	t.Helper()
	require.NoError(t, s.CreateNode(ctx, n))
	return n.ID
}

// TestNodeLifecycleMigration verifies the task-11 migration against a
// database that still has the pre-0006 schema: in-flight work keeps its
// node running, idle nodes wake up stopped, and concurrency is seeded from
// the old global worker_concurrency setting.
func TestNodeLifecycleMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	// Build the old (0001..0005) schema by hand, recording each migration
	// the way migrate() does so re-opening only applies 0006.
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	mustExec(t, raw, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`)
	migrations, err := loadMigrations()
	require.NoError(t, err)
	for _, m := range migrations {
		if m.name >= "0006_node_lifecycle.sql" {
			break
		}
		_, err := raw.Exec(m.sql)
		require.NoError(t, err)
		mustExec(t, raw, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, '2026-01-01T00:00:00Z')`, m.name)
	}
	ctx := context.Background()

	// Three nodes from the MVP era: one mid-encode, one idle, one revoked.
	mustExec(t, raw, `INSERT INTO nodes (id, name, key_hash, access_mode, status, enabled, created_at)
		VALUES ('busy', 'busy', 'h1', 'shared', 'approved', 1, '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO nodes (id, name, key_hash, access_mode, status, enabled, created_at)
		VALUES ('idle', 'idle', 'h2', 'shared', 'approved', 1, '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO nodes (id, name, key_hash, access_mode, status, enabled, created_at)
		VALUES ('gone', 'gone', 'h3', 'shared', 'revoked', 0, '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO libraries (id, name, path, target, created_at, updated_at)
		VALUES ('lib', 'Movies', '/media', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO files (id, library_id, path, status, created_at, updated_at)
		VALUES ('f1', 'lib', '/media/a.mkv', 'processing', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO jobs (id, file_id, library_id, node_id, status, created_at)
		VALUES ('j1', 'f1', 'lib', 'busy', 'running', '2026-01-01T00:00:00Z')`)
	mustExec(t, raw, `INSERT INTO settings (key, value) VALUES ('worker_concurrency', '4')`)
	require.NoError(t, raw.Close())

	// Re-opening applies 0006 on top of the old rows.
	s, err := New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	busy, err := s.GetNode(ctx, "busy")
	require.NoError(t, err)
	assert.Equal(t, NodeStateRunning, busy.DesiredState,
		"a node with in-flight work stays running across the upgrade")
	assert.Equal(t, 4, busy.Concurrency, "seeded from worker_concurrency")

	idle, err := s.GetNode(ctx, "idle")
	require.NoError(t, err)
	assert.Equal(t, NodeStateStopped, idle.DesiredState,
		"an idle node wakes up stopped and waits for a human")
	assert.Equal(t, 4, idle.Concurrency)

	gone, err := s.GetNode(ctx, "gone")
	require.NoError(t, err)
	assert.Equal(t, NodeStateStopped, gone.DesiredState)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}
