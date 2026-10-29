// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// failReport is a recorded /fail call.
type failReport struct {
	Error   string `json:"error"`
	LogTail string `json:"log_tail"`
}

// fakeAPI is an in-memory worker API server recording every call.
type fakeAPI struct {
	t   *testing.T
	key string

	mu           sync.Mutex
	queue        []string
	specs        map[string]*server.JobSpec
	progress     []ProgressInput
	progressJobs []string
	statuses     map[string]string // job id -> status to answer with
	complete     map[string]CompleteInput
	fails        map[string]failReport
	heartbeats   []HeartbeatInput
	concurrency  int
	authFailures int
	srv          *httptest.Server
}

func newFakeAPI(t *testing.T, key string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{
		t:        t,
		key:      key,
		specs:    map[string]*server.JobSpec{},
		statuses: map[string]string{},
		complete: map[string]CompleteInput{},
		fails:    map[string]failReport{},
	}
	mux := http.NewServeMux()
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != f.key {
				f.mu.Lock()
				f.authFailures++
				f.mu.Unlock()
				writeFakeError(w, http.StatusUnauthorized, "unauthorized", "bad key")
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("POST /api/v1/worker/heartbeat", guard(func(w http.ResponseWriter, r *http.Request) {
		var in HeartbeatInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.heartbeats = append(f.heartbeats, in)
		c := f.concurrency
		f.mu.Unlock()
		if c < 1 {
			c = 1
		}
		writeFakeJSON(w, http.StatusOK, map[string]int{"concurrency": c})
	}))
	mux.HandleFunc("POST /api/v1/worker/claim", guard(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		var id string
		if len(f.queue) > 0 {
			id = f.queue[0]
			f.queue = f.queue[1:]
		}
		f.mu.Unlock()
		if id == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeFakeJSON(w, http.StatusOK, ClaimedJob{ID: id})
	}))
	mux.HandleFunc("GET /api/v1/worker/jobs/{id}", guard(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		spec, ok := f.specs[r.PathValue("id")]
		f.mu.Unlock()
		if !ok {
			writeFakeError(w, http.StatusNotFound, "not_found", "no such job")
			return
		}
		writeFakeJSON(w, http.StatusOK, spec)
	}))
	mux.HandleFunc("POST /api/v1/worker/jobs/{id}/progress", guard(func(w http.ResponseWriter, r *http.Request) {
		var in ProgressInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		id := r.PathValue("id")
		f.mu.Lock()
		f.progress = append(f.progress, in)
		f.progressJobs = append(f.progressJobs, id)
		status := f.statuses[id]
		f.mu.Unlock()
		if status == "" {
			status = store.JobStatusRunning
		}
		writeFakeJSON(w, http.StatusOK, map[string]string{"status": status})
	}))
	mux.HandleFunc("POST /api/v1/worker/jobs/{id}/complete", guard(func(w http.ResponseWriter, r *http.Request) {
		var in CompleteInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.complete[r.PathValue("id")] = in
		f.mu.Unlock()
		writeFakeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/v1/worker/jobs/{id}/fail", guard(func(w http.ResponseWriter, r *http.Request) {
		var in failReport
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.fails[r.PathValue("id")] = in
		f.mu.Unlock()
		writeFakeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) enqueue(id string, spec *server.JobSpec) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs[id] = spec
	f.queue = append(f.queue, id)
}

func (f *fakeAPI) setStatus(jobID, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[jobID] = status
}

func (f *fakeAPI) completeOf(jobID string) CompleteInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.complete[jobID]
}

func (f *fakeAPI) failOf(jobID string) (failReport, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.fails[jobID]
	return r, ok
}

func (f *fakeAPI) heartbeatHadRunningJob(jobID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, hb := range f.heartbeats {
		for _, id := range hb.RunningJobIDs {
			if id == jobID {
				return true
			}
		}
	}
	return false
}

func writeFakeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeFakeError(w http.ResponseWriter, status int, code, message string) {
	writeFakeJSON(w, status, map[string]string{"error": message, "code": code})
}

// sourceFixture is a real file on disk standing in for the library source.
type sourceFixture struct {
	Path        string
	Size        int64
	Mtime       int64
	Fingerprint string
}

func writeSource(t *testing.T, dir, name, content string) sourceFixture {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	st, err := os.Stat(path)
	require.NoError(t, err)
	fp, err := probe.Fingerprint(path)
	require.NoError(t, err)
	return sourceFixture{Path: path, Size: st.Size(), Mtime: st.ModTime().Unix(), Fingerprint: fp}
}

// srcInfoF is the fake probe of the source: h264, 2 audio, 1 subtitle, 10s.
// It returns a fresh value per call: callers that tweak one stream must not
// corrupt the fixture for other tests.
func srcInfoF() probe.MediaInfo {
	return probe.MediaInfo{
		Container:       flow.ContainerMKV,
		DurationSeconds: 10,
		Video:           []probe.VideoStream{{Codec: flow.CodecH264, Width: 160, Height: 120}},
		Audio:           []probe.AudioStream{{Codec: "eac3", Channels: 6}, {Codec: "aac", Channels: 2}},
		Subtitle:        []probe.SubtitleStream{{Codec: "subrip", TextBased: true}},
	}
}

// outInfoF is the fake probe of a correct output.
func outInfoF() probe.MediaInfo {
	return probe.MediaInfo{
		Container:       flow.ContainerMKV,
		DurationSeconds: 10,
		Video:           []probe.VideoStream{{Codec: flow.CodecHEVC, Width: 160, Height: 120}},
		Audio:           []probe.AudioStream{{Codec: "eac3", Channels: 6}, {Codec: "aac", Channels: 2}},
		Subtitle:        []probe.SubtitleStream{{Codec: "subrip", TextBased: true}},
	}
}

// proberByBase fakes ffprobe per file base name.
func proberByBase(byBase map[string]probe.MediaInfo) Prober {
	return func(_ context.Context, _ string, path string) (probe.MediaInfo, error) {
		info, ok := byBase[filepath.Base(path)]
		if !ok {
			return probe.MediaInfo{}, fmt.Errorf("no fake probe for %s", path)
		}
		return info, nil
	}
}

// writeRun is a RunFn that writes a fixed payload to the output (last arg).
func writeRun(payload string, seen *[]string) encoders.RunFn {
	return func(_ context.Context, args []string) error {
		if seen != nil {
			*seen = append(*seen, strings.Join(args, " "))
		}
		return os.WriteFile(args[len(args)-1], []byte(payload), 0o644)
	}
}

type runnerOpts struct {
	byBase   map[string]probe.MediaInfo
	run      encoders.RunFn
	encs     []encoders.Encoder
	progress time.Duration
}

func newRunner(t *testing.T, api *fakeAPI, scratchDir string, opts runnerOpts) *runner {
	t.Helper()
	if opts.progress <= 0 {
		opts.progress = 20 * time.Millisecond
	}
	cfg := Config{
		ProgressInterval:  opts.progress,
		ClaimPollInterval: time.Millisecond,
		HeartbeatInterval: time.Hour,
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	prober := opts.byBase
	if prober == nil {
		prober = map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": outInfoF()}
	}
	return &runner{
		cfg:     cfg,
		client:  NewClient(api.srv.URL, api.key),
		caps:    encoders.Capability{Encoders: []encoders.EncoderInfo{{Name: "cpu", Codecs: []string{"hevc", "h264", "av1"}}}},
		encs:    opts.encs,
		prober:  proberByBase(prober),
		runFn:   opts.run,
		sc:      newScratch(scratchDir),
		log:     cfg.Log,
		tracker: &encoderTracker{},
	}
}

// defaultSpec builds a valid spec over a real source file.
func defaultSpec(t *testing.T, dir, jobID string) (sourceFixture, *server.JobSpec) {
	t.Helper()
	src := writeSource(t, dir, "movie.mp4", "SOURCE-CONTENT")
	return src, &server.JobSpec{
		ID: jobID, FileID: "file-1", LibraryID: "lib-1",
		Target:       flow.Default(),
		SourcePath:   src.Path,
		SourceSize:   src.Size,
		SourceMtime:  src.Mtime,
		Fingerprint:  src.Fingerprint,
		AccessMode:   store.NodeAccessShared,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
		BytesBefore:  src.Size,
	}
}

func TestRunnerHappyPathReplacesOriginal(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT-DATA", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, filepath.Join(dir, "movie.mkv"), in.OutputPath)
	require.Equal(t, "cpu", in.Encoder)
	require.Empty(t, in.FallbackReasons)
	require.Empty(t, in.SkippedReason)
	// LogTail comes from the ffmpeg stderr pipe; the Run override bypasses
	// it, so the integration test covers non-empty tails.

	// The original was replaced with the target container extension.
	out, err := os.ReadFile(in.OutputPath)
	require.NoError(t, err)
	require.Equal(t, "OUTPUT-DATA", string(out))
	_, err = os.Stat(src.Path)
	require.True(t, os.IsNotExist(err), "original with old extension must be gone")

	// Scratch is cleaned after success.
	_, err = os.Stat(filepath.Join(dir, "scratch", "job-job1"))
	require.True(t, os.IsNotExist(err))
}

// newCancelFlag returns a fresh cancellation flag for runner.run.
func newCancelFlag() *atomic.Bool { return &atomic.Bool{} }

func TestRunnerFailsOnVerificationAndKeepsOriginal(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	// The fake encoder produces a file whose probe says h264 (wrong codec).
	bad := outInfoF()
	bad.Video[0].Codec = flow.CodecH264
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:    writeRun("GARBAGE-LOOKS-VALID", nil),
		byBase: map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": bad},
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	rep, ok := api.failOf("job1")
	require.True(t, ok, "job must be reported failed")
	require.Contains(t, rep.Error, "output video is h264")

	// The original is untouched and the scratch output never left scratch.
	content, err := os.ReadFile(src.Path)
	require.NoError(t, err)
	require.Equal(t, "SOURCE-CONTENT", string(content))
	_, err = os.Stat(filepath.Join(dir, "movie.mkv"))
	require.True(t, os.IsNotExist(err))
}

func TestRunnerReportsEncodeFailure(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: func(context.Context, []string) error { return errors.New("boom") },
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "encode failed")
	require.Contains(t, rep.Error, "boom")
}

func TestRunnerFailsOnFingerprintMismatch(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	spec.Fingerprint = "1:1:deadbeef" // queued, then the file changed
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("x", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "source changed")
	_ = src
}

func TestRunnerFailsOnTransferMode(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	spec.AccessMode = store.NodeAccessTransfer
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("x", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "task 09")
}

func TestRunnerFallbackReasonsRecorded(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	seen := &[]string{}
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		encs: []encoders.Encoder{fakeGPU{}, encoders.CPU},
		run: func(_ context.Context, args []string) error {
			joined := strings.Join(args, " ")
			*seen = append(*seen, joined)
			if strings.Contains(joined, "fakegpu_codec") {
				return errors.New("fakegpu exploded")
			}
			return os.WriteFile(args[len(args)-1], []byte("OUTPUT"), 0o644)
		},
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, "cpu", in.Encoder)
	require.Equal(t, []string{"fakegpu: fakegpu exploded"}, in.FallbackReasons)
	require.Len(t, *seen, 2) // gpu attempt, then cpu
}

// fakeGPU is a stand-in GPU backend whose args the fake run can recognize.
type fakeGPU struct{}

func (fakeGPU) Name() string                   { return "fakegpu" }
func (fakeGPU) Codecs() []string               { return []string{"hevc"} }
func (fakeGPU) SelfTest(context.Context) error { return nil }
func (fakeGPU) Detect(context.Context, string) (encoders.Capability, error) {
	return encoders.Capability{Encoders: []encoders.EncoderInfo{{Name: "fakegpu", Codecs: []string{"hevc"}}}}, nil
}
func (fakeGPU) BuildArgs(spec encoders.Spec) []string {
	return []string{"-c:v", "fakegpu_codec", "-i", spec.Input, spec.Output}
}

func TestRunnerCancelViaProgressStatus(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)
	api.setStatus("job1", store.JobStatusCancelled)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: func(ctx context.Context, _ []string) error {
			<-ctx.Done() // encode runs until the cancel kills it
			return ctx.Err()
		},
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	// Nothing was reported: the server cancelled the job itself.
	_, reported := api.failOf("job1")
	require.False(t, reported)
	require.Empty(t, api.completeOf("job1").Encoder)
	// The status poll ran and observed the cancellation.
	require.Contains(t, api.progressStatusDebug(), "job1")
	// Scratch is cleaned after a cancel, too.
	_, err := os.Stat(filepath.Join(dir, "scratch", "job-job1"))
	require.True(t, os.IsNotExist(err))
}

func TestRunnerSkipNotSmaller(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	src, spec := defaultSpec(t, dir, "job1")
	api.enqueue("job1", spec)

	// The output is bigger than the source: replace_if_smaller skips.
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun(strings.Repeat("X", int(src.Size)+10), nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	in := api.completeOf("job1")
	require.Equal(t, spec.BytesBefore, in.BytesAfter)
	require.NotEmpty(t, in.SkippedReason)
	require.Empty(t, in.OutputPath)

	// Original kept.
	content, err := os.ReadFile(src.Path)
	require.NoError(t, err)
	require.Equal(t, "SOURCE-CONTENT", string(content))
}

// progressStatusDebug helps asserts read recorded state without races.
func (f *fakeAPI) progressStatusDebug() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.progressJobs
}
