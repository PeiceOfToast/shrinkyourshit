// SPDX-License-Identifier: AGPL-3.0-only

package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrAlreadyInstalled is returned when a plugin with the same name (or a
// target directory) already exists. The API maps it to a 409.
var ErrAlreadyInstalled = errors.New("plugin: already installed")

// ErrGitUnavailable is returned when a git install was asked for but the
// git binary is missing on this host. The API maps it to a 400.
var ErrGitUnavailable = errors.New("plugin: git is not available")

// Root returns the plugin root directory under the data dir (task 31:
// <data dir>/plugins/, on the /config volume in Docker so installed
// plugins survive a container replace).
func Root(dataDir string) string {
	return filepath.Join(dataDir, "plugins")
}

// Dir returns the install directory of one plugin.
func Dir(dataDir, name string) string {
	return filepath.Join(Root(dataDir), name)
}

// InstallFromPath installs a plugin from a directory that is already
// visible to the server (task 31): read and validate its manifest, then
// copy the tree into <dataDir>/plugins/<name>/, refusing an existing
// install. The returned manifest is the validated one.
func InstallFromPath(src string, dataDir string) (Manifest, error) {
	src = filepath.Clean(src)
	m, err := Load(src)
	if err != nil {
		return Manifest{}, err
	}
	dest := Dir(dataDir, m.Name)
	if err := installTree(src, dest); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// InstallFromGit installs a plugin from a git repository (task 31): clone
// --depth 1 (optionally --branch ref) into a temp dir, read and validate
// its manifest, then move the tree into place. The clone is discarded
// either way. The git binary must be on PATH; otherwise ErrGitUnavailable.
func InstallFromGit(ctx context.Context, gitURL, ref, dataDir string) (Manifest, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: installing from a git URL needs git on this host", ErrGitUnavailable)
	}
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return Manifest{}, fmt.Errorf("plugin: git_url is required")
	}
	ref = strings.TrimSpace(ref)

	tmp, err := os.MkdirTemp("", "sys-plugin-")
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	args := []string{"clone", "--quiet", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, gitURL, tmp)
	clone := exec.CommandContext(ctx, git, args...)
	// The clone output can name the URL, which may embed a token; keep it
	// out of error messages and logs.
	out, err := clone.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return Manifest{}, ctx.Err()
		}
		head := firstLine(string(out))
		return Manifest{}, fmt.Errorf("plugin: git clone failed (%s)", head)
	}

	// The clone root is either tmp itself or, for a repo with several
	// plugins... no — v1 installs single-plugin repos: the manifest sits
	// at the clone root. A missing manifest is the install error.
	m, err := Load(tmp)
	if err != nil {
		return Manifest{}, err
	}
	if err := installTree(tmp, Dir(dataDir, m.Name)); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// installTree copies the plugin tree from src to dest, refusing an
// existing destination (never clobber an installed plugin). The .git
// directory of a git clone is not copied. File modes are preserved so an
// executable script stays executable.
func installTree(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%w: %s exists; remove it first", ErrAlreadyInstalled, dest)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("plugin: stat %s: %w", dest, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("plugin: create %s: %w", filepath.Dir(dest), err)
	}
	if err := copyTree(src, dest); err != nil {
		// Leave nothing half-installed behind.
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}

// copyTree walks src and copies every entry to dest, skipping .git.
func copyTree(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("plugin: create %s: %w", dest, err)
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && rel == ".git" {
			return filepath.SkipDir
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil // symlinks and devices are not part of a plugin
		}
		return copyFileMode(path, target)
	})
}

// copyFileMode copies one file, preserving its permission bits.
func copyFileMode(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("plugin: open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	mode := os.FileMode(0o644)
	if info, err := in.Stat(); err == nil {
		mode = info.Mode().Perm()
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("plugin: write %s: %w", dest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("plugin: copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("plugin: close %s: %w", dest, err)
	}
	return nil
}

// firstLine returns the first non-empty line of s, trimmed, capped at
// 200 characters (an error head for messages).
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 200 {
				line = line[:200]
			}
			return line
		}
	}
	return "no output"
}

// Remove deletes an installed plugin's directory. The row goes with the
// caller (the service refuses while the plugin is enabled, so the
// directory is only removed for a disabled plugin).
func Remove(dataDir, name string) error {
	dir := Dir(dataDir, name)
	if dir == Root(dataDir) || !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(Root(dataDir))+string(filepath.Separator)) {
		return fmt.Errorf("plugin: refusing to remove %s", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("plugin: remove %s: %w", dir, err)
	}
	return nil
}
