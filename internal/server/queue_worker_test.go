// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// inspectedOutput is the fake inspector result for queue completion tests.
var inspectedOutput = ResultInfo{
	Fingerprint: "200:1700000001:fedcba",
	Size:        100,
	Mtime:       1700000001,
	Info: probe.MediaInfo{
		Container:       flow.ContainerMKV,
		DurationSeconds: 2,
		Video:           []probe.VideoStream{{Codec: flow.CodecHEVC}},
		Audio:           []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
	},
}

// claimJob queues a file, claims it for the node and returns the job.
func claimJob(t *testing.T, h *harness, nodeID string) (*store.Library, *store.File, *store.Job) {
	return claimJobPolicy(t, h, nodeID, store.ResultPolicyReplaceIfSmaller)
}

func claimJobPolicy(t *testing.T, h *harness, nodeID, policy string) (*store.Library, *store.File, *store.Job) {
	t.Helper()
	ctx := context.Background()
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	lib, err := h.svc.Libraries().Create(ctx, LibraryParams{
		Name: "Movies", Path: h.root, TargetJSON: target, ResultPolicy: policy,
	})
	require.NoError(t, err)

	f := &store.File{
		LibraryID: lib.ID, Path: h.root + "/movie.mp4", Size: 500,
		Status: store.FileStatusNew, ProbeJSON: `{"container":"mp4"}`,
		Fingerprint: "500:1700000000:abcdef",
	}
	require.NoError(t, h.st.UpsertFile(ctx, f))

	job, queued, err := h.svc.Queue().QueueJob(ctx, lib, f, []string{"video is h264"})
	require.NoError(t, err)
	require.True(t, queued)

	claimed, err := h.svc.Queue().Claim(ctx, nodeID)
	require.NoError(t, err)
	require.Equal(t, job.ID, claimed.ID)
	return lib, f, claimed
}

func TestProgressMovesClaimedToRunning(t *testing.T) {
	h := newHarness(t)
	_, f, job := claimJob(t, h, "node-1")

	got, err := h.svc.Queue().Progress(context.Background(), job.ID, "node-1", ProgressParams{
		Percent: 42.5, Speed: 2.0, ETASeconds: 30, Encoder: "cpu",
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusRunning, got.Status)
	require.InDelta(t, 42.5, got.Percent, 0.001)
	require.Equal(t, int64(30), got.ETASeconds)
	require.Equal(t, "cpu", got.EncoderUsed)
	require.False(t, got.StartedAt.IsZero())
	require.WithinDuration(t, time.Now(), got.HeartbeatAt, 5*time.Second)

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusProcessing, after.Status)
}

func TestProgressRejectsForeignOrInactiveJobs(t *testing.T) {
	h := newHarness(t)
	_, _, job := claimJob(t, h, "node-1")

	_, err := h.svc.Queue().Progress(context.Background(), job.ID, "node-2", ProgressParams{})
	require.ErrorIs(t, err, ErrJobNotClaimed)

	// Once done, no more progress.
	done, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		SkippedReason: "skip", Encoder: "cpu",
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusDone, done.Status)

	_, err = h.svc.Queue().Progress(context.Background(), job.ID, "node-1", ProgressParams{})
	require.ErrorIs(t, err, ErrJobNotActive)
}

func TestCompleteReplacedFile(t *testing.T) {
	h := newHarness(t)
	h.svc.Queue().SetInspector(func(_ context.Context, _, path string) (ResultInfo, error) {
		require.Equal(t, "/library/movie.mkv", path)
		return inspectedOutput, nil
	}, "")
	lib, f, job := claimJob(t, h, "node-1")

	got, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath:      "/library/movie.mkv",
		BytesAfter:      100,
		Encoder:         "cpu",
		FallbackReasons: []string{"nvenc: no device"},
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusDone, got.Status)
	require.Equal(t, "cpu", got.EncoderUsed)
	require.Equal(t, int64(100), got.BytesAfter)
	var fallback []string
	require.NoError(t, json.Unmarshal([]byte(got.FallbackReasons), &fallback))
	require.Equal(t, []string{"nvenc: no device"}, fallback)

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusSatisfied, after.Status)
	require.Equal(t, "/library/movie.mkv", after.Path)
	require.Equal(t, int64(100), after.Size)
	require.Equal(t, inspectedOutput.Fingerprint, after.Fingerprint)
	require.Equal(t, int64(400), after.BytesSaved) // 500 - 100
	require.Empty(t, after.SatisfiedReason)

	// The stored probe is the output probe, so a rescan reuses it.
	var info probe.MediaInfo
	require.NoError(t, json.Unmarshal([]byte(after.ProbeJSON), &info))
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)

	_ = lib
}

func TestCompleteSkippedOriginal(t *testing.T) {
	h := newHarness(t)
	// No inspector installed: a skip must never call it.
	calls := 0
	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		calls++
		return ResultInfo{}, nil
	}, "")
	_, f, job := claimJob(t, h, "node-1")

	got, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		SkippedReason: "output is not smaller than the original",
		BytesAfter:    500,
		Encoder:       "cpu",
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusDone, got.Status)

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusDone, after.Status)
	require.Equal(t, int64(0), after.BytesSaved)
	require.Equal(t, "output is not smaller than the original", after.SatisfiedReason)
	// Nothing changed on disk: fingerprint and probe stay from the scan.
	require.Equal(t, "500:1700000000:abcdef", after.Fingerprint)
	require.Equal(t, 0, calls)
}

func TestCompleteKeepBothKeepsOriginalRow(t *testing.T) {
	h := newHarness(t)
	h.svc.Queue().SetInspector(func(_ context.Context, _, path string) (ResultInfo, error) {
		// The sidecar is inspected but must not replace the original row.
		require.Equal(t, "/library/movie.shrunk.mkv", path)
		return inspectedOutput, nil
	}, "")
	lib, f, job := claimJobPolicy(t, h, "node-1", store.ResultPolicyKeepBoth)

	_, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath: "/library/movie.shrunk.mkv",
		BytesAfter: 100,
		Encoder:    "cpu",
	})
	require.NoError(t, err)

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusSatisfied, after.Status)
	// The row still points at the untouched original; keep_both must not
	// re-point it at the sidecar.
	require.Equal(t, lib.Path+"/movie.mp4", after.Path)
	require.Equal(t, "500:1700000000:abcdef", after.Fingerprint)
	require.Equal(t, int64(0), after.BytesSaved) // disk usage did not shrink
}

func TestCompleteDropsDuplicateRows(t *testing.T) {
	h := newHarness(t)
	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return inspectedOutput, nil
	}, "")
	_, f, job := claimJob(t, h, "node-1")

	// A racing scan created a row for the new path while the original row
	// still had the old one.
	racing := &store.File{LibraryID: f.LibraryID, Path: "/library/movie.mkv",
		Status: store.FileStatusSatisfied}
	require.NoError(t, h.st.UpsertFile(context.Background(), racing))

	_, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath: "/library/movie.mkv", BytesAfter: 100, Encoder: "cpu",
	})
	require.NoError(t, err)

	files, err := h.st.ListFiles(context.Background(), f.LibraryID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, f.ID, files[0].ID)
	require.Equal(t, "/library/movie.mkv", files[0].Path)
}

func TestCompleteInspectFailureFailsFile(t *testing.T) {
	h := newHarness(t)
	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return ResultInfo{}, context.DeadlineExceeded
	}, "")
	_, f, job := claimJob(t, h, "node-1")

	_, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath: "/library/movie.mkv", BytesAfter: 100, Encoder: "cpu",
	})
	require.NoError(t, err) // the job is done; the file records the problem

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusFailed, after.Status)
	require.Contains(t, after.SatisfiedReason, "output could not be verified")
}

func TestFailMarksJobAndFile(t *testing.T) {
	h := newHarness(t)
	_, f, job := claimJob(t, h, "node-1")

	huge := strings.Repeat("x", 200<<10)
	got, err := h.svc.Queue().Fail(context.Background(), job.ID, "node-1", FailParams{
		Error: "encode failed", LogTail: huge,
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusFailed, got.Status)
	require.Equal(t, "encode failed", got.Error)
	require.Equal(t, MaxLogTail, len(got.Log)) // capped tail
	require.True(t, strings.HasSuffix(got.Log, huge[len(huge)-MaxLogTail:]))

	after, err := h.st.GetFile(context.Background(), f.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusFailed, after.Status)

	// Retry moves both back to the queue.
	retried, err := h.svc.Queue().Retry(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusQueued, retried.Status)
}

func TestFailRejectsForeignJobs(t *testing.T) {
	h := newHarness(t)
	_, _, job := claimJob(t, h, "node-1")
	_, err := h.svc.Queue().Fail(context.Background(), job.ID, "node-2", FailParams{Error: "boom"})
	require.ErrorIs(t, err, ErrJobNotClaimed)
}
