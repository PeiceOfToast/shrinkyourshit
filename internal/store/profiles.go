// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Profile is a named, reusable encode graph with its tier and output policy
// (encode profiles wave, spec 5.1). Libraries reference one through
// Library.ProfileID. Graph is the canonical flow graph JSON, opaque here.
type Profile struct {
	ID           string
	Name         string
	Description  string
	Tier         string // basic | moderate | advanced
	Graph        string
	ResultPolicy string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProfileBackfill is one line of the report BackfillProfiles returns: a
// library that was given a profile on this run. The server logs one line per
// entry.
type ProfileBackfill struct {
	LibraryID   string
	LibraryName string
	ProfileID   string
	ProfileName string
}

// profileColumns is the column list shared by every profile query.
const profileColumns = `id, name, description, tier, graph, result_policy, created_at, updated_at`

// scanProfile reads one profile row.
func scanProfile(rows *sql.Rows) (*Profile, error) {
	var p Profile
	var created, updated string
	if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Tier, &p.Graph, &p.ResultPolicy, &created, &updated); err != nil {
		return nil, err
	}
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

// CreateProfile inserts a profile. ID, CreatedAt and UpdatedAt are filled
// when empty. The name must be unique (the schema enforces it).
func (s *SQLite) CreateProfile(ctx context.Context, p *Profile) error {
	if err := validateProfileTier(p.Tier); err != nil {
		return err
	}
	if err := validateResultPolicy(p.ResultPolicy); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = NewID()
	}
	now := nowUTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO profiles (`+profileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.Tier, p.Graph, p.ResultPolicy, fmtTime(p.CreatedAt), fmtTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create profile: %w", err)
	}
	return nil
}

// GetProfile returns one profile or ErrNotFound.
func (s *SQLite) GetProfile(ctx context.Context, id string) (*Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: get profile: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("store: get profile: %w", err)
		}
		return nil, ErrNotFound
	}
	return scanProfile(rows)
}

// ListProfiles returns all profiles, oldest first.
func (s *SQLite) ListProfiles(ctx context.Context) ([]*Profile, error) {
	return queryAll(ctx, s.db, `SELECT `+profileColumns+` FROM profiles ORDER BY created_at, id`, scanProfile)
}

// SetLibraryProfile points a library at a profile and touches nothing else.
// An empty profileID clears the reference. The caller checks that the profile
// exists; the foreign key rejects an unknown id.
func (s *SQLite) SetLibraryProfile(ctx context.Context, libraryID, profileID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE libraries SET profile_id = ? WHERE id = ?`, nullIfEmpty(profileID), libraryID)
	if err != nil {
		return fmt.Errorf("store: set library profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrProfileInUse is returned by DeleteProfile while a library references the
// profile (spec 5.3). The profile is left as it was.
var ErrProfileInUse = errors.New("store: profile is in use")

// UpdateProfile replaces a profile's name, description, tier, graph and result
// policy, and stamps updated_at. ID and CreatedAt are kept. ErrNotFound when
// no profile has the id. The name must stay unique (the schema enforces it).
func (s *SQLite) UpdateProfile(ctx context.Context, p *Profile) error {
	if err := validateProfileTier(p.Tier); err != nil {
		return err
	}
	if err := validateResultPolicy(p.ResultPolicy); err != nil {
		return err
	}
	p.UpdatedAt = nowUTC()
	res, err := s.db.ExecContext(ctx, `UPDATE profiles SET name = ?, description = ?, tier = ?, graph = ?, result_policy = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Description, p.Tier, p.Graph, p.ResultPolicy, fmtTime(p.UpdatedAt), p.ID)
	if err != nil {
		return fmt.Errorf("store: update profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProfile removes a profile unless a library references it. The check
// and the delete are one statement, so a library cannot be pointed at the
// profile between them. It returns ErrProfileInUse when a library still uses
// it and ErrNotFound when no profile has the id.
func (s *SQLite) DeleteProfile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ? AND NOT EXISTS (SELECT 1 FROM libraries WHERE profile_id = ?)`, id, id)
	if err != nil {
		return fmt.Errorf("store: delete profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := s.GetProfile(ctx, id); err != nil {
		return err
	}
	return ErrProfileInUse
}

// BackfillProfiles gives every library with no profile_id the profile profileID
// (spec 5.2), in creation order, and reports each library it assigned. It reads
// none of the legacy rule columns (flow_json, flow_source, target, easy,
// rule_tier, result_policy): the library's old rule is not looked at, only the
// profile it is given.
//
// Only one profile is used because a library row is unassigned only after a
// failed boot: every boot since v1.11 assigns every library, so matching each
// old graph against the profile table is no longer needed (task 76 notes).
//
// It is safe to run on every boot: a library that has a profile is skipped, so
// a second run assigns nothing and returns an empty report. ErrNotFound when no
// profile has the id. The whole run is one transaction.
func (s *SQLite) BackfillProfiles(ctx context.Context, profileID string) ([]ProfileBackfill, error) {
	if profileID == "" {
		return nil, errors.New("store: backfill profiles: empty profile id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var profileName string
	err = tx.QueryRowContext(ctx, `SELECT name FROM profiles WHERE id = ?`, profileID).Scan(&profileName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: find profile: %w", err)
	}

	// Drain the rows before the updates: the transaction has one connection.
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM libraries WHERE COALESCE(profile_id, '') = '' ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: list libraries: %w", err)
	}
	var libs []ProfileBackfill
	for rows.Next() {
		var e ProfileBackfill
		if err := rows.Scan(&e.LibraryID, &e.LibraryName); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: backfill profiles: scan library: %w", err)
		}
		libs = append(libs, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("store: backfill profiles: list libraries: %w", err)
	}
	_ = rows.Close()

	report := make([]ProfileBackfill, 0, len(libs))
	for _, e := range libs {
		if _, err := tx.ExecContext(ctx, `UPDATE libraries SET profile_id = ? WHERE id = ?`, profileID, e.LibraryID); err != nil {
			return nil, fmt.Errorf("store: backfill profiles: set profile for %s: %w", e.LibraryID, err)
		}
		e.ProfileID, e.ProfileName = profileID, profileName
		report = append(report, e)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: backfill profiles: commit: %w", err)
	}
	return report, nil
}

// nullIfEmpty maps "" to SQL NULL for the nullable profile_id column.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// validateProfileTier rejects tiers other than the three editors (spec 5.4).
func validateProfileTier(v string) error {
	switch v {
	case RuleTierBasic, RuleTierModerate, RuleTierAdvanced:
		return nil
	}
	return fmt.Errorf("store: invalid profile tier %q: want basic, moderate or advanced", v)
}
