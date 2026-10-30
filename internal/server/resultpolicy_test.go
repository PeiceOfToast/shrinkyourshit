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
		p.output, p.original, p.outSize, p.origSize, nil, "")
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
		p.output, p.original, p.origSize-1, p.origSize, nil, "")
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
		p.output, p.original, p.outSize, p.origSize, nil, "")
	require.NoError(t, err)
	require.Equal(t, p.original, res.FinalPath) // replaced in place
	content, err := os.ReadFile(p.original)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA-1000", string(content))
}

func TestPolicyKeepBoth(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	res, err := ApplyResultPolicy(context.Background(), store.ResultPolicyKeepBoth,
		p.output, p.original, p.outSize, p.origSize, []string{p.extract}, "")
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
		p.output, p.original, p.outSize, p.origSize, []string{p.extract}, "")
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
		p.output, p.original, p.origSize+5, p.origSize, []string{p.extract}, "")
	require.NoError(t, err)
	require.NotEmpty(t, res.SkipReason)
	// The original kept its embedded subtitle: the extract is scratch only.
	_, err = os.Stat(p.extract)
	require.NoError(t, err) // still in scratch; job cleanup removes it
}

func TestPolicyUnknownPolicy(t *testing.T) {
	p := newPolicyFS(t, "movie", ".mp4")
	_, err := ApplyResultPolicy(context.Background(), "wat", p.output, p.original, 1, 1, nil, "")
	require.ErrorContains(t, err, "unknown result policy")
}
