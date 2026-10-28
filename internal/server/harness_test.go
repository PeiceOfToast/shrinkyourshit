// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// fakeProber returns a canned MediaInfo per path, keyed on the file's base
// name; paths without a specific entry get the default.
type fakeProber struct {
	mu     sync.Mutex
	byName map[string]probe.MediaInfo
	fallback probe.MediaInfo
	calls  map[string]int
}

func newFakeProber(fallback probe.MediaInfo) *fakeProber {
	return &fakeProber{byName: map[string]probe.MediaInfo{}, fallback: fallback, calls: map[string]int{}}
}

func (p *fakeProber) Probe(_ context.Context, _ string, path string) (probe.MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[path]++
	info, ok := p.byName[filepath.Base(path)]
	if !ok {
		info = p.fallback
	}
	return info, nil
}

func (p *fakeProber) callsOf(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[path]
}

// hevcSatisfied satisfies the default Target.
var hevcSatisfied = probe.MediaInfo{
	Container: flow.ContainerMKV,
	Video:     []probe.VideoStream{{Codec: flow.CodecHEVC, Width: 160, Height: 120}},
	Audio:     []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
	Subtitle:  []probe.SubtitleStream{{Codec: "subrip", Language: "eng", TextBased: true}},
}

// h264Needs needs work under the default Target (h264 vs hevc).
var h264Needs = probe.MediaInfo{
	Container: flow.ContainerMKV,
	Video:     []probe.VideoStream{{Codec: flow.CodecH264, Width: 160, Height: 120}},
	Audio:     []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
}

// harness is a full server stack over a temp store and temp library dir,
// with a fake prober so unit tests never touch ffmpeg. Integration tests
// flip realProber to run ffprobe for real.
type harness struct {
	t          *testing.T
	st         store.Store
	dbPath     string
	root       string // library folder
	svc        *Server
	prober     *fakeProber
	realProber bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:      t,
		dbPath: filepath.Join(t.TempDir(), "data.db"),
		root:   filepath.Join(t.TempDir(), "library"),
	}
	require.NoError(t, os.MkdirAll(h.root, 0o755))
	h.construct(nil)
	t.Cleanup(h.stop)
	return h
}

// construct builds store + server on the existing paths. Used by both the
// initial build and restart (which reuses dbPath and root). A nil prober
// with realProber=false selects the fake; realProber selects real ffprobe.
func (h *harness) construct(prober Prober) {
	h.t.Helper()
	st, err := store.New(h.dbPath)
	require.NoError(h.t, err)
	h.st = st
	if h.prober == nil {
		h.prober = newFakeProber(hevcSatisfied)
	}
	useProber := prober
	if useProber == nil && !h.realProber {
		useProber = h.prober.Probe
	}
	h.svc = New(Config{
		Store:             st,
		Prober:            useProber,
		Debounce:          200 * time.Millisecond,
		StableWindow:      300 * time.Millisecond,
		SweepInterval:     50 * time.Millisecond,
		HeartbeatTimeout:  time.Hour, // background sweeper never interferes; tests sweep directly
		WatchPollInterval: time.Hour, // poll fallback never fires in tests
	})
	// Most tests scan explicitly; a create-triggered scan would race them.
	// The behavior gets its own dedicated test.
	h.svc.Libraries().SetScanHook(func(string) {})
	h.svc.Start(context.Background())
}

// stop shuts the server and the store down.
func (h *harness) stop() {
	if h.svc != nil {
		h.svc.Stop()
		h.svc = nil
	}
	if h.st != nil {
		_ = h.st.Close()
		h.st = nil
	}
}

// restart closes everything and reopens the same DB and library dir: the
// persistence-under-restart scenario.
func (h *harness) restart() {
	h.stop()
	h.construct(nil)
}

// createLibrary writes a library rooted at h.root and runs an initial scan.
func (h *harness) createLibrary(watch bool) *store.Library {
	h.t.Helper()
	target, err := json.Marshal(flow.Default())
	require.NoError(h.t, err)
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name:         "Movies",
		Path:         h.root,
		TargetJSON:   target,
		Watch:        watch,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(h.t, err)
	return lib
}

// scan runs a synchronous scan and fails the test on error.
func (h *harness) scan(libID string) ScanResult {
	h.t.Helper()
	res, err := h.svc.Scanner().ScanLibrary(context.Background(), libID)
	require.NoError(h.t, err)
	return res
}

// jobsOf returns all jobs for a file.
func (h *harness) jobsOf(fileID string) []*store.Job {
	h.t.Helper()
	jobs, err := h.st.ListJobs(context.Background(), store.JobFilter{FileID: fileID})
	require.NoError(h.t, err)
	return jobs
}

// files returns the library's files keyed by base name.
func (h *harness) files(libID string) map[string]*store.File {
	h.t.Helper()
	fs, err := h.st.ListFiles(context.Background(), libID)
	require.NoError(h.t, err)
	out := map[string]*store.File{}
	for _, f := range fs {
		out[filepath.Base(f.Path)] = f
	}
	return out
}
