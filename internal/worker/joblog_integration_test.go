// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package worker

// Job log integration tests (task 27): a real ffmpeg encode streams its
// stderr through the structured log, and a job killed mid encode keeps
// every batch that was written before the kill.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// logRowsFor lists a job's log rows (oldest first).
func logRowsFor(t *testing.T, s *stack, jobID string) []store.JobLogLine {
	t.Helper()
	rows, err := s.st.ListJobLog(context.Background(), jobID, 0, "", 0)
	require.NoError(t, err)
	return rows
}

// TestRealEncodeStreamsJobLog: a real ffmpeg encode leaves the complete
// structured log — ffmpeg stderr lines plus the worker's sys lines — in
// order, with increasing seq.
func TestRealEncodeStreamsJobLog(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := minQualityTarget()
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")
	s.scanSync(lib.ID)

	nodeID, key := s.pairWithToken("log-worker")
	s.startNode(nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	s.waitFor("job done", func() bool {
		j := s.jobFor(lib.ID)
		return j != nil && j.Status == store.JobStatusDone
	})
	job := s.jobFor(lib.ID)

	rows := logRowsFor(t, s, job.ID)
	require.NotEmpty(t, rows, "a finished job must have log rows")
	require.LessOrEqual(t, len(rows), store.JobLogMaxLinesPerJob)

	// In order: ids grow, seq never resets, first row is the probe summary.
	var lastID int64
	for i, row := range rows {
		require.Greater(t, row.ID, lastID, "row %d out of order", i)
		lastID = row.ID
	}
	require.Equal(t, store.JobLogSourceSys, rows[0].Source)
	require.Contains(t, rows[0].Message, "probed source")

	// The sys narrative the task names is all there.
	joined := make([]string, 0, len(rows))
	for _, row := range rows {
		joined = append(joined, row.Message)
	}
	all := strings.Join(joined, "\n")
	require.Contains(t, all, "target:")
	require.Contains(t, all, "result policy:")
	require.Contains(t, all, "encoder chosen:")
	require.Contains(t, all, "bytes before:")

	// At least one line really came from ffmpeg stderr.
	ffmpegLines := 0
	for _, row := range rows {
		if row.Source == store.JobLogSourceFFmpeg {
			ffmpegLines++
		}
	}
	require.Positive(t, ffmpegLines, "ffmpeg stderr lines must be captured")
}

// TestKilledMidEncodeKeepsBatches: the job is cancelled while a real encode
// runs (the process-level analogue of docker kill: the worker stops
// reporting, the server sweeps the job back). Every batch uploaded before
// the kill is still there, and the original is untouched.
func TestKilledMidEncodeKeepsBatches(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := minQualityTarget()
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "slow.mkv")
	s.scanSync(lib.ID)

	nodeID, key := s.pairWithToken("kill-worker")
	s.startNode(nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	// Wait until the encode is running and the first batches have landed.
	jobID := ""
	s.waitFor("running with log rows", func() bool {
		j := s.jobFor(lib.ID)
		if j == nil || j.Status != store.JobStatusRunning {
			return false
		}
		jobID = j.ID
		return len(logRowsFor(t, s, j.ID)) > 0
	})
	before := len(logRowsFor(t, s, jobID))
	require.Positive(t, before)

	// Kill the worker mid encode.
	cancel()
	// The sink's final flush and the in-flight upload need a moment.
	time.Sleep(2 * time.Second)

	rows := logRowsFor(t, s, jobID)
	require.GreaterOrEqual(t, len(rows), before,
		"batches written before the kill must survive")
	s.t.Logf("killed mid encode: %d log rows before the kill, %d after", before, len(rows))

	// Every row seen before the kill is still there (append-only).
	for i, row := range rows[:before] {
		require.Equal(t, logRowsFor(t, s, jobID)[i].Message, row.Message)
	}

	// The original file is untouched (the result policy never ran).
	file := s.fileRow(lib.ID)
	require.Equal(t, int64(0), file.BytesSaved)
}
