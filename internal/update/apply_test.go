// SPDX-License-Identifier: AGPL-3.0-only

package update_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testInstall builds a throwaway install: a fake running binary, a data
// dir with a database file and empty scratch. It returns options pointed
// at the fake release server.
func testInstall(t *testing.T, hub *fakeHub, currentVersion, binContent string) (update.Options, string) {
	t.Helper()
	root := t.TempDir()
	dataDir := ensureDir(t, filepath.Join(root, "config"))
	scratch := ensureDir(t, filepath.Join(root, "scratch"))
	binDir := ensureDir(t, filepath.Join(root, "bin"))
	exe := filepath.Join(binDir, "shrinkyourshit")
	writeFile(t, exe, binContent)
	writeFile(t, filepath.Join(dataDir, "shrinkyourshit.db"), "fake-database-content")
	return update.Options{
		DataDir:    dataDir,
		ScratchDir: scratch,
		Channel:    update.ChannelStable,
		Current: update.Info{
			Version: currentVersion,
			Commit:  "abc1234",
			Date:    "2026-01-02T03:04:05Z",
		},
		APIBase: hub.server.URL,
		Executable: func() (string, error) {
			return exe, nil
		},
	}, exe
}

func TestSelfUpdateAndRollback(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "new-binary-content")
	opts, exe := testInstall(t, hub, "v1.2.0", "old-binary-content")

	applied, err := update.Apply(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, "v1.2.0", applied.From)
	assert.Equal(t, "v1.2.3", applied.To)
	assert.Equal(t, exe, applied.Path)
	assert.Equal(t, exe+".prev", applied.Prev)

	// The swap happened: new binary in place, previous kept for rollback,
	// no staging leftovers.
	assert.Equal(t, "new-binary-content", readFile(t, exe))
	assert.Equal(t, "old-binary-content", readFile(t, exe+".prev"))
	_, err = os.Stat(exe + ".new")
	assert.True(t, os.IsNotExist(err), "no .new staging file may survive")

	// The database was backed up before the swap, named for the target
	// version.
	assert.Equal(t, "fake-database-content",
		readFile(t, filepath.Join(opts.DataDir, "backups", "pre-update-v1.2.3.db")))

	// Up to date now: a restarted process on v1.2.3 asking again answers
	// ErrUpToDate and nothing runs.
	opts.Current.Version = "v1.2.3"
	_, err = update.Apply(context.Background(), opts)
	assert.ErrorIs(t, err, update.ErrUpToDate)

	// Rollback restores the previous binary and consumes .prev.
	path, err := update.Rollback(opts)
	require.NoError(t, err)
	assert.Equal(t, exe, path)
	assert.Equal(t, "old-binary-content", readFile(t, exe))
	_, err = os.Stat(exe + ".prev")
	assert.True(t, os.IsNotExist(err), "the previous copy is consumed by rollback")
}

func TestRollbackWithoutPreviousFails(t *testing.T) {
	opts, exe := testInstall(t, newFakeHub(t, "v1.2.3", "bin"), "v1.2.0", "old")
	_, err := update.Rollback(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), exe+".prev")
}

func TestChecksumMismatchAborts(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "tampered-binary")
	hub.wrong = true
	opts, exe := testInstall(t, hub, "v1.2.0", "old-binary-content")

	applied, err := update.Apply(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, update.ErrChecksumMismatch)
	assert.Equal(t, update.Applied{}, applied)

	// The running binary is untouched, nothing was swapped or staged.
	assert.Equal(t, "old-binary-content", readFile(t, exe))
	_, err = os.Stat(exe + ".prev")
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(exe + ".new")
	assert.True(t, os.IsNotExist(err))

	// The tampered archive does not survive in the scratch dir either.
	_, err = os.Stat(filepath.Join(opts.ScratchDir, "update", update.ArchiveName("linux", "amd64")))
	assert.True(t, os.IsNotExist(err))
}

func TestChecksumEntryMissingAborts(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "binary")
	opts, exe := testInstall(t, hub, "v1.2.0", "old")
	// checksums.txt serves only another asset: the archive it names is
	// absent from the checksum file, so the download is refused.
	_ = hub
	hub.wrong = false
	// Simulate a missing entry by pointing the checker at an empty list.
	hub.checksumBody = "# no entries\n"
	_, err := update.Apply(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, update.ErrNoChecksumEntry)
	assert.Equal(t, "old", readFile(t, exe))
}

func TestContainerRefusesSelfUpdate(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	opts, exe := testInstall(t, hub, "v1.2.0", "old")
	t.Setenv("SHRINK_IN_CONTAINER", "1")

	applied, err := update.Apply(context.Background(), opts)
	require.Error(t, err)

	var cerr *update.ContainerError
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, "docker compose pull && docker compose up -d", cerr.Instruction)
	assert.Contains(t, err.Error(), "docker compose pull")
	assert.Equal(t, update.Applied{}, applied)

	// Nothing was touched: no backup dir, no release cache, no downloads.
	// (The fixture database file itself is the test's own, not the
	// updater's.)
	_, err = os.Stat(filepath.Join(opts.DataDir, "backups"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(opts.DataDir, "update-cache-stable.json"))
	assert.True(t, os.IsNotExist(err))
	entries, err := os.ReadDir(opts.ScratchDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the scratch dir must stay untouched")
	assert.Equal(t, "old", readFile(t, exe))
	assert.Equal(t, 0, hub.request, "no API call may even be attempted")
}

func TestApplyRefusesUnknownVersion(t *testing.T) {
	for _, v := range []string{"", "v0.0.0"} {
		hub := newFakeHub(t, "v1.2.3", "bin")
		opts, exe := testInstall(t, hub, v, "old")
		_, err := update.Apply(context.Background(), opts)
		assert.ErrorIs(t, err, update.ErrUnknownVersion, "version %q", v)
		assert.Equal(t, "old", readFile(t, exe))
		assert.Equal(t, 0, hub.request)
	}
}

func TestPreUpdateBackupRetention(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	opts, _ := testInstall(t, hub, "v1.2.0", "old")

	// Five snapshots from previous updates, oldest first.
	backups := filepath.Join(opts.DataDir, "backups")
	require.NoError(t, os.MkdirAll(backups, 0o700))
	for i := 1; i <= 5; i++ {
		p := filepath.Join(backups, fmt.Sprintf("pre-update-v0.0.%d.db", i))
		writeFile(t, p, "snapshot")
		mod := time.Now().Add(-time.Duration(6-i) * time.Hour)
		require.NoError(t, os.Chtimes(p, mod, mod))
	}

	applied, err := update.Apply(context.Background(), opts)
	require.NoError(t, err)

	// The new snapshot exists and only the three newest snapshots remain.
	require.NotEmpty(t, applied.BackupPath)
	entries, err := os.ReadDir(backups)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Len(t, names, 3, "at most three backups are kept, got %v", names)
	assert.Contains(t, names, "pre-update-v1.2.3.db")
	assert.Contains(t, names, "pre-update-v0.0.5.db")
	assert.Contains(t, names, "pre-update-v0.0.4.db")
}

func TestApplySkipBackup(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	opts, _ := testInstall(t, hub, "v1.2.0", "old")
	opts.SkipBackup = true

	applied, err := update.Apply(context.Background(), opts)
	require.NoError(t, err)
	assert.Empty(t, applied.BackupPath)
	_, err = os.Stat(filepath.Join(opts.DataDir, "backups"))
	assert.True(t, os.IsNotExist(err))
}

func TestApplyCustomBackupFunc(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	opts, _ := testInstall(t, hub, "v1.2.0", "old")
	var got string
	opts.Backup = func(ctx context.Context, dest string) error {
		got = dest
		return os.WriteFile(dest, []byte("vacuum-into-snapshot"), 0o600)
	}

	applied, err := update.Apply(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, applied.BackupPath, got)
	assert.Equal(t, "vacuum-into-snapshot", readFile(t, got))
}

func TestRollbackRestoresUpdatedBinary(t *testing.T) {
	// A rollback followed by a second rollback fails cleanly: one level of
	// history is the contract.
	hub := newFakeHub(t, "v1.2.3", "new")
	opts, _ := testInstall(t, hub, "v1.2.0", "old")
	_, err := update.Apply(context.Background(), opts)
	require.NoError(t, err)
	_, err = update.Rollback(opts)
	require.NoError(t, err)
	_, err = update.Rollback(opts)
	require.Error(t, err)
}
