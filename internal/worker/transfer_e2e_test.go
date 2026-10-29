// SPDX-License-Identifier: AGPL-3.0-only

package worker

// End-to-end acceptance tests for task 09: a real worker (HTTP client,
// scratch dir, fake encode) against a real server (store + services + API
// handler) over httptest. No ffmpeg anywhere: the encode and the probes are
// faked, so these run as plain unit tests (rule 11) while still exercising
// the full wire protocol — claim, access check, source download, chunked
// upload, checksum verification and server-side verify-and-replace.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/api"
	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// e2eEnv is one full stack: server store + services + HTTP API, plus a
// worker pointed at it.
type e2eEnv struct {
	t       *testing.T
	st      store.Store
	svc     *server.Server
	ts      *httptest.Server
	hooks   *wireHooks
	key     string
	node    *store.Node
	lib     *store.Library
	file    *store.File
	job     *store.Job
	srcDir  string // the server-side library dir
	scratch string // the worker's scratch dir
	srcData string
	outData string
	srcPath string
	cancel  context.CancelFunc
}

// wireHooks instruments the wire between worker and server: a source-call
// counter, a switch to abort the next source response mid-body (the
// "interrupting the download" acceptance item) and a switch to flip a byte
// in every uploaded chunk (the corrupted-upload item).
type wireHooks struct {
	t           *testing.T
	mu          sync.Mutex
	sourceCalls int
	dropNext    bool
	dropAt      int64
	corrupt     bool
}

// wrap installs the hooks around the API handler.
func (h *wireHooks) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isSource := r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source")
		h.mu.Lock()
		if isSource {
			h.sourceCalls++
		}
		drop, dropAt := false, int64(0)
		if isSource && h.dropNext {
			h.dropNext = false
			drop, dropAt = true, h.dropAt
		}
		corrupt := h.corrupt && r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/result")
		h.mu.Unlock()

		if corrupt && r.Body != nil {
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			require.NoError(h.t, err)
			if len(body) > 2 {
				body[1] ^= 0xFF // flip a byte in flight
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		if drop {
			w = &dropWriter{ResponseWriter: w, dropAt: dropAt}
		}
		next.ServeHTTP(w, r)
	})
}

// dropWriter aborts the connection mid-body (the client sees a broken
// transfer and must resume with Range).
type dropWriter struct {
	http.ResponseWriter
	n      int64
	dropAt int64
}

func (d *dropWriter) Write(p []byte) (int, error) {
	if d.n+int64(len(p)) > d.dropAt {
		n, _ := d.ResponseWriter.Write(p[:d.dropAt-d.n])
		d.n += int64(n)
		panic(http.ErrAbortHandler)
	}
	n, err := d.ResponseWriter.Write(p)
	d.n += int64(n)
	return n, err
}

// ReadFrom copies through Write in bounded chunks. Without it the embedded
// http.response.ReadFrom (promoted through the embedding) would bypass
// Write and stream the whole body without ever hitting the drop.
func (d *dropWriter) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, 32<<10)
	var total int64
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			wn, werr := d.Write(buf[:n])
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return total, nil
			}
			return total, rerr
		}
	}
}

// e2eProber fakes ffprobe for both sides: the source is h264/mp4, any
// output is hevc/mkv.
func e2eProber() server.Prober {
	return func(_ context.Context, _ string, path string) (probe.MediaInfo, error) {
		if strings.HasSuffix(path, ".mp4") {
			return probe.MediaInfo{
				Container:       flow.ContainerMP4,
				DurationSeconds: 10,
				Video:           []probe.VideoStream{{Codec: flow.CodecH264, Width: 320, Height: 240}},
				Audio:           []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
			}, nil
		}
		return probe.MediaInfo{
			Container:       flow.ContainerMKV,
			DurationSeconds: 10,
			Video:           []probe.VideoStream{{Codec: flow.CodecHEVC, Width: 320, Height: 240}},
			Audio:           []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
		}, nil
	}
}

// newE2E builds the stack with a queued job over a real source file. The
// node joins with the given access mode and optional path mappings (raw
// JSON).
func newE2E(t *testing.T, accessMode, mappings string) *e2eEnv {
	t.Helper()
	env := &e2eEnv{t: t, hooks: &wireHooks{t: t}}
	env.srcDir = filepath.Join(t.TempDir(), "library")
	env.scratch = filepath.Join(t.TempDir(), "worker-scratch")
	require.NoError(t, os.MkdirAll(env.srcDir, 0o755))

	env.srcData = strings.Repeat("MOVIE-FRAME-", 12_000) // ~144 KiB
	env.outData = "TINY-VERIFIED-OUTPUT"
	env.srcPath = filepath.Join(env.srcDir, "movie.mp4")
	require.NoError(t, os.WriteFile(env.srcPath, []byte(env.srcData), 0o644))
	st0, err := os.Stat(env.srcPath)
	require.NoError(t, err)
	fp, err := probe.Fingerprint(env.srcPath)
	require.NoError(t, err)

	st, err := store.New(filepath.Join(t.TempDir(), "e2e.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	env.st = st
	ctx := context.Background()

	// The server inspects finished outputs like the default inspector
	// would, but with the fake prober (no ffprobe in tests).
	inspect := func(_ context.Context, _ string, path string) (server.ResultInfo, error) {
		info, err := e2eProber()(ctx, "", path)
		if err != nil {
			return server.ResultInfo{}, err
		}
		s, err := os.Stat(path)
		if err != nil {
			return server.ResultInfo{}, err
		}
		fpo, err := probe.Fingerprint(path)
		if err != nil {
			return server.ResultInfo{}, err
		}
		return server.ResultInfo{Fingerprint: fpo, Size: s.Size(), Mtime: s.ModTime().Unix(), Info: info}, nil
	}
	env.svc = server.New(server.Config{
		Store:             st,
		Prober:            e2eProber(),
		Inspect:           inspect,
		ScratchDir:        filepath.Join(t.TempDir(), "server-scratch"),
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
	})
	env.svc.Libraries().SetScanHook(func(string) {})
	env.svc.Start(ctx)
	t.Cleanup(env.svc.Stop)

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	env.lib, err = env.svc.Libraries().Create(ctx, server.LibraryParams{
		Name: "Movies", Path: env.srcDir, TargetJSON: target,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)

	env.file = &store.File{LibraryID: env.lib.ID, Path: env.srcPath, Size: st0.Size(),
		Mtime: st0.ModTime().Unix(), Fingerprint: fp,
		Status: store.FileStatusNew, ProbeJSON: `{"container":"mp4"}`}
	require.NoError(t, st.UpsertFile(ctx, env.file))
	env.job, _, err = env.svc.Queue().QueueJob(ctx, env.lib, env.file, []string{"video is h264, target is hevc"})
	require.NoError(t, err)

	env.key = "e2e-node-key"
	env.node = &store.Node{ID: "e2e-node", Name: "remote", KeyHash: server.HashKey(env.key),
		AccessMode: accessMode, PathMappings: mappings,
		Status: store.NodeStatusApproved, Enabled: true,
		DesiredState: store.NodeStateRunning}
	require.NoError(t, st.CreateNode(ctx, env.node))

	static := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	handler := env.hooks.wrap(api.New("v0.0.0", static, env.svc))
	env.ts = httptest.NewServer(handler)
	t.Cleanup(env.ts.Close)
	return env
}

// startWorker runs a real worker against the stack and returns; the test's
// cleanup cancels it.
func (env *e2eEnv) startWorker() {
	env.t.Helper()
	w, err := New(Config{
		ServerURL:         env.ts.URL,
		Key:               env.key,
		ScratchDir:        env.scratch,
		HeartbeatInterval: 50 * time.Millisecond,
		ProgressInterval:  20 * time.Millisecond,
		ClaimPollInterval: 10 * time.Millisecond,
		Capabilities:      &encoders.Capability{},
		Prober: func(ctx context.Context, _ string, path string) (probe.MediaInfo, error) {
			return e2eProber()(ctx, "", path)
		},
		Run: func(_ context.Context, args []string) error {
			return os.WriteFile(args[len(args)-1], []byte(env.outData), 0o644)
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(env.t, err)
	ctx, cancel := context.WithCancel(context.Background())
	env.cancel = cancel
	env.t.Cleanup(cancel)
	go func() { _ = w.Run(ctx) }()
}

// waitForJob polls the store until the job reaches a terminal status and
// its file has settled (Complete touches the job first, then the file).
func (env *e2eEnv) waitForJob() *store.Job {
	env.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		job, err := env.st.GetJob(context.Background(), env.job.ID)
		require.NoError(env.t, err)
		file, ferr := env.st.GetFile(context.Background(), env.job.FileID)
		require.NoError(env.t, ferr)
		settled := false
		switch job.Status {
		case store.JobStatusDone:
			settled = file.Status != store.FileStatusProcessing && file.Status != store.FileStatusQueued
		case store.JobStatusFailed, store.JobStatusCancelled:
			settled = file.Status != store.FileStatusProcessing
		}
		if settled {
			return job
		}
		if time.Now().After(deadline) {
			env.t.Fatalf("job did not finish in time; job %s file %s", job.Status, file.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (env *e2eEnv) sourceCalls() int {
	env.hooks.mu.Lock()
	defer env.hooks.mu.Unlock()
	return env.hooks.sourceCalls
}

// TestE2ETransferModeReplacesOriginalOnServer is acceptance item 1: a
// worker with no mapping to the library runs in transfer mode end to end —
// source arrives with the right size, result is uploaded, checksum
// verified, original replaced server-side, job done with
// access_mode=transfer recorded.
func TestE2ETransferModeReplacesOriginalOnServer(t *testing.T) {
	env := newE2E(t, store.NodeAccessTransfer, "[]")
	env.startWorker()

	job := env.waitForJob()
	require.Equal(t, store.JobStatusDone, job.Status, "job error: %s", job.Error)
	require.Equal(t, store.NodeAccessTransfer, job.AccessMode, "the chosen mode is recorded on the job")
	require.GreaterOrEqual(t, env.sourceCalls(), 1, "the source was streamed to the node")

	// The original was replaced on the server: movie.mkv holds the
	// uploaded output, movie.mp4 is gone.
	out, err := os.ReadFile(filepath.Join(env.srcDir, "movie.mkv"))
	require.NoError(t, err)
	require.Equal(t, env.outData, string(out))
	_, err = os.Stat(env.srcPath)
	require.True(t, os.IsNotExist(err), "the original must be replaced")

	// The file row reflects the output identity and is satisfied.
	file, err := env.st.GetFile(context.Background(), env.file.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusSatisfied, file.Status)
	require.Equal(t, filepath.Join(env.srcDir, "movie.mkv"), file.Path)
	require.Equal(t, int64(len(env.outData)), file.Size)
	require.NotEqual(t, env.file.Fingerprint, file.Fingerprint)

	// The upload scratch on the server was cleaned up.
	_, err = os.Stat(env.svc.Transfer().UploadDir(job.ID))
	require.True(t, os.IsNotExist(err), "the verified upload is discarded after completion")
}

// TestE2ESharedModeTransfersNothing is acceptance item 2: a correct mapping
// passes the access check and the job runs in shared mode — the source
// endpoint is never called.
func TestE2ESharedModeTransfersNothing(t *testing.T) {
	env := newE2E(t, store.NodeAccessAuto, "[]")
	// The node sees the library at the same path (an identity mapping).
	env.node.PathMappings = `[{"server_path": "` + jsonQuote(env.srcDir) + `", "node_path": "` + jsonQuote(env.srcDir) + `"}]`
	require.NoError(t, env.st.UpdateNode(context.Background(), env.node))
	env.startWorker()
	job := env.waitForJob()
	require.Equal(t, store.JobStatusDone, job.Status, "job error: %s", job.Error)
	require.Equal(t, store.NodeAccessShared, job.AccessMode)
	require.Equal(t, 0, env.sourceCalls(), "no bytes may be transferred in shared mode")

	out, err := os.ReadFile(filepath.Join(env.srcDir, "movie.mkv"))
	require.NoError(t, err)
	require.Equal(t, env.outData, string(out))
}

// TestE2EDownloadResumesAfterInterrupt is acceptance item 3a: the download
// is interrupted mid-body by the (test) wire; the node resumes with Range
// and completes.
func TestE2EDownloadResumesAfterInterrupt(t *testing.T) {
	env := newE2E(t, store.NodeAccessTransfer, "[]")
	env.hooks.mu.Lock()
	env.hooks.dropNext = true
	env.hooks.dropAt = int64(len(env.srcData)) / 2
	env.hooks.mu.Unlock()
	env.startWorker()

	job := env.waitForJob()
	require.Equal(t, store.JobStatusDone, job.Status, "job error: %s", job.Error)
	require.Equal(t, store.NodeAccessTransfer, job.AccessMode)
	require.GreaterOrEqual(t, env.sourceCalls(), 2, "the interrupted download resumed")

	out, err := os.ReadFile(filepath.Join(env.srcDir, "movie.mkv"))
	require.NoError(t, err)
	require.Equal(t, env.outData, string(out))
}

// TestE2ECorruptedUploadFailsAndKeepsOriginal is acceptance item 3b: a byte
// flipped in flight fails the checksum; the job fails and the original is
// untouched.
func TestE2ECorruptedUploadFailsAndKeepsOriginal(t *testing.T) {
	env := newE2E(t, store.NodeAccessTransfer, "[]")
	env.hooks.mu.Lock()
	env.hooks.corrupt = true
	env.hooks.mu.Unlock()
	env.startWorker()

	job := env.waitForJob()
	require.Equal(t, store.JobStatusFailed, job.Status)
	require.Contains(t, job.Error, "checksum_mismatch")

	// The original is byte-for-byte untouched and no output exists.
	content, err := os.ReadFile(env.srcPath)
	require.NoError(t, err)
	require.Equal(t, env.srcData, string(content))
	_, err = os.Stat(filepath.Join(env.srcDir, "movie.mkv"))
	require.True(t, os.IsNotExist(err))

	file, err := env.st.GetFile(context.Background(), env.file.ID)
	require.NoError(t, err)
	require.Equal(t, store.FileStatusFailed, file.Status)
}

// TestE2ESameSizeDecoyFailsAccessCheck is acceptance item 4: a mapping that
// points at a different file of the same size fails the check; the job runs
// in transfer mode instead.
func TestE2ESameSizeDecoyFailsAccessCheck(t *testing.T) {
	decoyDir := filepath.Join(t.TempDir(), "decoy")
	require.NoError(t, os.MkdirAll(decoyDir, 0o755))
	decoy := strings.Repeat("DECOY-DECOY-", 12_000)
	require.Equal(t, int64(len(decoy)), int64(len(strings.Repeat("MOVIE-FRAME-", 12_000))),
		"the decoy must have the same size for this test to be meaningful")
	require.NoError(t, os.WriteFile(filepath.Join(decoyDir, "movie.mp4"), []byte(decoy), 0o644))

	env := newE2E(t, store.NodeAccessAuto, "[]")
	env.node.PathMappings = `[{"server_path": "` + jsonQuote(env.srcDir) + `", "node_path": "` + jsonQuote(decoyDir) + `"}]`
	require.NoError(t, env.st.UpdateNode(context.Background(), env.node))
	env.startWorker()

	job := env.waitForJob()
	require.Equal(t, store.JobStatusDone, job.Status, "job error: %s", job.Error)
	require.Equal(t, store.NodeAccessTransfer, job.AccessMode,
		"the same-size decoy must fail the shared check")
	require.GreaterOrEqual(t, env.sourceCalls(), 1)
}

// jsonQuote embeds a path into a JSON string literal.
func jsonQuote(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return strings.Trim(string(raw), `"`)
}
