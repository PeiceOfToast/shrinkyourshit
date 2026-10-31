// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Integration kinds (ARCHITECTURE.md; do not add values without updating
// it). Jellyfin and Emby share the Emby-style API; Sonarr and Radarr speak
// their v3 command API.
const (
	IntegrationKindJellyfin = "jellyfin"
	IntegrationKindEmby     = "emby"
	IntegrationKindPlex     = "plex"
	IntegrationKindSonarr   = "sonarr"
	IntegrationKindRadarr   = "radarr"
)

// IntegrationKindValues lists the valid integration kinds.
func IntegrationKindValues() []string {
	return []string{
		IntegrationKindJellyfin, IntegrationKindEmby, IntegrationKindPlex,
		IntegrationKindSonarr, IntegrationKindRadarr,
	}
}

// Integration is one remote server the refresher can notify after a job
// ends. The API key is stored in plain text (the outbound request must
// send it verbatim) and never leaves the server through the API.
type Integration struct {
	ID      string
	Kind    string
	Name    string
	BaseURL string
	APIKey  string
	// Enabled gates every notification; RefreshOnDone lets an
	// integration exist disabled for refresh but still reachable through
	// the manual test endpoint.
	Enabled       bool
	RefreshOnDone bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// integrationColumns is the column list shared by every integration query.
const integrationColumns = `id, kind, name, base_url, api_key, enabled, refresh_on_done, created_at, updated_at`

// scanIntegration reads one integration row.
func scanIntegration(rows *sql.Rows) (*Integration, error) {
	var (
		in      Integration
		enabled string
		onDone  string
		created string
		updated string
	)
	if err := rows.Scan(&in.ID, &in.Kind, &in.Name, &in.BaseURL, &in.APIKey, &enabled, &onDone, &created, &updated); err != nil {
		return nil, err
	}
	in.Enabled = parseBool(enabled)
	in.RefreshOnDone = parseBool(onDone)
	in.CreatedAt = parseTime(created)
	in.UpdatedAt = parseTime(updated)
	return &in, nil
}

// validateIntegrationKind rejects unknown integration kinds on write.
func validateIntegrationKind(v string) error {
	for _, ok := range IntegrationKindValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid integration kind %q: want one of %v", v, IntegrationKindValues())
}

// CreateIntegration inserts an integration. ID and CreatedAt are filled
// when empty.
func (s *SQLite) CreateIntegration(ctx context.Context, in *Integration) error {
	if err := validateIntegrationKind(in.Kind); err != nil {
		return err
	}
	if in.ID == "" {
		in.ID = NewID()
	}
	now := nowUTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO integrations (`+integrationColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.Kind, in.Name, in.BaseURL, in.APIKey, fmtBool(in.Enabled), fmtBool(in.RefreshOnDone), fmtTime(in.CreatedAt), fmtTime(in.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create integration: %w", err)
	}
	return nil
}

// GetIntegration returns one integration or ErrNotFound.
func (s *SQLite) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+integrationColumns+` FROM integrations WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	in, err := scanIntegration(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan integration: %w", err)
	}
	return in, nil
}

// ListIntegrations returns all integrations, oldest first.
func (s *SQLite) ListIntegrations(ctx context.Context) ([]*Integration, error) {
	return queryAll(ctx, s.db, `SELECT `+integrationColumns+` FROM integrations ORDER BY created_at`, scanIntegration)
}

// UpdateIntegration overwrites an existing integration by ID. UpdatedAt is
// refreshed.
func (s *SQLite) UpdateIntegration(ctx context.Context, in *Integration) error {
	if err := validateIntegrationKind(in.Kind); err != nil {
		return err
	}
	in.UpdatedAt = nowUTC()
	res, err := s.db.ExecContext(ctx, `UPDATE integrations SET kind = ?, name = ?, base_url = ?, api_key = ?, enabled = ?, refresh_on_done = ?, updated_at = ? WHERE id = ?`,
		in.Kind, in.Name, in.BaseURL, in.APIKey, fmtBool(in.Enabled), fmtBool(in.RefreshOnDone), fmtTime(in.UpdatedAt), in.ID)
	if err != nil {
		return fmt.Errorf("store: update integration: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteIntegration removes an integration row.
func (s *SQLite) DeleteIntegration(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM integrations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete integration: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
