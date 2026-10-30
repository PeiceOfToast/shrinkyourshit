// SPDX-License-Identifier: AGPL-3.0-only

// Package update is the self-update path for a binary install
// (ARCHITECTURE.md "Update path"). It knows the latest GitHub release, can
// verify and apply it, and can roll back.
//
// Rules that hold for every entry point:
//
//   - Nothing auto-applies. `shrink update` is always a human command; the
//     in-app UI (task 17) also only ever reports or asks.
//   - A download that fails its sha256 (checksums.txt from the release)
//     aborts with the running binary untouched.
//   - The previous binary is kept as <binary>.prev for --rollback.
//   - Inside a container the updater refuses with the exact
//     `docker compose pull` instruction instead of touching files.
package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Runtime identities reported by /api/v1/version and `shrink version`.
const (
	RuntimeBinary    = "binary"
	RuntimeContainer = "container"
)

// composePullInstruction is the exact command a container install runs to
// update (a container image cannot meaningfully replace its own file).
const composePullInstruction = "docker compose pull && docker compose up -d"

// IsContainer reports whether this process runs inside a container:
// /.dockerenv exists or SHRINK_IN_CONTAINER=1 (ARCHITECTURE.md).
func IsContainer() bool {
	if os.Getenv("SHRINK_IN_CONTAINER") == "1" {
		return true
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// Runtime names how this install runs: binary or container.
func Runtime() string {
	if IsContainer() {
		return RuntimeContainer
	}
	return RuntimeBinary
}

// Info is the build identity of the running binary. Releases inject all
// three via -ldflags; a build without them reports v0.0.0 and refuses to
// self update (ARCHITECTURE.md: releases always inject).
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Options configures Latest, Apply and Rollback.
type Options struct {
	// DataDir holds the release cache and the pre-update database backups.
	DataDir string
	// ScratchDir receives downloads before they are verified and extracted.
	ScratchDir string
	// Channel selects the release feed: "stable" (default) or "prerelease".
	Channel string
	// Current is the running binary's build identity.
	Current Info
	// SkipBackup skips the pre-update database backup. Default (false) is
	// on: the backup_before_update setting defaults to true.
	SkipBackup bool
	// Backup writes a consistent database snapshot to dest. The CLI wires
	// the store's VACUUM INTO snapshot here so a live server yields a
	// consistent copy. Nil falls back to a plain file copy of
	// <data-dir>/shrinkyourshit.db (fine while the server is down).
	Backup func(ctx context.Context, dest string) error
	// HTTP overrides the HTTP client (tests). Nil uses a default client.
	HTTP *http.Client
	// APIBase overrides the GitHub API root (tests only).
	APIBase string
	// Now overrides time.Now (tests only).
	Now func() time.Time
	// Executable overrides os.Executable (tests only).
	Executable func() (string, error)
	// Log receives progress; nil falls back to slog.Default().
	Log *slog.Logger
}

// Applied is the result of a successful update.
type Applied struct {
	From       string // version replaced
	To         string // version installed
	Path       string // installed binary path
	Prev       string // previous binary kept at <path>.prev
	BackupPath string // database backup, empty when none was made
}

// Errors the CLI turns into messages.
var (
	// ErrUpToDate means the running version already is the channel's latest.
	ErrUpToDate = errors.New("already on the latest release")
	// ErrUnknownVersion means the binary was built without release flags.
	ErrUnknownVersion = errors.New("version not injected at build time; rebuild with the release flags to self update")
	// ErrChecksumMismatch wraps a failed sha256 verification.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrNoAsset means the release has no archive for this platform.
	ErrNoAsset = errors.New("no release asset for this platform")
	// ErrNoChecksumEntry means checksums.txt does not cover the asset.
	ErrNoChecksumEntry = errors.New("checksums.txt does not cover this asset")
	// ErrArchiveLayout means the archive did not hold exactly one file.
	ErrArchiveLayout = errors.New("unexpected archive layout")
)

// ContainerError refuses a self update inside a container. Instruction is
// the exact command the user runs instead.
type ContainerError struct {
	Instruction string
}

func (e *ContainerError) Error() string {
	return "running in a container; update the image instead:\n  " + e.Instruction
}

// errExitZero lives in cmd (main.go): the update package never exits the
// process itself.

// logger returns the caller's logger or the default.
func (o Options) logger() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

// now returns the (overridable) current time.
func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// executablePath resolves the running binary's path.
func (o Options) executablePath() (string, error) {
	if o.Executable != nil {
		return o.Executable()
	}
	return os.Executable()
}

// httpClient returns the (overridable) HTTP client.
func (o Options) httpClient() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// backupDatabase writes the pre-update snapshot to
// <data-dir>/backups/pre-update-<version>.db and keeps the newest
// maxBackups of them.
func backupDatabase(ctx context.Context, opts Options, version string) (string, error) {
	if opts.SkipBackup {
		return "", nil
	}
	dest := filepath.Join(opts.DataDir, "backups", fmt.Sprintf("pre-update-%s.db", version))
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", fmt.Errorf("create backups dir: %w", err)
	}
	backup := opts.Backup
	if backup == nil {
		backup = func(ctx context.Context, dest string) error {
			return copyFileSync(filepath.Join(opts.DataDir, dbFileName), dest, 0o600)
		}
	}
	if err := backup(ctx, dest); err != nil {
		return "", err
	}
	if err := pruneBackups(filepath.Dir(dest), maxDatabaseBackups); err != nil {
		// Retention is housekeeping; a failed prune must not abort the
		// update, the next run prunes again.
		opts.logger().Warn("update: prune old database backups", "err", err)
	}
	return dest, nil
}

const (
	dbFileName         = "shrinkyourshit.db"
	maxDatabaseBackups = 3
)

// pruneBackups removes the oldest pre-update snapshots beyond keep.
func pruneBackups(dir string, keep int) error {
	matches, err := filepath.Glob(filepath.Join(dir, "pre-update-*.db"))
	if err != nil {
		return err
	}
	type snapshot struct {
		path string
		mod  time.Time
	}
	snaps := make([]snapshot, 0, len(matches))
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		snaps = append(snaps, snapshot{path: p, mod: info.ModTime()})
	}
	// Newest first; only prune when there is something to prune.
	slices.SortFunc(snaps, func(a, b snapshot) int { return b.mod.Compare(a.mod) })
	for _, s := range snaps[min(keep, len(snaps)):] {
		if err := os.Remove(s.path); err != nil {
			return fmt.Errorf("remove %s: %w", s.path, err)
		}
	}
	return nil
}
