// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Shared-storage access check (task 09). The node asks the server for a
// 1 MiB sample of the job's source at a random offset, reads the same
// bytes through its path mapping and compares. Match → the library is
// visible (shared); anything else → the server transfers the file. The
// outcome is cached per library for 10 minutes so a whole library does not
// re-probe its mount on every job.

// AccessCacheTTL is how long a per-library shared/transfer decision is
// reused (task 09).
const AccessCacheTTL = 10 * time.Minute

// accessCache remembers the last decision per library.
type accessCache struct {
	mu    sync.Mutex
	byLib map[string]cacheEntry
}

type cacheEntry struct {
	mode string
	at   time.Time
}

func newAccessCache() *accessCache {
	return &accessCache{byLib: map[string]cacheEntry{}}
}

// get returns the cached decision for a library when it is fresh.
func (c *accessCache) get(libraryID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byLib[libraryID]
	if !ok || time.Since(e.at) >= AccessCacheTTL {
		return "", false
	}
	return e.mode, true
}

func (c *accessCache) put(libraryID, mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byLib[libraryID] = cacheEntry{mode: mode, at: time.Now()}
}

// resolveAccessMode decides how this node reaches the job's source:
// configured shared/transfer pass through; auto asks the server for a
// sample and compares (with the per-library cache in front). The returned
// mode is one of store.NodeAccessShared / store.NodeAccessTransfer.
func resolveAccessMode(ctx context.Context, client *Client, spec *server.JobSpec,
	cache *accessCache, log *slog.Logger) (string, error) {
	switch spec.AccessMode {
	case store.NodeAccessShared:
		return store.NodeAccessShared, nil
	case store.NodeAccessTransfer:
		return store.NodeAccessTransfer, nil
	case store.NodeAccessAuto:
	default:
		return "", fmt.Errorf("worker: unknown access mode %q", spec.AccessMode)
	}

	if mode, ok := cache.get(spec.LibraryID); ok {
		log.Debug("worker: access mode from cache", "library", spec.LibraryID, "mode", mode)
		return mode, nil
	}

	challenge, err := client.AccessCheck(ctx, spec.ID, spec.LibraryID)
	if err != nil {
		// No answer means no verified shared view: transfer is the safe
		// choice and keeps the job moving.
		log.Warn("worker: access check failed; falling back to transfer", "job", spec.ID, "err", err)
		cache.put(spec.LibraryID, store.NodeAccessTransfer)
		return store.NodeAccessTransfer, nil
	}
	mode, err := verifySample(challenge)
	if err != nil {
		// The mapped path is not the file the server described: transfer.
		log.Info("worker: shared-storage check failed; using transfer", "job", spec.ID, "err", err)
		mode = store.NodeAccessTransfer
	}
	cache.put(spec.LibraryID, mode)
	return mode, nil
}

// verifySample reproduces the server's sample through the node-side path.
func verifySample(ch *AccessChallenge) (string, error) {
	st, err := os.Stat(ch.Path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", ch.Path, err)
	}
	if st.Size() != ch.Size {
		return "", fmt.Errorf("file size %d does not match the server's %d", st.Size(), ch.Size)
	}
	f, err := os.Open(ch.Path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", ch.Path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(ch.SampleOffset, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek %s: %w", ch.Path, err)
	}
	h := sha256.New()
	length := min(int64(1<<20), ch.Size-ch.SampleOffset)
	if _, err := io.CopyN(h, f, length); err != nil {
		return "", fmt.Errorf("read sample of %s: %w", ch.Path, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != ch.SampleSHA256 {
		return "", fmt.Errorf("sample digest does not match the server's copy of %s", ch.Path)
	}
	return store.NodeAccessShared, nil
}
