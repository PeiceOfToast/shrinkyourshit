// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queueFixture prepares a library plus one file that needs work and
// returns (lib, file).
func queueFixture(t *testing.T, h *harness) (*store.Library, *store.File) {
	t.Helper()
	lib := h.createLibrary(false)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "movie.mkv", 2048)
	h.scan(lib.ID)
	fs := h.files(lib.ID)
	require.Len(t, fs, 1)
	for _, f := range fs {
		return lib, f
	}
	return nil, nil
}

func TestQueueClaimConcurrentNoDuplicates(t *testing.T) {
	h := newHarness(t)
	lib, _ := queueFixture(t, h)

	// Nine more queued jobs (the fixture file already queued one): ten in
	// total across ten files.
	for i := range 9 {
		name := "jobfile" + string(rune('a'+i)) + ".mkv"
		h.prober.mu.Lock()
		h.prober.byName[name] = h264Needs
		h.prober.mu.Unlock()
		writeFixture(t, h.root, name, 1024)
	}
	h.scan(lib.ID)
	require.Len(t, h.files(lib.ID), 10)

	// 50 goroutines claim concurrently: exactly 10 claims, no duplicates
	// (acceptance item).
	const workers = 50
	var mu sync.Mutex
	claimed := map[string]string{}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, err := h.svc.Queue().Claim(context.Background(), "node-x")
				if err != nil {
					return // queue drained
				}
				mu.Lock()
				_, dup := claimed[job.ID]
				claimed[job.ID] = job.NodeID
				mu.Unlock()
				if dup {
					t.Errorf("job %s claimed twice", job.ID)
					return
				}
			}
		}()
	}
	wg.Wait()
	assert.Len(t, claimed, 10)

	// Claiming also moves the files to processing.
	processing := 0
	for _, f := range h.files(lib.ID) {
		if f.Status == store.FileStatusProcessing {
			processing++
		}
	}
	assert.Equal(t, 10, processing)
}

func TestQueueSweeperRequeuesAndFails(t *testing.T) {
	h := newHarness(t)
	_, file := queueFixture(t, h)
	ctx := context.Background()

	// Claim, then let the heartbeat go stale.
	job, err := h.svc.Queue().Claim(ctx, "node-a")
	require.NoError(t, err)
	job.Status = store.JobStatusRunning
	job.StartedAt = time.Now().UTC().Add(-time.Hour)
	require.NoError(t, h.st.UpdateJob(ctx, job))
	// Push the heartbeat into the past: the sweeper uses now-120s.
	job.HeartbeatAt = time.Now().UTC().Add(-2 * time.Minute)
	require.NoError(t, h.st.UpdateJob(ctx, job))

	// The default sweep interval is too slow for a test; sweep directly.
	moved, err := h.svc.Queue().Sweep(ctx, 0) // timeout 0 → cutoff = now
	require.NoError(t, err)
	require.Len(t, moved, 1)
	assert.Equal(t, store.JobStatusQueued, moved[0].Status)
	assert.Equal(t, 1, moved[0].Attempt)

	f, err := h.st.GetFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FileStatusQueued, f.Status, "file handed back to the queue")

	// Two more timeout cycles: attempt 2 requeues, attempt 3 fails.
	for i := 0; i < 2; i++ {
		job, err = h.svc.Queue().Claim(ctx, "node-a")
		require.NoError(t, err)
		job.Status = store.JobStatusRunning
		stale := time.Now().UTC().Add(-2 * time.Minute)
		job.HeartbeatAt = stale
		require.NoError(t, h.st.UpdateJob(ctx, job))
		moved, err = h.svc.Queue().Sweep(ctx, 0)
		require.NoError(t, err)
		require.Len(t, moved, 1)
	}
	assert.Equal(t, store.JobStatusFailed, moved[0].Status)
	assert.Equal(t, 3, moved[0].Attempt)
	assert.NotEmpty(t, moved[0].Error)

	f, err = h.st.GetFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FileStatusFailed, f.Status)
}

func TestQueueCancelAndRetry(t *testing.T) {
	h := newHarness(t)
	_, file := queueFixture(t, h)
	ctx := context.Background()
	q := h.svc.Queue()

	// Cancel a queued job: the file returns to new.
	job, err := q.Claim(ctx, "node-a")
	require.NoError(t, err)
	cancelled, err := q.Cancel(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, store.JobStatusCancelled, cancelled.Status)
	f, err := h.st.GetFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FileStatusNew, f.Status)

	// Cancelling again is a conflict.
	_, err = q.Cancel(ctx, job.ID)
	assert.ErrorIs(t, err, ErrJobNotActive)

	// Retry brings it back to queued with a fresh attempt count.
	retried, err := q.Retry(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, store.JobStatusQueued, retried.Status)
	assert.Equal(t, 0, retried.Attempt)
	f, err = h.st.GetFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FileStatusQueued, f.Status)

	// Retrying an active job is a conflict.
	_, err = q.Retry(ctx, job.ID)
	assert.ErrorIs(t, err, ErrJobNotRetryable)
}

func TestQueueRetryRefusesSatisfiedFile(t *testing.T) {
	h := newHarness(t)
	_, file := queueFixture(t, h)
	ctx := context.Background()

	job, err := h.svc.Queue().Claim(ctx, "node-a")
	require.NoError(t, err)
	_, err = h.svc.Queue().Cancel(ctx, job.ID)
	require.NoError(t, err)

	// The file became satisfied meanwhile (e.g. the target changed).
	file.Status = store.FileStatusSatisfied
	require.NoError(t, h.st.UpdateFile(ctx, file))
	_, err = h.svc.Queue().Retry(ctx, job.ID)
	assert.ErrorIs(t, err, ErrFileSatisfied)
}
