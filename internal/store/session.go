// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Session is one admin login session. Only the SHA-256 hex hash of the raw
// token is stored; the raw token lives in the browser cookie.
type Session struct {
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// sessionColumns is the column list shared by every session query.
const sessionColumns = `token_hash, created_at, expires_at`

// scanSession reads one session row.
func scanSession(rows *sql.Rows) (*Session, error) {
	var s Session
	var created, expires string
	if err := rows.Scan(&s.TokenHash, &created, &expires); err != nil {
		return nil, err
	}
	s.CreatedAt = parseTime(created)
	s.ExpiresAt = parseTime(expires)
	return &s, nil
}

// CreateSession inserts a session. Only the token hash is stored.
func (s *SQLite) CreateSession(ctx context.Context, sess *Session) error {
	if sess.TokenHash == "" {
		return fmt.Errorf("store: create session: empty token hash")
	}
	now := nowUTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.ExpiresAt.IsZero() {
		return fmt.Errorf("store: create session: empty expiry")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (`+sessionColumns+`) VALUES (?, ?, ?)`,
		sess.TokenHash, fmtTime(sess.CreatedAt), fmtTime(sess.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// GetSessionByHash returns one session by the hash of its raw token, or
// ErrNotFound.
func (s *SQLite) GetSessionByHash(ctx context.Context, tokenHash string) (*Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	sess, err := scanSession(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan session: %w", err)
	}
	return sess, nil
}

// DeleteSession removes one session by token hash (logout).
func (s *SQLite) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes every session past its expiry and returns
// how many rows it removed. The auth service calls it after each login so
// the table does not grow without bound.
func (s *SQLite) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, fmtTime(now))
	if err != nil {
		return 0, fmt.Errorf("store: delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: expired session count: %w", err)
	}
	return n, nil
}
