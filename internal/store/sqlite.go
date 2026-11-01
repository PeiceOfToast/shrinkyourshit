// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// SQLite implements Store on top of a single SQLite file.
type SQLite struct {
	db *sql.DB
}

// Compile-time check that SQLite satisfies the Store interface.
var _ Store = (*SQLite)(nil)

// New opens (creating if needed) the SQLite database at path, applies
// pending migrations and returns a ready Store. The parent directory is
// created. A single connection serializes writes, which avoids SQLITE_BUSY
// at home-lab scale; Postgres behind this interface can parallelize later.
func New(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// One connection keeps writes serialized so concurrent claim/queue
	// traffic never hits SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	// Switch the database to incremental auto vacuum (task 27): freed log
	// pages come back with PRAGMA incremental_vacuum after a prune. Must
	// run outside a transaction (a migration runs in one, where the pragma
	// is silently ignored). On an empty database the mode applies
	// immediately; an existing database needs one VACUUM to switch modes.
	if _, err := db.ExecContext(context.Background(), `PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: set auto vacuum: %w", err)
	}
	s := &SQLite{db: db}
	if err := s.applyAutoVacuum(context.Background()); err != nil {
		_ = s.Close()
		return nil, err
	}
	if err := migrate(context.Background(), db); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// applyAutoVacuum runs one VACUUM when the incremental mode has not stuck
// yet (a database that existed before task 27): auto_vacuum changes only
// take effect after a rebuild. Fresh databases read 2 (INCREMENTAL) here
// and pay no rebuild.
func (s *SQLite) applyAutoVacuum(ctx context.Context) error {
	var mode int
	if err := s.db.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return fmt.Errorf("store: read auto vacuum mode: %w", err)
	}
	if mode == 2 { // 2 = SQLITE_AUTOVACUUM_INCREMENTAL
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("store: vacuum to apply auto vacuum mode: %w", err)
	}
	return nil
}

// incrementalVacuum returns freed pages to the file system (task 27). Must
// run outside a transaction. No-op (and never an error) when the database
// is not in incremental mode. The pragma returns one row per page freed and
// only runs while it is being stepped, so the result set is drained to the
// end (an ExecContext stops after the first row and frees just one page).
func (s *SQLite) incrementalVacuum(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA incremental_vacuum(0)`)
	if err != nil {
		return fmt.Errorf("store: incremental vacuum: %w", err)
	}
	for rows.Next() {
	}
	return rows.Close()
}

// Close releases the database handle.
func (s *SQLite) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// Ping checks that the database is reachable.
func (s *SQLite) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Backup writes a consistent snapshot of the whole database to dest with
// VACUUM INTO (see the Store interface). An existing dest file is removed
// first because VACUUM INTO refuses to overwrite.
func (s *SQLite) Backup(ctx context.Context, dest string) error {
	if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: backup: remove stale %s: %w", dest, err)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("store: backup to %s: %w", dest, err)
	}
	return nil
}

// nowUTC returns the current time in UTC.
func nowUTC() time.Time { return time.Now().UTC() }

// fmtTime formats a time for storage (RFC 3339 UTC with nanoseconds so
// updates within the same second stay ordered); a zero time is an empty
// string so the column default applies.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime reads a stored time; an empty column is the zero time.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// fmtBool stores a bool as 0/1.
func fmtBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// mustMarshalKinds serializes file kinds as a JSON array string.
func mustMarshalKinds(kinds []string) string {
	raw, err := json.Marshal(kinds)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// parseBool reads a stored 0/1.
func parseBool(s string) bool { return s == "1" }

// NewID generates a UUID v4 string. STACK.md lists no uuid library, so it
// is built from crypto/rand.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand only fails when the OS entropy source is broken;
		// panicking here is the honest response.
		panic(fmt.Sprintf("store: generate id: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(b[:])
	return strings.Join([]string{
		h[0:8], h[8:12], h[12:16], h[16:20], h[20:32],
	}, "-")
}

// isNotFound maps sql.ErrNoRows to ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// scanOne runs a query expected to return at most one row, scanning into
// dest. sql.ErrNoRows becomes ErrNotFound.
func scanOne(ctx context.Context, db *sql.DB, query string, dest []any, args ...any) error {
	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(dest...); err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("store: query: %w", err)
	}
	return nil
}

// queryAll runs a query and scans each row into a slice.
func queryAll[T any](ctx context.Context, db *sql.DB, query string, newScan func(rows *sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		item, err := newScan(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan row: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows: %w", err)
	}
	return out, nil
}
