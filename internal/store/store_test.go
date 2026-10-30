// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTemp opens a fresh SQLite store in a temp dir.
func openTemp(t *testing.T) *SQLite {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func validLibrary() *Library {
	return &Library{
		Name:         "Movies",
		Path:         "/media/movies",
		TargetJSON:   `{"video":{"codec":"hevc","quality":3},"audio":{"mode":"keep_all"},"subtitles":{"mode":"keep_all"},"container":"mkv"}`,
		Watch:        true,
		Schedule:     "0 3 * * *",
		ResultPolicy: ResultPolicyReplaceIfSmaller,
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	first, err := New(path)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, first.CreateLibrary(ctx, validLibrary()))
	require.NoError(t, first.Close())

	// Re-opening applies migrations again against the existing data:
	// tracked in schema_migrations, so nothing is re-run and the row
	// survives.
	second, err := New(path)
	require.NoError(t, err)
	defer func() { _ = second.Close() }()
	libs, err := second.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 1)
	assert.Equal(t, "Movies", libs[0].Name)

	// schema_migrations recorded every migration exactly once (the count
	// tracks the number of embedded migration files).
	migrations, err := loadMigrations()
	require.NoError(t, err)
	require.NotEmpty(t, migrations)
	var count int
	require.NoError(t, second.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, len(migrations), count)
}

func TestLibraryCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))
	assert.NotEmpty(t, lib.ID)
	assert.False(t, lib.CreatedAt.IsZero())
	assert.Equal(t, lib.CreatedAt, lib.UpdatedAt)

	got, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, lib.Name, got.Name)
	assert.Equal(t, lib.Path, got.Path)
	assert.Equal(t, lib.TargetJSON, got.TargetJSON)
	assert.True(t, got.Watch)
	assert.Equal(t, "0 3 * * *", got.Schedule)
	assert.Equal(t, ResultPolicyReplaceIfSmaller, got.ResultPolicy)

	got.Name = "Series"
	require.NoError(t, s.UpdateLibrary(ctx, got))
	updated, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Series", updated.Name)
	assert.False(t, updated.UpdatedAt.Equal(updated.CreatedAt), "update refreshes updated_at")

	// Second library lists both, oldest first.
	require.NoError(t, s.CreateLibrary(ctx, validLibrary()))
	libs, err := s.ListLibraries(ctx)
	require.NoError(t, err)
	assert.Len(t, libs, 2)

	// Cascade: deleting the library removes its files and jobs.
	f := &File{LibraryID: lib.ID, Path: "/media/movies/a.mkv", Status: FileStatusNew}
	require.NoError(t, s.UpsertFile(ctx, f))
	require.NoError(t, s.DeleteLibrary(ctx, lib.ID))
	_, err = s.GetFile(ctx, f.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = s.GetLibrary(ctx, lib.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.GetLibrary(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestLibraryEasyRoundTrip covers the easy column (task 07): it persists on
// create and update, and stays empty for target-only libraries. Existing
// rows written before migration 0003 read back with an empty easy value.
func TestLibraryEasyRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	// An easy-mode library stores both the compiled target and the easy
	// input that produced it.
	lib := validLibrary()
	lib.EasyJSON = `{"quality":3,"remove_subtitles":true,"audio":2}`
	require.NoError(t, s.CreateLibrary(ctx, lib))
	got, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, lib.EasyJSON, got.EasyJSON)

	// Saving new easy state overwrites it.
	got.EasyJSON = `{"quality":5,"remove_subtitles":false,"audio":4}`
	require.NoError(t, s.UpdateLibrary(ctx, got))
	updated, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, `{"quality":5,"remove_subtitles":false,"audio":4}`, updated.EasyJSON)

	// A target-only library (advanced editor) has an empty easy value.
	plain := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, plain))
	got, err = s.GetLibrary(ctx, plain.ID)
	require.NoError(t, err)
	assert.Empty(t, got.EasyJSON)

	// A row written before migration 0003 (easy column default '') lists
	// with an empty easy value rather than failing the scan.
	libs, err := s.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 2)
	for _, l := range libs {
		assert.NotNil(t, l)
	}
}

// TestLibraryLastScanTime covers the last_scan_at column (task 19): a fresh
// library has never been scanned (zero time), SetLibraryScanTime stamps it
// without touching the rest of the row, and an unknown id is ErrNotFound.
func TestLibraryLastScanTime(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))
	got, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.True(t, got.LastScanAt.IsZero(), "a fresh library has never been scanned")

	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.SetLibraryScanTime(ctx, lib.ID, at))
	stamped, err := s.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.True(t, stamped.LastScanAt.Equal(at), "the stamp round trips")
	// The dedicated update must not touch the rest of the row.
	assert.Equal(t, lib.Name, stamped.Name)
	assert.True(t, stamped.UpdatedAt.Equal(got.UpdatedAt), "the stamp leaves updated_at alone")

	err = s.SetLibraryScanTime(ctx, "does-not-exist", at)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFileCRUDAndUpsert(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))

	f := &File{
		LibraryID:       lib.ID,
		Path:            "/media/movies/movie.mkv",
		Size:            123,
		Mtime:           1700000000,
		Fingerprint:     "123:1700000000:abc",
		ProbeJSON:       `{"container":"mkv"}`,
		Status:          FileStatusNew,
		SatisfiedReason: "video is h264, target is hevc",
	}
	require.NoError(t, s.UpsertFile(ctx, f))
	assert.NotEmpty(t, f.ID)

	got, err := s.GetFile(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, f.ProbeJSON, got.ProbeJSON)
	assert.Equal(t, FileStatusNew, got.Status)
	assert.Equal(t, f.SatisfiedReason, got.SatisfiedReason)

	// Same (library, path) upserts instead of duplicating.
	f.Status = FileStatusSatisfied
	f.SatisfiedReason = ""
	f.BytesSaved = 500
	require.NoError(t, s.UpsertFile(ctx, f))
	files, err := s.ListFiles(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, FileStatusSatisfied, files[0].Status)
	assert.Equal(t, int64(500), files[0].BytesSaved)

	// By-path lookup.
	byPath, err := s.GetFileByPath(ctx, lib.ID, "/media/movies/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, f.ID, byPath.ID)
	_, err = s.GetFileByPath(ctx, lib.ID, "/media/movies/other.mkv")
	assert.ErrorIs(t, err, ErrNotFound)

	// Status filter and update.
	require.NoError(t, s.UpdateFile(ctx, &File{
		ID: f.ID, LibraryID: lib.ID, Path: "/media/movies/movie.mkv",
		Status: FileStatusQueued, ProbeJSON: got.ProbeJSON,
	}))
	queued, err := s.ListFilesByStatus(ctx, lib.ID, FileStatusQueued)
	require.NoError(t, err)
	assert.Len(t, queued, 1)
	_, err = s.ListFilesByStatus(ctx, lib.ID, FileStatusDone)
	require.NoError(t, err, "every defined status is a valid filter")

	_, err = s.GetFile(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.DeleteFile(ctx, f.ID))
	files, err = s.ListFiles(ctx, lib.ID)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestJobCRUDAndFilter(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))
	f := &File{LibraryID: lib.ID, Path: "/media/movies/a.mkv", Status: FileStatusQueued}
	require.NoError(t, s.UpsertFile(ctx, f))

	j := &Job{
		FileID:      f.ID,
		LibraryID:   lib.ID,
		Status:      JobStatusQueued,
		Priority:    5,
		Attempt:     1,
		Percent:     42.5,
		Speed:       3.2,
		ETASeconds:  120,
		BytesBefore: 1000,
		BytesAfter:  400,
		Log:         "progress",
	}
	require.NoError(t, s.CreateJob(ctx, j))
	assert.NotEmpty(t, j.ID)

	got, err := s.GetJob(ctx, j.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusQueued, got.Status)
	assert.Equal(t, 5, got.Priority)
	assert.Equal(t, 42.5, got.Percent)
	assert.Equal(t, 3.2, got.Speed)
	assert.Equal(t, int64(120), got.ETASeconds)
	assert.Equal(t, "[]", got.FallbackReasons, "empty fallback reasons default to []")

	got.Status = JobStatusRunning
	got.NodeID = "node-1"
	got.EncoderUsed = "nvenc"
	got.FallbackReasons = `["no gpu"]`
	got.AccessMode = NodeAccessTransfer
	require.NoError(t, s.UpdateJob(ctx, got))
	updated, err := s.GetJob(ctx, j.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusRunning, updated.Status)
	assert.Equal(t, `["no gpu"]`, updated.FallbackReasons)
	assert.Equal(t, NodeAccessTransfer, updated.AccessMode, "job access mode round-trips")

	// Filters.
	done := &Job{FileID: f.ID, LibraryID: lib.ID, Status: JobStatusDone}
	require.NoError(t, s.CreateJob(ctx, done))
	all, err := s.ListJobs(ctx, JobFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
	running, err := s.ListJobs(ctx, JobFilter{Status: JobStatusRunning})
	require.NoError(t, err)
	require.Len(t, running, 1)
	assert.Equal(t, j.ID, running[0].ID)

	_, err = s.GetJob(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.DeleteJob(ctx, j.ID))
	_, err = s.GetJob(ctx, j.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestNodeCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	n := &Node{
		Name:         "worker-1",
		KeyHash:      "hash",
		OS:           "linux",
		Arch:         "amd64",
		Capabilities: `{"encoders":[{"name":"nvenc","codecs":["hevc"]}]}`,
		AccessMode:   NodeAccessAuto,
		PathMappings: `[{"library":"/media","node":"/mnt/media"}]`,
		Status:       NodeStatusPending,
		Enabled:      true,
	}
	require.NoError(t, s.CreateNode(ctx, n))
	assert.NotEmpty(t, n.ID)

	got, err := s.GetNode(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, "worker-1", got.Name)
	assert.Equal(t, NodeStatusPending, got.Status)
	assert.Equal(t, NodeAccessAuto, got.AccessMode)
	assert.True(t, got.Enabled)
	assert.False(t, got.CreatedAt.IsZero())

	got.Status = NodeStatusApproved
	got.Name = "renamed"
	got.ThroughputLimit = 25 // MB/s
	require.NoError(t, s.UpdateNode(ctx, got))
	updated, err := s.GetNode(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, NodeStatusApproved, updated.Status)
	assert.Equal(t, "renamed", updated.Name)
	assert.Equal(t, int64(25), updated.ThroughputLimit, "node throughput limit round-trips")

	require.NoError(t, s.CreateNode(ctx, &Node{Name: "worker-2", AccessMode: NodeAccessTransfer, Status: NodeStatusPending}))
	nodes, err := s.ListNodes(ctx)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	_, err = s.GetNode(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.DeleteNode(ctx, n.ID))
	_, err = s.GetNode(ctx, n.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPairingTokenCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	tok := &PairingToken{TokenHash: "abc123", ExpiresAt: nowUTC().Add(24 * time.Hour)}
	require.NoError(t, s.CreatePairingToken(ctx, tok))
	assert.NotEmpty(t, tok.ID)

	byHash, err := s.GetPairingTokenByHash(ctx, "abc123")
	require.NoError(t, err)
	assert.Equal(t, tok.ID, byHash.ID)
	assert.True(t, byHash.ExpiresAt.After(nowUTC()))
	assert.True(t, byHash.UsedAt.IsZero())

	byID, err := s.GetPairingToken(ctx, tok.ID)
	require.NoError(t, err)
	assert.Equal(t, "abc123", byID.TokenHash)

	// Mark used.
	byHash.UsedAt = nowUTC()
	require.NoError(t, s.UpdatePairingToken(ctx, byHash))
	used, err := s.GetPairingTokenByHash(ctx, "abc123")
	require.NoError(t, err)
	assert.False(t, used.UsedAt.IsZero())

	toks, err := s.ListPairingTokens(ctx)
	require.NoError(t, err)
	assert.Len(t, toks, 1)

	require.NoError(t, s.DeletePairingToken(ctx, tok.ID))
	_, err = s.GetPairingTokenByHash(ctx, "abc123")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSettingsCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	_, ok, err := s.GetSetting(ctx, SettingLoginEnabled)
	require.NoError(t, err)
	assert.False(t, ok, "missing key reports not-found without error")

	require.NoError(t, s.SetSetting(ctx, SettingLoginEnabled, "true"))
	value, ok, err := s.GetSetting(ctx, SettingLoginEnabled)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "true", value)

	// Overwrite.
	require.NoError(t, s.SetSetting(ctx, SettingLoginEnabled, "false"))
	value, ok, _ = s.GetSetting(ctx, SettingLoginEnabled)
	assert.True(t, ok)
	assert.Equal(t, "false", value)

	require.NoError(t, s.SetSetting(ctx, SettingWorkerConcurrency, "2"))
	all, err := s.ListSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		SettingLoginEnabled:      "false",
		SettingWorkerConcurrency: "2",
	}, all)

	require.NoError(t, s.DeleteSetting(ctx, SettingLoginEnabled))
	_, ok, err = s.GetSetting(ctx, SettingLoginEnabled)
	require.NoError(t, err)
	assert.False(t, ok)

	// Deleting a missing key is not an error.
	require.NoError(t, s.DeleteSetting(ctx, SettingLoginEnabled))
}

func TestEnumValidationRejectsBadValues(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))

	// Result policy.
	bad := validLibrary()
	bad.ResultPolicy = "overwrite_always"
	assert.Error(t, s.CreateLibrary(ctx, bad))
	bad2 := validLibrary()
	bad2.ResultPolicy = ResultPolicyKeepBoth
	assert.NoError(t, s.CreateLibrary(ctx, bad2))

	// File status.
	for _, status := range []string{"gone", "", "SATISFIED"} {
		err := s.UpsertFile(ctx, &File{LibraryID: lib.ID, Path: "/x.mkv", Status: status})
		assert.Error(t, err, "file status %q must be rejected", status)
	}
	for _, status := range FileStatusValues() {
		f := &File{LibraryID: lib.ID, Path: "/f-" + status + ".mkv", Status: status}
		assert.NoError(t, s.UpsertFile(ctx, f), "defined status %q must be accepted", status)
		_ = s.DeleteFile(ctx, f.ID)
	}

	// Job status.
	for _, status := range []string{"working", ""} {
		assert.Error(t, s.CreateJob(ctx, &Job{FileID: "x", LibraryID: lib.ID, Status: status}), "job status %q", status)
	}
	f := &File{LibraryID: lib.ID, Path: "/j.mkv", Status: FileStatusNew}
	require.NoError(t, s.UpsertFile(ctx, f))
	for _, status := range JobStatusValues() {
		assert.NoError(t, s.CreateJob(ctx, &Job{FileID: f.ID, LibraryID: lib.ID, Status: status}), "job status %q", status)
	}

	// Node status and access mode.
	for _, status := range []string{"active", ""} {
		assert.Error(t, s.CreateNode(ctx, &Node{Name: "n", Status: status}), "node status %q", status)
	}
	for _, mode := range []string{"both", ""} {
		assert.Error(t, s.CreateNode(ctx, &Node{Name: "n", Status: NodeStatusPending, AccessMode: mode}), "access mode %q", mode)
	}
	for _, status := range NodeStatusValues() {
		assert.NoError(t, s.CreateNode(ctx, &Node{Name: "n-" + status, Status: status, AccessMode: NodeAccessShared}), "node status %q", status)
	}
	for _, mode := range NodeAccessModeValues() {
		assert.NoError(t, s.CreateNode(ctx, &Node{Name: "m-" + mode, Status: NodeStatusApproved, AccessMode: mode}), "access mode %q", mode)
	}
}

func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	uuidV4 := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	for i := 0; i < 100; i++ {
		id := NewID()
		assert.Regexp(t, uuidV4, id)
		assert.False(t, seen[id], "ids must be unique")
		seen[id] = true
	}
}
