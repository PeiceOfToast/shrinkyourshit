// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// Source download (task 09). The server streams the job's source with
// Range support and the file fingerprint as ETag; the node writes it into
// the job's scratch dir, resuming a partial download with Range + If-Range,
// and finally verifies size and fingerprint so an interrupted or stale
// transfer can never reach the encoder.

// downloadAttempts bounds how often a broken transfer is resumed before the
// job fails.
const downloadAttempts = 5

// downloadSource fetches the job source into dir (named source.<ext>) and
// returns its path. limitMBs caps the transfer speed (0 = unlimited).
func downloadSource(ctx context.Context, client *Client, spec *server.JobSpec, dir string, limitMBs int64, log *slog.Logger) (string, error) {
	local := filepath.Join(dir, "source"+filepath.Ext(spec.SourcePath))
	etag := `"` + spec.Fingerprint + `"`

	// A complete download left by an earlier attempt of this job is reused
	// when it still verifies; anything else is removed.
	if st, err := os.Stat(local); err == nil && st.Size() == spec.SourceSize {
		if err := finishDownload(local, spec); err == nil {
			log.Debug("worker: reusing the downloaded source of an earlier attempt", "job", spec.ID)
			return local, nil
		}
		_ = os.Remove(local)
	} else if err == nil {
		_ = os.Remove(local) // partial or oversized: start over
	}

	for attempt := 0; attempt < downloadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		start := int64(0)
		if st, err := os.Stat(local); err == nil && st.Size() < spec.SourceSize {
			start = st.Size() // resume where the last attempt stopped
		} else if err == nil {
			_ = os.Remove(local) // oversized nonsense state
		}

		resp, err := client.SourceStream(ctx, spec.ID, etag, start)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			// The connection died before the response started: resume on
			// the next attempt.
			log.Warn("worker: source stream failed; resuming", "job", spec.ID, "attempt", attempt, "err", err)
			continue
		}

		// A 200 (instead of 206) means the server ignored the range or the
		// If-Range check failed: the body starts at byte 0, so restart.
		if start > 0 && resp.StatusCode != http.StatusPartialContent {
			log.Info("worker: restarting the source download from zero", "job", spec.ID,
				"status", resp.StatusCode)
			_ = os.Remove(local)
			_ = resp.Body.Close()
			continue
		}

		// On a fresh download the ETag must be the fingerprint the job was
		// queued with; on a resume it already matched (If-Range).
		if start == 0 && resp.Header.Get("ETag") != "" && resp.Header.Get("ETag") != etag {
			_ = resp.Body.Close()
			return "", fmt.Errorf("server's source fingerprint %s does not match the job's %s",
				resp.Header.Get("ETag"), etag)
		}

		var reader io.Reader = resp.Body
		if limitMBs > 0 {
			reader = newPacedReader(resp.Body, limitMBs*1_000_000, ctx)
		}
		f, ferr := os.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if ferr != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("open download target: %w", ferr)
		}
		_, copyErr := io.Copy(f, reader)
		closeErr := f.Close()
		_ = resp.Body.Close()
		if copyErr != nil || closeErr != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			// Broken mid-transfer: the loop resumes with Range.
			log.Warn("worker: source transfer interrupted; resuming", "job", spec.ID, "attempt", attempt,
				"err", errors.Join(copyErr, closeErr))
			continue
		}

		// Full size on disk: set the source's mtime (part of the
		// fingerprint) and verify the content.
		if err := os.Chtimes(local, time.Unix(spec.SourceMtime, 0), time.Unix(spec.SourceMtime, 0)); err != nil {
			return "", fmt.Errorf("set source mtime: %w", err)
		}
		if err := finishDownload(local, spec); err != nil {
			_ = os.Remove(local)
			return "", err
		}
		return local, nil
	}
	return "", fmt.Errorf("source transfer did not complete after %d attempts", downloadAttempts)
}

// finishDownload checks a completed download: the size, then the
// fingerprint (the mtime must already be restored).
func finishDownload(local string, spec *server.JobSpec) error {
	st, err := os.Stat(local)
	if err != nil {
		return fmt.Errorf("stat downloaded source: %w", err)
	}
	if st.Size() != spec.SourceSize {
		return fmt.Errorf("downloaded source is %d bytes, expected %d", st.Size(), spec.SourceSize)
	}
	fp, err := probe.Fingerprint(local)
	if err != nil {
		return fmt.Errorf("fingerprint downloaded source: %w", err)
	}
	if fp != spec.Fingerprint {
		return fmt.Errorf("downloaded source fingerprint %s does not match the job's %s", fp, spec.Fingerprint)
	}
	return nil
}
