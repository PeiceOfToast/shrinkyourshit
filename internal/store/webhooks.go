// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Webhook event names (task 25; documented in API.md). The events column
// stores them comma separated.
const (
	WebhookEventJobDone     = "job.done"
	WebhookEventJobFailed   = "job.failed"
	WebhookEventLibScanned  = "library.scanned"
)

// WebhookEventValues lists the events a webhook can subscribe to.
func WebhookEventValues() []string {
	return []string{WebhookEventJobDone, WebhookEventJobFailed, WebhookEventLibScanned}
}

// WebhookDeliveriesKeep is how many delivery rows the prune keeps per hook
// (task 25: the newest 50).
const WebhookDeliveriesKeep = 50

// Webhook is one outbound hook: the server POSTs the event JSON to URL,
// signed with Secret, for every event in Events. The secret is generated
// server side at create time and never leaves the server through the API
// after the create response.
type Webhook struct {
	ID     string
	Name   string
	URL    string
	Secret string
	// Events is the comma separated subscription list.
	Events    string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EventList splits the stored events column into its names.
func (w *Webhook) EventList() []string {
	var out []string
	for _, name := range strings.Split(w.Events, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// webhookColumns is the column list shared by every webhook query.
const webhookColumns = `id, name, url, secret, events, enabled, created_at, updated_at`

// scanWebhook reads one webhook row.
func scanWebhook(rows *sql.Rows) (*Webhook, error) {
	var (
		w       Webhook
		enabled string
		created string
		updated string
	)
	if err := rows.Scan(&w.ID, &w.Name, &w.URL, &w.Secret, &w.Events, &enabled, &created, &updated); err != nil {
		return nil, err
	}
	w.Enabled = parseBool(enabled)
	w.CreatedAt = parseTime(created)
	w.UpdatedAt = parseTime(updated)
	return &w, nil
}

// CreateWebhook inserts a webhook. ID and CreatedAt are filled when empty.
func (s *SQLite) CreateWebhook(ctx context.Context, w *Webhook) error {
	if w.ID == "" {
		w.ID = NewID()
	}
	now := nowUTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhooks (`+webhookColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.URL, w.Secret, w.Events, fmtBool(w.Enabled), fmtTime(w.CreatedAt), fmtTime(w.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create webhook: %w", err)
	}
	return nil
}

// GetWebhook returns one webhook or ErrNotFound.
func (s *SQLite) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	w, err := scanWebhook(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan webhook: %w", err)
	}
	return w, nil
}

// ListWebhooks returns all webhooks, oldest first.
func (s *SQLite) ListWebhooks(ctx context.Context) ([]*Webhook, error) {
	return queryAll(ctx, s.db, `SELECT `+webhookColumns+` FROM webhooks ORDER BY created_at`, scanWebhook)
}

// UpdateWebhook overwrites an existing webhook by ID. UpdatedAt is
// refreshed. The secret is only changed by an explicit Secret value; the
// API never round-trips it.
func (s *SQLite) UpdateWebhook(ctx context.Context, w *Webhook) error {
	w.UpdatedAt = nowUTC()
	res, err := s.db.ExecContext(ctx, `UPDATE webhooks SET name = ?, url = ?, secret = ?, events = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		w.Name, w.URL, w.Secret, w.Events, fmtBool(w.Enabled), fmtTime(w.UpdatedAt), w.ID)
	if err != nil {
		return fmt.Errorf("store: update webhook: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWebhook removes the webhook and its delivery history.
func (s *SQLite) DeleteWebhook(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE webhook_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete webhook deliveries: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete webhook: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// WebhookDelivery is one recorded delivery outcome: the remote's status
// (0 when the request never got an answer) and, on a non-2xx or a
// transport failure, the head of the body or the error text.
type WebhookDelivery struct {
	ID        string
	WebhookID string
	Event     string
	Status    int
	Error     string
	CreatedAt time.Time
}

// deliveryColumns is the column list shared by every delivery query.
const deliveryColumns = `id, webhook_id, event, status, error, created_at`

// scanDelivery reads one delivery row.
func scanDelivery(rows *sql.Rows) (*WebhookDelivery, error) {
	var (
		d       WebhookDelivery
		created string
	)
	if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Status, &d.Error, &created); err != nil {
		return nil, err
	}
	d.CreatedAt = parseTime(created)
	return &d, nil
}

// AddWebhookDelivery records one delivery outcome. ID and CreatedAt are
// filled when empty.
func (s *SQLite) AddWebhookDelivery(ctx context.Context, d *WebhookDelivery) error {
	if d.ID == "" {
		d.ID = NewID()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = nowUTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_deliveries (`+deliveryColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.WebhookID, d.Event, d.Status, d.Error, fmtTime(d.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: add webhook delivery: %w", err)
	}
	return nil
}

// ListWebhookDeliveries returns a hook's deliveries, newest first, up to
// limit rows (0 means the default: the newest 50).
func (s *SQLite) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]*WebhookDelivery, error) {
	if limit <= 0 {
		limit = WebhookDeliveriesKeep
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`,
		webhookID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*WebhookDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan delivery: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate deliveries: %w", err)
	}
	return out, nil
}

// PruneWebhookDeliveries keeps the newest keep rows per hook and returns
// how many rows it removed. The delivery history must not grow forever:
// every delivered event adds a row, so the dispatcher prunes after each
// insert.
func (s *SQLite) PruneWebhookDeliveries(ctx context.Context, keep int) (int64, error) {
	if keep <= 0 {
		keep = WebhookDeliveriesKeep
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE id IN (
      SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY webhook_id ORDER BY created_at DESC, id DESC) AS rank
        FROM webhook_deliveries
      ) WHERE rank > ?
    )`, keep)
	if err != nil {
		return 0, fmt.Errorf("store: prune webhook deliveries: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
