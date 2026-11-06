// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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

// ProfileBackfill is one line of the report BackfillProfiles returns: which
// profile a library was given and how. The server logs one line per entry.
type ProfileBackfill struct {
	LibraryID   string
	LibraryName string
	ProfileID   string
	ProfileName string
	// Created is true when a new profile was inserted; false when an existing
	// profile with the same graph, tier and result policy was reused.
	Created bool
	// Seeded is true when the library had no stored graph and the fallback
	// graph was used.
	Seeded bool
	// Err is set when this library could not be assigned (its stored graph
	// is not valid JSON). The library keeps a NULL profile_id; the others are
	// still assigned.
	Err error
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

// BackfillProfiles gives every library with no profile_id a profile (spec
// 5.2). It is the one time boot step that the v1.10 -> v1.11 migration needs,
// and it is safe to run on every boot: libraries that already have a profile
// are skipped, so a second run changes nothing.
//
// For each library, in creation order:
//   - the stored graph is canonicalised (keys sorted, no whitespace) and hashed
//     with sha256;
//   - an existing profile with the same hash, tier and result policy is reused,
//     so two libraries with identical graphs share one profile;
//   - otherwise a new profile is created, named after the library (with a
//     numeric suffix if the name is taken), carrying the library's tier and
//     result policy;
//   - a library with no stored graph gets fallbackGraph as its graph and is
//     reported as Seeded.
//
// Nothing is deleted and the library's graph, tier and policy columns are left
// as they are. The whole run is one transaction. A library whose stored graph
// is not valid JSON is reported with Err and skipped, not fatal.
func (s *SQLite) BackfillProfiles(ctx context.Context, fallbackGraph string) ([]ProfileBackfill, error) {
	fallback, err := canonicalJSON(fallbackGraph)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: fallback graph: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	libs, err := unassignedLibraries(ctx, tx)
	if err != nil {
		return nil, err
	}
	var report []ProfileBackfill
	for _, lib := range libs {
		entry := ProfileBackfill{LibraryID: lib.ID, LibraryName: lib.Name}
		graph := fallback
		if strings.TrimSpace(lib.FlowJSON) == "" {
			entry.Seeded = true
		} else {
			canon, cerr := canonicalJSON(lib.FlowJSON)
			if cerr != nil {
				entry.Err = cerr
				report = append(report, entry)
				continue
			}
			graph = canon
		}
		tier := lib.RuleTier
		if tier == "" {
			tier = RuleTierBasic
		}
		hash := sha256Hex(graph)

		existing, err := findProfileFor(ctx, tx, hash, tier, lib.ResultPolicy)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			entry.ProfileID, entry.ProfileName = existing.ID, existing.Name
		} else {
			name, err := freeProfileName(ctx, tx, lib.Name)
			if err != nil {
				return nil, err
			}
			now := nowUTC()
			p := Profile{ID: NewID(), Name: name, Tier: tier, Graph: graph, ResultPolicy: lib.ResultPolicy, CreatedAt: now, UpdatedAt: now}
			if err := validateResultPolicy(p.ResultPolicy); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO profiles (`+profileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, p.Name, p.Description, p.Tier, p.Graph, p.ResultPolicy, fmtTime(p.CreatedAt), fmtTime(p.UpdatedAt)); err != nil {
				return nil, fmt.Errorf("store: backfill profiles: insert profile for %s: %w", lib.ID, err)
			}
			entry.ProfileID, entry.ProfileName, entry.Created = p.ID, p.Name, true
		}
		if _, err := tx.ExecContext(ctx, `UPDATE libraries SET profile_id = ? WHERE id = ?`, entry.ProfileID, lib.ID); err != nil {
			return nil, fmt.Errorf("store: backfill profiles: set profile for %s: %w", lib.ID, err)
		}
		report = append(report, entry)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: backfill profiles: commit: %w", err)
	}
	return report, nil
}

// unassignedLibraries reads the libraries with no profile_id, oldest first.
// It drains the rows before returning so the transaction can run other
// statements on its single connection.
func unassignedLibraries(ctx context.Context, tx *sql.Tx) ([]*Library, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+libraryColumns+` FROM libraries WHERE COALESCE(profile_id, '') = '' ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: list libraries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*Library
	for rows.Next() {
		lib, err := scanLibrary(rows)
		if err != nil {
			return nil, fmt.Errorf("store: backfill profiles: scan library: %w", err)
		}
		out = append(out, lib)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: backfill profiles: list libraries: %w", err)
	}
	return out, nil
}

// findProfileFor returns the first profile whose stored graph hashes to hash
// and whose tier and result policy match, or nil.
func findProfileFor(ctx context.Context, tx *sql.Tx, hash, tier, policy string) (*Profile, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+profileColumns+` FROM profiles WHERE tier = ? AND result_policy = ? ORDER BY created_at, id`, tier, policy)
	if err != nil {
		return nil, fmt.Errorf("store: backfill profiles: find profile: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var found *Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("store: backfill profiles: scan profile: %w", err)
		}
		if found == nil && sha256Hex(p.Graph) == hash {
			found = p
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: backfill profiles: find profile: %w", err)
	}
	return found, nil
}

// freeProfileName returns base when no profile uses it, otherwise "base (2)",
// "base (3)" and so on.
func freeProfileName(ctx context.Context, tx *sql.Tx, base string) (string, error) {
	if strings.TrimSpace(base) == "" {
		base = "Profile"
	}
	name := base
	for n := 2; ; n++ {
		var taken int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE name = ?`, name).Scan(&taken); err != nil {
			return "", fmt.Errorf("store: backfill profiles: check name: %w", err)
		}
		if taken == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}
}

// canonicalJSON re-encodes a JSON document with object keys sorted and no
// insignificant whitespace. Numbers keep their literal text. Two graphs that
// differ only in formatting produce the same output, so their hashes match.
func canonicalJSON(raw string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("store: canonical json: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", errors.New("store: canonical json: trailing data after the document")
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("store: canonical json: %w", err)
	}
	return string(out), nil
}

// sha256Hex is the lowercase hex sha256 of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
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
