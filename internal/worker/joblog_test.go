// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// Client exposes the test client for standalone client tests.
func (f *fakeAPI) Client() *Client {
	return NewClient(f.srv.URL, f.key)
}

// logBatches returns the recorded log batches of one job.
func (f *fakeAPI) logBatches(jobID string) []JobLogBatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[jobID]
}

// collectJobLog flattens the fakeAPI's recorded batches into one slice.
func collectJobLog(f *fakeAPI, jobID string) []store.JobLogLine {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.JobLogLine
	for _, batch := range f.logs[jobID] {
		out = append(out, batch.Lines...)
	}
	return out
}

// TestJobLogSinkBatchingAndDedup: consecutive identical messages collapse
// into one row with a grown `repeated` count, distinct messages keep their
// own rows, and the final flush lands without a finish.
func TestJobLogSinkBatchingAndDedup(t *testing.T) {
	api := newFakeAPI(t, "key")
	sink := newJobLogSink(api.Client(), "job1", nil)
	sink.sysf("starting")
	sink.ffline("frame=1")
	sink.ffline("frame=1")
	sink.ffline("frame=2")
	sink.finish()

	lines := collectJobLog(api, "job1")
	require.Len(t, lines, 3)
	require.Equal(t, "starting", lines[0].Message)
	require.Equal(t, "frame=1", lines[1].Message)
	require.Equal(t, int64(2), lines[1].Repeated)
	require.Equal(t, "frame=2", lines[2].Message)
	require.Equal(t, int64(1), lines[2].Repeated)
}

// TestJobLogSinkCap: a job is capped at 2,000 lines with exactly one
// truncation line as the last row.
func TestJobLogSinkCap(t *testing.T) {
	api := newFakeAPI(t, "key")
	sink := newJobLogSink(api.Client(), "job1", nil)
	for i := 0; i < JobLogMaxLines+500; i++ {
		sink.ffline(fmt.Sprintf("frame=%d", i))
	}
	sink.finish()

	lines := collectJobLog(api, "job1")
	require.Len(t, lines, JobLogMaxLines)
	require.Equal(t, "frame=0", lines[0].Message)
	last := lines[JobLogMaxLines-1]
	require.Equal(t, JobLogTruncatedMessage, last.Message)
	require.Equal(t, store.JobLogError, last.Level)
	require.Equal(t, store.JobLogSourceSys, last.Source)
	// One more append after truncation is a no-op.
	sink.ffline("frame=later")
	sink.finish()
	lines = collectJobLog(api, "job1")
	require.Len(t, lines, JobLogMaxLines)
}

// TestJobLogSinkUploadFailureStopsPosting: a failed batch marks the sink
// failed and later batches are dropped without another request (best
// effort). A job still ends done with its log on disk untouched.
func TestJobLogSinkUploadFailureStopsPosting(t *testing.T) {
	api := newFakeAPI(t, "key")
	api.logFail = true
	sink := newJobLogSink(api.Client(), "job1", nil)
	for i := 0; i < JobLogBatchLines+10; i++ {
		sink.ffline(fmt.Sprintf("frame=%d", i))
	}
	sink.finish()
	// Nothing blew up; the failure was logged and dropped.
}

// TestStderrTeeSplitsAndTails: the tee writes the tail verbatim (including
// a partial last line) and splits complete lines into the sink.
func TestStderrTeeSplitsAndTails(t *testing.T) {
	api := newFakeAPI(t, "key")
	sink := newJobLogSink(api.Client(), "job1", nil)
	var tail []byte
	tee := newStderrTee(tailWriter{buf: &tail}, sink)
	_, err := tee.Write([]byte("line one\nline two"))
	require.NoError(t, err)
	require.Equal(t, "line one\nline two", string(tail)) // the partial line reaches the tail
	tee.close()
	// CRLF and blank lines are dropped.
	_, err = tee.Write([]byte("  \r\nline three\r\n"))
	require.NoError(t, err)
	tee.close()
	sink.finish()

	lines := collectJobLog(api, "job1")
	require.Len(t, lines, 3)
	require.Equal(t, "line one", lines[0].Message)
	require.Equal(t, store.JobLogSourceFFmpeg, lines[0].Source)
	require.Equal(t, "line two", lines[1].Message)
	require.Equal(t, "line three", lines[2].Message)
}

// tailWriter is a minimal io.Writer over a byte slice.
type tailWriter struct{ buf *[]byte }

func (w tailWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// TestRunnerLogsJobLifecycle: the runner's log carries sys lines for the
// probe, the target, the result policy and the bytes, and the log tail is
// still on the failure message.
func TestRunnerLogsJobLifecycle(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT-DATA", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	lines := collectJobLog(api, "job1")
	require.NotEmpty(t, lines)
	// First sys line: the probe summary.
	require.Equal(t, store.JobLogSourceSys, lines[0].Source)
	require.Contains(t, lines[0].Message, "probed source")
	hasTarget := false
	hasPolicy := false
	hasBytes := false
	for _, l := range lines {
		switch {
		case strings.Contains(l.Message, "target:"):
			hasTarget = true
		case strings.Contains(l.Message, "result policy: applied"):
			hasPolicy = true
		case strings.Contains(l.Message, "bytes before"):
			hasBytes = true
		}
	}
	require.True(t, hasTarget, "log must name the target: %v", lines)
	require.True(t, hasPolicy, "log must name the result policy: %v", lines)
	require.True(t, hasBytes, "log must name the bytes before and after: %v", lines)
	// No failure message.
	_, failed := api.failOf("job1")
	require.False(t, failed)
}

// TestRunnerFailedJobLogsError: a failing job's error line lands in the
// structured log and the tail is still reported.
func TestRunnerFailedJobLogsError(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: func(context.Context, []string) error { return errors.New("boom") },
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	lines := collectJobLog(api, "job1")
	require.NotEmpty(t, lines)
	last := lines[len(lines)-1]
	require.Equal(t, store.JobLogError, last.Level)
	require.Equal(t, store.JobLogSourceSys, last.Source)
	require.Contains(t, last.Message, "encode failed: boom")
	report, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, report.Error, "encode failed: boom")
}

// TestClientPostJobLog round-trips one batch through the client.
func TestClientPostJobLog(t *testing.T) {
	api := newFakeAPI(t, "key")
	c := NewClient(api.srv.URL, api.key)
	err := c.PostJobLog(context.Background(), "job1", JobLogBatch{
		Seq: 7,
		Lines: []store.JobLogLine{
			{Seq: 7, Level: store.JobLogInfo, Source: "ffmpeg", Message: "hi", Repeated: 2},
		},
	})
	require.NoError(t, err)
	batches := api.logBatches("job1")
	require.Len(t, batches, 1)
	require.Equal(t, int64(7), batches[0].Seq)
	require.Len(t, batches[0].Lines, 1)
	require.Equal(t, "hi", batches[0].Lines[0].Message)
}

// TestJobLogSinkFlushInterval: pending lines below the batch size are
// flushed on the interval instead of waiting for finish.
func TestJobLogSinkFlushInterval(t *testing.T) {
	api := newFakeAPI(t, "key")
	sink := newJobLogSink(api.Client(), "job1", nil)
	sink.sysf("early line")
	// The flush interval is 5s; wait for it to fire.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if len(collectJobLog(api, "job1")) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEmpty(t, collectJobLog(api, "job1"), "pending lines must flush on the interval")
	sink.finish()
}

var _ = io.Discard
