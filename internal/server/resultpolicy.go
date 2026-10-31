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

// MoveDirective carries the flow's move step (task 24) into the result
// policy: where the produced file ends up and how its path is built. The
// zero value means no move step: results land beside the original, exactly
// as before. The paths are in the caller's view — the worker passes its
// node-side spec mapping, the server its library-side paths.
type MoveDirective struct {
	// Destination is the absolute destination root, outside the library
	// (enforced when the flow is saved).
	Destination string
	// KeepTree rebuilds the original's directory layout under the
	// destination.
	KeepTree bool
	// RelDir is the original's directory relative to the library root,
	// view-agnostic ("" for a file at the root). Used when KeepTree is set.
	RelDir string
}

// destinationDir resolves the directory the move writes into, or "" when no
// move is configured. It never resolves inside the original's own folder: a
// moved file sitting beside its sources would be picked up again by the
// next scan.
func (m MoveDirective) destinationDir(original string) (string, error) {
	if m.Destination == "" {
		return "", nil
	}
	dir := m.Destination
	if m.KeepTree && m.RelDir != "" {
		dir = filepath.Join(dir, filepath.FromSlash(m.RelDir))
	}
	if src := filepath.Dir(original); sameOrInside(src, dir) {
		return "", fmt.Errorf("move destination %s is inside the source folder %s", dir, src)
	}
	return dir, nil
}

// sameOrInside reports whether path is root or lies inside it
// (path-boundary aware).
func sameOrInside(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// RelDirUnder returns the directory of path relative to root, for the move
// step's keep_tree (task 24). Empty when the file sits at the root, or
// outside it (which should never happen for a library file).
func RelDirUnder(root, path string) string {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(filepath.Dir(path)))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return rel
}

// expandRename fills a rename template (task 14): {name} is the original
// base name, {ext} the output extension. Unknown tokens are left alone; a
// template that does not mention {name} cannot be validated (the step
// validation rejects it), so every output keeps its own name.
func expandRename(template, origBase, ext string) string {
	out := strings.ReplaceAll(template, "{name}", origBase)
	out = strings.ReplaceAll(out, "{ext}", strings.TrimPrefix(ext, "."))
	return out
}

// ApplyResultPolicy moves the verified output to its final place
// (ARCHITECTURE.md result policies and replacement rules):
//
//   - replace_if_smaller: replace only when the output is strictly smaller,
//     otherwise keep the original and report why;
//   - replace: always replace;
//   - keep_both: write <name>.shrunk.<ext> beside the untouched original —
//     or, when renameTemplate is set (task 14: a rename step in the flow),
//     under the template-expanded name. Tokens: {name} is the original base
//     name, {ext} the output extension. Templates never hold a path
//     separator (validated on save), so the output stays beside the original.
//
// When the flow carries a move step (task 24, move.Destination set), the
// produced file goes to the destination directory instead of beside the
// original — the template still names the file (keep-both), the destination
// names the directory. Replace policies remove the original only after the
// moved file is verified at its expected size; keep-both keeps it.
//
// Replacement is atomic: copy to <dest-dir>/.<name>.shrink-tmp, fsync, then
// rename over the destination. The original is removed only after the
// verified output exists in place, and only when the extension changed.
// Extracted subtitle files move beside the final output; extracts are
// dropped when the original is kept (it still holds the embedded tracks).
func ApplyResultPolicy(ctx context.Context, policy, output, original string, outSize, originalSize int64, extracts []string, renameTemplate string, move MoveDirective) (ApplyResult, error) {
	ext := filepath.Ext(output)
	dir := filepath.Dir(original)
	origBase := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	moveDir, err := move.destinationDir(original)
	if err != nil {
		return ApplyResult{}, err
	}

	switch policy {
	case store.ResultPolicyReplaceIfSmaller:
		if outSize >= originalSize {
			return ApplyResult{SkipReason: fmt.Sprintf(
				"output (%s) is not smaller than the original (%s); original kept",
				HumanBytes(outSize), HumanBytes(originalSize))}, nil
		}
		if moveDir != "" {
			return moveInto(ctx, output, original, moveDir, origBase+ext, outSize, extracts, true)
		}
		return replace(ctx, output, original, dir, origBase, ext, extracts)
	case store.ResultPolicyReplace:
		if moveDir != "" {
			return moveInto(ctx, output, original, moveDir, origBase+ext, outSize, extracts, true)
		}
		return replace(ctx, output, original, dir, origBase, ext, extracts)
	case store.ResultPolicyKeepBoth:
		base := origBase + ".shrunk"
		if renameTemplate != "" {
			base = expandRename(renameTemplate, origBase, ext)
		}
		if moveDir != "" {
			// Keep-both semantics: the produced file moves away, the
			// original stays.
			return moveInto(ctx, output, original, moveDir, base+ext, outSize, extracts, false)
		}
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

// moveInto places the verified output at destDir/name (task 24's move
// step). It never clobbers: an existing destination fails before anything
// is touched, leaving the scratch output and the original in place. The
// move is a rename, with a verified copy across filesystems; the original
// is removed only after the destination exists at the expected size, and
// only when the policy asked for it.
func moveInto(ctx context.Context, output, original, destDir, name string, outSize int64, extracts []string, removeOriginal bool) (ApplyResult, error) {
	dest := filepath.Join(destDir, name)
	if _, err := os.Stat(dest); err == nil {
		return ApplyResult{}, fmt.Errorf("move destination %s already exists; not overwriting it, the original %s is untouched", dest, original)
	} else if !os.IsNotExist(err) {
		return ApplyResult{}, fmt.Errorf("stat move destination %s: %w", dest, err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("create move destination %s: %w", destDir, err)
	}
	if err := os.Rename(output, dest); err != nil {
		// Different filesystems (or a restricted scratch dir): copy, fsync,
		// then drop the scratch copy. The scratch dir is wiped after the
		// job anyway, so a failed drop is harmless.
		if err := copyFile(ctx, output, dest); err != nil {
			return ApplyResult{}, fmt.Errorf("copy %s to %s: %w", output, dest, err)
		}
		_ = os.Remove(output)
	}
	st, err := os.Stat(dest)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("verify moved file %s: %w", dest, err)
	}
	if st.Size() != outSize {
		return ApplyResult{}, fmt.Errorf("moved file %s holds %d bytes, want %d", dest, st.Size(), outSize)
	}
	finalBase := strings.TrimSuffix(name, filepath.Ext(name))
	if err := moveExtracts(ctx, extracts, destDir, finalBase); err != nil {
		return ApplyResult{}, err
	}
	if removeOriginal {
		if err := os.Remove(original); err != nil && !os.IsNotExist(err) {
			return ApplyResult{}, fmt.Errorf("remove original %s: %w", original, err)
		}
	}
	return ApplyResult{FinalPath: dest}, nil
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

// HumanBytes formats a byte count for skip reasons and logs. Exported so
// the worker's transfer skip message matches the server's exactly.
func HumanBytes(n int64) string {
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
