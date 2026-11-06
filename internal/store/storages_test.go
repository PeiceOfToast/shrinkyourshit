// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storageEnvKey returns a base64 key of 32 copies of fill, for SHRINK_STORAGE_KEY.
func storageEnvKey(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, secret.KeySize))
}

// openStorageDB opens a fresh database with the given key source.
func openStorageDB(t *testing.T, src *secret.Source) (*SQLite, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := New(path, WithSecrets(src))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func validStorage(name string) *Storage {
	return &Storage{
		Name:     name,
		Protocol: StorageProtocolSMB,
		Source:   "//nas.lan/media",
		Options:  "vers=3.0",
		Username: "media",
		Domain:   "LAB",
		Password: "hunter2-correct-horse",
		ReadOnly: false,
		Enabled:  true,
	}
}

// A database created by v1.10 (migrations through 0016 applied, legacy rows in
// place) migrates to 0017 on open. A second open is a no-op.
func TestStoragesMigrationFromV110(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`)
	require.NoError(t, err)
	migrations, err := loadMigrations()
	require.NoError(t, err)
	for _, m := range migrations {
		if m.name >= "0017_storages.sql" {
			continue
		}
		_, err := db.ExecContext(ctx, m.sql)
		require.NoError(t, err, m.name)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.name, "2026-11-01T00:00:00Z")
		require.NoError(t, err, m.name)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO libraries (id, name, path, target, easy, flow_json, flow_source, watch, schedule, result_policy, created_at, updated_at, file_kinds, rule_tier)
		VALUES ('lib1', 'Movies', '/media/movies', '{}', '', '', '', 0, '', 'replace_if_smaller', '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z', '["video"]', 'basic')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	st, err := New(path)
	require.NoError(t, err, "the v1.11 release opens a v1.10 database")
	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 1, "legacy rows survive the migration")
	list, err := st.ListStorages(ctx)
	require.NoError(t, err)
	assert.Empty(t, list, "the new table starts empty")
	require.NoError(t, st.Close())

	st, err = New(path)
	require.NoError(t, err, "a second open is a no-op")
	defer func() { _ = st.Close() }()

	var applied int
	require.NoError(t, st.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE name = '0017_storages.sql'`).Scan(&applied))
	assert.Equal(t, 1, applied, "0017 is recorded exactly once")
}

// Create, read, list, update and delete round trip every column.
func TestStorageCRUDRoundTrip(t *testing.T) {
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(1), ""))
	ctx := context.Background()

	st := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, st))
	require.NotEmpty(t, st.ID)
	assert.True(t, st.HasSecret, "a password was stored")
	assert.Empty(t, st.Password, "the write-only password is cleared after create")

	got, err := s.GetStorage(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, "movies", got.Name)
	assert.Equal(t, StorageProtocolSMB, got.Protocol)
	assert.Equal(t, "//nas.lan/media", got.Source)
	assert.Equal(t, "vers=3.0", got.Options)
	assert.Equal(t, "media", got.Username)
	assert.Equal(t, "LAB", got.Domain)
	assert.False(t, got.ReadOnly)
	assert.True(t, got.Enabled)
	assert.True(t, got.HasSecret)
	assert.False(t, got.CreatedAt.IsZero())

	byName, err := s.GetStorageByName(ctx, "movies")
	require.NoError(t, err)
	assert.Equal(t, st.ID, byName.ID)

	second := validStorage("archive")
	second.Protocol = StorageProtocolNFS
	second.Source = "nas.lan:/export/archive"
	second.Password = ""
	require.NoError(t, s.CreateStorage(ctx, second))
	assert.False(t, second.HasSecret, "no password, no secret")

	list, err := s.ListStorages(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "archive", list[0].Name, "ordered by name")
	assert.Equal(t, "movies", list[1].Name)

	got.ReadOnly = true
	got.Enabled = false
	got.Options = "vers=3.1"
	require.NoError(t, s.UpdateStorage(ctx, got))
	assert.True(t, got.ReadOnly)
	assert.False(t, got.Enabled)
	assert.True(t, got.HasSecret, "an update without a password keeps the stored one")

	reread, err := s.GetStorage(ctx, st.ID)
	require.NoError(t, err)
	assert.True(t, reread.ReadOnly)
	assert.False(t, reread.Enabled)
	assert.Equal(t, "vers=3.1", reread.Options)

	require.NoError(t, s.DeleteStorage(ctx, st.ID))
	_, err = s.GetStorage(ctx, st.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, s.DeleteStorage(ctx, st.ID), ErrNotFound)
	assert.ErrorIs(t, s.UpdateStorage(ctx, &Storage{ID: st.ID, Name: "movies", Protocol: StorageProtocolSMB}), ErrNotFound)
}

// The password never comes back from any read path. It is absent from the
// structs, from JSON, from %v and %+v output, and from the stored row, which
// holds only ciphertext. StorageSecret is the one read that returns it.
func TestStoragePasswordNeverReturned(t *testing.T) {
	const password = "hunter2-correct-horse"
	s, path := openStorageDB(t, secret.NewSource(storageEnvKey(2), ""))
	ctx := context.Background()

	st := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, st))

	check := func(label string, got *Storage) {
		t.Helper()
		assert.Empty(t, got.Password, label)
		assert.True(t, got.HasSecret, label)
		body, err := json.Marshal(got)
		require.NoError(t, err)
		assert.NotContains(t, string(body), password, label+": JSON")
		assert.NotContains(t, fmt.Sprintf("%v %+v %#v", got, got, got), password, label+": fmt")
	}

	got, err := s.GetStorage(ctx, st.ID)
	require.NoError(t, err)
	check("GetStorage", got)

	byName, err := s.GetStorageByName(ctx, "movies")
	require.NoError(t, err)
	check("GetStorageByName", byName)

	list, err := s.ListStorages(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	check("ListStorages", list[0])

	// Update with a new password: still never returned.
	got.Password = "a-new-password"
	require.NoError(t, s.UpdateStorage(ctx, got))
	assert.Empty(t, got.Password)
	assert.NotContains(t, fmt.Sprintf("%+v", got), "a-new-password")

	// The stored row holds only ciphertext.
	var raw string
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT secret_enc FROM storages WHERE id = ?`, st.ID).Scan(&raw))
	assert.NotEmpty(t, raw)
	assert.NotContains(t, raw, "a-new-password")
	assert.NotContains(t, raw, password)
	require.NoError(t, s.Close())

	// The database file itself never holds the plaintext password.
	fileBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(fileBytes, []byte("a-new-password")), "plaintext in the database file")

	// StorageSecret is the single path that returns the plaintext.
	reopened, err := New(path, WithSecrets(secret.NewSource(storageEnvKey(2), "")))
	require.NoError(t, err)
	defer func() { _ = reopened.Close() }()
	plain, err := reopened.StorageSecret(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, "a-new-password", plain)
}

// Storage name validation: slug rules, enforced by the store.
func TestStorageNameValidation(t *testing.T) {
	good := []string{"movies", "nas-2", "a", "0", "tv-shows-2026"}
	bad := []string{"", "Movies", "a_b", "-x", "x-", "a b", "a/b", "../etc", "a.b", "é", "-", strings.Repeat("a", 65)}
	for _, name := range good {
		assert.NoError(t, ValidStorageName(name), name)
	}
	for _, name := range bad {
		assert.Error(t, ValidStorageName(name), "%q must be rejected", name)
	}
	assert.NoError(t, ValidStorageName(strings.Repeat("a", 64)), "64 characters is the limit")

	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(3), ""))
	ctx := context.Background()
	st := validStorage("Bad_Name")
	err := s.CreateStorage(ctx, st)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bad_Name")
}

// Names are unique, and the error says so.
func TestStorageNameUnique(t *testing.T) {
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(4), ""))
	ctx := context.Background()
	require.NoError(t, s.CreateStorage(ctx, validStorage("movies")))

	err := s.CreateStorage(ctx, validStorage("movies"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in use")
}

// Protocol must be nfs or smb.
func TestStorageProtocolValidation(t *testing.T) {
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(5), ""))
	ctx := context.Background()
	st := validStorage("movies")
	st.Protocol = "ftp"
	assert.Error(t, s.CreateStorage(ctx, st))
}

// An update that omits the password keeps the sealed one; a new password
// replaces it.
func TestStorageUpdatePasswordSemantics(t *testing.T) {
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(6), ""))
	ctx := context.Background()
	st := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, st))

	before, err := s.StorageSecret(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, "hunter2-correct-horse", before)

	st.Enabled = false
	require.NoError(t, s.UpdateStorage(ctx, st))
	kept, err := s.StorageSecret(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, before, kept, "empty password keeps the stored one")

	st.Password = "rotated"
	require.NoError(t, s.UpdateStorage(ctx, st))
	rotated, err := s.StorageSecret(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, "rotated", rotated)
}

// A storage without a password has no secret: StorageSecret returns "".
func TestStorageSecretEmptyWhenNoPassword(t *testing.T) {
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(7), ""))
	ctx := context.Background()
	st := validStorage("open")
	st.Protocol = StorageProtocolNFS
	st.Password = ""
	require.NoError(t, s.CreateStorage(ctx, st))

	plain, err := s.StorageSecret(ctx, st.ID)
	require.NoError(t, err)
	assert.Empty(t, plain)

	_, err = s.StorageSecret(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

// Without a storage key source, a password cannot be stored: fail closed.
func TestStorageNoKeyFailsClosed(t *testing.T) {
	s, _ := openStorageDB(t, nil)
	ctx := context.Background()

	st := validStorage("movies")
	err := s.CreateStorage(ctx, st)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no storage key")
	_, err = s.GetStorageByName(ctx, "movies")
	assert.ErrorIs(t, err, ErrNotFound, "nothing was written")

	// A passwordless storage still works without a key.
	open := validStorage("open")
	open.Protocol = StorageProtocolNFS
	open.Password = ""
	require.NoError(t, s.CreateStorage(ctx, open))
}

// A wrong key fails closed with an error and no plaintext.
func TestStorageWrongKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, path := openStorageDB(t, secret.NewSource(storageEnvKey(8), ""))
	st := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, st))
	require.NoError(t, s.Close())

	other, err := New(path, WithSecrets(secret.NewSource(storageEnvKey(9), "")))
	require.NoError(t, err)
	defer func() { _ = other.Close() }()

	plain, err := other.StorageSecret(ctx, st.ID)
	assert.Error(t, err)
	assert.Empty(t, plain)
	assert.NotContains(t, fmt.Sprint(err), "hunter2")
}

// A tampered or truncated ciphertext fails closed.
func TestStorageTamperedCiphertextFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(10), ""))
	st := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, st))

	var sealed string
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT secret_enc FROM storages WHERE id = ?`, st.ID).Scan(&sealed))
	raw, err := base64.StdEncoding.DecodeString(sealed)
	require.NoError(t, err)

	flipped := append([]byte(nil), raw...)
	flipped[len(flipped)-1] ^= 0x01
	truncated := raw[:len(raw)-5]

	for label, value := range map[string][]byte{"tampered": flipped, "truncated": truncated} {
		_, err := s.db.ExecContext(ctx, `UPDATE storages SET secret_enc = ? WHERE id = ?`,
			base64.StdEncoding.EncodeToString(value), st.ID)
		require.NoError(t, err)
		plain, err := s.StorageSecret(ctx, st.ID)
		assert.Error(t, err, label)
		assert.Empty(t, plain, label)
	}
}

// A sealed password copied to another row does not open: the storage id is
// bound into the ciphertext as additional data.
func TestStorageSecretBoundToRow(t *testing.T) {
	ctx := context.Background()
	s, _ := openStorageDB(t, secret.NewSource(storageEnvKey(11), ""))
	a := validStorage("movies")
	require.NoError(t, s.CreateStorage(ctx, a))
	b := validStorage("archive")
	b.Password = "other"
	require.NoError(t, s.CreateStorage(ctx, b))

	var sealedA string
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT secret_enc FROM storages WHERE id = ?`, a.ID).Scan(&sealedA))
	_, err := s.db.ExecContext(ctx, `UPDATE storages SET secret_enc = ? WHERE id = ?`, sealedA, b.ID)
	require.NoError(t, err)

	plain, err := s.StorageSecret(ctx, b.ID)
	assert.Error(t, err)
	assert.Empty(t, plain)
}

// The key source creates the key file on first use, not at open.
func TestStorageKeyFileCreatedOnFirstUse(t *testing.T) {
	ctx := context.Background()
	keyPath := filepath.Join(t.TempDir(), "storage.key")
	s, _ := openStorageDB(t, secret.NewSource("", keyPath))
	open := &Storage{Name: "open", Protocol: StorageProtocolNFS, Source: "nas:/x", Enabled: true}
	require.NoError(t, s.CreateStorage(ctx, open))
	assert.NoFileExists(t, keyPath, "no password, so no key is needed yet")

	require.NoError(t, s.CreateStorage(ctx, validStorage("movies")))
	assert.FileExists(t, keyPath)
}
