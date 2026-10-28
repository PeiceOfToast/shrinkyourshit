// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one applied migration file.
type migration struct {
	name string
	sql  string
}

// loadMigrations reads every migrations/*.sql in filename order. Plain SQL
// files applied in order, tracked in schema_migrations (STACK.md).
func loadMigrations() ([]migration, error) {
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("store: glob migrations: %w", err)
	}
	sort.Strings(entries)
	var out []migration
	for _, entry := range entries {
		data, err := migrationsFS.ReadFile(entry)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %s: %w", entry, err)
		}
		out = append(out, migration{
			name: strings.TrimPrefix(entry, "migrations/"),
			sql:  string(data),
		})
	}
	return out, nil
}

// migrate applies all pending migrations inside a transaction. Re-applying
// an already-applied migration is a no-op: the schema_migrations table
// tracks what has run, and the statements are idempotent anyway.
func migrate(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	for _, m := range migrations {
		var applied int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, m.name).Scan(&applied); err != nil {
			return fmt.Errorf("store: check migration %s: %w", m.name, err)
		}
		if applied > 0 {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: apply migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.name, nowUTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: record migration %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", m.name, err)
		}
	}
	return nil
}
