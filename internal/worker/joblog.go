// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Job log capture (task 27): the worker streams its job's complete log —
// ffmpeg stderr lines plus its own sys lines — to the server in batches,
// where they are stored as rows in SQLite. The upload is best effort: a
// failed batch is logged at debug and dropped, and never fails the job.
const (
	// JobLogMaxLines is the per-job line cap (task 27). The sink stops
	// accepting content at 1999 lines and appends one final error line
	// ("log truncated at 2000 lines"), so a job's log never exceeds 2000
	// rows. The store enforces the same cap on append.
	JobLogMaxLines = 2000
	// JobLogBatchLines flushes a batch when this many lines accumulate.
	JobLogBatchLines = 200
	// JobLogFlushInterval is how often the sink flushes pending lines even
	// when the batch size has not filled.
	JobLogFlushInterval = 5 * time.Second
	// JobLogMaxMessageBytes caps a single message; a longer one is cut and
	// marked with JobLogTruncatedSuffix.
	JobLogMaxMessageBytes = 4096
	// JobLogTruncatedSuffix marks a message that was cut at the cap.
	JobLogTruncatedSuffix = " [truncated]"
	// JobLogTruncatedMessage is the one error line appended when the cap is
	// reached; it names the cap the task defines.
	JobLogTruncatedMessage = "log truncated at 2000 lines"
	// jobLogQueueDepth bounds the batches waiting for the uploader; beyond
	// it the oldest-not-yet-sent batch is dropped (best effort).
	jobLogQueueDepth = 16
)

// jobLogSink collects one job's log lines and uploads them in batches.
// Appends come from several goroutines (the ffmpeg pipe writer, the runner)
// and are serialized under mu; the upload runs on one goroutine so a slow
// server never blocks an encode. Consecutive identical messages collapse
// into one line with a grown `repeated` count (the last line of a pending
// batch), which is what stops an ffmpeg warning loop from eating the table.
type jobLogSink struct {
	client *Client
	jobID  string
	log    *slog.Logger

	mu        sync.Mutex
	seq       int64 // next line's chunk order
	pending   []store.JobLogLine
	count     int // accepted rows, capped at JobLogMaxLines
	truncated bool
	failed    bool // an upload failed; later batches are dropped

	ch   chan []store.JobLogLine
	stop chan struct{}
	done chan struct{}
}

// newJobLogSink starts a sink and its upload goroutine. finish stops both.
func newJobLogSink(client *Client, jobID string, log *slog.Logger) *jobLogSink {
	if log == nil {
		log = slog.Default()
	}
	s := &jobLogSink{
		client: client,
		jobID:  jobID,
		log:    log,
		ch:     make(chan []store.JobLogLine, jobLogQueueDepth),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go s.loop()
	return s
}

// append records one line. The cap check runs before the dedupe: once the
// job is truncated the marker line is the last thing accepted.
func (s *jobLogSink) append(level, source, message string) {
	s.mu.Lock()
	if s.truncated {
		s.mu.Unlock()
		return
	}
	// Consecutive identical messages collapse into the last pending line.
	if n := len(s.pending); n > 0 {
		tail := &s.pending[n-1]
		if tail.Level == level && tail.Source == source && tail.Message == message {
			tail.Repeated++
			s.mu.Unlock()
			return
		}
	}
	s.seq++
	line := store.JobLogLine{
		Seq:      s.seq,
		Level:    level,
		Source:   source,
		Message:  message,
		Repeated: 1,
	}
	if len(line.Message) > JobLogMaxMessageBytes {
		line.Message = line.Message[:JobLogMaxMessageBytes] + JobLogTruncatedSuffix
	}
	// One final row is reserved for the truncation marker, so the job's log
	// never exceeds JobLogMaxLines rows.
	if s.count >= JobLogMaxLines-1 {
		s.truncated = true
		line = store.JobLogLine{Seq: s.seq, Level: store.JobLogError,
			Source: store.JobLogSourceSys, Message: JobLogTruncatedMessage, Repeated: 1}
	}
	s.pending = append(s.pending, line)
	s.count++
	if len(s.pending) < JobLogBatchLines {
		s.mu.Unlock()
		return
	}
	batch := s.pending
	s.pending = nil
	s.mu.Unlock()
	s.putBatch(batch)
}

// putBatch queues a batch for the upload goroutine; a full queue drops it
// (the upload is best effort).
func (s *jobLogSink) putBatch(batch []store.JobLogLine) {
	select {
	case s.ch <- batch:
	default:
		s.log.Debug("worker: job log batch dropped (uploader behind)",
			"job", s.jobID, "lines", len(batch))
	}
}

// sysf appends a sys line at info level.
func (s *jobLogSink) sysf(format string, args ...any) {
	s.append(store.JobLogInfo, store.JobLogSourceSys, fmt.Sprintf(format, args...))
}

// warnf appends a sys line at warn level.
func (s *jobLogSink) warnf(format string, args ...any) {
	s.append(store.JobLogWarn, store.JobLogSourceSys, fmt.Sprintf(format, args...))
}

// errorf appends a sys line at error level.
func (s *jobLogSink) errorf(format string, args ...any) {
	s.append(store.JobLogError, store.JobLogSourceSys, fmt.Sprintf(format, args...))
}

// ffline appends one ffmpeg stderr line.
func (s *jobLogSink) ffline(line string) {
	s.append(store.JobLogInfo, store.JobLogSourceFFmpeg, line)
}

// pluginf appends one plugin output line (task 32, step plugins run on the
// node too): the plugin's name leads the message, stdout at info.
func (s *jobLogSink) pluginInfof(name, format string, args ...any) {
	s.append(store.JobLogInfo, store.JobLogSourcePlugin,
		fmt.Sprintf("%s: %s", name, fmt.Sprintf(format, args...)))
}

// pluginWarnf appends one plugin output line at warn level (stderr).
func (s *jobLogSink) pluginWarnf(name, format string, args ...any) {
	s.append(store.JobLogWarn, store.JobLogSourcePlugin,
		fmt.Sprintf("%s: %s", name, fmt.Sprintf(format, args...)))
}

// loop uploads batches as they arrive and flushes pending lines on the
// interval. On stop it drains everything still queued or pending.
func (s *jobLogSink) loop() {
	defer close(s.done)
	ctx := context.WithoutCancel(context.Background())
	ticker := time.NewTicker(JobLogFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case batch := <-s.ch:
			s.post(ctx, batch)
		case <-ticker.C:
			s.flushPending()
		case <-s.stop:
			s.drain()
			return
		}
	}
}

// flushPending snapshots and queues whatever has accumulated.
func (s *jobLogSink) flushPending() {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.pending
	s.pending = nil
	s.mu.Unlock()
	s.putBatch(batch)
}

// drain posts everything still queued or pending (the final flush on
// completion and failure).
func (s *jobLogSink) drain() {
	ctx := context.WithoutCancel(context.Background())
	for {
		select {
		case batch := <-s.ch:
			s.post(ctx, batch)
			continue
		default:
		}
		break
	}
	s.flushPending()
	// One queued batch may have been enqueued between the default and the
	// flush; give it a bounded chance to be picked up.
	select {
	case batch := <-s.ch:
		s.post(ctx, batch)
	default:
	}
}

// post uploads one batch. A failure marks the sink failed: later batches
// are dropped without another request (best effort, never fatal).
func (s *jobLogSink) post(ctx context.Context, batch []store.JobLogLine) {
	if len(batch) == 0 {
		return
	}
	s.mu.Lock()
	if s.failed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	err := s.client.PostJobLog(ctx, s.jobID, JobLogBatch{Seq: batch[0].Seq, Lines: batch})
	if err != nil {
		s.mu.Lock()
		s.failed = true
		s.mu.Unlock()
		s.log.Debug("worker: job log upload failed; dropping the rest", "job", s.jobID, "err", err)
	}
}

// finish stops the sink and waits for the final flush. Idempotent.
func (s *jobLogSink) finish() {
	select {
	case <-s.stop:
		return
	default:
	}
	close(s.stop)
	<-s.done
}

// stderrTee writes the ffmpeg stderr to the 64 KiB tail unchanged and, in
// parallel, splits it into lines for the structured log. One writer per
// encode attempt; written only by the pipe goroutine. A nil sink keeps the
// tail working for tests that do not capture a structured log.
type stderrTee struct {
	tail io.Writer
	sink *jobLogSink
	part []byte
}

func newStderrTee(tail io.Writer, sink *jobLogSink) *stderrTee {
	return &stderrTee{tail: tail, sink: sink}
}

func (w *stderrTee) Write(p []byte) (int, error) {
	// The tail must keep every byte, even a partial last line.
	if _, err := w.tail.Write(p); err != nil {
		return 0, err
	}
	if w.sink == nil {
		return len(p), nil
	}
	w.part = append(w.part, p...)
	for {
		i := bytes.IndexByte(w.part, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.part[:i]), "\r")
		w.part = w.part[i+1:]
		if strings.TrimSpace(line) != "" {
			w.sink.ffline(line)
		}
	}
	return len(p), nil
}

// close flushes a trailing line that arrived without a newline.
func (w *stderrTee) close() {
	if w.sink == nil || len(w.part) == 0 {
		return
	}
	line := strings.TrimRight(string(w.part), "\r")
	w.part = nil
	if strings.TrimSpace(line) != "" {
		w.sink.ffline(line)
	}
}
