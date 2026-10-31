// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// policyTestFS builds a scratch output, an original library file and a
// scratch extract.
type policyTestFS struct {
	t        *testing.T
	dir      string
	output   string
	original string
	extract  string
	outSize  int64
	origSize int64
}

func newPolicyFS(t *testing.T, origName, origExt string) *policyTestFS {
	t.Helper()
	dir := t.TempDir()
	p := &policyTestFS{
		t:        t,
		dir:      dir,
		output:   filepath.Join(dir, "scratch", "output.mkv"),
		original: filepath.Join(dir, "library", origName+origExt),
		extract:  filepath.Join(dir, "scratch", "output.eng.srt"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(p.output), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(p.original), 0o755))
	p.outSize = writeTestFile(t, p.output, "OUTPUT-DATA-1000")
	p.origSize = writeTestFile(t, p.original, "ORIGINAL-DATA")
	writeTestFile(t, p.extract, "1\n00:00:00,000 --> 00:00:01,000\nHi\n")
	return p
}

// writeTestFile writes content and returns its length.
func writeTestFile(t *testing.T, path, content string) int64 {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}

func TestPolicyReplaceIfSmallerSkips(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplaceIfSmaller,
		p.output, p.original, p.outSize, p.origSize, nil, "", MoveDirective{})
	require.NoError(t, err)
	require.NotEmpty(t, res.SkipReason)
	require.Empty(t, res.FinalPath)
	require.Contains(t, res.SkipReason, "not smaller")
	// Original untouched.
	content, err := os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL-DATA", string(content))
}

func TestPolicyReplaceIfSmallerReplaces(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplaceIfSmaller,
		p.output, p.original, p.origSize-1, p.origSize, nil, "", MoveDirective{})
	require.NoError(t, err)
	require.Empty(t, res.SkipReason)
	require.Equal(t, filepath.Join(p.dir, "library", "movie.mkv"), res.FinalPath)

	// The new file exists with the new extension; the old one is gone.
	content, err := os.ReadFile(res.FinalPath)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
	_, err = os.Stat(p.original)
	require.True(t, os.IsNotExist(err))

	// No staging leftovers.
	leftovers, err := filepath.Glob(filepath.Join(p.dir, "library", ".*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
}

func TestPolicyReplaceSameExtensionRemovesNothing(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mkv")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, nil, "", MoveDirective{})
	require.NoError(t, err)
	require.Equal(t, p.original, res.FinalPath) // replaced in place
	content, err := os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
}

func TestPolicyKeepBoth(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyKeepBoth,
		p.output, p.original, p.outSize, p.origSize, []string{p.extract}, "", MoveDirective{})
	require.NoError(t, err)
	require.Empty(t, res.SkipReason)

	sidecar := filepath.Join(p.dir, "library", "movie.shrunk.mkv")
	require.Equal(t, sidecar, res.FinalPath)
	content, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))

	// The original is untouched and the extract moved beside the sidecar.
	content, err = os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL-DATA", string(content))
	extracted, err := os.ReadFile(filepath.Join(p.dir, "library", "movie.shrunk.eng.srt"))
	require.NoError(t, err)
	require.Contains(t, string(extracted), "Hi")
}

func TestPolicyExtractsFollowRenamedOutput(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, []string{p.extract}, "", MoveDirective{})
	require.NoError(t, err)
	extracted, err := os.ReadFile(filepath.Join(p.dir, "library", "movie.eng.srt"))
	require.NoError(t, err)
	require.Contains(t, string(extracted), "Hi")
	// The scratch copy stays until the job directory is cleaned up.
	_, err = os.Stat(p.extract)
	require.NoError(t, err)
	_ = res
}

func TestPolicyExtractsDroppedWhenSkipped(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplaceIfSmaller,
		p.output, p.original, p.origSize+5, p.origSize, []string{p.extract}, "", MoveDirective{})
	require.NoError(t, err)
	require.NotEmpty(t, res.SkipReason)
	// The original kept its embedded subtitle: the extract is scratch only.
	_, err = os.Stat(p.extract)
	require.NoError(t, err) // still in scratch; job cleanup removes it
}

func TestPolicyUnknownPolicy(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	_, err := ApplyResultPolicy(context.Background(), "wat", p.output, p.original, 1, 1, nil, "", MoveDirective{})
	require.ErrorContains(t, err, "unknown result policy")
}

// moveDest is a destination folder outside the test library.
func moveDest(p *policyTestFS) string {
	return filepath.Join(p.dir, "moved")
}

// TestPolicyMoveFlatReplacesOriginal: the move places the produced file at
// the destination and removes the original only afterwards (task 24).
func TestPolicyMoveFlatReplacesOriginal(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	dest := moveDest(p)
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, nil, "", MoveDirective{Destination: dest})
	require.NoError(t, err)
	require.Empty(t, res.SkipReason)
	require.Equal(t, filepath.Join(dest, "movie.mkv"), res.FinalPath)

	content, err := os.ReadFile(res.FinalPath)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
	_, err = os.Stat(p.original)
	require.True(t, os.IsNotExist(err), "the original is removed after the move")
}

// TestPolicyMoveKeepTree: keep_tree rebuilds the original's path relative to
// the library root under the destination (task 24).
func TestPolicyMoveKeepTree(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	nested := filepath.Join(p.dir, "library", "sub", "deep", "movie.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	origSize := writeTestFile(t, nested, "ORIGINAL-DATA")

	dest := moveDest(p)
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, nested, p.outSize, origSize, nil, "",
		MoveDirective{Destination: dest, KeepTree: true, RelDir: "sub/deep"})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dest, "sub", "deep", "movie.mkv"), res.FinalPath)

	info, err := os.Stat(filepath.Join(dest, "sub"))
	require.NoError(t, err)
	require.True(t, info.IsDir())
	content, err := os.ReadFile(res.FinalPath)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
	_, err = os.Stat(nested)
	require.True(t, os.IsNotExist(err))
}

// TestPolicyMoveNeverClobbers: an existing destination fails with both
// paths named, and neither file is touched (task 24).
func TestPolicyMoveNeverClobbers(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	dest := moveDest(p)
	existing := filepath.Join(dest, "movie.mkv")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	writeTestFile(t, existing, "ALREADY-THERE")

	_, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, nil, "", MoveDirective{Destination: dest})
	require.ErrorContains(t, err, existing)
	require.ErrorContains(t, err, p.original)

	// The output is still where it was, and the original is intact.
	content, err := os.ReadFile(p.output)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
	content, err = os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL-DATA", string(content))
	content, err = os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "ALREADY-THERE", string(content))
}

// TestPolicyMoveCrossDevice: when the rename fails, the verified copy
// fallback still produces the file and removes the original (task 24).
// The scratch dir is made unwritable so os.Rename cannot remove the output
// from it — the same failure a real cross-filesystem move produces.
func TestPolicyMoveCrossDevice(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	dest := moveDest(p)
	scratchDir := filepath.Dir(p.output)
	require.NoError(t, os.Chmod(scratchDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(scratchDir, 0o755) })

	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, nil, "", MoveDirective{Destination: dest})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dest, "movie.mkv"), res.FinalPath)

	content, err := os.ReadFile(res.FinalPath)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
	_, err = os.Stat(p.original)
	require.True(t, os.IsNotExist(err))
}

// TestPolicyMoveInsideSourceFolderRefused: a destination inside the source
// file's own directory is refused before anything moves (task 24).
func TestPolicyMoveInsideSourceFolderRefused(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	_, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplace,
		p.output, p.original, p.outSize, p.origSize, nil, "",
		MoveDirective{Destination: filepath.Dir(p.original)})
	require.ErrorContains(t, err, "inside the source folder")
	content, err := os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL-DATA", string(content))
}

// TestPolicyMoveKeepBothKeepsOriginal: with keep-both the produced file
// moves away and the original stays (task 24).
func TestPolicyMoveKeepBothKeepsOriginal(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	dest := moveDest(p)
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyKeepBoth,
		p.output, p.original, p.outSize, p.origSize, []string{p.extract}, "",
		MoveDirective{Destination: dest})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dest, "movie.shrunk.mkv"), res.FinalPath)

	content, err := os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "ORIGINAL-DATA", string(content))
	// The extract followed the moved file.
	extract, err := os.ReadFile(filepath.Join(dest, "movie.shrunk.eng.srt"))
	require.NoError(t, err)
	require.Contains(t, string(extract), "Hi")
}

// TestPolicyMoveSkippedWhenNotSmaller: the skip happens before any move;
// the original is kept and the output stays in scratch (task 24).
func TestPolicyMoveSkippedWhenNotSmaller(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyReplaceIfSmaller,
		p.output, p.original, p.origSize+5, p.origSize, nil, "",
		MoveDirective{Destination: moveDest(p)})
	require.NoError(t, err)
	require.NotEmpty(t, res.SkipReason)
	_, err = os.Stat(p.output)
	require.NoError(t, err)
	_, err = os.Stat(p.original)
	require.NoError(t, err)
	_, err = os.Stat(moveDest(p))
	require.True(t, os.IsNotExist(err), "nothing is created when the move is skipped")
}
