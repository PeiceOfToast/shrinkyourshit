// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"
)

// libraryColumns is the column list shared by every library query.
const libraryColumns = `id, name, path, target, watch, schedule, result_policy, created_at, updated_at`

// scanLibrary reads one library row.
func scanLibrary(rows *sql.Rows) (*Library, error) {
	var (
		l       Library
		watch   string
		created string
		updated string
	)
	if err := rows.Scan(&l.ID, &l.Name, &l.Path, &l.TargetJSON, &watch, &l.Schedule, &l.ResultPolicy, &created, &updated); err != nil {
		return nil, err
	}
	l.Watch = parseBool(watch)
	l.CreatedAt = parseTime(created)
	l.UpdatedAt = parseTime(updated)
	return &l, nil
}

// libraryDest builds the scan destination for a single library row.
func libraryDest(l *Library, watch *string, created, updated *string) []any {
	return []any{&l.ID, &l.Name, &l.Path, &l.TargetJSON, watch, &l.Schedule, &l.ResultPolicy, created, updated}
}

// CreateLibrary inserts a library. ID, CreatedAt and UpdatedAt are filled
// when empty.
func (s *SQLite) CreateLibrary(ctx context.Context, l *Library) error {
	if err := validateResultPolicy(l.ResultPolicy); err != nil {
		return err
	}
	if l.ID == "" {
		l.ID = NewID()
	}
	now := nowUTC()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now
	}
	l.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO libraries (`+libraryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.Name, l.Path, l.TargetJSON, fmtBool(l.Watch), l.Schedule, l.ResultPolicy, fmtTime(l.CreatedAt), fmtTime(l.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create library: %w", err)
	}
	return nil
}

// GetLibrary returns one library or ErrNotFound.
func (s *SQLite) GetLibrary(ctx context.Context, id string) (*Library, error) {
	var l Library
	var watch, created, updated string
	err := scanOne(ctx, s.db, `SELECT `+libraryColumns+` FROM libraries WHERE id = ?`,
		libraryDest(&l, &watch, &created, &updated), id)
	if err != nil {
		return nil, err
	}
	l.Watch = parseBool(watch)
	l.CreatedAt = parseTime(created)
	l.UpdatedAt = parseTime(updated)
	return &l, nil
}

// ListLibraries returns all libraries, oldest first.
func (s *SQLite) ListLibraries(ctx context.Context) ([]*Library, error) {
	return queryAll(ctx, s.db, `SELECT `+libraryColumns+` FROM libraries ORDER BY created_at`, scanLibrary)
}

// UpdateLibrary overwrites an existing library. UpdatedAt is refreshed.
func (s *SQLite) UpdateLibrary(ctx context.Context, l *Library) error {
	if err := validateResultPolicy(l.ResultPolicy); err != nil {
		return err
	}
	l.UpdatedAt = nowUTC()
	res, err := s.db.ExecContext(ctx, `UPDATE libraries SET name = ?, path = ?, target = ?, watch = ?, schedule = ?, result_policy = ?, updated_at = ? WHERE id = ?`,
		l.Name, l.Path, l.TargetJSON, fmtBool(l.Watch), l.Schedule, l.ResultPolicy, fmtTime(l.UpdatedAt), l.ID)
	if err != nil {
		return fmt.Errorf("store: update library: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLibrary removes a library and, through ON DELETE CASCADE, its
// files and jobs. Files on disk are never touched.
func (s *SQLite) DeleteLibrary(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete library: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// fileColumns is the column list shared by every file query.
const fileColumns = `id, library_id, path, size, mtime, fingerprint, probe, status, satisfied_reason, last_job_id, bytes_saved, created_at, updated_at`

// scanFile reads one file row.
func scanFile(rows *sql.Rows) (*File, error) {
	var f File
	var created, updated string
	if err := rows.Scan(&f.ID, &f.LibraryID, &f.Path, &f.Size, &f.Mtime, &f.Fingerprint, &f.ProbeJSON, &f.Status, &f.SatisfiedReason, &f.LastJobID, &f.BytesSaved, &created, &updated); err != nil {
		return nil, err
	}
	f.CreatedAt = parseTime(created)
	f.UpdatedAt = parseTime(updated)
	return &f, nil
}

// UpsertFile inserts a file or updates the existing row keyed on
// (library_id, path). The scanner calls it for new and changed files alike.
func (s *SQLite) UpsertFile(ctx context.Context, f *File) error {
	if err := validateFileStatus(f.Status); err != nil {
		return err
	}
	if f.ID == "" {
		f.ID = NewID()
	}
	now := nowUTC()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO files (`+fileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, path) DO UPDATE SET
		  size = excluded.size, mtime = excluded.mtime, fingerprint = excluded.fingerprint,
		  probe = excluded.probe, status = excluded.status, satisfied_reason = excluded.satisfied_reason,
		  last_job_id = excluded.last_job_id, bytes_saved = excluded.bytes_saved, updated_at = excluded.updated_at`,
		f.ID, f.LibraryID, f.Path, f.Size, f.Mtime, f.Fingerprint, f.ProbeJSON, f.Status, f.SatisfiedReason, f.LastJobID, f.BytesSaved, fmtTime(f.CreatedAt), fmtTime(f.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: upsert file: %w", err)
	}
	return nil
}

// GetFile returns one file or ErrNotFound.
func (s *SQLite) GetFile(ctx context.Context, id string) (*File, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+fileColumns+` FROM files WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	f, err := scanFile(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan file: %w", err)
	}
	return f, nil
}

// GetFileByPath returns one file by library and path, or ErrNotFound.
func (s *SQLite) GetFileByPath(ctx context.Context, libraryID, path string) (*File, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+fileColumns+` FROM files WHERE library_id = ? AND path = ?`, libraryID, path)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	f, err := scanFile(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan file: %w", err)
	}
	return f, nil
}

// ListFiles returns all files of a library, oldest first.
func (s *SQLite) ListFiles(ctx context.Context, libraryID string) ([]*File, error) {
	return queryAll(ctx, s.db, `SELECT `+fileColumns+` FROM files WHERE library_id = ? ORDER BY created_at`, scanFile, libraryID)
}

// ListFilesByStatus returns files of a library in one status.
func (s *SQLite) ListFilesByStatus(ctx context.Context, libraryID, status string) ([]*File, error) {
	if err := validateFileStatus(status); err != nil {
		return nil, err
	}
	return queryAll(ctx, s.db, `SELECT `+fileColumns+` FROM files WHERE library_id = ? AND status = ? ORDER BY created_at`, scanFile, libraryID, status)
}

// UpdateFile overwrites an existing file by ID. UpdatedAt is refreshed.
func (s *SQLite) UpdateFile(ctx context.Context, f *File) error {
	if err := validateFileStatus(f.Status); err != nil {
		return err
	}
	f.UpdatedAt = nowUTC()
	res, err := s.db.ExecContext(ctx, `UPDATE files SET library_id = ?, path = ?, size = ?, mtime = ?, fingerprint = ?, probe = ?, status = ?, satisfied_reason = ?, last_job_id = ?, bytes_saved = ?, updated_at = ? WHERE id = ?`,
		f.LibraryID, f.Path, f.Size, f.Mtime, f.Fingerprint, f.ProbeJSON, f.Status, f.SatisfiedReason, f.LastJobID, f.BytesSaved, fmtTime(f.UpdatedAt), f.ID)
	if err != nil {
		return fmt.Errorf("store: update file: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFile removes a file row.
func (s *SQLite) DeleteFile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete file: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// jobColumns is the column list shared by every job query.
const jobColumns = `id, file_id, library_id, node_id, status, priority, attempt, encoder_used, fallback_reasons, percent, speed, eta_seconds, bytes_before, bytes_after, log, error, claimed_at, started_at, finished_at, heartbeat_at, created_at`

// scanJob reads one job row.
func scanJob(rows *sql.Rows) (*Job, error) {
	var j Job
	var claimed, started, finished, heartbeat, created sql.NullString
	err := rows.Scan(&j.ID, &j.FileID, &j.LibraryID, &j.NodeID, &j.Status, &j.Priority, &j.Attempt, &j.EncoderUsed, &j.FallbackReasons, &j.Percent, &j.Speed, &j.ETASeconds, &j.BytesBefore, &j.BytesAfter, &j.Log, &j.Error, &claimed, &started, &finished, &heartbeat, &created)
	if err != nil {
		return nil, err
	}
	j.ClaimedAt = parseTime(claimed.String)
	j.StartedAt = parseTime(started.String)
	j.FinishedAt = parseTime(finished.String)
	j.HeartbeatAt = parseTime(heartbeat.String)
	j.CreatedAt = parseTime(created.String)
	return &j, nil
}

// CreateJob inserts a job. ID and CreatedAt are filled when empty.
func (s *SQLite) CreateJob(ctx context.Context, j *Job) error {
	if err := validateJobStatus(j.Status); err != nil {
		return err
	}
	if j.ID == "" {
		j.ID = NewID()
	}
	if j.FallbackReasons == "" {
		j.FallbackReasons = "[]"
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = nowUTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs (`+jobColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.FileID, j.LibraryID, j.NodeID, j.Status, j.Priority, j.Attempt, j.EncoderUsed, j.FallbackReasons, j.Percent, j.Speed, j.ETASeconds, j.BytesBefore, j.BytesAfter, j.Log, j.Error,
		fmtTime(j.ClaimedAt), fmtTime(j.StartedAt), fmtTime(j.FinishedAt), fmtTime(j.HeartbeatAt), fmtTime(j.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: create job: %w", err)
	}
	return nil
}

// GetJob returns one job or ErrNotFound.
func (s *SQLite) GetJob(ctx context.Context, id string) (*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	j, err := scanJob(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan job: %w", err)
	}
	return j, nil
}

// ListJobs returns jobs matching the filter, oldest first.
func (s *SQLite) ListJobs(ctx context.Context, filter JobFilter) ([]*Job, error) {
	query := `SELECT ` + jobColumns + ` FROM jobs`
	var conds []string
	var args []any
	if filter.Status != "" {
		if err := validateJobStatus(filter.Status); err != nil {
			return nil, err
		}
		conds = append(conds, `status = ?`)
		args = append(args, filter.Status)
	}
	if filter.FileID != "" {
		conds = append(conds, `file_id = ?`)
		args = append(args, filter.FileID)
	}
	if filter.LibraryID != "" {
		conds = append(conds, `library_id = ?`)
		args = append(args, filter.LibraryID)
	}
	if filter.NodeID != "" {
		conds = append(conds, `node_id = ?`)
		args = append(args, filter.NodeID)
	}
	if len(conds) > 0 {
		query += ` WHERE ` + join(conds, ` AND `)
	}
	query += ` ORDER BY created_at`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d OFFSET %d`, filter.Limit, filter.Offset)
	}
	return queryAll(ctx, s.db, query, scanJob, args...)
}

// UpdateJob overwrites an existing job by ID.
func (s *SQLite) UpdateJob(ctx context.Context, j *Job) error {
	if err := validateJobStatus(j.Status); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET file_id = ?, library_id = ?, node_id = ?, status = ?, priority = ?, attempt = ?, encoder_used = ?, fallback_reasons = ?, percent = ?, speed = ?, eta_seconds = ?, bytes_before = ?, bytes_after = ?, log = ?, error = ?, claimed_at = ?, started_at = ?, finished_at = ?, heartbeat_at = ? WHERE id = ?`,
		j.FileID, j.LibraryID, j.NodeID, j.Status, j.Priority, j.Attempt, j.EncoderUsed, j.FallbackReasons, j.Percent, j.Speed, j.ETASeconds, j.BytesBefore, j.BytesAfter, j.Log, j.Error,
		fmtTime(j.ClaimedAt), fmtTime(j.StartedAt), fmtTime(j.FinishedAt), fmtTime(j.HeartbeatAt), j.ID)
	if err != nil {
		return fmt.Errorf("store: update job: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteJob removes a job row.
func (s *SQLite) DeleteJob(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete job: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// nodeColumns is the column list shared by every node query.
const nodeColumns = `id, name, key_hash, os, arch, capabilities, access_mode, path_mappings, status, enabled, last_heartbeat_at, created_at`

// scanNode reads one node row.
func scanNode(rows *sql.Rows) (*Node, error) {
	var n Node
	var enabled string
	var heartbeat, created sql.NullString
	if err := rows.Scan(&n.ID, &n.Name, &n.KeyHash, &n.OS, &n.Arch, &n.Capabilities, &n.AccessMode, &n.PathMappings, &n.Status, &enabled, &heartbeat, &created); err != nil {
		return nil, err
	}
	n.Enabled = parseBool(enabled)
	n.LastHeartbeatAt = parseTime(heartbeat.String)
	n.CreatedAt = parseTime(created.String)
	return &n, nil
}

// CreateNode inserts a node. ID and CreatedAt are filled when empty.
func (s *SQLite) CreateNode(ctx context.Context, n *Node) error {
	if err := validateNodeStatus(n.Status); err != nil {
		return err
	}
	if err := validateNodeAccessMode(n.AccessMode); err != nil {
		return err
	}
	if n.ID == "" {
		n.ID = NewID()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = nowUTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nodes (`+nodeColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.KeyHash, n.OS, n.Arch, n.Capabilities, n.AccessMode, n.PathMappings, n.Status, fmtBool(n.Enabled), fmtTime(n.LastHeartbeatAt), fmtTime(n.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: create node: %w", err)
	}
	return nil
}

// GetNode returns one node or ErrNotFound.
func (s *SQLite) GetNode(ctx context.Context, id string) (*Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	n, err := scanNode(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan node: %w", err)
	}
	return n, nil
}

// GetNodeByKeyHash returns one node by the hash of its raw key, or
// ErrNotFound.
func (s *SQLite) GetNodeByKeyHash(ctx context.Context, keyHash string) (*Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE key_hash = ?`, keyHash)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	n, err := scanNode(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan node: %w", err)
	}
	return n, nil
}

// ListNodes returns all nodes, oldest first.
func (s *SQLite) ListNodes(ctx context.Context) ([]*Node, error) {
	return queryAll(ctx, s.db, `SELECT `+nodeColumns+` FROM nodes ORDER BY created_at`, scanNode)
}

// UpdateNode overwrites an existing node by ID.
func (s *SQLite) UpdateNode(ctx context.Context, n *Node) error {
	if err := validateNodeStatus(n.Status); err != nil {
		return err
	}
	if err := validateNodeAccessMode(n.AccessMode); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE nodes SET name = ?, key_hash = ?, os = ?, arch = ?, capabilities = ?, access_mode = ?, path_mappings = ?, status = ?, enabled = ?, last_heartbeat_at = ? WHERE id = ?`,
		n.Name, n.KeyHash, n.OS, n.Arch, n.Capabilities, n.AccessMode, n.PathMappings, n.Status, fmtBool(n.Enabled), fmtTime(n.LastHeartbeatAt), n.ID)
	if err != nil {
		return fmt.Errorf("store: update node: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNode removes a node row.
func (s *SQLite) DeleteNode(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete node: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// tokenColumns is the column list shared by every pairing token query.
const tokenColumns = `id, token_hash, expires_at, used_at`

// scanToken reads one pairing token row.
func scanToken(rows *sql.Rows) (*PairingToken, error) {
	var t PairingToken
	var expires, used string
	if err := rows.Scan(&t.ID, &t.TokenHash, &expires, &used); err != nil {
		return nil, err
	}
	t.ExpiresAt = parseTime(expires)
	t.UsedAt = parseTime(used)
	return &t, nil
}

// CreatePairingToken inserts a token. ID is filled when empty. Only the
// hash of the raw token is stored.
func (s *SQLite) CreatePairingToken(ctx context.Context, t *PairingToken) error {
	if t.ID == "" {
		t.ID = NewID()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO pairing_tokens (`+tokenColumns+`) VALUES (?, ?, ?, ?)`,
		t.ID, t.TokenHash, fmtTime(t.ExpiresAt), fmtTime(t.UsedAt))
	if err != nil {
		return fmt.Errorf("store: create pairing token: %w", err)
	}
	return nil
}

// GetPairingToken returns one token or ErrNotFound.
func (s *SQLite) GetPairingToken(ctx context.Context, id string) (*PairingToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenColumns+` FROM pairing_tokens WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	t, err := scanToken(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan pairing token: %w", err)
	}
	return t, nil
}

// GetPairingTokenByHash looks a token up by the hash of the raw token.
func (s *SQLite) GetPairingTokenByHash(ctx context.Context, tokenHash string) (*PairingToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenColumns+` FROM pairing_tokens WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	t, err := scanToken(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan pairing token: %w", err)
	}
	return t, nil
}

// ListPairingTokens returns all tokens, oldest first.
func (s *SQLite) ListPairingTokens(ctx context.Context) ([]*PairingToken, error) {
	return queryAll(ctx, s.db, `SELECT `+tokenColumns+` FROM pairing_tokens ORDER BY expires_at`, scanToken)
}

// UpdatePairingToken overwrites an existing token by ID.
func (s *SQLite) UpdatePairingToken(ctx context.Context, t *PairingToken) error {
	res, err := s.db.ExecContext(ctx, `UPDATE pairing_tokens SET token_hash = ?, expires_at = ?, used_at = ? WHERE id = ?`,
		t.TokenHash, fmtTime(t.ExpiresAt), fmtTime(t.UsedAt), t.ID)
	if err != nil {
		return fmt.Errorf("store: update pairing token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePairingToken removes a token row.
func (s *SQLite) DeletePairingToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pairing_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete pairing token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSetting returns one setting value and whether it exists.
func (s *SQLite) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("store: get setting: %w", err)
	}
	return value, true, nil
}

// SetSetting inserts or overwrites a setting.
func (s *SQLite) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: set setting: %w", err)
	}
	return nil
}

// DeleteSetting removes a setting; missing keys are not an error.
func (s *SQLite) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("store: delete setting: %w", err)
	}
	return nil
}

// ListSettings returns every setting as a map.
func (s *SQLite) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: scan setting: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows: %w", err)
	}
	return out, nil
}

// join concatenates strings with a separator.
func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
