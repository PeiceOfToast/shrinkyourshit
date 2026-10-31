// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/discovery"
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
	// ScratchDir is where transfer-mode result uploads land before the
	// server applies them (task 09). Empty falls back to a directory
	// under the OS temp path; main wires the configured scratch dir.
	ScratchDir string
	// DataDir is the data directory (update cache, pre-update backups);
	// task 17. Empty falls back to the OS temp dir.
	DataDir string

	// JobLogPruneInterval overrides the hourly job-log retention cadence
	// (task 27); 0 selects the default. Exists so tests can run the loop
	// at a fast tick.
	JobLogPruneInterval time.Duration

	// Update tunes the in-app update service (task 17): build identity,
	// restart hook and test overrides.
	Update UpdateOptions

	// Discover finds servers on the LAN for the setup wizard's join card
	// (task 08). Nil → the wizard list stays empty. Wired by main to
	// discovery.Discover.
	Discover func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo
	// SelfID is this server's instance id (its built-in node id); entries
	// with the same id are hidden from the wizard's server list.
	SelfID string
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
// auth, settings, stats and transfer.
type Server struct {
	cfg          Config
	log          *slog.Logger
	events       *EventBus
	queue        *Queue
	scanner      *Scanner
	watcher      *Watcher
	scheduler    *Scheduler
	libs         *LibraryManager
	nodes        *NodeRegistry
	auth         *Auth
	settings     *Settings
	stats        *StatsService
	transfer     *Transfer
	update       *UpdateCheck
	integrations *Refresher
	webhooks     *Dispatcher

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex

	// Discovered-servers cache (setup wizard, task 08).
	discMu    sync.Mutex
	discCache []discovery.ServerInfo
	discAt    time.Time
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
	nodes.SetQueue(queue)
	auth := NewAuth(cfg.Store, cfg.Log)
	settings := NewSettings(cfg.Store, cfg.Log)
	stats := NewStats(cfg.Store)
	queue.SetInspector(cfg.Inspect, cfg.FFprobePath)
	transfer := NewTransfer(cfg.Store, cfg.ScratchDir)
	// The update service (task 17) defaults its dirs to the server's: a
	// release cache or backup must never land outside the data dir just
	// because main only set the top-level fields.
	updOpts := cfg.Update
	if updOpts.DataDir == "" {
		updOpts.DataDir = cfg.DataDir
	}
	if updOpts.ScratchDir == "" {
		updOpts.ScratchDir = cfg.ScratchDir
	}
	updateSvc := NewUpdateCheck(events, settings, cfg.Store, updOpts, cfg.Log)
	// External integrations (task 23) and webhooks (task 25) share one
	// outbound hook: the queue hands each lifecycle event to the router,
	// which fans it out to the refresher and the webhook dispatcher. The
	// scanner reports only to the dispatcher (library.scanned).
	integrations := NewRefresher(cfg.Store, cfg.Log)
	webhooks := NewWebhookDispatcher(cfg.Store, cfg.Log, cfg.Update.Current.Version)
	queue.SetNotifier(&notifierRouter{integrations: integrations, webhooks: webhooks})
	scanner.SetNotifier(webhooks)
	return &Server{cfg: cfg, log: cfg.Log, events: events, queue: queue,
		scanner: scanner, watcher: watcher, scheduler: scheduler, libs: libs, nodes: nodes,
		auth: auth, settings: settings, stats: stats, transfer: transfer, update: updateSvc,
		integrations: integrations, webhooks: webhooks}
}

// Events exposes the event bus (the SSE endpoint subscribes here).
func (s *Server) Events() *EventBus { return s.events }

// SetDiscover wires the LAN discovery hook after construction (main sets
// it from config; tests inject fakes). discover is called with the scan
// timeout; selfID hides the server's own announcement.
func (s *Server) SetDiscover(discover func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo, selfID string) {
	s.cfg.Discover = discover
	s.cfg.SelfID = selfID
	s.discCache, s.discAt = nil, time.Time{} // drop any cached scan
}

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

// Transfer exposes the transfer service (worker access checks and result
// uploads; task 09).
func (s *Server) Transfer() *Transfer { return s.transfer }

// Update exposes the in-app update service (task 17).
func (s *Server) Update() *UpdateCheck { return s.update }

// Integrations exposes the refresher (task 23): the API's Test button and
// the queue's completion hook.
func (s *Server) Integrations() *Refresher { return s.integrations }

// Webhooks exposes the webhook dispatcher (task 25): the API's Test button
// and the queue's and scanner's event hook.
func (s *Server) Webhooks() *Dispatcher { return s.webhooks }

// discoveredTTL is how long Discovered reuses the last LAN scan.
const discoveredTTL = 10 * time.Second

// Discovered lists the servers found on the LAN for the setup wizard's
// join card, hiding this server itself (Config.SelfID). Results are
// cached for discoveredTTL so a wizard refresh does not rescan. Without
// Config.Discover the list is always empty.
func (s *Server) Discovered(ctx context.Context) []discovery.ServerInfo {
	if s.cfg.Discover == nil {
		return nil
	}
	s.discMu.Lock()
	defer s.discMu.Unlock()
	if s.discCache != nil && time.Since(s.discAt) < discoveredTTL {
		return s.discCache
	}
	found := s.cfg.Discover(ctx, 2*time.Second)
	var out []discovery.ServerInfo
	for _, info := range found {
		if s.cfg.SelfID != "" && info.ID == s.cfg.SelfID {
			continue // that is us
		}
		out = append(out, info)
	}
	s.discCache, s.discAt = out, time.Now()
	return out
}

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

	// Libraries created before the flow columns exist get their canonical
	// rule graph compiled and stored once (task 14, no manual migration).
	s.libs.backfillFlows(ctx)

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

	s.wg.Add(3)
	go s.sweepLoop(s.ctx)
	go s.pruneJobLogLoop(s.ctx)
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
