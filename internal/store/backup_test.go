// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteBackup covers the pre-update snapshot (task 16): the backup is
// a readable database holding the live data, and re-backing up over an
// existing file works (VACUUM INTO refuses to overwrite; Backup removes
// the stale file first).
func TestSQLiteBackup(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	require.NoError(t, st.SetSetting(ctx, SettingUpdateChannel, "prerelease"))

	backup := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, st.Backup(ctx, backup))

	restored, err := New(backup)
	require.NoError(t, err)
	defer func() { _ = restored.Close() }()
	value, ok, err := restored.GetSetting(ctx, SettingUpdateChannel)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "prerelease", value)

	// A second backup over the existing file must succeed.
	require.NoError(t, st.Backup(ctx, backup))
}
