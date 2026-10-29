// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Worker lifecycle unit tests (task 11) against the fake API. The fake's
// heartbeat answers the instruction, which is exactly the seam the worker
// honours. The full start/stop/drain round trips over a real server run in
// the integration build.

// runWorker starts a worker and registers cleanup that stops it.
func runWorker(t *testing.T, api *fakeAPI, dir string, run encodersRunFn, mutate func(*Config)) {
	t.Helper()
	cfg := testWorkerConfig(api, filepath.Join(dir, "scratch"), run)
	if mutate != nil {
		mutate(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("worker run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	})
}

// encodersRunFn aliases the Run seam type for readability here.
type encodersRunFn = func(ctx context.Context, args []string) error

// TestWorkerStoppedMakesZeroClaimCalls is acceptance item 1: a worker on a
// stopped node heartbeats and never claims — a fresh install with queued
// files stays untouched until the node is started.
func TestWorkerStoppedMakesZeroClaimCalls(t *testing.T) {
	api := newFakeAPI(t, "key")
	api.desiredState = NodeStateStopped
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	runWorker(t, api, dir, writeRun("x", nil), nil)

	// Several heartbeats go out, zero claims happen.
	waitUntil(t, "several heartbeats", func() bool { return api.heartbeatCount() >= 3 })
	require.Zero(t, api.claimCount(), "a stopped node makes no claim calls")
	require.Equal(t, NodeStateStopped, api.heartbeats[len(api.heartbeats)-1].State,
		"the heartbeat reports the state the worker honours")
}

// TestWorkerStartTakesEffectOnHeartbeat: the instruction flips from
// stopped to running without a process restart — the next heartbeat
// answers running and the queued job is claimed.
func TestWorkerStartTakesEffectOnHeartbeat(t *testing.T) {
	api := newFakeAPI(t, "key")
	api.desiredState = NodeStateStopped
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	runWorker(t, api, dir, writeRun("x", nil), nil)

	waitUntil(t, "stopped phase", func() bool { return api.heartbeatCount() >= 2 })
	require.Zero(t, api.claimCount())

	// The admin starts the node: the next heartbeat carries running.
	api.mu.Lock()
	api.desiredState = NodeStateRunning
	api.mu.Unlock()

	waitUntil(t, "claim after start", func() bool { return api.claimCount() >= 1 })
	waitUntil(t, "job completion", func() bool {
		return api.completeOf("job1").Encoder != ""
	})
}

// TestWorkerDrainFinishesAndStops: a draining worker finishes the job it
// holds, claims nothing new, and reports stopped once idle.
func TestWorkerDrainFinishesAndStops(t *testing.T) {
	api := newFakeAPI(t, "key")
	api.desiredState = NodeStateRunning
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	// The fake run blocks until the heartbeat loop has seen the job
	// running, giving the test a window to issue the drain.
	running := api.notifyWhenRunning("job1")
	run := func(ctx context.Context, args []string) error {
		select {
		case <-running:
		case <-time.After(5 * time.Second):
			return errors.New("no heartbeat reported job1 as running")
		case <-ctx.Done():
			return ctx.Err()
		}
		return os.WriteFile(args[len(args)-1], []byte("OUTPUT"), 0o644)
	}
	runWorker(t, api, dir, run, nil)

	waitUntil(t, "job running", func() bool {
		return api.heartbeatHadRunningJob("job1")
	})

	// A second queued job must never be claimed after the drain starts.
	// The drain is set before job2 is enqueued, so no claim can ever see
	// the job while the node is still running: claims that race ahead of
	// the drain find an empty queue, claims after it are refused by the
	// fake with 409 node_not_running, exactly like the real server.
	api.mu.Lock()
	api.desiredState = NodeStateDraining
	api.mu.Unlock()
	_, spec2 := defaultSpec(t, dir, "job2")
	api.enqueue("job2", spec2)

	waitUntil(t, "job1 completes", func() bool {
		return api.completeOf("job1").Encoder != ""
	})
	// Once idle the worker reports stopped; job2 is never claimed.
	waitUntil(t, "stopped report", func() bool {
		for _, hb := range api.heartbeatsWithState() {
			if hb.State == NodeStateStopped {
				return true
			}
		}
		return false
	})
	// The invariant the drain guarantees: no new work was handed out —
	// job2 is still queued, was never claimed and never completed. Claim
	// *attempts* may keep firing until the heartbeat delivers the drain
	// instruction, so a raw attempt count would race the heartbeat; the
	// refusals are the contract, not a claim-call tally.
	require.Contains(t, api.queuedIDs(), "job2", "a draining node is handed no new jobs")
	require.Empty(t, api.completeOf("job2").Encoder, "a draining node claims nothing new")
}
