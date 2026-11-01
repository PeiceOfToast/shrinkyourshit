// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Plugin types (task 31; step added by task 32). The store stores the
// manifest's type verbatim; the canonical value list lives in
// internal/plugin (which this list must mirror — the reverse import would
// be a cycle), so the columns are validated against this list and
// internal/plugin asserts its own values against the same strings.
const (
	PluginTypePreflight   = "preflight"
	PluginTypePostprocess = "postprocess"
	PluginTypeNotify      = "notify"
	PluginTypeStep        = "step"
)

// PluginTypeValues lists the valid plugin types.
func PluginTypeValues() []string {
	return []string{PluginTypePreflight, PluginTypePostprocess, PluginTypeNotify, PluginTypeStep}
}

// Plugin is one installed plugin row (task 31). The manifest lives on
// disk; this row carries its identity, the enabled switch (installed
// plugins are disabled until someone turns them on), the source it was
// installed from and the collected settings values.
type Plugin struct {
	ID          string
	Name        string
	Version     string
	Type        string
	Enabled     bool
	Source      string
	Settings    string // JSON object of collected setting values
	InstalledAt time.Time
}

// SettingValues decodes the stored settings JSON into a map, tolerating
// an empty column.
func (p *Plugin) SettingValues() map[string]string {
	out := map[string]string{}
	if p.Settings == "" {
		return out
	}
	_ = json.Unmarshal([]byte(p.Settings), &out)
	return out
}

// pluginColumns is the column list shared by every plugin query.
const pluginColumns = `id, name, version, type, enabled, source, settings, installed_at`

// scanPlugin reads one plugin row.
func scanPlugin(rows *sql.Rows) (*Plugin, error) {
	var (
		p       Plugin
		enabled string
		instAt  string
	)
	if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Type, &enabled, &p.Source, &p.Settings, &instAt); err != nil {
		return nil, err
	}
	p.Enabled = parseBool(enabled)
	p.InstalledAt = parseTime(instAt)
	return &p, nil
}

// validatePluginType rejects unknown plugin types on write.
func validatePluginType(v string) error {
	for _, ok := range PluginTypeValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid plugin type %q: want one of %v", v, PluginTypeValues())
}

// CreatePlugin inserts a plugin. ID and InstalledAt are filled when
// empty. The name is UNIQUE: installing the same name twice fails here.
func (s *SQLite) CreatePlugin(ctx context.Context, p *Plugin) error {
	if err := validatePluginType(p.Type); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.InstalledAt.IsZero() {
		p.InstalledAt = nowUTC()
	}
	if p.Settings == "" {
		p.Settings = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugins (`+pluginColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Version, p.Type, fmtBool(p.Enabled), p.Source, p.Settings, fmtTime(p.InstalledAt))
	if err != nil {
		return fmt.Errorf("store: create plugin: %w", err)
	}
	return nil
}

// GetPlugin returns one plugin or ErrNotFound.
func (s *SQLite) GetPlugin(ctx context.Context, id string) (*Plugin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pluginColumns+` FROM plugins WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	p, err := scanPlugin(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan plugin: %w", err)
	}
	return p, nil
}

// GetPluginByName returns one plugin by its manifest name or ErrNotFound.
func (s *SQLite) GetPluginByName(ctx context.Context, name string) (*Plugin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pluginColumns+` FROM plugins WHERE name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	p, err := scanPlugin(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan plugin: %w", err)
	}
	return p, nil
}

// ListPlugins returns every installed plugin, oldest first.
func (s *SQLite) ListPlugins(ctx context.Context) ([]*Plugin, error) {
	return queryAll(ctx, s.db, `SELECT `+pluginColumns+` FROM plugins ORDER BY installed_at, id`, scanPlugin)
}

// ListEnabledPlugins returns every enabled plugin of one type, oldest
// first (the phase hooks ask exactly this).
func (s *SQLite) ListEnabledPlugins(ctx context.Context, pluginType string) ([]*Plugin, error) {
	if err := validatePluginType(pluginType); err != nil {
		return nil, err
	}
	return queryAll(ctx, s.db,
		`SELECT `+pluginColumns+` FROM plugins WHERE type = ? AND enabled = ? ORDER BY installed_at, id`,
		scanPlugin, pluginType, fmtBool(true))
}

// UpdatePlugin overwrites an existing plugin by ID. Used for the enabled
// switch and the collected settings values; the identity columns change
// only through an uninstall/reinstall.
func (s *SQLite) UpdatePlugin(ctx context.Context, p *Plugin) error {
	if err := validatePluginType(p.Type); err != nil {
		return err
	}
	if p.Settings == "" {
		p.Settings = "{}"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE plugins SET name = ?, version = ?, type = ?, enabled = ?, source = ?, settings = ? WHERE id = ?`,
		p.Name, p.Version, p.Type, fmtBool(p.Enabled), p.Source, p.Settings, p.ID)
	if err != nil {
		return fmt.Errorf("store: update plugin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlugin removes the row.
func (s *SQLite) DeletePlugin(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete plugin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
