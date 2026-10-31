// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validPlugin returns a plugin row the store accepts.
func validPlugin() *Plugin {
	return &Plugin{
		Name:     "notify-telegram",
		Version:  "1.0.0",
		Type:     PluginTypeNotify,
		Source:   "https://github.com/example/notify-telegram",
		Settings: `{"chat_id":"123"}`,
	}
}

// A fresh database migrates to 0013: the plugins table exists, rounds
// trip rows and enforces the task's rules (disabled by default, unique
// names).
func TestPluginsMigrationAndCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	// The task's migration is explicit: enabled defaults to 0 and
	// settings to '{}'. Prove both defaults by inserting the bare minimum
	// through raw SQL — the column defaults are part of the contract.
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugins (id, name, version, type, source, installed_at) VALUES ('x', 'bare', '1', 'notify', 'src', '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	bare, err := s.GetPlugin(ctx, "x")
	require.NoError(t, err)
	assert.False(t, bare.Enabled, "installed plugins are disabled by default")
	assert.Equal(t, "{}", bare.Settings)
	assert.Equal(t, map[string]string{}, bare.SettingValues())

	p := validPlugin()
	require.NoError(t, s.CreatePlugin(ctx, p))
	require.NotEmpty(t, p.ID)
	require.False(t, p.InstalledAt.IsZero())

	got, err := s.GetPlugin(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "notify-telegram", got.Name)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Equal(t, PluginTypeNotify, got.Type)
	assert.False(t, got.Enabled)
	assert.Equal(t, map[string]string{"chat_id": "123"}, got.SettingValues())

	// Name is unique.
	dup := validPlugin()
	err = s.CreatePlugin(ctx, dup)
	require.Error(t, err)

	// GetPluginByName and ListEnabledPlugins.
	byName, err := s.GetPluginByName(ctx, "notify-telegram")
	require.NoError(t, err)
	assert.Equal(t, p.ID, byName.ID)

	// A second plugin of the same type, disabled and enabled.
	second := validPlugin()
	second.Name = "notify-slack"
	second.Enabled = true
	require.NoError(t, s.CreatePlugin(ctx, second))

	enabled, err := s.ListEnabledPlugins(ctx, PluginTypeNotify)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, second.ID, enabled[0].ID)

	all, err := s.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// Unknown type is rejected on write.
	bad := validPlugin()
	bad.Type = "flow_step"
	require.Error(t, s.CreatePlugin(ctx, bad))

	// Update toggles enabled and settings.
	got.Enabled = true
	got.Settings = `{"chat_id":"456"}`
	require.NoError(t, s.UpdatePlugin(ctx, got))
	reread, err := s.GetPlugin(ctx, got.ID)
	require.NoError(t, err)
	assert.True(t, reread.Enabled)
	assert.Equal(t, map[string]string{"chat_id": "456"}, reread.SettingValues())

	// Update of an unknown id is ErrNotFound.
	missing := validPlugin()
	missing.ID = "nope"
	missing.InstalledAt = time.Now()
	require.ErrorIs(t, s.UpdatePlugin(ctx, missing), ErrNotFound)

	// Delete.
	require.NoError(t, s.DeletePlugin(ctx, got.ID))
	_, err = s.GetPlugin(ctx, got.ID)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, s.DeletePlugin(ctx, got.ID), ErrNotFound)
}
