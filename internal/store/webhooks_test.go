// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validWebhook returns a webhook row the store accepts.
func validWebhook() *Webhook {
	return &Webhook{
		Name:     "Discord",
		URL:      "https://discord.com/api/webhooks/1/abc",
		Secret:   "0123456789abcdef0123456789abcdef",
		Events:   WebhookEventJobDone + "," + WebhookEventJobFailed,
		Enabled:  true,
	}
}

// A fresh database migrates to 0011: the webhooks tables exist and round
// trip rows (the task's migration step).
func TestWebhooksMigrationAndCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	w := validWebhook()
	require.NoError(t, s.CreateWebhook(ctx, w))
	require.NotEmpty(t, w.ID)
	require.False(t, w.CreatedAt.IsZero())
	require.False(t, w.UpdatedAt.IsZero())

	got, err := s.GetWebhook(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
	assert.Equal(t, "Discord", got.Name)
	assert.Equal(t, "https://discord.com/api/webhooks/1/abc", got.URL)
	assert.Equal(t, w.Secret, got.Secret)
	assert.Equal(t, []string{WebhookEventJobDone, WebhookEventJobFailed}, got.EventList())
	assert.True(t, got.Enabled)

	// List oldest first.
	second := validWebhook()
	second.Name = "Slack"
	require.NoError(t, s.CreateWebhook(ctx, second))
	list, err := s.ListWebhooks(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, w.ID, list[0].ID)
	assert.Equal(t, second.ID, list[1].ID)

	// Update refreshes the row and stamps updated_at.
	got.Name = "Discord #media"
	got.Enabled = false
	require.NoError(t, s.UpdateWebhook(ctx, got))
	reread, err := s.GetWebhook(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, "Discord #media", reread.Name)
	assert.False(t, reread.Enabled)
	assert.False(t, reread.UpdatedAt.Before(got.UpdatedAt))

	// Unknown ids are ErrNotFound.
	_, err = s.GetWebhook(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)
	row := validWebhook()
	row.ID = "nope"
	assert.ErrorIs(t, s.UpdateWebhook(ctx, row), ErrNotFound)

	// Delete removes the row; a second delete is ErrNotFound.
	require.NoError(t, s.DeleteWebhook(ctx, got.ID))
	list, err = s.ListWebhooks(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.ErrorIs(t, s.DeleteWebhook(ctx, got.ID), ErrNotFound)
}

// A webhook's delivery history is recorded, listed newest first and
// deleted with its hook.
func TestWebhookDeliveries(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	w := validWebhook()
	require.NoError(t, s.CreateWebhook(ctx, w))

	first := &WebhookDelivery{WebhookID: w.ID, Event: WebhookEventJobDone, Status: 500, Error: "boom"}
	require.NoError(t, s.AddWebhookDelivery(ctx, first))
	require.NotEmpty(t, first.ID)
	time.Sleep(10 * time.Millisecond) // created_at has millisecond precision
	second := &WebhookDelivery{WebhookID: w.ID, Event: WebhookEventLibScanned, Status: 200}
	require.NoError(t, s.AddWebhookDelivery(ctx, second))
	// Another hook's rows never mix in.
	other := &WebhookDelivery{WebhookID: "other-hook", Event: WebhookEventJobDone, Status: 204}
	require.NoError(t, s.AddWebhookDelivery(ctx, other))

	rows, err := s.ListWebhookDeliveries(ctx, w.ID, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, second.ID, rows[0].ID, "newest first")
	assert.Equal(t, first.ID, rows[1].ID)
	assert.Equal(t, 500, rows[1].Status)
	assert.Equal(t, "boom", rows[1].Error)
	assert.Zero(t, rows[0].Error)

	// The limit trims the page.
	rows, err = s.ListWebhookDeliveries(ctx, w.ID, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, second.ID, rows[0].ID)

	// An unknown hook has no deliveries.
	rows, err = s.ListWebhookDeliveries(ctx, "nope", 0)
	require.NoError(t, err)
	assert.Empty(t, rows)

	// Deleting the hook removes its deliveries, leaves the rest.
	require.NoError(t, s.DeleteWebhook(ctx, w.ID))
	rows, err = s.ListWebhookDeliveries(ctx, w.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, rows)
	rows, err = s.ListWebhookDeliveries(ctx, other.WebhookID, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// The prune keeps the newest keep rows per hook (task 25: 50) and returns
// how many rows it removed.
func TestWebhookDeliveriesPrune(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	w := validWebhook()
	require.NoError(t, s.CreateWebhook(ctx, w))

	for i := 0; i < 55; i++ {
		time.Sleep(2 * time.Millisecond) // ordering comes from created_at
		require.NoError(t, s.AddWebhookDelivery(ctx, &WebhookDelivery{
			WebhookID: w.ID, Event: WebhookEventJobDone, Status: 200,
		}))
	}

	removed, err := s.PruneWebhookDeliveries(ctx, WebhookDeliveriesKeep)
	require.NoError(t, err)
	assert.EqualValues(t, 5, removed)

	rows, err := s.ListWebhookDeliveries(ctx, w.ID, 0)
	require.NoError(t, err)
	require.Len(t, rows, WebhookDeliveriesKeep)

	// Nothing to remove on a second pass.
	removed, err = s.PruneWebhookDeliveries(ctx, WebhookDeliveriesKeep)
	require.NoError(t, err)
	assert.EqualValues(t, 0, removed)
}
