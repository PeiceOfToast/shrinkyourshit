// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Config tunes the server services. Zero values select the defaults; the
// probe override exists for tests.
type Config struct {
	Store store.Store
	Log   *slog.Logger

	// FFprobePath is the ffprobe binary; empty means "on PATH".
	FFprobePath string
	// Prober overrides ffprobe (tests).
	Prober Prober
	// ProbeConcurrency bounds simultaneous probes; default 4.
	ProbeConcurrency int
	// HeartbeatTimeout is the stale-job threshold; default 120s.
	HeartbeatTimeout time.Duration
	// SweepInterval is how often stale jobs are checked; default 30s.
	SweepInterval time.Duration
	// Debounce collapses watch event bursts; default 2s.
	Debounce time.Duration
	// StableWindow is how long a file must not grow before scanning; default 5s.
	StableWindow time.Duration
	// WatchPollInterval is the fallback poll period; default 5m.
	WatchPollInterval time.Duration
	// Inspect overrides output inspection on job completion (tests):
	// fingerprint, size and probe of the produced file. Nil uses stat +
	// fingerprint + ffprobe (the server reads the library directly in
	// shared mode; transfer mode arrives with task 09).
	Inspect ResultInspector
}

// ResultInfo is what job completion records about the produced file.
type ResultInfo struct {
	Fingerprint string
	Size        int64
	Mtime       int64 // unix seconds
	Info        probe.MediaInfo
}

// ResultInspector inspects one output file (see Config.Inspect).
type ResultInspector func(ctx context.Context, ffprobePath, path string) (ResultInfo, error)

// defaultInspector stats, fingerprints and probes the file with ffprobe.
func defaultInspector(ctx context.Context, ffprobePath, path string) (ResultInfo, error) {
	info, err := probe.Probe(ctx, ffprobePath, path)
	if err != nil {
		return ResultInfo{}, err
	}
	fp, err := probe.Fingerprint(path)
	if err != nil {
		return ResultInfo{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return ResultInfo{}, err
	}
	return ResultInfo{
		Fingerprint: fp,
		Size:        st.Size(),
		Mtime:       st.ModTime().Unix(),
		Info:        info,
	}, nil
}

// Server holds the server-side services and their lifecycle: event bus,
// queue, scanner, watcher, scheduler, library manager, node registry,
// auth and settings.
type Server struct {
	cfg       Config
	log       *slog.Logger
	events    *EventBus
	queue     *Queue
	scanner   *Scanner
	watcher   *Watcher
	scheduler *Scheduler
	libs      *LibraryManager
	nodes     *NodeRegistry
	auth      *Auth
	settings  *Settings
	stats     *StatsService

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

// New builds the services. Call Start to begin scanning and watching.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	events := NewEventBus(cfg.Log)
	queue := NewQueue(cfg.Store, events, cfg.Log)
	scanner := NewScanner(cfg.Store, queue, events, ScannerConfig{
		FFprobePath:      cfg.FFprobePath,
		Prober:           cfg.Prober,
		ProbeConcurrency: cfg.ProbeConcurrency,
		StableWindow:     cfg.StableWindow,
	}, cfg.Log)
	watcher := NewWatcher(scanner, cfg.Log, cfg.Debounce, cfg.WatchPollInterval)
	scheduler := NewScheduler(cfg.Store, scanner, cfg.Log, 0)
	libs := NewLibraryManager(cfg.Store, queue, events, watcher, cfg.Log)
	libs.SetScanHook(scanner.ScanAsync)
	nodes := NewNodeRegistry(cfg.Store, events, cfg.Log)
	auth := NewAuth(cfg.Store, cfg.Log)
	settings := NewSettings(cfg.Store, cfg.Log)
	stats := NewStats(cfg.Store)
	queue.SetInspector(cfg.Inspect, cfg.FFprobePath)
	return &Server{cfg: cfg, log: cfg.Log, events: events, queue: queue,
		scanner: scanner, watcher: watcher, scheduler: scheduler, libs: libs, nodes: nodes,
		auth: auth, settings: settings, stats: stats}
}

// Events exposes the event bus (the SSE endpoint subscribes here).
func (s *Server) Events() *EventBus { return s.events }

// Nodes exposes the node registry (worker auth, heartbeat, task 08 pairing).
func (s *Server) Nodes() *NodeRegistry { return s.nodes }

// Libraries exposes the library manager (library API endpoints).
func (s *Server) Libraries() *LibraryManager { return s.libs }

// Queue exposes the queue (jobs API endpoints; task 05 adds worker claim).
func (s *Server) Queue() *Queue { return s.queue }

// Scanner exposes the scanner (the scan endpoint).
func (s *Server) Scanner() *Scanner { return s.scanner }

// Store exposes the underlying store (read-only listing in the API).
func (s *Server) Store() store.Store { return s.cfg.Store }

// Auth exposes the auth service (setup, login, sessions, passwords).
func (s *Server) Auth() *Auth { return s.auth }

// Settings exposes the settings service (GET/PUT /settings).
func (s *Server) Settings() *Settings { return s.settings }

// Stats exposes the stats service (GET /stats; task 07).
func (s *Server) Stats() *StatsService { return s.stats }

// Start launches the background loops: an initial scan per library, a
// watcher for every watched library, the schedule loop and the stale-job
// sweeper. It returns immediately; Stop waits for shutdown.
func (s *Server) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.scanner.setContext(s.ctx)

	libs, err := s.libs.List(ctx)
	if err != nil {
		s.log.Error("server: list libraries at startup", "err", err)
	}
	for _, lib := range libs {
		if lib.Schedule != "" {
			if _, err := ParseSchedule(lib.Schedule); err != nil {
				s.log.Error("server: bad schedule in stored library",
					"library", lib.ID, "schedule", lib.Schedule, "err", err)
			}
		}
		if lib.Watch {
			s.watcher.Add(lib)
		}
		s.scanner.ScanAsync(lib.ID) // startup scan
	}

	s.wg.Add(2)
	go s.sweepLoop(s.ctx)
	go func() {
		defer s.wg.Done()
		s.scheduler.Run(s.ctx)
	}()
	s.log.Info("server services started")
}

// Stop cancels all loops and waits for them to finish.
func (s *Server) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	s.watcher.Stop()
	s.wg.Wait()
}

// sweepLoop checks for heartbeat-timeout jobs on a fixed interval.
func (s *Server) sweepLoop(ctx context.Context) {
	defer s.wg.Done()
	timeout := s.cfg.HeartbeatTimeout
	if timeout <= 0 {
		timeout = HeartbeatTimeoutDefault
	}
	interval := s.cfg.SweepInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.queue.Sweep(ctx, timeout); err != nil && !errors.Is(err, context.Canceled) {
				s.log.Error("server: sweep", "err", err)
			}
		}
	}
}
