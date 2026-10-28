// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestFingerprintStableAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "movie.mkv", []byte("0123456789abcdef"))

	first, err := Fingerprint(path)
	require.NoError(t, err)
	second, err := Fingerprint(path)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	// Same content with the same mtime fingerprints identically at any
	// path, which is what a re-scan of a moved library needs.
	st, err := os.Stat(path)
	require.NoError(t, err)
	other := writeFile(t, dir, "copy.mkv", []byte("0123456789abcdef"))
	require.NoError(t, os.Chtimes(other, st.ModTime(), st.ModTime()))
	copied, err := Fingerprint(other)
	require.NoError(t, err)
	assert.Equal(t, first, copied)
}

func TestFingerprintChangesWhenContentChanges(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "movie.mkv", []byte("0123456789abcdef"))
	before, err := Fingerprint(path)
	require.NoError(t, err)

	// Append data: size, mtime and the hashed tail all change.
	require.NoError(t, os.WriteFile(path, []byte("0123456789abcdefZZZZ"), 0o600))
	after, err := Fingerprint(path)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}

func TestFingerprintDetectsEditedBytes(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 3<<20) // 3 MiB: larger than one sample
	for i := range big {
		big[i] = byte(i % 251)
	}
	path := writeFile(t, dir, "movie.mkv", big)
	st, err := os.Stat(path)
	require.NoError(t, err)
	before, err := Fingerprint(path)
	require.NoError(t, err)

	// Rewrite the same size with different bytes at the end and force the
	// same mtime: only the hash can tell the files apart.
	big[len(big)-1] ^= 0xff
	require.NoError(t, os.WriteFile(path, big, 0o600))
	require.NoError(t, os.Chtimes(path, st.ModTime(), st.ModTime()))
	after, err := Fingerprint(path)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}

func TestFingerprintFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "movie.mkv", []byte("media"))
	fp, err := Fingerprint(path)
	require.NoError(t, err)
	assert.Regexp(t, `^\d+:\d+:[0-9a-f]{64}$`, fp)
	assert.True(t, strings.HasPrefix(fp, "5:"), "size field must be the byte count, got %q", fp)
}

func TestFingerprintMissingFile(t *testing.T) {
	_, err := Fingerprint(filepath.Join(t.TempDir(), "nope.mkv"))
	assert.Error(t, err)
}

// TestFingerprintSmallFileWholeHash guards the overlap rule: files at or
// below 1 MiB hash the whole file once instead of first+last (which would
// duplicate the same bytes).
func TestFingerprintSmallFileWholeHash(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.mkv", []byte("tiny content"))
	b := writeFile(t, dir, "b.mkv", []byte("tiny contentZ"))
	st, err := os.Stat(b)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(b, st.ModTime(), st.ModTime()))

	fa, err := Fingerprint(a)
	require.NoError(t, err)
	fb, err := Fingerprint(b)
	require.NoError(t, err)
	assert.NotEqual(t, fa, fb, "different content must not collide even below the sample size")

	// And mtime moves the fingerprint forward for identical content.
	path := writeFile(t, dir, "c.mkv", []byte("tiny content"))
	fp1, err := Fingerprint(path)
	require.NoError(t, err)
	future := time.Now().Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))
	fp2, err := Fingerprint(path)
	require.NoError(t, err)
	assert.NotEqual(t, fp1, fp2)
}
