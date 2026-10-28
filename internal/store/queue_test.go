// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queueFile inserts a file row to attach jobs to and returns its ID.
func queueFile(t *testing.T, s *SQLite, libraryID, path string) string {
	t.Helper()
	f := &File{LibraryID: libraryID, Path: path, Status: FileStatusQueued}
	require.NoError(t, s.UpsertFile(context.Background(), f))
	return f.ID
}

func queuedJob(fileID, libraryID string, priority int, created time.Time) *Job {
	return &Job{
		FileID:    fileID,
		LibraryID: libraryID,
		Status:    JobStatusQueued,
		Priority:  priority,
		CreatedAt: created,
	}
}

func TestClaimNextJobOrdering(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))

	base := time.Now().UTC().Add(-time.Hour)
	ids := make([]string, 3)
	// Three queued jobs: priorities 0, 5, 5. The priority-5 job created
	// first must be claimed before the later one.
	for i, p := range []int{0, 5, 5} {
		f := queueFile(t, s, lib.ID, fmt.Sprintf("/media/movies/%d.mkv", i))
		j := queuedJob(f, lib.ID, p, base.Add(time.Duration(i)*time.Minute))
		if i == 1 {
			// The second job is the oldest of the two priority-5 jobs.
			j.CreatedAt = base.Add(-time.Minute)
		}
		require.NoError(t, s.CreateJob(ctx, j))
		ids[i] = j.ID
	}

	j, err := s.ClaimNextJob(ctx, "node-a")
	require.NoError(t, err)
	assert.Equal(t, ids[1], j.ID, "highest priority, oldest first")
	assert.Equal(t, JobStatusClaimed, j.Status)
	assert.Equal(t, "node-a", j.NodeID)
	assert.False(t, j.ClaimedAt.IsZero())
	assert.False(t, j.HeartbeatAt.IsZero())

	j, err = s.ClaimNextJob(ctx, "node-b")
	require.NoError(t, err)
	assert.Equal(t, ids[2], j.ID)

	j, err = s.ClaimNextJob(ctx, "node-a")
	require.NoError(t, err)
	assert.Equal(t, ids[0], j.ID)

	_, err = s.ClaimNextJob(ctx, "node-a")
	assert.ErrorIs(t, err, ErrQueueEmpty)
}

func TestClaimNextJobConcurrent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))

	const total = 10
	for i := range total {
		f := queueFile(t, s, lib.ID, fmt.Sprintf("/media/movies/%d.mkv", i))
		require.NoError(t, s.CreateJob(ctx, queuedJob(f, lib.ID, 0, time.Now().UTC())))
	}

	// 50 goroutines race for 10 queued jobs: exactly 10 claims, no
	// duplicates (acceptance item).
	const workers = 50
	var mu sync.Mutex
	claimed := map[string]string{} // job id -> node id
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				j, err := s.ClaimNextJob(ctx, fmt.Sprintf("node-%d", w))
				if err != nil {
					return // queue drained for this worker
				}
				mu.Lock()
				_, dup := claimed[j.ID]
				claimed[j.ID] = j.NodeID
				mu.Unlock()
				if dup {
					t.Errorf("job %s claimed twice", j.ID)
					return
				}
			}
		}()
	}
	wg.Wait()

	assert.Len(t, claimed, total)
}

func TestRequeueStaleJobs(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))
	f := queueFile(t, s, lib.ID, "/media/movies/a.mkv")

	newRunningJob := func(status string, heartbeat time.Time) *Job {
		j := &Job{FileID: f, LibraryID: lib.ID, Status: status, NodeID: "node-a"}
		require.NoError(t, s.CreateJob(ctx, j))
		j.Status = status
		j.NodeID = "node-a"
		j.ClaimedAt = heartbeat.Add(-time.Minute)
		j.HeartbeatAt = heartbeat
		j.StartedAt = heartbeat.Add(-time.Minute)
		j.Percent = 42
		j.Speed = 1.5
		j.ETASeconds = 30
		require.NoError(t, s.UpdateJob(ctx, j))
		return j
	}

	cutoff := time.Now().UTC().Add(-2 * time.Minute)

	t.Run("fresh heartbeat is kept", func(t *testing.T) {
		j := newRunningJob(JobStatusRunning, time.Now().UTC())
		moved, err := s.RequeueStaleJobs(ctx, cutoff, 3)
		require.NoError(t, err)
		assert.Empty(t, moved)
		got, err := s.GetJob(ctx, j.ID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusRunning, got.Status)
	})

	t.Run("stale returns to queued with attempt incremented", func(t *testing.T) {
		j := newRunningJob(JobStatusRunning, cutoff.Add(-time.Minute))
		moved, err := s.RequeueStaleJobs(ctx, cutoff, 3)
		require.NoError(t, err)
		require.Len(t, moved, 1)
		assert.Equal(t, j.ID, moved[0].ID)
		assert.Equal(t, JobStatusQueued, moved[0].Status)
		assert.Equal(t, 1, moved[0].Attempt)
		assert.Empty(t, moved[0].NodeID)
		assert.True(t, moved[0].ClaimedAt.IsZero())
		assert.True(t, moved[0].HeartbeatAt.IsZero())
		assert.Zero(t, moved[0].Percent, "progress of the dead attempt is cleared")
	})

	t.Run("claimed jobs are also swept", func(t *testing.T) {
		j := newRunningJob(JobStatusClaimed, cutoff.Add(-time.Minute))
		moved, err := s.RequeueStaleJobs(ctx, cutoff, 3)
		require.NoError(t, err)
		require.Len(t, moved, 1)
		assert.Equal(t, j.ID, moved[0].ID)
		assert.Equal(t, JobStatusQueued, moved[0].Status)
	})

	t.Run("third timeout fails the job", func(t *testing.T) {
		j := newRunningJob(JobStatusRunning, cutoff.Add(-time.Minute))
		j.Attempt = 2
		require.NoError(t, s.UpdateJob(ctx, j))
		moved, err := s.RequeueStaleJobs(ctx, cutoff, 3)
		require.NoError(t, err)
		require.Len(t, moved, 1)
		assert.Equal(t, JobStatusFailed, moved[0].Status)
		assert.Equal(t, 3, moved[0].Attempt)
		assert.NotEmpty(t, moved[0].Error)
	})
}
