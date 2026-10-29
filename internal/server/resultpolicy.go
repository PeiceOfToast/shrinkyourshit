// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Result-policy application (task 05), shared by the worker (which applies
// it next to the library in shared mode) and the server (which applies it in
// transfer mode after verifying the uploaded output). It lives here because
// internal/worker imports internal/server, not the other way around.

// ApplyResult is the outcome of applying a result policy to the verified
// output: its final path in the library, or a skip reason when the original
// was kept.
type ApplyResult struct {
	FinalPath  string // "" when the original was kept
	SkipReason string
}

// ApplyResultPolicy moves the verified output to its final place
// (ARCHITECTURE.md result policies and replacement rules):
//
//   - replace_if_smaller: replace only when the output is strictly smaller,
//     otherwise keep the original and report why;
//   - replace: always replace;
//   - keep_both: write <name>.shrunk.<ext> beside the untouched original.
//
// Replacement is atomic: copy to <dest-dir>/.<name>.shrink-tmp, fsync, then
// rename over the destination. The original is removed only after the
// verified output exists in place, and only when the extension changed.
// Extracted subtitle files move beside the final output; extracts are
// dropped when the original is kept (it still holds the embedded tracks).
func ApplyResultPolicy(ctx context.Context, policy, output, original string, outSize, originalSize int64, extracts []string) (ApplyResult, error) {
	ext := filepath.Ext(output)
	dir := filepath.Dir(original)
	origBase := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))

	switch policy {
	case store.ResultPolicyReplaceIfSmaller:
		if outSize >= originalSize {
			return ApplyResult{SkipReason: fmt.Sprintf(
				"output (%s) is not smaller than the original (%s); original kept",
				humanBytes(outSize), humanBytes(originalSize))}, nil
		}
		return replace(ctx, output, original, dir, origBase, ext, extracts)
	case store.ResultPolicyReplace:
		return replace(ctx, output, original, dir, origBase, ext, extracts)
	case store.ResultPolicyKeepBoth:
		base := origBase + ".shrunk"
		dest := filepath.Join(dir, base+ext)
		if err := copyFile(ctx, output, dest); err != nil {
			return ApplyResult{}, fmt.Errorf("write %s: %w", dest, err)
		}
		if err := moveExtracts(ctx, extracts, dir, base); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{FinalPath: dest}, nil
	default:
		return ApplyResult{}, fmt.Errorf("unknown result policy %q", policy)
	}
}

// replace copies the output over original (possibly with a new extension).
func replace(ctx context.Context, output, original, dir, origBase, ext string, extracts []string) (ApplyResult, error) {
	dest := filepath.Join(dir, origBase+ext)
	if dest == original {
		// Same path: rename replaces the file atomically in one step.
		tmp := filepath.Join(dir, "."+origBase+".shrink-tmp")
		if err := copyFile(ctx, output, tmp); err != nil {
			return ApplyResult{}, fmt.Errorf("stage %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return ApplyResult{}, fmt.Errorf("replace %s: %w", dest, err)
		}
	} else {
		tmp := filepath.Join(dir, "."+filepath.Base(dest)+".shrink-tmp")
		if err := copyFile(ctx, output, tmp); err != nil {
			return ApplyResult{}, fmt.Errorf("stage %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return ApplyResult{}, fmt.Errorf("replace %s: %w", dest, err)
		}
		// The verified output is in place; now (and only now) the original
		// with the old extension goes away.
		if err := os.Remove(original); err != nil && !os.IsNotExist(err) {
			return ApplyResult{}, fmt.Errorf("remove original %s: %w", original, err)
		}
	}
	if err := moveExtracts(ctx, extracts, dir, strings.TrimSuffix(filepath.Base(dest), ext)); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{FinalPath: dest}, nil
}

// moveExtracts copies scratch extract files (named base.<...>.srt) beside
// the final output as finalBase.<...>.srt.
func moveExtracts(ctx context.Context, extracts []string, destDir, finalBase string) error {
	for _, e := range extracts {
		name := filepath.Base(e)
		suffix := strings.TrimPrefix(name, "output") // ".eng.srt", ".eng.2.srt", ...
		dest := filepath.Join(destDir, finalBase+suffix)
		if err := copyFile(ctx, e, dest); err != nil {
			return fmt.Errorf("write extract %s: %w", dest, err)
		}
	}
	return nil
}

// copyFile copies src to dst durably: write, fsync the file, then fsync the
// directory so a crash cannot leave a half-written replacement. The
// destination's permission bits follow the source when readable.
func copyFile(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	mode := os.FileMode(0o644)
	if info, err := in.Stat(); err == nil {
		mode = info.Mode().Perm()
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if dir := filepath.Dir(dst); dir != "" {
		if d, err := os.Open(dir); err == nil {
			_ = d.Sync()
			_ = d.Close()
		}
	}
	return nil
}

// humanBytes formats a byte count for skip reasons and logs.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, u := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, u)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
