// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Transfer owns the server side of transfer-mode jobs (task 09): the
// shared-storage challenge for the node's access check, and the chunked,
// checksum-verified result upload that lands in the server's scratch dir.
// The verify-and-replace of that upload happens in the API layer (which
// reuses ApplyResultPolicy) before Queue.Complete records the outcome.

// SampleSize is how many bytes the shared-storage check hashes (task 09:
// a 1 MiB sample at a random offset).
const SampleSize = 1 << 20 // 1 MiB

// Transfer state errors the API maps to HTTP statuses.
var (
	// ErrUploadGap: a chunk starts beyond what has been received (a server
	// restart lost the partial file mid-upload) — the node restarts from 0.
	ErrUploadGap = errors.New("transfer: chunk does not continue the upload")
	// ErrChecksumMismatch: the final upload failed the X-Checksum-SHA256
	// verify; the partial upload is discarded, nothing was touched.
	ErrChecksumMismatch = errors.New("transfer: result checksum mismatch")
)

// MaxChunkBytes bounds one uploaded chunk (the worker uses 8 MiB; the
// server accepts generous slack so slower links can send bigger chunks).
const MaxChunkBytes = 64 << 20 // 64 MiB

// Transfer is the server-side transfer service. Safe for concurrent use.
type Transfer struct {
	st      store.Store
	scratch string // root of the upload area
}

// NewTransfer builds the service. An empty scratch dir falls back to a
// directory under the OS temp path.
func NewTransfer(st store.Store, scratchDir string) *Transfer {
	if scratchDir == "" {
		scratchDir = filepath.Join(os.TempDir(), "shrinkyourshit-uploads")
	}
	return &Transfer{st: st, scratch: filepath.Join(scratchDir, "uploads")}
}

// AccessChallenge is the shared-storage check payload (API.md, task 09):
// the node must hash the same 1 MiB sample from the same offset of the file
// it sees through its path mappings and produce the same digest.
type AccessChallenge struct {
	// Path is the node-side path of the source (the node's mappings are
	// already applied, exactly like the job spec's source_path).
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	SampleOffset int64  `json:"sample_offset"`
	SampleSHA256 string `json:"sample_sha256"`
}

// Challenge builds the check for one job's source file. The sample offset
// is random so a node cannot pass by pointing its mapping at a file that
// happens to match some fixed region.
func (t *Transfer) Challenge(node *store.Node, f *store.File) (AccessChallenge, error) {
	st, err := os.Stat(f.Path)
	if err != nil {
		return AccessChallenge{}, fmt.Errorf("transfer: stat source %s: %w", f.Path, err)
	}
	size := st.Size()
	offset := int64(0)
	if size > 1 {
		n, err := rand.Int(rand.Reader, big.NewInt(size))
		if err != nil {
			return AccessChallenge{}, fmt.Errorf("transfer: sample offset: %w", err)
		}
		offset = n.Int64()
	}
	length := min(SampleSize, size-offset)
	sum, err := hashFileSample(f.Path, offset, length)
	if err != nil {
		return AccessChallenge{}, err
	}
	return AccessChallenge{
		Path:         ResolveNodePath(node, f.Path),
		Size:         size,
		SampleOffset: offset,
		SampleSHA256: sum,
	}, nil
}

// hashFileSample hashes length bytes of a file starting at offset.
func hashFileSample(path string, offset, length int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("transfer: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", fmt.Errorf("transfer: seek %s: %w", path, err)
		}
	}
	h := sha256.New()
	if _, err := io.CopyN(h, f, length); err != nil {
		return "", fmt.Errorf("transfer: sample %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// jobUploadDir is the server-side scratch directory of one job's upload.
func (t *Transfer) jobUploadDir(jobID string) string {
	return filepath.Join(t.scratch, "job-"+jobID)
}

// verifiedOutputPath is where a fully verified upload is parked (renamed
// out of the .part name so an unfinished upload can never be mistaken for
// a verified result).
func (t *Transfer) verifiedOutputPath(jobID, ext string) string {
	return filepath.Join(t.jobUploadDir(jobID), "output"+ext)
}

// outputExt resolves the target container extension of a job's library.
func (t *Transfer) outputExt(ctx context.Context, job *store.Job) (string, error) {
	lib, err := t.st.GetLibrary(ctx, job.LibraryID)
	if err != nil {
		return "", fmt.Errorf("transfer: library lookup: %w", err)
	}
	target, err := flow.ParseTarget([]byte(lib.TargetJSON))
	if err != nil {
		return "", fmt.Errorf("transfer: stored target: %w", err)
	}
	ext := ".mkv"
	if target.Container == flow.ContainerMP4 {
		ext = ".mp4"
	}
	return ext, nil
}

// PutChunk writes one upload chunk (API.md task 09). start/end/total come
// from the chunk's Content-Range (this chunk carries bytes start..end of a
// total-sized file); body carries exactly end-start+1 bytes; checksum is
// the X-Checksum-SHA256 of the whole file, required on the final chunk and
// verified before the upload counts as received. Chunks may repeat (a
// timed-out re-send overwrites its region); a chunk beyond the received
// prefix is ErrUploadGap. The first chunk (start 0) discards any previous
// upload of this job: a fresh attempt replaces stale state.
func (t *Transfer) PutChunk(ctx context.Context, job *store.Job, start, end, total int64, body io.Reader, checksum string) (bool, error) {
	if start < 0 || end < start || total <= 0 || end >= total {
		return false, fmt.Errorf("transfer: invalid chunk range")
	}
	count := end - start + 1
	if count > MaxChunkBytes {
		return false, fmt.Errorf("transfer: chunk of %d bytes exceeds the %d byte limit", count, MaxChunkBytes)
	}
	ext, err := t.outputExt(ctx, job)
	if err != nil {
		return false, err
	}
	dir := t.jobUploadDir(job.ID)
	if start == 0 {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("transfer: create upload dir: %w", err)
		}
		// A new upload supersedes any earlier (stale) verified result of
		// the same job — e.g. after an attempt was requeued.
		_ = os.Remove(t.verifiedOutputPath(job.ID, ext))
	}
	partPath := filepath.Join(dir, "output"+ext+".part")
	cur := int64(0)
	if st, serr := os.Stat(partPath); serr == nil {
		cur = st.Size()
	}
	if start > cur {
		return false, ErrUploadGap
	}
	// A start below the received prefix is a retransmit of an
	// already-acknowledged chunk; the write below overwrites the region
	// and truncates the tail to this chunk's end.

	// Copy exactly the chunk's bytes from the body into the file.
	flags := os.O_WRONLY | os.O_CREATE
	if start == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return false, fmt.Errorf("transfer: open upload: %w", err)
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("transfer: seek upload: %w", err)
	}
	if _, err := io.CopyN(f, body, count); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("transfer: write chunk: %w", err)
	}
	if err := f.Truncate(start + count); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("transfer: truncate upload: %w", err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("transfer: close upload: %w", err)
	}

	if end+1 < total {
		return false, nil // more chunks to come
	}

	// Final chunk: the whole file must be there and match the checksum.
	if checksum == "" {
		return false, fmt.Errorf("transfer: the final chunk must carry X-Checksum-SHA256")
	}
	st, err := os.Stat(partPath)
	if err != nil {
		return false, fmt.Errorf("transfer: stat upload: %w", err)
	}
	if st.Size() != total {
		return false, ErrChecksumMismatch
	}
	sum, err := hashFileSample(partPath, 0, total)
	if err != nil {
		return false, err
	}
	if sum != checksum {
		_ = os.Remove(partPath) // a corrupt upload is not worth disk space
		return false, ErrChecksumMismatch
	}
	if err := os.Rename(partPath, t.verifiedOutputPath(job.ID, ext)); err != nil {
		return false, fmt.Errorf("transfer: finalize upload: %w", err)
	}
	return true, nil
}

// VerifiedOutput returns the verified upload of a job (its scratch path and
// size). store.ErrNotFound when nothing verified exists yet.
func (t *Transfer) VerifiedOutput(ctx context.Context, job *store.Job) (string, int64, error) {
	ext, err := t.outputExt(ctx, job)
	if err != nil {
		return "", 0, err
	}
	path := t.verifiedOutputPath(job.ID, ext)
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, store.ErrNotFound
		}
		return "", 0, fmt.Errorf("transfer: stat upload: %w", err)
	}
	return path, st.Size(), nil
}

// Discard removes a job's upload scratch (after the result was applied, on
// failure, or when the job was cancelled). Safe to call any time.
func (t *Transfer) Discard(jobID string) {
	_ = os.RemoveAll(t.jobUploadDir(jobID))
}
