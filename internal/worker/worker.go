// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// Default cadences (task 05): heartbeat every 30s, progress every 2s, and a
// 2s claim poll when the queue is empty. Tests override them.
const (
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultProgressInterval  = 2 * time.Second
	DefaultClaimPollInterval = 2 * time.Second
	DefaultMaxSlots          = 4
)

// Node lifecycle states as the server reports them (task 11). The worker
// starts in the unknown state and claims nothing until the first
// heartbeat answers.
const (
	NodeStateStopped  = "stopped"
	NodeStateRunning  = "running"
	NodeStateDraining = "draining"
)

// Config tunes a worker. Zero values select the defaults; the test seams
// (Capabilities, Encoders, Run, Prober) replace hardware detection and
// process execution in tests.
type Config struct {
	// ServerURL is the server base URL, e.g. http://192.168.1.10:8765.
	ServerURL string
	// Key is this node's key (built-in: <data-dir>/node.key).
	Key string
	// NodeName is this node's display name, used in error messages that
	// name the node (task 32: a plugin step whose directory is missing).
	NodeName string
	// DataDir is this node's data directory: step plugins (task 32) run
	// from <data dir>/plugins/<name>/.
	DataDir string
	// ScratchDir is the local temp space for encodes.
	ScratchDir string
	// FFmpegPath is the ffmpeg binary; empty means "on PATH". ffprobe is
	// assumed beside it.
	FFmpegPath string
	// FFprobePath is the ffprobe binary; empty means "on PATH".
	FFprobePath string
	// Log may be nil (slog default).
	Log *slog.Logger
	// MaxSlots caps the claim goroutines; the active count follows the
	// concurrency the server reports on heartbeat. Default 4.
	MaxSlots int
	// HeartbeatInterval, ProgressInterval and ClaimPollInterval override
	// the cadences above (tests).
	HeartbeatInterval time.Duration
	ProgressInterval  time.Duration
	ClaimPollInterval time.Duration

	// Capabilities skips DetectAll when set (tests).
	Capabilities *encoders.Capability
	// Encoders overrides Registry(target, caps) (tests).
	Encoders []encoders.Encoder
	// Run replaces the ffmpeg subprocess (tests).
	Run encoders.RunFn
	// Prober replaces ffprobe (tests).
	Prober Prober
}

func (c Config) heartbeatInterval() time.Duration {
	if c.HeartbeatInterval > 0 {
		return c.HeartbeatInterval
	}
	return DefaultHeartbeatInterval
}

func (c Config) progressInterval() time.Duration {
	if c.ProgressInterval > 0 {
		return c.ProgressInterval
	}
	return DefaultProgressInterval
}

func (c Config) claimPollInterval() time.Duration {
	if c.ClaimPollInterval > 0 {
		return c.ClaimPollInterval
	}
	return DefaultClaimPollInterval
}

// capabilities is the full node capabilities shape (ARCHITECTURE.md):
// {encoders: [...], cpu_cores, ffmpeg_version}.
type capabilities struct {
	Encoders      []encoders.EncoderInfo `json:"encoders"`
	CPUCores      int                    `json:"cpu_cores"`
	FFmpegVersion string                 `json:"ffmpeg_version"`
}

// Worker claims jobs through the worker API and runs them. The built-in
// worker and remote workers are the same code (ARCHITECTURE.md).
//
// Lifecycle (task 11): the worker never claims anything until its first
// heartbeat answers, and honours the desired_state the response carries —
// stopped (heartbeat only), running (claim up to the node's concurrency),
// draining (finish what is held, claim nothing, report stopped once idle).
// A stop-now aborts in-flight encodes silently; the sweeper requeues those
// jobs after the heartbeat timeout and the original files stay untouched.
type Worker struct {
	cfg    Config
	log    *slog.Logger
	client *Client
	runner *runner
	sc     *scratch

	mu         sync.Mutex
	desired    int    // concurrency cap from the last heartbeat
	state      string // "" until the first heartbeat, then stopped|running|draining
	active     int    // slots currently holding a token
	activeJobs map[string]bool
	cancels    map[string]context.CancelFunc // abort handles for in-flight jobs
	caps       encoders.Capability
	capsJSON   json.RawMessage
}

// New builds a worker. Run does the lifecycle work.
func New(cfg Config) (*Worker, error) {
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return nil, errors.New("worker: server URL is required")
	}
	if strings.TrimSpace(cfg.Key) == "" {
		return nil, errors.New("worker: node key is required")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	slots := cfg.MaxSlots
	if slots < 1 {
		slots = DefaultMaxSlots
	}
	cfg.MaxSlots = slots

	client := NewClient(cfg.ServerURL, cfg.Key)
	prober := cfg.Prober
	if prober == nil {
		prober = func(ctx context.Context, ffprobePath, path string) (probe.MediaInfo, error) {
			return probe.Probe(ctx, ffprobePath, path)
		}
	}
	w := &Worker{
		cfg:        cfg,
		log:        cfg.Log,
		client:     client,
		sc:         newScratch(cfg.ScratchDir),
		activeJobs: map[string]bool{},
		cancels:    map[string]context.CancelFunc{},
	}
	w.runner = &runner{
		cfg:     cfg,
		client:  client,
		encs:    cfg.Encoders,
		prober:  prober,
		runFn:   cfg.Run,
		plugs:   plugin.NewRunner(cfg.DataDir),
		sc:      w.sc,
		access:  newAccessCache(),
		log:     cfg.Log,
		tracker: &encoderTracker{},
	}
	return w, nil
}

// Run detects hardware, cleans orphaned scratch, then runs the heartbeat
// loop and the claim slots until ctx is done. It returns an error only for
// broken configuration; runtime failures (heartbeat, claim, encodes) are
// logged and retried.
func Run(ctx context.Context, cfg Config) error {
	w, err := New(cfg)
	if err != nil {
		return err
	}
	return w.Run(ctx)
}

// Run starts the worker loops and blocks until ctx is done. Nothing is
// claimed until the first heartbeat answers and says running (task 11).
func (w *Worker) Run(ctx context.Context) error {
	w.detect(ctx)

	// Scratch leftovers belong to jobs of a dead worker process; the
	// server requeues those through the heartbeat timeout.
	w.sc.cleanupOrphans()

	w.mu.Lock()
	w.desired = 1 // until the first heartbeat answers
	w.mu.Unlock()

	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		w.heartbeatLoop(ctx)
	}()

	var wg sync.WaitGroup
	for i := 0; i < w.cfg.MaxSlots; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.slotLoop(ctx)
		}()
	}

	<-ctx.Done()
	wg.Wait()
	<-hbDone
	w.log.Info("worker stopped")
	return nil
}

// detect runs hardware detection (task 04) unless the config preseeds it
// (tests). Detection is cached per process. A worker whose detection fails
// keeps running: encodes fail with clear errors instead of taking the whole
// process down (a role-both serve must keep serving the API).
func (w *Worker) detect(ctx context.Context) {
	if w.cfg.Capabilities != nil {
		w.caps = *w.cfg.Capabilities
		w.runner.caps = *w.cfg.Capabilities
		w.capsJSON = w.capabilitiesPayload(*w.cfg.Capabilities)
		w.log.Info("worker: capabilities preset", "encoders", encoderNames(*w.cfg.Capabilities))
		return
	}
	caps, err := encoders.DetectAll(ctx, w.cfg.FFmpegPath)
	if err != nil {
		w.log.Error("worker: hardware detection failed; encodes will fail until ffmpeg is usable", "err", err)
		w.capsJSON = json.RawMessage(`{}`)
		return
	}
	w.caps = caps
	w.runner.caps = caps
	w.capsJSON = w.capabilitiesPayload(caps)
	w.log.Info("worker: hardware detected",
		"encoders", encoderNames(caps), "ffmpeg", w.cfg.FFmpegOrPath())
}

// capabilitiesPayload assembles the full capabilities shape. The ffmpeg
// version is best effort; it is metadata for the UI, never a gate.
func (w *Worker) capabilitiesPayload(caps encoders.Capability) json.RawMessage {
	raw, err := json.Marshal(capabilities{
		Encoders:      caps.Encoders,
		CPUCores:      runtime.NumCPU(),
		FFmpegVersion: ffmpegVersion(w.cfg.FFmpegOrPath()),
	})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// heartbeatLoop sends a heartbeat immediately and then on the interval,
// refreshing the node record, the active jobs' heartbeats and the
// instruction this worker honours (desired_state, concurrency).
func (w *Worker) heartbeatLoop(ctx context.Context) {
	beat := func() {
		in := HeartbeatInput{
			Capabilities:  w.capsJSON,
			RunningJobIDs: w.activeJobIDs(),
			State:         w.reportState(),
		}
		n, err := w.client.Heartbeat(ctx, in)
		if err != nil {
			if !errors.Is(err, context.Canceled) && ctx.Err() == nil {
				w.log.Warn("worker: heartbeat", "err", err)
			}
			return
		}
		w.applyHeartbeat(n)
	}
	beat()
	ticker := time.NewTicker(w.cfg.heartbeatInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beat()
		}
	}
}

// applyHeartbeat adopts the server's instruction. Going to stopped aborts
// every in-flight encode (stop-now): the runner exits without reporting,
// the job's heartbeat goes stale and the sweeper requeues it.
func (w *Worker) applyHeartbeat(n *HeartbeatState) {
	w.mu.Lock()
	prev := w.state
	w.state = n.DesiredState
	w.desired = n.Concurrency
	ids := make([]string, 0, len(w.cancels))
	for id := range w.cancels {
		ids = append(ids, id)
	}
	w.mu.Unlock()

	if prev != n.DesiredState {
		w.log.Info("worker: node state", "desired_state", n.DesiredState,
			"concurrency", n.Concurrency)
	}
	if n.DesiredState == NodeStateStopped && len(ids) > 0 {
		w.log.Info("worker: node stopped, abandoning in-flight jobs (they return to the queue)",
			"jobs", ids)
		for _, id := range ids {
			w.mu.Lock()
			cancel := w.cancels[id]
			w.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	}
}

// reportState returns the state this heartbeat should report and resolves
// a finished drain: a draining worker with nothing in flight is stopped,
// and reporting that lets the server end the drain.
func (w *Worker) reportState() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == NodeStateDraining && len(w.activeJobs) == 0 {
		w.log.Info("worker: drain complete, reporting stopped")
		w.state = NodeStateStopped
	}
	return w.state
}

// slotLoop claims and runs jobs, one at a time, while it holds a
// concurrency token and the node is started.
func (w *Worker) slotLoop(ctx context.Context) {
	for {
		if !w.acquire(ctx) {
			return
		}
		job, err := w.client.Claim(ctx)
		if err != nil {
			w.release()
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			if isAPIStatus(err, http.StatusConflict) {
				// The server refuses claims from a node that is not
				// running; the heartbeat carries the instruction and the
				// gate below reacts to it.
				if !w.sleepCtx(ctx, w.cfg.claimPollInterval()) {
					return
				}
				continue
			}
			w.log.Warn("worker: claim", "err", err)
			if !w.sleepCtx(ctx, w.cfg.claimPollInterval()) {
				return
			}
			continue
		}
		if job == nil {
			w.release()
			if !w.sleepCtx(ctx, w.cfg.claimPollInterval()) {
				return
			}
			continue
		}
		// Track the claim before anything else: a drain that goes idle
		// must not race a just-claimed job.
		w.addActiveJob(job.ID)
		w.runJob(ctx, job.ID)
		w.removeActiveJob(job.ID)
		w.release()
	}
}

// runJob fetches the spec and hands the job to the runner. The job context
// is cancelable so a stop-now can abandon the encode; the runner exits
// silently on cancel and the sweeper requeues the job.
func (w *Worker) runJob(ctx context.Context, jobID string) {
	log := w.log.With("job", jobID)
	spec, err := w.client.JobSpec(ctx, jobID)
	if err != nil {
		// A job another node owns, or one that vanished: drop it; the
		// sweeper requeues orphaned claims.
		log.Warn("worker: job spec", "err", err)
		return
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.registerCancel(jobID, cancel)
	defer w.unregisterCancel(jobID)

	cancelled := &atomic.Bool{}
	if err := w.runner.run(jobCtx, spec, cancelled); err != nil {
		log.Error("worker: job runner", "err", err)
	}
}

// acquire blocks until the worker may claim — the lifecycle state is known
// and running (task 11: stopped and draining nodes claim nothing) — and a
// concurrency token is free, or the worker shuts down.
func (w *Worker) acquire(ctx context.Context) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		w.mu.Lock()
		may := w.state == NodeStateRunning && w.active < w.desired
		if may {
			w.active++
		}
		w.mu.Unlock()
		if may {
			return true
		}
		if !w.sleepCtx(ctx, 200*time.Millisecond) {
			return false
		}
	}
}

func (w *Worker) release() {
	w.mu.Lock()
	if w.active > 0 {
		w.active--
	}
	w.mu.Unlock()
}

// registerCancel remembers how to abandon an in-flight job (stop-now).
func (w *Worker) registerCancel(jobID string, cancel context.CancelFunc) {
	w.mu.Lock()
	w.cancels[jobID] = cancel
	w.mu.Unlock()
}

func (w *Worker) unregisterCancel(jobID string) {
	w.mu.Lock()
	delete(w.cancels, jobID)
	w.mu.Unlock()
}

// activeJobIDs snapshots the claimed/running job ids for the heartbeat.
func (w *Worker) activeJobIDs() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.activeJobs))
	for id := range w.activeJobs {
		out = append(out, id)
	}
	return out
}

func (w *Worker) addActiveJob(id string) {
	w.mu.Lock()
	w.activeJobs[id] = true
	w.mu.Unlock()
}

func (w *Worker) removeActiveJob(id string) {
	w.mu.Lock()
	delete(w.activeJobs, id)
	w.mu.Unlock()
}

// sleepCtx waits d or until ctx is done; false means shutdown.
func (w *Worker) sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// FFmpegOrPath is the ffmpeg binary path, defaulting to "ffmpeg".
func (c Config) FFmpegOrPath() string {
	if c.FFmpegPath == "" {
		return "ffmpeg"
	}
	return c.FFmpegPath
}

// FFprobeOrPath is the ffprobe binary path, defaulting to "ffprobe".
func (c Config) FFprobeOrPath() string {
	if c.FFprobePath == "" {
		return "ffprobe"
	}
	return c.FFprobePath
}

// encoderNames lists detected backend names for logs.
func encoderNames(caps encoders.Capability) []string {
	out := make([]string, 0, len(caps.Encoders))
	for _, e := range caps.Encoders {
		out = append(out, e.Name)
	}
	return out
}

// ffmpegVersion returns the first line of "ffmpeg -version", or "".
func ffmpegVersion(ffmpegPath string) string {
	out, err := exec.Command(ffmpegPath, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
