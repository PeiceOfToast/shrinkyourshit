// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validIntegration returns an integration row the store accepts.
func validIntegration() *Integration {
	return &Integration{
		Kind:          IntegrationKindJellyfin,
		Name:          "Living room",
		BaseURL:       "http://192.168.1.20:8096",
		APIKey:        "secret-key-value",
		Enabled:       true,
		RefreshOnDone: true,
	}
}

// A fresh database migrates to 0010: the integrations table exists and
// round trips a row (acceptance item 1).
func TestIntegrationsMigrationAndCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	in := validIntegration()
	require.NoError(t, s.CreateIntegration(ctx, in))
	require.NotEmpty(t, in.ID)
	require.False(t, in.CreatedAt.IsZero())
	require.False(t, in.UpdatedAt.IsZero())

	got, err := s.GetIntegration(ctx, in.ID)
	require.NoError(t, err)
	assert.Equal(t, in.ID, got.ID)
	assert.Equal(t, IntegrationKindJellyfin, got.Kind)
	assert.Equal(t, "Living room", got.Name)
	assert.Equal(t, "http://192.168.1.20:8096", got.BaseURL)
	assert.Equal(t, "secret-key-value", got.APIKey)
	assert.True(t, got.Enabled)
	assert.True(t, got.RefreshOnDone)

	// List oldest first.
	second := validIntegration()
	second.Kind = IntegrationKindPlex
	second.Name = "Plex"
	require.NoError(t, s.CreateIntegration(ctx, second))
	list, err := s.ListIntegrations(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, in.ID, list[0].ID)
	assert.Equal(t, second.ID, list[1].ID)

	// Update refreshes the row and stamps updated_at.
	got.Name = "Living room TV"
	got.Enabled = false
	require.NoError(t, s.UpdateIntegration(ctx, got))
	reread, err := s.GetIntegration(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, "Living room TV", reread.Name)
	assert.False(t, reread.Enabled)
	assert.False(t, reread.UpdatedAt.Before(got.UpdatedAt))

	// Delete removes the row; a second delete is ErrNotFound.
	require.NoError(t, s.DeleteIntegration(ctx, got.ID))
	require.NoError(t, s.DeleteIntegration(ctx, second.ID))
	list, err = s.ListIntegrations(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.ErrorIs(t, s.DeleteIntegration(ctx, got.ID), ErrNotFound)
}

func TestIntegrationUnknownIDs(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	_, err := s.GetIntegration(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)

	row := validIntegration()
	row.ID = "nope"
	assert.ErrorIs(t, s.UpdateIntegration(ctx, row), ErrNotFound)
}

func TestIntegrationKindValidation(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	row := validIntegration()
	row.Kind = "kodi"
	assert.Error(t, s.CreateIntegration(ctx, row))
	assert.Empty(t, row.ID, "no id is minted when the write is refused")

	row.Kind = IntegrationKindSonarr
	require.NoError(t, s.CreateIntegration(ctx, row))
	row.Kind = "nzbget"
	assert.Error(t, s.UpdateIntegration(ctx, row))
}
