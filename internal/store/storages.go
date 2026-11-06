// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/secret"
)

// Storage protocols (ARCHITECTURE.md; do not add values without updating it).
const (
	StorageProtocolNFS = "nfs"
	StorageProtocolSMB = "smb"
)

// StorageProtocolValues lists the protocols a storage can use.
func StorageProtocolValues() []string {
	return []string{StorageProtocolNFS, StorageProtocolSMB}
}

// storageNameMax is the longest slug accepted (it becomes a path segment).
const storageNameMax = 64

// storageNameRE is a slug: lowercase letters, digits and dashes, not starting
// or ending with a dash. The slug is the mountpoint name (/storage/<name>).
var storageNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidStorageName reports whether name is usable as a storage slug.
func ValidStorageName(name string) error {
	if name == "" {
		return errors.New("store: storage name is required")
	}
	if len(name) > storageNameMax {
		return fmt.Errorf("store: invalid storage name %q: at most %d characters", name, storageNameMax)
	}
	if !storageNameRE.MatchString(name) {
		return fmt.Errorf("store: invalid storage name %q: use lowercase letters, digits and dashes, without a leading or trailing dash", name)
	}
	return nil
}

// Storage is one network share the server can mount (spec 4.3). Name is the
// slug and the mountpoint (/storage/<name>).
//
// Password is write only: set it on create or update, and the store seals it
// with the storage key and clears the field after a successful write. No read
// method fills it, and it is excluded from JSON. HasSecret reports whether a
// sealed password is stored. The only read that returns plaintext is
// SQLite.StorageSecret, for the mount executor.
type Storage struct {
	ID       string
	Name     string
	Protocol string // nfs | smb
	Source   string // host:/export or //host/share
	Options  string // mount -o string, validated by the API (task 65)
	Username string // smb only
	Domain   string // smb only

	Password  string `json:"-"`
	HasSecret bool

	ReadOnly  bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// String redacts the password so a Storage printed with %v or %+v, or logged,
// never shows it.
func (s Storage) String() string {
	return fmt.Sprintf("storage(%s %s)", s.Name, s.Protocol)
}

// storageColumns is the column list shared by every storage query. secret_enc
// is read only to derive HasSecret; the ciphertext never reaches the struct.
const storageColumns = `id, name, protocol, source, options, username, domain, secret_enc, read_only, enabled, created_at, updated_at`

// scanStorage reads one storage row.
func scanStorage(rows *sql.Rows) (*Storage, error) {
	var (
		st        Storage
		secretEnc string
		readOnly  string
		enabled   string
		created   string
		updated   string
	)
	if err := rows.Scan(&st.ID, &st.Name, &st.Protocol, &st.Source, &st.Options, &st.Username, &st.Domain,
		&secretEnc, &readOnly, &enabled, &created, &updated); err != nil {
		return nil, err
	}
	st.HasSecret = secretEnc != ""
	st.ReadOnly = parseBool(readOnly)
	st.Enabled = parseBool(enabled)
	st.CreatedAt = parseTime(created)
	st.UpdatedAt = parseTime(updated)
	return &st, nil
}

// validateStorage checks the fields the store owns. Option, source and
// credential rules belong to the API (task 65).
func validateStorage(st *Storage) error {
	if err := ValidStorageName(st.Name); err != nil {
		return err
	}
	for _, ok := range StorageProtocolValues() {
		if st.Protocol == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid storage protocol %q: want one of %v", st.Protocol, StorageProtocolValues())
}

// sealPassword returns the sealed form of password, bound to the storage id as
// additional data, or "" when there is no password. It fails closed when no
// storage key source is configured.
func (s *SQLite) sealPassword(id, password string) (string, error) {
	if password == "" {
		return "", nil
	}
	if s.secrets == nil {
		return "", errors.New("store: no storage key configured, a password cannot be stored")
	}
	box, err := s.secrets.Box()
	if err != nil {
		return "", fmt.Errorf("store: storage key: %w", err)
	}
	sealed, err := box.Seal([]byte(password), []byte(id))
	if err != nil {
		return "", fmt.Errorf("store: seal storage password: %w", err)
	}
	return sealed, nil
}

// mapStorageNameErr turns the unique-name violation into a clear error.
func mapStorageNameErr(err error, name string) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: storages.name") {
		return fmt.Errorf("store: storage name %q is already in use", name)
	}
	return err
}

// CreateStorage inserts a storage. ID and timestamps are filled when empty. A
// non-empty Password is sealed; it is cleared on success.
func (s *SQLite) CreateStorage(ctx context.Context, st *Storage) error {
	if err := validateStorage(st); err != nil {
		return err
	}
	if st.ID == "" {
		st.ID = NewID()
	}
	now := nowUTC()
	if st.CreatedAt.IsZero() {
		st.CreatedAt = now
	}
	st.UpdatedAt = now
	sealed, err := s.sealPassword(st.ID, st.Password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO storages (`+storageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Name, st.Protocol, st.Source, st.Options, st.Username, st.Domain, sealed,
		fmtBool(st.ReadOnly), fmtBool(st.Enabled), fmtTime(st.CreatedAt), fmtTime(st.UpdatedAt))
	if err != nil {
		return mapStorageNameErr(fmt.Errorf("store: create storage: %w", err), st.Name)
	}
	st.HasSecret = sealed != ""
	st.Password = ""
	return nil
}

// GetStorage returns one storage by id, or ErrNotFound.
func (s *SQLite) GetStorage(ctx context.Context, id string) (*Storage, error) {
	return s.getStorage(ctx, `SELECT `+storageColumns+` FROM storages WHERE id = ?`, id)
}

// GetStorageByName returns one storage by slug, or ErrNotFound.
func (s *SQLite) GetStorageByName(ctx context.Context, name string) (*Storage, error) {
	return s.getStorage(ctx, `SELECT `+storageColumns+` FROM storages WHERE name = ?`, name)
}

// getStorage runs a single-row storage query.
func (s *SQLite) getStorage(ctx context.Context, query string, arg string) (*Storage, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	st, err := scanStorage(rows)
	if err != nil {
		return nil, fmt.Errorf("store: scan storage: %w", err)
	}
	return st, nil
}

// ListStorages returns every storage ordered by name.
func (s *SQLite) ListStorages(ctx context.Context) ([]*Storage, error) {
	return queryAll(ctx, s.db, `SELECT `+storageColumns+` FROM storages ORDER BY name`, scanStorage)
}

// UpdateStorage overwrites an existing storage by ID. An empty Password keeps
// the stored one; a non-empty Password is re-sealed. On success *st is refreshed
// from the row, so HasSecret and UpdatedAt are current and Password is empty.
func (s *SQLite) UpdateStorage(ctx context.Context, st *Storage) error {
	if err := validateStorage(st); err != nil {
		return err
	}
	st.UpdatedAt = nowUTC()
	var (
		res sql.Result
		err error
	)
	if st.Password != "" {
		sealed, sealErr := s.sealPassword(st.ID, st.Password)
		if sealErr != nil {
			return sealErr
		}
		res, err = s.db.ExecContext(ctx, `UPDATE storages SET name = ?, protocol = ?, source = ?, options = ?, username = ?, domain = ?, secret_enc = ?, read_only = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			st.Name, st.Protocol, st.Source, st.Options, st.Username, st.Domain, sealed,
			fmtBool(st.ReadOnly), fmtBool(st.Enabled), fmtTime(st.UpdatedAt), st.ID)
	} else {
		res, err = s.db.ExecContext(ctx, `UPDATE storages SET name = ?, protocol = ?, source = ?, options = ?, username = ?, domain = ?, read_only = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			st.Name, st.Protocol, st.Source, st.Options, st.Username, st.Domain,
			fmtBool(st.ReadOnly), fmtBool(st.Enabled), fmtTime(st.UpdatedAt), st.ID)
	}
	if err != nil {
		return mapStorageNameErr(fmt.Errorf("store: update storage: %w", err), st.Name)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	fresh, err := s.GetStorage(ctx, st.ID)
	if err != nil {
		return err
	}
	*st = *fresh
	return nil
}

// DeleteStorage removes a storage row, or returns ErrNotFound. It does not
// unmount anything; the mount executor (task 66) owns that.
func (s *SQLite) DeleteStorage(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM storages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete storage: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// StorageSecret returns the plaintext password for a storage, for the mount
// executor only. It is the one read path that decrypts, and the API must not
// call it. A storage with no password returns "" and no error. A missing key,
// a tampered value or a wrong key returns an error and no plaintext.
func (s *SQLite) StorageSecret(ctx context.Context, id string) (string, error) {
	var sealed string
	err := s.db.QueryRowContext(ctx, `SELECT secret_enc FROM storages WHERE id = ?`, id).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: query storage secret: %w", err)
	}
	if sealed == "" {
		return "", nil
	}
	if s.secrets == nil {
		return "", errors.New("store: no storage key configured, the stored password cannot be opened")
	}
	box, err := s.secrets.Box()
	if err != nil {
		return "", fmt.Errorf("store: storage key: %w", err)
	}
	plain, err := box.Open(sealed, []byte(id))
	if err != nil {
		return "", fmt.Errorf("store: storage secret %s: %w", id, err)
	}
	return string(plain), nil
}

// Option configures New. Options are optional, so existing callers of
// New(path) are unaffected.
type Option func(*SQLite)

// WithSecrets gives the store the storage key source used to seal and open
// storage passwords. Without it, writing a password fails closed.
func WithSecrets(src *secret.Source) Option {
	return func(s *SQLite) { s.secrets = src }
}
