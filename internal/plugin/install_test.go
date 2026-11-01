// SPDX-License-Identifier: AGPL-3.0-only

package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeManifest writes a plugin.yaml into dir.
func writeManifest(t *testing.T, dir, name, typ string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	raw := `name: ` + name + `
version: 1.2.3
type: ` + typ + `
command: ["echo", "hi"]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o644))
}

func TestInstallFromPath(t *testing.T) {
	dataDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "my-plugin")
	writeManifest(t, src, "my-plugin", TypePostprocess)
	// An extra file travels along; an executable script keeps its mode.
	require.NoError(t, os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755))

	m, err := InstallFromPath(src, dataDir)
	require.NoError(t, err)
	assert.Equal(t, "my-plugin", m.Name)

	// Installed into place, disabled-by-default lives in the DB; the
	// directory is complete and executable.
	installed := Dir(dataDir, "my-plugin")
	raw, err := os.ReadFile(filepath.Join(installed, FileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "name: my-plugin")
	info, err := os.Stat(filepath.Join(installed, "run.sh"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "the executable bit travels with the file")

	// A second install of the same name is refused, and the original
	// directory is untouched.
	_, err = InstallFromPath(src, dataDir)
	require.ErrorIs(t, err, ErrAlreadyInstalled)
}

func TestInstallFromPathInvalidManifestNamesField(t *testing.T) {
	src := filepath.Join(t.TempDir(), "bad-plugin")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, FileName), []byte(
		"name: bad-plugin\ntype: not-a-type\ncommand: [\"echo\"]\n"), 0o644))

	dataDir := t.TempDir()
	_, err := InstallFromPath(src, dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type:", "the message names the field")
	assert.NoDirExists(t, Dir(dataDir, "bad-plugin"), "nothing is installed on a rejection")
}

func TestInstallFromPathMissingManifest(t *testing.T) {
	_, err := InstallFromPath(filepath.Join(t.TempDir(), "empty"), t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin.yaml")
}

// TestInstallWasmFromPath (task 33): a wasm plugin installs like any
// other; the entry module travels with the manifest.
func TestInstallWasmFromPath(t *testing.T) {
	dataDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "wasm-plugin")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, FileName), []byte(
		"name: wasm-plugin\ntype: step\nruntime: wasm\nentry: hello.wasm\n"), 0o644))
	fixture, err := os.ReadFile(filepath.Join("testdata", "hello.wasm"))
	require.NoError(t, err, "the committed fixture must be present")
	require.NoError(t, os.WriteFile(filepath.Join(src, "hello.wasm"), fixture, 0o644))

	m, err := InstallFromPath(src, dataDir)
	require.NoError(t, err)
	assert.Equal(t, RuntimeWASM, m.Runtime)
	assert.Equal(t, MemoryPagesDefault, m.MemoryPages, "memory_pages defaults to 1024 pages")

	installed, err := os.ReadFile(filepath.Join(Dir(dataDir, "wasm-plugin"), "hello.wasm"))
	require.NoError(t, err)
	assert.Equal(t, fixture, installed)
}

// TestInstallWasmBrokenEntryRejected (task 33): an entry that does not
// parse is rejected at install time naming the field, and nothing is left
// half-installed.
func TestInstallWasmBrokenEntryRejected(t *testing.T) {
	dataDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "broken-wasm")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, FileName), []byte(
		"name: broken-wasm\ntype: step\nruntime: wasm\nentry: main.wasm\n"), 0o644))
	// The \0asm magic passes the fast check; the garbage body fails the
	// compile-time verification.
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.wasm"),
		[]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff}, 0o644))

	_, err := InstallFromPath(src, dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry:", "the message names the field")
	assert.Contains(t, err.Error(), "main.wasm")
	assert.NoDirExists(t, Dir(dataDir, "broken-wasm"), "nothing is installed on a rejection")
}

// TestInstallWasmNonModuleEntryRejected (task 33): a file without the
// wasm magic bytes is rejected before wazero even parses one.
func TestInstallWasmNonModuleEntryRejected(t *testing.T) {
	dataDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "not-wasm")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, FileName), []byte(
		"name: not-wasm\ntype: step\nruntime: wasm\nentry: main.wasm\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.wasm"), []byte("MZ not wasm"), 0o644))

	_, err := InstallFromPath(src, dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "magic bytes")
	assert.NoDirExists(t, Dir(dataDir, "not-wasm"))
}

// TestInstallWasmMissingEntryRejected (task 33): an entry that does not
// exist in the plugin directory is an install-time error naming the field.
func TestInstallWasmMissingEntryRejected(t *testing.T) {
	dataDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "ghost-wasm")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, FileName), []byte(
		"name: ghost-wasm\ntype: step\nruntime: wasm\nentry: missing.wasm\n"), 0o644))

	_, err := InstallFromPath(src, dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry:")
	assert.Contains(t, err.Error(), "missing.wasm")
	assert.NoDirExists(t, Dir(dataDir, "ghost-wasm"))
}

// gitAvailable reports whether the git binary exists; the git install
// test needs it (CI installs git; a bare container may not).
func gitAvailable(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed; the git install test needs it")
	}
	return git
}

// initPluginRepo builds a local repository with one plugin at the root
// and returns its path. It uses the real git binary (git init/commit).
func initPluginRepo(t *testing.T, name, typ string, ref bool) string {
	t.Helper()
	git := gitAvailable(t)
	repo := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	writeManifest(t, repo, name, typ)
	run := func(args ...string) {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "--initial-branch", "main", "-q")
	run("add", FileName)
	run("commit", "-q", "-m", "plugin")
	if ref {
		run("branch", "v2")
	}
	return repo
}

func TestInstallFromGitFileURL(t *testing.T) {
	gitAvailable(t)
	repo := initPluginRepo(t, "git-plugin", TypeNotify, false)
	dataDir := t.TempDir()

	// A local file:// repository is the test stand-in for a remote URL
	// (the acceptance item); the service runs the same clone it would run
	// for https://.
	m, err := InstallFromGit(context.Background(), "file://"+repo, "", dataDir)
	require.NoError(t, err)
	assert.Equal(t, "git-plugin", m.Name)
	assert.Equal(t, TypeNotify, m.Type)

	installed := Dir(dataDir, "git-plugin")
	raw, err := os.ReadFile(filepath.Join(installed, FileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "name: git-plugin")
	assert.NoDirExists(t, filepath.Join(installed, ".git"), "the clone's .git is not copied")

	// A shallow clone of the same name again is refused.
	_, err = InstallFromGit(context.Background(), "file://"+repo, "", dataDir)
	require.ErrorIs(t, err, ErrAlreadyInstalled)
}

func TestInstallFromGitWithRef(t *testing.T) {
	gitAvailable(t)
	repo := initPluginRepo(t, "git-plugin", TypePreflight, true)
	dataDir := t.TempDir()

	m, err := InstallFromGit(context.Background(), "file://"+repo, "v2", dataDir)
	require.NoError(t, err)
	assert.Equal(t, "git-plugin", m.Name)
	assert.DirExists(t, Dir(dataDir, "git-plugin"))
}

func TestInstallFromGitBadManifestRejected(t *testing.T) {
	gitAvailable(t)
	repo := initPluginRepo(t, "git-plugin", "nope", false)
	dataDir := t.TempDir()

	_, err := InstallFromGit(context.Background(), "file://"+repo, "", dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type:", "the message names the field")
	assert.NoDirExists(t, Dir(dataDir, "git-plugin"), "nothing is installed on a rejection")
}

func TestInstallFromGitUnknownURLFails(t *testing.T) {
	gitAvailable(t)
	dataDir := t.TempDir()
	// A file:// URL that does not exist fails the clone with a short
	// message that does not leak command internals.
	_, err := InstallFromGit(context.Background(), "file:///definitely/not/here.git", "", dataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git clone failed")
}

func TestRemoveDeletesDirectoryOnlyInsideRoot(t *testing.T) {
	dataDir := t.TempDir()
	dir := Dir(dataDir, "gone-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte("name: gone-plugin\n"), 0o644))

	require.NoError(t, Remove(dataDir, "gone-plugin"))
	assert.NoDirExists(t, dir)

	// A name that escapes the root is refused (defensive; validated
	// manifests cannot produce one).
	require.Error(t, Remove(dataDir, ".."))
	require.Error(t, Remove(dataDir, "."))
	require.Error(t, Remove(dataDir, ""))
}
