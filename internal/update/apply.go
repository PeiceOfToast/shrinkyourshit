// SPDX-License-Identifier: AGPL-3.0-only

package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Apply updates the running binary to the channel's latest release, in the
// order the ARCHITECTURE.md update path fixes:
//
//  1. refuse inside a container (ContainerError with the pull instruction);
//  2. refuse a binary built without release flags (v0.0.0);
//  3. resolve the latest release (cached, see Latest);
//  4. stop when already on it (ErrUpToDate);
//  5. back up the database to <data-dir>/backups/pre-update-<version>.db
//     (last three kept) unless SkipBackup;
//  6. download the platform archive into the scratch dir, verify its
//     sha256 against the release's checksums.txt (any mismatch or missing
//     entry aborts), extract the one binary it contains;
//  7. swap atomically: stage the new binary beside the old one, keep the
//     old as <binary>.prev, rename the new over the old, fsync the dir.
//
// The restart itself is the caller's business (systemd bounce or exec; see
// cmd/shrinkyourshit). On any failure before the swap, nothing on disk
// changed but the scratch dir.
func Apply(ctx context.Context, opts Options) (Applied, error) {
	opts.progress(StageChecking)
	if IsContainer() {
		return Applied{}, &ContainerError{Instruction: composePullInstruction}
	}
	if opts.Current.Version == "" || opts.Current.Version == "v0.0.0" {
		return Applied{}, ErrUnknownVersion
	}

	rel, err := Latest(ctx, opts)
	if err != nil {
		return Applied{}, err
	}
	if rel.Tag == opts.Current.Version {
		return Applied{}, ErrUpToDate
	}

	log := opts.logger()
	opts.progress(StageBackup)
	backupPath, err := backupDatabase(ctx, opts, rel.Tag)
	if err != nil {
		return Applied{}, fmt.Errorf("update: database backup: %w", err)
	}
	if backupPath != "" {
		log.Info("update: database backed up", "path", backupPath)
	}

	exe, err := opts.executablePath()
	if err != nil {
		return Applied{}, fmt.Errorf("update: locate the running binary: %w", err)
	}
	newBin, err := fetchAndVerify(ctx, opts, rel)
	if err != nil {
		return Applied{}, err
	}
	opts.progress(StageSwapping)
	prev, err := swapBinary(exe, newBin)
	if err != nil {
		return Applied{}, fmt.Errorf("update: swap binary: %w", err)
	}
	log.Info("update: binary replaced", "path", exe, "from", opts.Current.Version, "to", rel.Tag)
	return Applied{
		From:       opts.Current.Version,
		To:         rel.Tag,
		Path:       exe,
		Prev:       prev,
		BackupPath: backupPath,
	}, nil
}

// fetchAndVerify downloads the platform asset and checksums.txt into
// <scratch>/update, verifies the archive's sha256 (refusing on any
// mismatch or missing entry), extracts the single binary and returns its
// path in the scratch dir.
func fetchAndVerify(ctx context.Context, opts Options, rel Release) (string, error) {
	work := filepath.Join(opts.ScratchDir, "update")
	if err := os.MkdirAll(work, 0o700); err != nil {
		return "", fmt.Errorf("update: scratch: %w", err)
	}

	asset, err := SelectAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", fmt.Errorf("update: %w", err)
	}

	// checksums.txt first: nothing is trusted before it is on disk.
	sums, err := downloadString(ctx, opts, checksumsURL(rel))
	if err != nil {
		return "", fmt.Errorf("update: checksums.txt: %w", err)
	}
	checksums := ParseChecksums(sums)
	want, ok := checksums[asset.Name]
	if !ok {
		return "", fmt.Errorf("update: %w (%s)", ErrNoChecksumEntry, asset.Name)
	}

	opts.progress(StageDownloading)
	archivePath := filepath.Join(work, asset.Name)
	if err := downloadFile(ctx, opts, asset.URL, archivePath); err != nil {
		return "", fmt.Errorf("update: download %s: %w", asset.Name, err)
	}
	opts.progress(StageVerifying)
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return "", fmt.Errorf("update: hash %s: %w", asset.Name, err)
	}
	if actual != want {
		// The tampered download must not survive: remove it, leave the
		// running binary untouched (it was never touched yet).
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("update: %w for %s: want %s, got %s", ErrChecksumMismatch, asset.Name, want, actual)
	}

	bin, err := extractSingleBinary(archivePath, work)
	if err != nil {
		return "", fmt.Errorf("update: extract %s: %w", asset.Name, err)
	}
	return bin, nil
}

// checksumsURL guesses the release download URL of checksums.txt from the
// asset list: the release workflow always publishes it, so it rides with
// the assets' common prefix. Falls back to the tag-based URL.
func checksumsURL(rel Release) string {
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			return a.URL
		}
	}
	if len(rel.Assets) > 0 {
		base := rel.Assets[0].URL
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			return base[:i+1] + "checksums.txt"
		}
	}
	return ""
}

// downloadString fetches a small text asset.
func downloadString(ctx context.Context, opts Options, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("no url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// downloadFile streams url to dest (fsynced).
func downloadFile(ctx context.Context, opts Options, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return writeFileSync(dest, resp.Body)
}

// writeFileSync writes r to dest and fsyncs before close, so the rename
// dance in swapBinary never stages half a file.
func writeFileSync(dest string, r io.Reader) error {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// fileSHA256 hex-digests a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractSingleBinary pulls the one regular file out of a .tar.gz or .zip
// archive into destDir and returns its path. The release layout puts
// exactly one binary at the archive root; anything else is refused
// (ErrArchiveLayout) instead of guessed at, and entries with path
// separators are rejected outright so a crafted archive cannot write
// outside destDir.
func extractSingleBinary(archivePath, destDir string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, destDir)
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGz(archivePath, destDir)
	default:
		return "", fmt.Errorf("%w: unsupported archive type", ErrArchiveLayout)
	}
}

func extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%w: no file entry in %s", ErrArchiveLayout, filepath.Base(archivePath))
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := safeEntryName(hdr.Name)
		if !ok {
			return "", fmt.Errorf("%w: refusing entry %q", ErrArchiveLayout, hdr.Name)
		}
		return copyOut(tr, filepath.Join(destDir, name), hdr.FileInfo().Mode())
	}
}

func extractZip(archivePath, destDir string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name, ok := safeEntryName(zf.Name)
		if !ok {
			return "", fmt.Errorf("%w: refusing entry %q", ErrArchiveLayout, zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return "", err
		}
		path, err := copyOut(rc, filepath.Join(destDir, name), zipMode(zf))
		_ = rc.Close()
		return path, err
	}
	return "", fmt.Errorf("%w: no file entry in %s", ErrArchiveLayout, filepath.Base(archivePath))
}

// zipMode recovers the unix mode bits stored in the zip's external
// attributes; archives created by python's zipfile carry them.
func zipMode(zf *zip.File) os.FileMode {
	mode := zf.Mode()
	if mode&0o111 == 0 {
		mode |= 0o755 // archives without mode bits: executables
	}
	return mode
}

// safeEntryName accepts plain file names only (no directories, no
// separators, no escaping the destination).
func safeEntryName(name string) (string, bool) {
	name = filepath.ToSlash(name)
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", false
	}
	return name, true
}

// copyOut streams an archive reader to destPath with the given mode.
func copyOut(r io.Reader, destPath string, mode os.FileMode) (string, error) {
	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return destPath, nil
}

// swapBinary puts newBin where exe is, atomically and with the previous
// binary kept for rollback:
//
//  1. copy newBin to <exe>.new (same directory, so the final rename never
//     crosses a filesystem), fsync it;
//  2. rename exe → <exe>.prev (the old binary is now the rollback point);
//  3. rename <exe>.new → exe; fsync the directory.
//
// If step 3 fails, step 2 is undone so a running install is never left
// without a binary. Both renames are legal while the process runs: unix
// allows renaming a mapped executable, and Windows allows renaming (not
// deleting) the image file of a running process.
func swapBinary(exe, newBin string) (prev string, err error) {
	prev = exe + ".prev"
	staged := exe + ".new"

	if err := copyFileSync(newBin, staged, 0o755); err != nil {
		return "", fmt.Errorf("stage: %w", err)
	}
	if err := os.Rename(exe, prev); err != nil {
		return "", fmt.Errorf("keep previous: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		// Undo the first rename; the update failed but the install must
		// stay runnable.
		if rbErr := os.Rename(prev, exe); rbErr != nil {
			return "", fmt.Errorf("swap: %v (and restore failed: %v)", err, rbErr)
		}
		return "", err
	}
	if err := syncDir(filepath.Dir(exe)); err != nil {
		return "", err
	}
	return prev, nil
}

// copyFileSync copies src to dest with the given mode, fsyncing before
// close.
func copyFileSync(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// syncDir flushes a directory entry change to disk; Windows has no
// portable directory fsync and needs none for the rename dance above.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// Rollback restores <binary>.prev over the running binary and returns the
// binary's path; the caller then restarts (bounce or exec). The previous
// copy is removed after a successful restore. If the currently installed
// (updated) binary cannot be deleted — Windows keeps a running image
// locked — it is left beside the binary as <binary>.updated, removable
// after the next restart.
func Rollback(opts Options) (string, error) {
	exe, err := opts.executablePath()
	if err != nil {
		return "", fmt.Errorf("update: locate the running binary: %w", err)
	}
	prev := exe + ".prev"
	if _, err := os.Stat(prev); err != nil {
		return "", fmt.Errorf("update: rollback: no previous binary at %s: %w", prev, err)
	}

	staged := exe + ".new"
	if err := copyFileSync(prev, staged, 0o755); err != nil {
		return "", fmt.Errorf("update: rollback: stage: %w", err)
	}
	// Move the running image aside first (Windows cannot rename over it),
	// then put the staged previous binary in place.
	updated := exe + ".updated"
	if err := os.Rename(exe, updated); err != nil {
		return "", fmt.Errorf("update: rollback: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		if rbErr := os.Rename(updated, exe); rbErr != nil {
			return "", fmt.Errorf("update: rollback: %v (and restore failed: %v)", err, rbErr)
		}
		return "", fmt.Errorf("update: rollback: %w", err)
	}
	if err := syncDir(filepath.Dir(exe)); err != nil {
		return "", err
	}
	// The updated binary is no longer installed; drop both leftovers.
	_ = os.Remove(updated)
	_ = os.Remove(prev)
	opts.logger().Info("update: rolled back", "path", exe)
	return exe, nil
}
