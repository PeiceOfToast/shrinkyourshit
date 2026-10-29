// SPDX-License-Identifier: AGPL-3.0-only

package worker

// Scratch tests (the policy tests moved with ApplyResultPolicy to
// internal/server/resultpolicy_test.go).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeTestFile writes content and returns its length.
func writeTestFile(t *testing.T, path, content string) int64 {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}


func TestExtractFilesListsScratchSubs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "output.mkv"), "x")
	writeTestFile(t, filepath.Join(dir, "output.eng.srt"), "a")
	writeTestFile(t, filepath.Join(dir, "output.eng.2.srt"), "b")
	writeTestFile(t, filepath.Join(dir, "output.ger.srt"), "c")
	writeTestFile(t, filepath.Join(dir, "other.eng.srt"), "d") // not our base

	s := newScratch(t.TempDir())
	files := s.extractFiles(dir, "output")
	require.Len(t, files, 3)
}

func TestScratchCleanupOrphans(t *testing.T) {
	root := t.TempDir()
	s := newScratch(root)
	dir, err := s.ensure("job-a")
	require.NoError(t, err)
	writeTestFile(t, filepath.Join(dir, "source.mkv"), "x")
	writeTestFile(t, filepath.Join(root, "stray"), "y")

	s.cleanupOrphans()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestScratchRemoveJob(t *testing.T) {
	s := newScratch(t.TempDir())
	dir, err := s.ensure("job-a")
	require.NoError(t, err)
	writeTestFile(t, filepath.Join(dir, "f"), "x")
	s.remove("job-a")
	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))
}
