// SPDX-License-Identifier: AGPL-3.0-only

package update_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReleaseArtifacts runs the packaging half of scripts/release.sh — the
// exact script the release workflow and `make release-cross` use — over
// fake binaries and checks the release layout the updater consumes: five
// archives with the right names, each holding exactly one binary, and a
// checksums.txt in sha256sum format that verifies (and that refuses a
// tampered archive).
func TestReleaseArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("shells out to scripts/release.sh; skipped in -short")
	}
	script := filepath.Join("..", "..", "scripts", "release.sh")
	require.FileExists(t, script)

	dist := t.TempDir()
	targets := []string{
		"linux-amd64", "linux-arm64", "windows-amd64.exe", "darwin-amd64", "darwin-arm64",
	}
	for _, tt := range targets {
		writeFile(t, filepath.Join(dist, "shrinkyourshit-"+tt), "fake binary "+tt)
	}

	runOK(t, "sh", script, "package", dist)

	// Exactly the five expected archives exist, with the names the asset
	// selection computes from the other side.
	wantArchives := []string{
		update.ArchiveName("linux", "amd64"),
		update.ArchiveName("linux", "arm64"),
		update.ArchiveName("windows", "amd64"),
		update.ArchiveName("darwin", "amd64"),
		update.ArchiveName("darwin", "arm64"),
	}
	var archives []string
	entries, err := os.ReadDir(dist)
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
			archives = append(archives, name)
		}
	}
	assert.ElementsMatch(t, wantArchives, archives)

	// Each archive contains exactly one regular file, named after the
	// binary it packages, and its content survived.
	for _, tt := range targets {
		binName := "shrinkyourshit-" + tt
		if strings.HasSuffix(tt, ".exe") {
			name, got := zipSingleEntry(t, filepath.Join(dist, update.ArchiveName("windows", "amd64")))
			assert.Equal(t, binName, name, "zip entry name")
			assert.Equal(t, "fake binary "+tt, got)
		} else {
			name, got := tarSingleEntry(t, filepath.Join(dist, binName+".tar.gz"))
			assert.Equal(t, binName, name)
			assert.Equal(t, "fake binary "+tt, got)
		}
	}

	// checksums.txt: sha256sum format over every archive, and the script's
	// own verifier accepts it.
	sums := readFile(t, filepath.Join(dist, "checksums.txt"))
	lines := nonEmptyLines(sums)
	assert.Len(t, lines, 5, "one line per archive:\n%s", sums)
	for _, line := range lines {
		assert.Regexp(t, `^[0-9a-f]{64}  \S+$`, line, "sha256sum format")
	}
	runOK(t, "sh", script, "verify", dist)

	// A tampered archive must fail verification.
	tampered := filepath.Join(dist, wantArchives[0])
	writeFile(t, tampered, "tampered")
	out, err := exec.Command("sh", script, "verify", dist).CombinedOutput()
	require.Error(t, err, "verify must reject a tampered archive; output:\n%s", out)
	assert.Contains(t, string(out), "checksum mismatch")
}

// runOK runs a command and fails the test with its output on error.
func runOK(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	require.NoError(t, err, "%s %v failed:\n%s", name, args, out)
}

// tarSingleEntry returns the name and content of the one regular file in
// a tar.gz.
func tarSingleEntry(t *testing.T, path string) (string, string) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	require.NoError(t, err)
	data, err := io.ReadAll(tr)
	require.NoError(t, err)
	_, err = tr.Next()
	require.ErrorIs(t, err, io.EOF, "exactly one entry")
	return hdr.Name, string(data)
}

// zipSingleEntry returns the name and content of the one file in a zip
// archive.
func zipSingleEntry(t *testing.T, path string) (string, string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	require.Len(t, zr.File, 1, "exactly one entry")
	rc, err := zr.File[0].Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	return zr.File[0].Name, string(data)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
