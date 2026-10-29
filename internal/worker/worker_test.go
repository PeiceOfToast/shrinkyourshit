// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// testWorkerConfig builds a fast, deterministic worker config for tests.
func testWorkerConfig(api *fakeAPI, scratchDir string, run encoders.RunFn) Config {
	return Config{
		ServerURL:         api.srv.URL,
		Key:               api.key,
		ScratchDir:        scratchDir,
		HeartbeatInterval: 30 * time.Millisecond,
		ProgressInterval:  10 * time.Millisecond,
		ClaimPollInterval: 5 * time.Millisecond,
		MaxSlots:          2,
		Capabilities:      &encoders.Capability{Encoders: []encoders.EncoderInfo{{Name: "cpu", Codecs: []string{"hevc"}}}},
		Encoders:          []encoders.Encoder{encoders.CPU},
		Run:               run,
		Prober: proberByBase(map[string]probe.MediaInfo{
			"source.mp4": srcInfoF(),
			"output.mkv": outInfoF(),
		}),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// waitUntil polls cond until it holds or the deadline passes.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWorkerRunProcessesQueuedJob(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	// The heartbeat must report the desired concurrency; the worker learns it.
	api.concurrency = 1

	// The instant fake run could finish before the first heartbeat fires,
	// so the run blocks until the fake API has actually recorded a
	// heartbeat that reports job1 as running, then produces the output.
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

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, testWorkerConfig(api, filepath.Join(dir, "scratch"), run))
	}()

	waitUntil(t, "job completion", func() bool { return api.completeOf("job1").Encoder != "" })
	cancel()
	require.NoError(t, <-done)

	in := api.completeOf("job1")
	require.Equal(t, "cpu", in.Encoder)
	require.Equal(t, filepath.Join(dir, "movie.mkv"), in.OutputPath)

	// The heartbeat carried the running job id while the job was active.
	require.True(t, api.heartbeatHadRunningJob("job1"))

	// Scratch cleaned up after the job.
	_, err := filepath.Glob(filepath.Join(dir, "scratch", "*"))
	require.NoError(t, err)
}

func TestWorkerRunCleansOrphanedScratchOnStart(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	scratchDir := filepath.Join(dir, "scratch")
	require.NoError(t, mkAll(filepath.Join(scratchDir, "job-orphan")))
	require.NoError(t, writeFile(filepath.Join(scratchDir, "job-orphan", "leftover"), "junk"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, testWorkerConfig(api, scratchDir, writeRun("x", nil))) }()

	waitUntil(t, "orphan cleanup", func() bool {
		entries, err := listDir(scratchDir)
		return err == nil && len(entries) == 0
	})
	cancel()
	require.NoError(t, <-done)
}

func TestWorkerRunFailsJobWhenRunnerFails(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, testWorkerConfig(api, filepath.Join(dir, "scratch"),
			func(context.Context, []string) error { return errors.New("kaboom") }))
	}()

	waitUntil(t, "fail report", func() bool {
		_, ok := api.failOf("job1")
		return ok
	})
	cancel()
	require.NoError(t, <-done)

	rep, _ := api.failOf("job1")
	require.Contains(t, rep.Error, "kaboom")
}

func TestWorkerRejectsBadConfig(t *testing.T) {
	_, err := New(Config{Key: "k"})
	require.ErrorContains(t, err, "server URL")
	_, err = New(Config{ServerURL: "http://x"})
	require.ErrorContains(t, err, "node key")
}

func TestClientClaimEmptyAndAuth(t *testing.T) {
	api := newFakeAPI(t, "key")
	cl := NewClient(api.srv.URL, "key")

	job, err := cl.Claim(context.Background())
	require.NoError(t, err)
	require.Nil(t, job) // 204

	// Wrong key gets a typed 401.
	bad := NewClient(api.srv.URL, "wrong")
	_, err = bad.Claim(context.Background())
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)

	// Heartbeat parses concurrency and floors it at 1.
	n, err := cl.Heartbeat(context.Background(), HeartbeatInput{})
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestClientServerDown(t *testing.T) {
	cl := NewClient("http://127.0.0.1:1", "key") // nothing listens there
	_, err := cl.Claim(context.Background())
	require.Error(t, err)
}

func TestWaitUntilHelper(t *testing.T) {
	waitUntil(t, "immediate", func() bool { return true })
}

// mkAll creates a directory (test helper around os.MkdirAll).
func mkAll(path string) error { return os.MkdirAll(path, 0o755) }

// writeFile writes a file (test helper around os.WriteFile).
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// listDir lists a directory's entry names.
func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}
