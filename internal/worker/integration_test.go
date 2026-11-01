// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/api"
	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
	"github.com/stretchr/testify/require"
)

// stackConfig tunes the stack for one test.
type stackConfig struct {
	heartbeatTimeout time.Duration // 0 → default (irrelevant for most tests)
	sweepInterval    time.Duration // 0 → 30s default
}

// stack is the full role-both composition: store, server services, admin
// API on a real HTTP listener, built-in node and an in-process worker —
// what cmd/shrinkyourshit serve wires, minus flag parsing.
type stack struct {
	t        *testing.T
	st       store.Store
	svc      *server.Server
	apiSrv   *httptest.Server
	dataDir  string
	library  string
	scratch  string
	key      string
	fixtures testutil.Fixtures
	ffmpeg   string
}

func newStack(t *testing.T, cfg stackConfig) *stack {
	t.Helper()
	ffmpeg, _ := testutil.RequireFFmpeg(t)

	dataDir := t.TempDir()
	library := t.TempDir()
	scratch := filepath.Join(dataDir, "scratch")

	st, err := store.New(filepath.Join(dataDir, "shrinkyourshit.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	svcCfg := server.Config{
		Store:             st,
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		DataDir:           dataDir,
		StableWindow:      200 * time.Millisecond,
		Debounce:          200 * time.Millisecond,
		WatchPollInterval: time.Hour,
	}
	if cfg.heartbeatTimeout > 0 {
		svcCfg.HeartbeatTimeout = cfg.heartbeatTimeout
		svcCfg.SweepInterval = cfg.sweepInterval
	}
	svc := server.New(svcCfg)

	ctx := context.Background()
	key, builtin, err := svc.Nodes().EnsureBuiltinNode(ctx, dataDir, server.BuiltinNodeName("testhost"))
	require.NoError(t, err)

	// The pipeline tests below exercise a started node (task 11: a fresh
	// install starts stopped). The lifecycle tests control the state
	// explicitly — stop or pair fresh nodes as needed.
	_, err = svc.Nodes().Start(ctx, builtin.ID)
	require.NoError(t, err)

	svc.Start(ctx)
	t.Cleanup(svc.Stop)

	static := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>test</html>")}}
	apiSrv := httptest.NewServer(api.New("v0.0.0", static, svc))
	t.Cleanup(apiSrv.Close)

	s := &stack{
		t: t, st: st, svc: svc, apiSrv: apiSrv,
		dataDir: dataDir, library: library, scratch: scratch,
		key: key, fixtures: testutil.GenerateFixtures(t, ffmpeg, t.TempDir()),
		ffmpeg: ffmpeg,
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	return s
}

// createLibrary adds a library over the fixtures with the given policy.
func (s *stack) createLibrary(policy string) *store.Library {
	return s.createLibraryTarget(flow.Default(), policy)
}

// createLibraryTarget adds a library with an explicit Target. Quality 1
// keeps outputs clearly smaller than the tiny synthetic fixtures.
func (s *stack) createLibraryTarget(target flow.Target, policy string) *store.Library {
	s.t.Helper()
	require.NoError(s.t, target.Validate())
	raw, err := json.Marshal(target)
	require.NoError(s.t, err)
	lib, err := s.svc.Libraries().Create(context.Background(), server.LibraryParams{
		Name: "Movies", Path: s.library, TargetJSON: raw, ResultPolicy: policy,
	})
	require.NoError(s.t, err)
	return lib
}

// copyFixtureIn places a fixture into the library folder under name.
func (s *stack) copyFixtureIn(src, name string) string {
	s.t.Helper()
	dst := filepath.Join(s.library, name)
	data, err := os.ReadFile(src)
	require.NoError(s.t, err)
	require.NoError(s.t, os.WriteFile(dst, data, 0o644))
	return dst
}

// fileHash reads a file for byte-comparison.
func (s *stack) fileHash(path string) []byte {
	s.t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(s.t, err)
	return data
}

// workerConfig builds a worker config against this stack; tweak adjusts it.
func (s *stack) workerConfig(tweak func(*Config)) Config {
	cfg := Config{
		ServerURL:         s.apiSrv.URL,
		Key:               s.key,
		NodeName:          "testhost", // the builtin node's name (task 32 error messages)
		DataDir:           s.dataDir,
		ScratchDir:        s.scratch,
		FFmpegPath:        s.ffmpeg,
		HeartbeatInterval: 200 * time.Millisecond,
		ProgressInterval:  200 * time.Millisecond,
		ClaimPollInterval: 100 * time.Millisecond,
		MaxSlots:          2,
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if tweak != nil {
		tweak(&cfg)
	}
	return cfg
}

// runWorker starts a worker on the given context.
func (s *stack) runWorker(ctx context.Context, cfg Config) {
	w, err := New(cfg)
	require.NoError(s.t, err)
	go func() { _ = w.Run(ctx) }()
}

// waitFor polls until cond holds or the test times out.
func (s *stack) waitFor(what string, cond func() bool) {
	s.t.Helper()
	s.waitForDesc(what, cond, nil)
}

// waitForDesc is waitFor with a state dump when it times out.
func (s *stack) waitForDesc(what string, cond func() bool, describe func() string) {
	s.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if describe != nil {
		s.t.Logf("state while waiting for %s: %s", what, describe())
	}
	s.t.Fatalf("timed out waiting for %s", what)
}

// startNode starts a node through the lifecycle endpoint (task 11), the
// way the admin's Start click does.
func (s *stack) startNode(nodeID string) {
	s.t.Helper()
	resp, err := http.Post(s.apiSrv.URL+"/api/v1/nodes/"+nodeID+"/start", "application/json", nil)
	require.NoError(s.t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(s.t, http.StatusOK, resp.StatusCode, "start %s", nodeID)
}

// stopNode stops a node through the lifecycle endpoint with the given
// drain flag.
func (s *stack) stopNode(nodeID string, drain bool) {
	s.t.Helper()
	body := fmt.Sprintf(`{"drain": %t}`, drain)
	resp, err := http.Post(s.apiSrv.URL+"/api/v1/nodes/"+nodeID+"/stop", "application/json", strings.NewReader(body))
	require.NoError(s.t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(s.t, http.StatusOK, resp.StatusCode, "stop %s", nodeID)
}

// nodeRow returns the node row for an id.
func (s *stack) nodeRow(nodeID string) *store.Node {
	s.t.Helper()
	node, err := s.st.GetNode(context.Background(), nodeID)
	require.NoError(s.t, err)
	return node
}

// jobFor returns the (single) job of the library.
func (s *stack) jobFor(libID string) *store.Job {
	s.t.Helper()
	jobs, err := s.st.ListJobs(context.Background(), store.JobFilter{LibraryID: libID})
	require.NoError(s.t, err)
	if len(jobs) == 0 {
		return nil
	}
	return jobs[0]
}

// jobLog returns the job's log text (all lines joined) — the plugin's
// stdout lines land here (task 27's viewer, task 32's plugin source).
func (s *stack) jobLog(libID string) string {
	s.t.Helper()
	job := s.jobFor(libID)
	require.NotNil(s.t, job)
	lines, err := s.st.ListJobLog(context.Background(), job.ID, 0, "", 200)
	require.NoError(s.t, err)
	out := ""
	for _, l := range lines {
		out += l.Message + "\n"
	}
	return out
}

// fileRow returns the (single) file of the library.
func (s *stack) fileRow(libID string) *store.File {
	s.t.Helper()
	files, err := s.st.ListFiles(context.Background(), libID)
	require.NoError(s.t, err)
	require.Len(s.t, files, 1)
	return files[0]
}

// scanSync runs a synchronous scan.
func (s *stack) scanSync(libID string) server.ScanResult {
	s.t.Helper()
	res, err := s.svc.Scanner().ScanLibrary(context.Background(), libID)
	require.NoError(s.t, err)
	return res
}

// adminCancel cancels a job through the admin API (as the UI would).
func (s *stack) adminCancel(jobID string) int {
	resp, err := http.Post(s.apiSrv.URL+"/api/v1/jobs/"+jobID+"/cancel", "application/json", nil)
	require.NoError(s.t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// writeFakeFFmpeg installs a shell script posing as ffmpeg. The script
// answers -version (used by the capabilities payload) and applies action to
// the last argument (the output path) for encodes.
func writeFakeFFmpeg(t *testing.T, dir, action string) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
# Fake ffmpeg for tests: %s
for last; do :; done
case "$1" in
  -version) echo "ffmpeg version 0.0-fake"; exit 0;;
esac
%s
`, action, action)
	path := filepath.Join(dir, "fake-ffmpeg")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// TestLegacyTargetStillRuns is acceptance item 4: a library written before
// task 14 (Target only, empty flow columns) still encodes correctly with no
// manual migration — the job spec builder compiles the flow from the stored
// Target on read.
func TestLegacyTargetStillRuns(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)

	// Simulate a legacy row: strip the flow columns the manager wrote.
	row, err := s.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	row.FlowJSON = ""
	row.FlowSource = ""
	require.NoError(t, s.st.UpdateLibrary(context.Background(), row))

	origPath := s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")
	origSize := int64(len(s.fileHash(origPath)))

	_ = s.scanSync(lib.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		f := s.fileRow(lib.ID)
		return f.Status != store.FileStatusProcessing && f.Status != store.FileStatusQueued
	})

	job := s.jobFor(lib.ID)
	require.Greater(t, job.BytesAfter, int64(0))
	require.Less(t, job.BytesAfter, job.BytesBefore, "output must be smaller")

	// The original is replaced in place with hevc video.
	info, err := probe.Probe(context.Background(), "", origPath)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)
	st, err := os.Stat(origPath)
	require.NoError(t, err)
	require.Less(t, st.Size(), origSize)

	row2 := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row2.Status)

	// A rescan queues nothing: the legacy library reached the same settled
	// state a task-14 library would.
	res := s.scanSync(lib.ID)
	require.Equal(t, 0, res.Queued)
}

// TestIntegrationWorkerEndToEnd is acceptance item 1: a library with one
// h264 fixture, role-both stack, job completes, output is hevc and smaller,
// the original is replaced in place, the file is satisfied and a rescan
// queues nothing.
func TestIntegrationWorkerEndToEnd(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin // tiny fixtures: guarantee a smaller output
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	// A deliberately wasteful h264 source (crf 1) so any sane hevc target
	// is clearly smaller; the stock fixture is already efficient and would
	// make the skip path fire instead.
	origPath := s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")
	origSize := int64(len(s.fileHash(origPath)))

	res := s.scanSync(lib.ID)
	_ = res // the async initial scan may have queued the job first

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	job := s.jobFor(lib.ID)
	require.NotEmpty(t, job.EncoderUsed)
	require.Equal(t, origSize, job.BytesBefore)
	require.Greater(t, job.BytesAfter, int64(0))
	require.Less(t, job.BytesAfter, job.BytesBefore, "output must be smaller")
	var fallbacks []string
	require.NoError(t, json.Unmarshal([]byte(job.FallbackReasons), &fallbacks))

	// The original is replaced (in place; same extension), hevc and smaller.
	info, err := probe.Probe(context.Background(), "", origPath)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)
	st, err := os.Stat(origPath)
	require.NoError(t, err)
	require.Less(t, st.Size(), origSize)

	// The file row is satisfied with the new fingerprint and bytes saved.
	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row.Status)
	require.Equal(t, origPath, row.Path)
	newFP, err := probe.Fingerprint(origPath)
	require.NoError(t, err)
	require.Equal(t, newFP, row.Fingerprint)
	require.Equal(t, job.BytesBefore-job.BytesAfter, row.BytesSaved)

	// A rescan queues nothing.
	res = s.scanSync(lib.ID)
	require.Equal(t, 0, res.Queued)
	require.Equal(t, 1, res.Skipped)
}

// TestIntegrationCorruptOutputFails is acceptance item 2: a fake ffmpeg
// that writes garbage fails verification; the original is unchanged and the
// job is failed with a log tail.
func TestIntegrationCorruptOutputFails(t *testing.T) {
	s := newStack(t, stackConfig{})
	lib := s.createLibrary(store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(s.fixtures.H264TwoAudioSubtitleMKV, "movie.mkv")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	// Capabilities preset so detection never runs the fake binary.
	fake := writeFakeFFmpeg(t, s.dataDir,
		`head -c 4096 /dev/zero > "$last"; echo "corrupt-fake-ffmpeg ran" >&2; exit 0`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) {
		c.FFmpegPath = fake
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	s.waitFor("job failed", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusFailed
	})

	job := s.jobFor(lib.ID)
	require.Contains(t, job.Log, "corrupt-fake-ffmpeg ran")
	require.NotEmpty(t, job.Error) // probe or verification failure, per the task

	// The original is byte-identical and no output appeared.
	require.Equal(t, origData, s.fileHash(origPath))
	entries, err := os.ReadDir(s.library)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Scratch was cleaned even on failure.
	s.waitForNoScratch()
}

// TestIntegrationNotSmallerSkips is acceptance item 3: replace_if_smaller
// with a forced larger output keeps the original and marks the job done
// with bytes_saved 0.
func TestIntegrationNotSmallerSkips(t *testing.T) {
	s := newStack(t, stackConfig{})
	lib := s.createLibrary(store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(s.fixtures.H264TwoAudioSubtitleMKV, "movie.mp4")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	// A valid hevc output that is larger than the source: same track
	// layout (2 audio + 1 subtitle), padded with a huge comment tag.
	big := makeLargeHEVC(t, s.ffmpeg)
	fake := writeFakeFFmpeg(t, s.dataDir, fmt.Sprintf("cp %q \"$last\"; exit 0", big))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) {
		c.FFmpegPath = fake
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	job := s.jobFor(lib.ID)
	require.Equal(t, job.BytesBefore, job.BytesAfter) // recorded as no change

	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusDone, row.Status)
	require.Equal(t, int64(0), row.BytesSaved)
	require.NotEmpty(t, row.SatisfiedReason)

	// The original is untouched.
	require.Equal(t, origData, s.fileHash(origPath))
	s.waitForNoScratch()
}

// createLibraryFlow adds a library and stores an explicit rule graph on it
// (task 24): UpdateFlow re-syncs the Target from the compiled flow, so the
// graph is the single source of truth for the jobs.
func (s *stack) createLibraryFlow(g flow.Graph, policy string) *store.Library {
	s.t.Helper()
	lib := s.createLibraryTarget(flow.Default(), policy)
	if _, err := s.svc.Libraries().UpdateFlow(context.Background(), lib.ID, "steps", g); err != nil {
		s.t.Fatalf("store flow: %v", err)
	}
	return lib
}

// healthCheckGraph remuxes to MKV and verifies the result with a
// health_check step carrying hcParams ("{}" = the defaults).
func healthCheckGraph(hcParams string) flow.Graph {
	g := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "remux", Type: flow.StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
			{ID: "hc", Type: flow.StepHealthCheck, Params: []byte(hcParams)},
		},
		Edges: []flow.Edge{{From: "probe", To: "remux"}, {From: "remux", To: "hc"}},
	}
	return g
}

// TestIntegrationHealthCheckPasses: a real remux plus a real decode check
// completes; the healthy output replaces the original (task 24).
func TestIntegrationHealthCheckPasses(t *testing.T) {
	s := newStack(t, stackConfig{})
	lib := s.createLibraryFlow(healthCheckGraph("{}"), store.ResultPolicyReplace)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")

	s.scanSync(lib.ID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	// The remuxed output replaced the original and still plays.
	out, err := probe.Probe(context.Background(), "", filepath.Join(s.library, "movie.mkv"))
	require.NoError(t, err)
	require.Equal(t, flow.CodecH264, out.Video[0].Codec)
	_, err = os.Stat(origPath)
	require.True(t, os.IsNotExist(err))
	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row.Status)
	s.waitForNoScratch()
}

// TestIntegrationHealthCheckDecodeFails: an output that fails its decode
// check fails the health_check job; the original stays byte-identical
// (task 24 acceptance). The fake ffmpeg performs the real remux and
// corrupts the result, and the decode check — the health check's ffmpeg
// call — fails the way a real ffmpeg fails on undecodable data. The failure
// is scripted because how much corruption a given ffmpeg build tolerates
// before -xerror trips varies; the pass side of the decode command is
// proven for real by TestIntegrationHealthCheckPasses.
func TestIntegrationHealthCheckDecodeFails(t *testing.T) {
	s := newStack(t, stackConfig{})
	lib := s.createLibraryFlow(healthCheckGraph("{}"), store.ResultPolicyReplace)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID)

	real := strconv.Quote(s.ffmpeg)
	fake := writeFakeFFmpeg(t, s.dataDir, `
for a; do
  if [ "$a" = "null" ]; then
    echo "[h264] Invalid data found when processing input" >&2
    exit 183
  fi
done
`+real+` -hide_banner -loglevel error "$@" || exit 1
SZ=$(stat -c%s "$last")
dd if=/dev/urandom of="$last" bs=1 seek=$((SZ / 2)) count=$((SZ / 3)) conv=notrunc 2>/dev/null
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) {
		c.FFmpegPath = fake
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	s.waitFor("job failed", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusFailed
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	job := s.jobFor(lib.ID)
	require.Contains(t, job.Error, "health check failed")
	require.Contains(t, job.Error, "decode check failed")
	require.Contains(t, job.Error, "Invalid data found")

	// The original is byte-identical and nothing replaced it.
	require.Equal(t, origData, s.fileHash(origPath))
	entries, err := os.ReadDir(s.library)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	s.waitForNoScratch()
}

// TestIntegrationMovePlacesResult: the move step puts the finished file
// into a folder outside the library and the original is gone (task 24).
func TestIntegrationMovePlacesResult(t *testing.T) {
	s := newStack(t, stackConfig{})
	dest := t.TempDir() // outside the library, as the save-time check demands
	g := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "remux", Type: flow.StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
			{ID: "mv", Type: flow.StepMove, Params: []byte(`{"destination":"` + dest + `","keep_tree":false}`)},
		},
		Edges: []flow.Edge{{From: "probe", To: "remux"}, {From: "remux", To: "mv"}},
	}
	lib := s.createLibraryFlow(g, store.ResultPolicyReplace)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	s.scanSync(lib.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	moved := filepath.Join(dest, "movie.mkv")
	st, err := os.Stat(moved)
	require.NoError(t, err, "the result must sit at the destination")
	require.Greater(t, st.Size(), int64(0))
	_, err = os.Stat(origPath)
	require.True(t, os.IsNotExist(err), "the original is removed after the verified move")

	// The file row follows the moved file.
	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row.Status)
	require.Equal(t, moved, row.Path)
	s.waitForNoScratch()
}

// TestIntegrationKeepBoth is acceptance item 4: keep_both writes
// <name>.shrunk.mkv beside the original.
func TestIntegrationKeepBoth(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	lib := s.createLibraryTarget(target, store.ResultPolicyKeepBoth)
	origPath := s.copyFixtureIn(s.fixtures.H264TwoAudioSubtitleMKV, "movie.mp4")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	sidecar := filepath.Join(s.library, "movie.shrunk.mkv")
	info, err := probe.Probe(context.Background(), "", sidecar)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)

	// The original is byte-identical and the row still points at it.
	require.Equal(t, origData, s.fileHash(origPath))
	row := s.fileRow(lib.ID)
	require.Equal(t, origPath, row.Path)
	require.Equal(t, store.FileStatusSatisfied, row.Status)

	s.waitForNoScratch()
}

/* --- task 32: step plugins --- */

// installStepPluginDir writes a step plugin into a temp dir and installs it
// into the stack's data dir through the plugin service (enabled), the way
// the Plugins page does.
func (s *stack) installStepPluginDir(name, script string, timeoutSec int) {
	s.t.Helper()
	src := filepath.Join(s.t.TempDir(), name)
	require.NoError(s.t, os.MkdirAll(src, 0o755))
	scriptPath := filepath.Join(src, "run.sh")
	require.NoError(s.t, os.WriteFile(scriptPath, []byte(script+"\n"), 0o755))
	timeoutLine := ""
	if timeoutSec > 0 {
		timeoutLine = fmt.Sprintf("timeout_seconds: %d\n", timeoutSec)
	}
	manifest := "name: " + name + `
version: 1.0.0
type: step
description: test plugin
command: ["sh", "` + scriptPath + `"]
` + timeoutLine
	require.NoError(s.t, os.WriteFile(filepath.Join(src, plugin.FileName), []byte(manifest), 0o644))
	inst, err := s.svc.Plugins().Install(context.Background(), server.InstallRequest{Path: src})
	require.NoError(s.t, err)
	yes := true
	_, err = s.svc.Plugins().Update(context.Background(), inst.ID, server.UpdateRequest{Enabled: &yes})
	require.NoError(s.t, err)
}

// pluginStepGraph is probe → plugin → if_codec_not <codec> → transcode →
// remux (guard empty: probe → plugin → remux): the plugin runs before the
// guard, so a plugin that rewrites the file changes the guard's decision.
func pluginStepGraph(pluginName, guardCodec string) flow.Graph {
	nodes := []flow.Step{
		{ID: "probe", Type: flow.StepProbe},
		{ID: "plug", Type: flow.StepPlugin, Params: json.RawMessage(`{"plugin":"` + pluginName + `"}`)},
	}
	edges := []flow.Edge{{From: "probe", To: "plug"}}
	if guardCodec != "" {
		nodes = append(nodes,
			flow.Step{ID: "guard", Type: flow.StepIfCodecNot, Params: json.RawMessage(`{"codec":"` + guardCodec + `"}`)},
			flow.Step{ID: "transcode", Type: flow.StepTranscodeVideo, Params: json.RawMessage(`{"codec":"h264","quality":3}`)})
		edges = append(edges, flow.Edge{From: "plug", To: "guard"}, flow.Edge{From: "guard", To: "transcode"})
	} else {
		nodes = append(nodes, flow.Step{ID: "transcode", Type: flow.StepTranscodeVideo, Params: json.RawMessage(`{"codec":"h264","quality":3}`)})
		edges = append(edges, flow.Edge{From: "plug", To: "transcode"})
	}
	nodes = append(nodes, flow.Step{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)})
	edges = append(edges, flow.Edge{From: "transcode", To: "remux"})
	return flow.Graph{Schema: flow.FlowSchema, Nodes: nodes, Edges: edges}
}

// TestIntegrationPluginStepRewritesForLaterGuard is acceptance item 4: an
// in-place rewrite step plugin is visible to a later guard. The plugin
// replaces the working file with a big hevc copy; the if_codec_not hevc
// guard then FAILS (the file is hevc), so no transcode runs — where the
// original h264 file would have passed the guard and been re-encoded. The
// output is the plugin's file streaming through with video copy, and the
// replace policy puts it in place of the original.
func TestIntegrationPluginStepRewritesForLaterGuard(t *testing.T) {
	s := newStack(t, stackConfig{})

	// The plugin copies a big hevc file over the working file in place
	// (its last argument is the working file).
	big := makeLargeHEVC(t, s.ffmpeg)
	s.installStepPluginDir("bignifier",
		"cp "+strconv.Quote(big)+" \"$1\"\nprintf 'bignifier ran\n'", 0)

	lib := s.createLibraryFlow(pluginStepGraph("bignifier", "hevc"), store.ResultPolicyReplace)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	// The guard saw the plugin's hevc rewrite and failed: no transcode,
	// and the plugin's file became the output. It is far bigger than the
	// original, and the replace policy put it in the original's place.
	job := s.jobFor(lib.ID)
	require.Greater(t, job.BytesAfter, job.BytesBefore, "the plugin's big file became the output")
	row := s.fileRow(lib.ID)
	require.Equal(t, strings.TrimSuffix(origPath, ".mp4")+".mkv", row.Path,
		"the flow's remux step renames the output container")
	require.Contains(t, s.jobLog(lib.ID), "bignifier ran")

	// The file on disk is the plugin's hevc rewrite (the guard's skip of
	// the transcode left video copy, so the plugin's file streamed
	// through untouched).
	info, err := probe.Probe(context.Background(), "", row.Path)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)
	require.Greater(t, info.SizeBytes, int64(300000), "the padding attachment survived the copy")
	s.waitForNoScratch()
}

// TestIntegrationPluginStepNotSmallerSkips is acceptance item 4's flip
// side: a plugin's rewrite is what the skip_if_smaller policy evaluates.
// The plugin replaces the working file with a stripped, much smaller file;
// the guard passes (it is h264, not hevc), the transcode consumes the
// plugin's file, and the output — built from the plugin's rewrite — is
// smaller than the original and replaces it.
func TestIntegrationPluginStepNotSmallerSkips(t *testing.T) {
	s := newStack(t, stackConfig{})

	// The plugin strips the working file down to video only (much smaller
	// than the 2-audio original) by rewriting it in place.
	small := makeTinyFixture(t, s.ffmpeg)
	s.installStepPluginDir("stripper",
		"cp "+strconv.Quote(small)+" \"$1\"\nprintf 'stripper ran\n'", 0)

	lib := s.createLibraryFlow(pluginStepGraph("stripper", "hevc"), store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	s.scanSync(lib.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	// The encode consumed the plugin's stripped rewrite: the output is
	// smaller than the original (which the policy replaced the original
	// with) and carries the plugin's track layout (video only, no audio).
	job := s.jobFor(lib.ID)
	require.Less(t, job.BytesAfter, job.BytesBefore, "the plugin's rewrite made the output smaller")
	row := s.fileRow(lib.ID)
	require.Equal(t, strings.TrimSuffix(origPath, ".mp4")+".mkv", row.Path,
		"the flow's remux step renames the output container")
	require.Contains(t, s.jobLog(lib.ID), "stripper ran")

	// The file on disk is the encode of the plugin's stripped file: one
	// video, zero audio (the original had two).
	info, err := probe.Probe(context.Background(), "", row.Path)
	require.NoError(t, err)
	require.Len(t, info.Video, 1)
	require.Empty(t, info.Audio, "the plugin's stripped file was the encode's input")
	_, statErr := os.Stat(origPath)
	require.True(s.t, os.IsNotExist(statErr), "the replace policy removed the original (renamed container)")
	s.waitForNoScratch()
}

// TestIntegrationPluginStepFailsJob is acceptance item 5: a step plugin
// that exits non-zero fails the job and the original file is byte
// identical (the result policy never runs).
func TestIntegrationPluginStepFailsJob(t *testing.T) {
	s := newStack(t, stackConfig{})
	s.installStepPluginDir("breakit", "exit 3\n", 0)

	lib := s.createLibraryFlow(pluginStepGraph("breakit", "hevc"), store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job failed", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusFailed
	})
	job := s.jobFor(lib.ID)
	require.Contains(t, job.Error, "plugin breakit")
	require.Contains(t, job.Error, "exit status 3")

	// The original is byte identical (the result policy never ran).
	require.Equal(t, origData, s.fileHash(origPath))
	row := s.fileRow(lib.ID)
	require.Equal(t, origPath, row.Path)
}

// TestIntegrationPluginStepTimeout is acceptance item 6: a step plugin
// that sleeps past its timeout is killed and the job fails naming the
// plugin.
func TestIntegrationPluginStepTimeout(t *testing.T) {
	s := newStack(t, stackConfig{})
	s.installStepPluginDir("sleepy", "sleep 30\n", 1)

	lib := s.createLibraryFlow(pluginStepGraph("sleepy", "hevc"), store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(s.fixtures.H264AudioMP4, "movie.mp4")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(nil))

	s.waitFor("job failed", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusFailed
	})
	job := s.jobFor(lib.ID)
	require.Contains(t, job.Error, "plugin sleepy")
	require.Contains(t, job.Error, "timed out")
	require.Equal(t, origData, s.fileHash(origPath))
}

// TestIntegrationCancelStopsFFmpeg is acceptance item 6: cancel via API
// stops ffmpeg within 5s and the scratch dir is gone.
func TestIntegrationCancelStopsFFmpeg(t *testing.T) {	s := newStack(t, stackConfig{})
	lib := s.createLibrary(store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "long.mkv")
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// CPU only, with real detection skipped for speed and determinism.
	s.runWorker(ctx, s.workerConfig(func(c *Config) {
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	var jobID string
	s.waitForDesc("job running", func() bool {
		job := s.jobFor(lib.ID)
		if job == nil {
			return false
		}
		jobID = job.ID
		return job.Status == store.JobStatusRunning
	}, func() string {
		job := s.jobFor(lib.ID)
		if job == nil {
			return "no job yet"
		}
		files, _ := s.st.ListFiles(context.Background(), lib.ID)
		row := "no file"
		if len(files) > 0 {
			row = fmt.Sprintf("file status=%s probe=%d bytes", files[0].Status, len(files[0].ProbeJSON))
		}
		return fmt.Sprintf("job status=%s attempt=%d node=%s; %s", job.Status, job.Attempt, job.NodeID, row)
	})

	start := time.Now()
	require.Equal(t, http.StatusNoContent, s.adminCancel(jobID))

	// Within 5s: job cancelled, ffmpeg stopped (scratch dir removed).
	s.waitFor("cancel settles within 5s", func() bool {
		if time.Since(start) > 5*time.Second {
			return true // let the assertions below report the failure
		}
		job, err := s.st.GetJob(context.Background(), jobID)
		if err != nil {
			return false
		}
		if job.Status != store.JobStatusCancelled {
			return false
		}
		_, statErr := os.Stat(filepath.Join(s.scratch, "job-"+jobID))
		return os.IsNotExist(statErr)
	})
	job, err := s.st.GetJob(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusCancelled, job.Status)
	require.Less(t, time.Since(start), 5*time.Second, "cancel must settle within 5s")
}

// TestIntegrationKillAndRestart is acceptance item 5: killing the worker
// mid-job leaves the original intact; after the heartbeat timeout the job
// returns to queued and completes on retry; orphaned scratch is cleaned on
// worker restart.
func TestIntegrationKillAndRestart(t *testing.T) {
	s := newStack(t, stackConfig{
		// Generous enough that a loaded CI machine still gets its first
		// progress report through before the sweeper requeues.
		heartbeatTimeout: 3 * time.Second,
		sweepInterval:    250 * time.Millisecond,
	})
	lib := s.createLibrary(store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "long.mkv")
	origData := s.fileHash(origPath)
	s.scanSync(lib.ID) // the create hook fired before the fixture landed

	worker1Ctx, cancel1 := context.WithCancel(context.Background())
	s.runWorker(worker1Ctx, s.workerConfig(func(c *Config) {
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	var jobID string
	s.waitFor("job running", func() bool {
		job := s.jobFor(lib.ID)
		if job == nil {
			return false
		}
		jobID = job.ID
		return job.Status == store.JobStatusRunning
	})

	// "Kill" the worker: cancel its context mid-encode.
	cancel1()
	// Give the kill time to propagate so the retry below cannot race a
	// dying process writing to the same scratch dir.
	time.Sleep(400 * time.Millisecond)

	// The original is intact and no replacement appeared.
	require.Equal(t, origData, s.fileHash(origPath))
	entries, err := os.ReadDir(s.library)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Plant an orphaned scratch dir the way a SIGKILLed worker would.
	orphan := filepath.Join(s.scratch, "job-orphaned")
	require.NoError(t, os.MkdirAll(filepath.Join(orphan, "source.mkv"), 0o755))

	// The heartbeat timeout returns the job to the queue.
	s.waitFor("job requeued by sweeper", func() bool {
		job, err := s.st.GetJob(context.Background(), jobID)
		return err == nil && job.Status == store.JobStatusQueued && job.Attempt == 1
	})

	// A restarted worker cleans orphans and completes the job.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	s.runWorker(ctx2, s.workerConfig(func(c *Config) {
		c.Capabilities = &encoders.Capability{}
		c.Encoders = []encoders.Encoder{encoders.CPU}
	}))

	s.waitFor("job done on retry", func() bool {
		job, err := s.st.GetJob(context.Background(), jobID)
		return err == nil && job.Status == store.JobStatusDone
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status != store.FileStatusProcessing && row.Status != store.FileStatusQueued
	})

	// The replacement is in place and the file is satisfied.
	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row.Status)
	_, err = os.Stat(orphan)
	require.True(t, os.IsNotExist(err), "orphaned scratch cleaned on restart")
	s.waitForNoScratch()
}

// --- test media and small helpers ---

// makeLargeHEVC builds a valid hevc mkv with the same track layout as the
// h264 fixture (2 audio + 1 subtitle) but padded far beyond its size, so
// replace_if_smaller must skip it. The padding rides along as a large file
// attachment (a command-line comment tag would exceed the exec arg limit).
func makeLargeHEVC(t *testing.T, ffmpeg string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "big.hevc.mkv")
	srt := filepath.Join(dir, "subs.srt")
	require.NoError(t, os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n"), 0o600))
	padding := filepath.Join(dir, "padding.txt")
	require.NoError(t, os.WriteFile(padding, []byte(strings.Repeat("p", 400000)), 0o600))
	run(t, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-i", srt,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s",
		"-c:v", "libx265", "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "copy", "-c:s", "copy",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=ger",
		"-metadata:s:s:0", "language=eng",
		"-attach", padding,
		"-metadata:s:t", "mimetype=text/plain",
		out,
	)
	st, err := os.Stat(out)
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(300000), "padding must dominate the size")
	return out
}

// makeFatH264 builds an h264 fixture with the same track layout as the
// shared fixture set (2 audio + 1 subtitle) at crf 1, i.e. far larger than
// a normal encode of the same synthetic content.
func makeFatH264(t *testing.T, ffmpeg string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "fat.mkv")
	srt := filepath.Join(dir, "subs.srt")
	require.NoError(t, os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n"), 0o600))
	run(t, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-i", srt,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "1", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k", "-c:s", "srt",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=ger",
		"-metadata:s:s:0", "language=eng",
		out,
	)
	return out
}

// makeTinyFixture builds a tiny video-only fixture: one video stream, no
// audio, no subtitles — much smaller than the shared 2-audio fixtures. The
// plugin step tests use it as the file a rewrite plugin leaves behind.
func makeTinyFixture(t *testing.T, ffmpeg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tiny.mkv")
	run(t, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10",
		"-map", "0:v",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "30", "-pix_fmt", "yuv420p",
		out,
	)
	return out
}

// makeLongFixture builds a file whose x265 medium re-encode takes several
// seconds, giving cancel and kill tests a window to act in. Temporal noise
// keeps the encoder busy (plain testsrc compresses in under a second).
func makeLongFixture(t *testing.T, ffmpeg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "long.mkv")
	run(t, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=15:size=1920x1080:rate=24,noise=alls=20:allf=t",
		"-f", "lavfi", "-i", "sine=duration=15",
		"-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "25", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k",
		out,
	)
	return out
}

// run executes ffmpeg and fails the test on error.
func run(t *testing.T, ffmpeg string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v failed: %v\n%s", args, err, out)
	}
}

// waitForNoScratch waits for the scratch root to empty: the runner removes
// its job directory after it reports, so cleanup trails the job status by
// a few milliseconds.
func (s *stack) waitForNoScratch() {
	s.t.Helper()
	s.waitFor("scratch cleanup", func() bool {
		entries, err := os.ReadDir(s.scratch)
		return os.IsNotExist(err) || (err == nil && len(entries) == 0)
	})
}

// --- task 08: pairing, revocation and shared mode ---

// pairWithToken runs the real pairing flow over the HTTP API with a fresh
// one-time token and returns the node id and key.
func (s *stack) pairWithToken(name string) (nodeID, key string) {
	s.t.Helper()
	raw, _, err := s.svc.Nodes().CreateToken(context.Background())
	require.NoError(s.t, err)
	resp, err := NewClient(s.apiSrv.URL, "").Pair(context.Background(), PairInput{
		Token: raw, Name: name, OS: "linux", Arch: "amd64",
	})
	require.NoError(s.t, err)
	require.Equal(s.t, "approved", resp.Status)
	require.NotEmpty(s.t, resp.NodeKey)
	return resp.NodeID, resp.NodeKey
}

// TestIntegrationTokenPairing is acceptance item 2: a worker with a valid
// token is approved immediately, claims a job and completes it; the used
// token is rejected with 401 afterwards.
func TestIntegrationTokenPairing(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")

	res := s.scanSync(lib.ID)
	_ = res

	nodeID, key := s.pairWithToken("tokened-worker")
	require.NotEmpty(t, nodeID)
	// The admin starts the node (task 11: pairing alone starts nothing).
	s.startNode(nodeID)

	// A second token pairs fine once, then is spent: the repeat is 401.
	raw2, _, err := s.svc.Nodes().CreateToken(context.Background())
	require.NoError(t, err)
	_, err = NewClient(s.apiSrv.URL, "").Pair(context.Background(), PairInput{
		Token: raw2, Name: "throwaway",
	})
	require.NoError(t, err)
	_, err = NewClient(s.apiSrv.URL, "").Pair(context.Background(), PairInput{
		Token: raw2, Name: "throwaway",
	})
	require.True(t, isAPIStatus(err, http.StatusUnauthorized), "used token must be 401, got %v", err)

	// The paired worker claims and completes the job.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	s.waitFor("job done by token-paired worker", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone && job.NodeID == nodeID
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status == store.FileStatusSatisfied
	})
	row := s.fileRow(lib.ID)
	require.Equal(t, origPath, row.Path)
	require.Greater(t, row.BytesSaved, int64(0))
}

// TestIntegrationApprovalPairing is acceptance item 3: a worker without a
// token shows up pending, cannot claim, gets its key after the admin
// approves it, and then completes a job.
func TestIntegrationApprovalPairing(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")
	s.scanSync(lib.ID)

	// The worker pairs without a token: EnsurePaired blocks in the poll
	// loop until the admin approves.
	paired := make(chan error, 1)
	var pairKey, pairURL string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		url, key, err := EnsurePaired(ctx, PairConfig{
			ServerURL:    s.apiSrv.URL,
			NodeName:     "approval-worker",
			DataDir:      t.TempDir(),
			PollInterval: 100 * time.Millisecond,
			Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		pairURL, pairKey = url, key
		paired <- err
	}()

	// The pending node appears.
	var pendingID string
	s.waitFor("pending node appears", func() bool {
		nodes, err := s.st.ListNodes(context.Background())
		if err != nil {
			return false
		}
		for _, n := range nodes {
			if n.Name == "approval-worker" && n.Status == store.NodeStatusPending {
				pendingID = n.ID
				return true
			}
		}
		return false
	})

	// Without a key the worker cannot claim: its request is 401.
	_, err := NewClient(s.apiSrv.URL, "").Claim(context.Background())
	require.True(t, isAPIStatus(err, http.StatusUnauthorized), "unpaired claim must be 401, got %v", err)

	// The admin approves, starts the node, and EnsurePaired returns with
	// the key.
	_, err = s.svc.Nodes().Approve(context.Background(), pendingID)
	require.NoError(t, err)
	s.startNode(pendingID)
	require.NoError(t, <-paired)
	require.Equal(t, s.apiSrv.URL, pairURL)
	require.NotEmpty(t, pairKey)

	// The approved worker claims and completes.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	s.runWorker(ctx2, s.workerConfig(func(c *Config) { c.Key = pairKey }))
	s.waitFor("job done by approved worker", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone && job.NodeID == pendingID
	})
}

// TestIntegrationRevokeRequeuesForBuiltinWorker is acceptance item 4:
// revoking a node kills its access (401), returns its running job to the
// queue and the built-in worker finishes it.
func TestIntegrationRevokeRequeuesForBuiltinWorker(t *testing.T) {
	s := newStack(t, stackConfig{})
	lib := s.createLibrary(store.ResultPolicyReplaceIfSmaller)
	s.copyFixtureIn(makeLongFixture(t, s.ffmpeg), "long.mkv")
	s.scanSync(lib.ID)

	// A remote node takes the job (the built-in worker only starts after
	// the revoke, so the remote reliably gets it first).
	nodeID, key := s.pairWithToken("doomed-worker")
	s.startNode(nodeID)
	ctxRemote, cancelRemote := context.WithCancel(context.Background())
	defer cancelRemote()
	s.runWorker(ctxRemote, s.workerConfig(func(c *Config) { c.Key = key }))

	s.waitFor("remote node running the job", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusRunning && job.NodeID == nodeID
	})

	// Revoke: the key stops working, the running job returns to the queue.
	_, err := s.svc.Nodes().Revoke(context.Background(), nodeID)
	require.NoError(t, err)
	s.waitFor("job requeued on revoke", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusQueued && job.Attempt == 1
	})
	_, err = NewClient(s.apiSrv.URL, key).Heartbeat(context.Background(), HeartbeatInput{})
	require.True(t, isAPIStatus(err, http.StatusUnauthorized), "revoked node must get 401, got %v", err)

	// The remote encode cannot apply anything anymore: stop it cleanly.
	cancelRemote()

	// The built-in worker (its key from the stack setup) finishes the job.
	builtin, err := s.svc.Nodes().Authenticate(context.Background(), s.key)
	require.NoError(t, err)
	ctxBuiltin, cancelBuiltin := context.WithCancel(context.Background())
	defer cancelBuiltin()
	s.runWorker(ctxBuiltin, s.workerConfig(nil))

	s.waitFor("job done by built-in worker", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone && job.NodeID == builtin.ID
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status == store.FileStatusSatisfied
	})
	row := s.fileRow(lib.ID)
	require.Equal(t, store.FileStatusSatisfied, row.Status)
}

// TestIntegrationSharedModePathMapping is acceptance item 5: a node in
// shared mode with a path mapping resolves the source path through it and
// completes the job. On one machine the "different mount" is a symlink to
// the same storage — different path, same bytes, exactly the shared-mount
// semantics the mapping exists for.
func TestIntegrationSharedModePathMapping(t *testing.T) {
	s := newStack(t, stackConfig{})
	target := flow.Default()
	target.Video.Quality = flow.QualityMin
	lib := s.createLibraryTarget(target, store.ResultPolicyReplaceIfSmaller)
	origPath := s.copyFixtureIn(makeFatH264(t, s.ffmpeg), "movie.mkv")

	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(s.library, alias), "symlink must work on this platform")

	nodeID, key := s.pairWithToken("mapped-worker")
	s.startNode(nodeID)
	mappings := fmt.Sprintf(`[{"server_path": %q, "node_path": %q}]`, s.library, alias)
	_, err := s.svc.Nodes().Update(context.Background(), nodeID, server.NodeUpdateParams{
		PathMappings: &mappings,
	})
	require.NoError(t, err)
	s.scanSync(lib.ID)

	// The job spec hands the node its own view of the source. Task 09
	// changed the spec semantics: the node's configured mode passes
	// through and `auto` is resolved by the worker with the shared-storage
	// check (this node was paired in auto mode).
	claimed, err := NewClient(s.apiSrv.URL, key).Claim(context.Background())
	require.NoError(t, err)
	require.NotNil(t, claimed)
	spec, err := NewClient(s.apiSrv.URL, key).JobSpec(context.Background(), claimed.ID)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(alias, "movie.mkv"), spec.SourcePath)
	require.Equal(t, store.NodeAccessAuto, spec.AccessMode)

	// Hand the job back so the real worker can take it.
	_, err = s.svc.Queue().Cancel(context.Background(), claimed.ID)
	require.NoError(t, err)
	_, err = s.svc.Queue().Retry(context.Background(), claimed.ID)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runWorker(ctx, s.workerConfig(func(c *Config) { c.Key = key }))

	s.waitFor("mapped job done", func() bool {
		job := s.jobFor(lib.ID)
		return job != nil && job.Status == store.JobStatusDone && job.NodeID == nodeID
	})
	s.waitFor("file settled", func() bool {
		row := s.fileRow(lib.ID)
		return row.Status == store.FileStatusSatisfied
	})

	// The library row kept the server-side path, and the file behind it
	// (shared storage: the alias points at the same directory) is the
	// encoded output.
	row := s.fileRow(lib.ID)
	require.Equal(t, origPath, row.Path)
	info, err := probe.Probe(context.Background(), "", row.Path)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, info.Video[0].Codec)
	require.Greater(t, row.BytesSaved, int64(0))
}
