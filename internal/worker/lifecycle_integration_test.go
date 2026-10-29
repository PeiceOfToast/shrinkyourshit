// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package worker

// Node lifecycle integration tests (task 11): a real worker against a real
// server over HTTP, with ffmpeg doing the encodes. Acceptance items 2–4.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// minQualityTarget is the default Target at minimum quality so tiny
// synthetic fixtures always produce a smaller output.
func minQualityTarget() flow.Target {
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	return target
}

// TestNodeStartTakesEffectOnHeartbeat is acceptance item 2: a stopped node
// claims nothing; POST /nodes/{id}/start makes it claim on the next
// heartbeat, without restarting the worker process.
func TestNodeStartTakesEffectOnHeartbeat(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := minQualityTarget()
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")
	s.scanSync(lib.ID)

	// A freshly paired node is stopped (pairing alone starts nothing).
	nodeID, key := s.pairWithToken("lifecycle-worker")
	require.Equal(t, store.NodeStateStopped, s.nodeRow(nodeID).DesiredState)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	// While stopped: heartbeats happen (the worker is alive), claims do
	// not. The job stays queued for a full second of heartbeat cycles.
	time.Sleep(time.Second)
	job := s.jobFor(lib.ID)
	require.NotNil(t, job)
	require.Equal(t, store.JobStatusQueued, job.Status,
		"a stopped node must not claim anything")

	// The admin starts the node; the worker learns it from the heartbeat.
	s.startNode(nodeID)

	s.waitFor("job claimed after start", func() bool {
		j := s.jobFor(lib.ID)
		return j != nil && j.Status == store.JobStatusRunning && j.NodeID == nodeID
	})
	s.waitFor("job done after start", func() bool {
		j := s.jobFor(lib.ID)
		return j != nil && j.Status == store.JobStatusDone
	})
	require.Equal(t, store.NodeStateRunning, s.nodeRow(nodeID).DesiredState)
}

// TestNodeDrainFinishesCurrentJob is acceptance item 3: stop with drain
// finishes the in-flight job, claims nothing new and ends in stopped.
func TestNodeDrainFinishesCurrentJob(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := minQualityTarget()
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "slow.mkv")
	s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "fast.mkv")
	s.scanSync(lib.ID)

	nodeID, key := s.pairWithToken("drain-worker")
	s.startNode(nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	// The long job is in flight and a second one is queued behind it.
	s.waitFor("long job running", func() bool {
		jobs, err := s.st.ListJobs(context.Background(), store.JobFilter{LibraryID: lib.ID, Status: store.JobStatusRunning})
		return err == nil && len(jobs) == 1
	})
	jobs, err := s.st.ListJobs(context.Background(), store.JobFilter{LibraryID: lib.ID, Status: store.JobStatusQueued})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "the second file is queued behind the running one")
	secondJob := jobs[0]

	// Drain: finish what is held, claim nothing new.
	s.stopNode(nodeID, true)

	s.waitFor("drained job done", func() bool {
		j, err := s.st.GetJob(context.Background(), secondJob.ID)
		return err == nil && j.Status == store.JobStatusQueued
	})
	s.waitFor("drain completes", func() bool {
		j, err := s.st.GetJob(context.Background(), s.jobFor(lib.ID).ID)
		return err == nil && j.Status == store.JobStatusDone
	})

	// The worker went idle and reported stopped: the node ends stopped.
	s.waitFor("node ends stopped", func() bool {
		return s.nodeRow(nodeID).DesiredState == store.NodeStateStopped
	})

	// The second job was never claimed.
	second, err := s.st.GetJob(context.Background(), secondJob.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, second.Status)
	require.Empty(t, second.NodeID)
	// No new claims after the drain: give a stopped worker a moment.
	time.Sleep(500 * time.Millisecond)
	second, err = s.st.GetJob(context.Background(), secondJob.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, second.Status)
}

// TestNodeStopRequeuesJob is acceptance item 4: stop without drain stops
// claiming immediately; the in-flight job returns to queued after the
// heartbeat timeout and the original file is untouched.
func TestNodeStopRequeuesJob(t *testing.T) {
	s := newStack(t, stackConfig{
		heartbeatTimeout: 3 * time.Second,
		sweepInterval:    250 * time.Millisecond,
	})
	target := minQualityTarget()
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "long.mkv")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID)

	nodeID, key := s.pairWithToken("stop-worker")
	s.startNode(nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	var jobID string
	s.waitFor("job running", func() bool {
		j := s.jobFor(lib.ID)
		if j == nil {
			return false
		}
		jobID = j.ID
		return j.Status == store.JobStatusRunning
	})

	// Stop now: the worker abandons the encode (no report), the job's
	// heartbeat goes stale, the sweeper requeues it.
	s.stopNode(nodeID, false)

	s.waitFor("job requeued after heartbeat timeout", func() bool {
		j, err := s.st.GetJob(context.Background(), jobID)
		if err != nil || j.Status != store.JobStatusQueued || j.Attempt != 1 {
			return false
		}
		// The sweep also hands the file back to the queue; wait for both
		// so the assertions below cannot race the sweeper.
		row := s.fileRow(lib.ID)
		return row.Status == store.FileStatusQueued
	})

	// The original is byte-identical and no replacement appeared.
	require.Equal(t, origData, s.fileHash(origPath))
	entries, err := os.ReadDir(s.library)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// The stopped node does not pick the job back up.
	time.Sleep(time.Second)
	j, err := s.st.GetJob(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, j.Status)
	require.Empty(t, j.NodeID)
}

