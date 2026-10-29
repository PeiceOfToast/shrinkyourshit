// SPDX-License-Identifier: AGPL-3.0-only

// Command shrinkyourshit is the single binary for all roles. Subcommands:
// serve (default), worker, version.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/api"
	"github.com/PeiceOfToast/shrinkyourshit/internal/config"
	"github.com/PeiceOfToast/shrinkyourshit/internal/discovery"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/webui"
	workerpkg "github.com/PeiceOfToast/shrinkyourshit/internal/worker"
	"github.com/spf13/pflag"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "v0.0.0"

func main() {
	if err := run(os.Args[1:], version); err != nil {
		fmt.Fprintln(os.Stderr, "shrinkyourshit:", err)
		os.Exit(1)
	}
}

func run(args []string, version string) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "serve":
		err := serve(args[1:], version)
		// pflag has already printed usage; --help is not an error.
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	case "worker":
		err := worker(args[1:])
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	case "version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want serve, worker or version)", args[0])
	}
}

// discoverFn builds the LAN discovery function for a config.
func discoverFn(cfg config.Config) func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo {
	return func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo {
		return discovery.Discover(ctx, timeout, discovery.Options{
			MDNS:      cfg.DiscoveryMDNS,
			Broadcast: cfg.DiscoveryBroadcast,
			Port:      cfg.DiscoveryPort,
		})
	}
}

// announce starts LAN advertising for a server (mDNS + broadcast probe
// answer). It runs until its context is done; failures are logged, never
// fatal (the API works without discovery).
func announce(ctx context.Context, cfg config.Config, selfID, v string, log *slog.Logger) {
	err := discovery.Announce(ctx, discovery.Announcement{
		ID:      selfID,
		Name:    cfg.NodeName,
		Version: v,
		Port:    cfg.Port,
		Options: discovery.Options{
			MDNS:      cfg.DiscoveryMDNS,
			Broadcast: cfg.DiscoveryBroadcast,
			Port:      cfg.DiscoveryPort,
		},
		Log: log,
	})
	if err != nil && ctx.Err() == nil {
		log.Warn("server: discovery announce stopped", "err", err)
	}
}

// serve starts the HTTP server on the configured port. Role both also runs
// the built-in worker in-process, through the same worker API a remote
// worker uses (ARCHITECTURE.md). A role of worker turns serve into the
// worker flow only: that is how a machine that joined a server keeps
// working when the same container or systemd unit starts it again.
func serve(args []string, version string) error {
	cfg, err := config.Load(config.Options{Args: args})
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Role worker: no local server, just the worker against SHRINK_SERVER
	// (or a discovered one).
	if cfg.Role == config.RoleWorker {
		return runWorkerFlow(ctx, cfg, logger)
	}

	static, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embedded web UI: %w", err)
	}

	// The store lives in the data dir; everything else lands there too
	// (settings, node key).
	st, err := store.New(filepath.Join(cfg.DataDir, "shrinkyourshit.db"))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	svc := server.New(server.Config{
		Store:       st,
		Log:         logger,
		FFprobePath: cfg.FFprobePath(),
		ScratchDir:  cfg.ScratchDir,
	})

	// First serve start creates the built-in node and its key file; later
	// starts reuse both (server.NodeRegistry.EnsureBuiltinNode).
	nodeKey, builtin, err := svc.Nodes().EnsureBuiltinNode(ctx, cfg.DataDir, server.BuiltinNodeName(cfg.NodeName))
	if err != nil {
		return err
	}

	svc.Start(ctx)

	// LAN discovery: advertise this server and answer probes; the setup
	// wizard's join card reads the discovered list (own id hidden).
	announceCtx, stopAnnounce := context.WithCancel(ctx)
	defer stopAnnounce()
	go announce(announceCtx, cfg, builtin.ID, version, logger)
	svc.SetDiscover(discoverFn(cfg), builtin.ID)

	// The setup wizard's "join" choice: persist role=worker, then switch
	// this process over (HTTP down, pairing flow, worker loop).
	type joinChoice struct{ url, token string }
	joined := make(chan joinChoice, 1)
	handler := api.New(version, static, svc, api.WithJoin(func(serverURL, token string) error {
		if err := config.JoinExisting(cfg.DataDir, serverURL); err != nil {
			return err
		}
		joined <- joinChoice{url: serverURL, token: token}
		return nil
	}), api.WithBuiltinNode(builtin.ID))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	logger.Info("server listening",
		"addr", srv.Addr,
		"role", cfg.Role,
		"version", version,
		"data_dir", cfg.DataDir,
	)

	// Role both starts the built-in worker against this server through the
	// same HTTP API a remote worker uses. It has its own context so a
	// mid-run join can stop it before the worker switch.
	workerCtx, cancelBuiltin := context.WithCancel(ctx)
	defer cancelBuiltin()
	if cfg.Role == config.RoleBoth {
		go func() {
			wcfg := workerpkg.Config{
				ServerURL:   fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
				Key:         nodeKey,
				ScratchDir:  cfg.ScratchDir,
				FFmpegPath:  cfg.FFmpegPath,
				FFprobePath: cfg.FFprobePath(),
				Log:         logger,
			}
			if err := workerpkg.Run(workerCtx, wcfg); err != nil && workerCtx.Err() == nil {
				logger.Error("built-in worker stopped", "err", err)
			}
		}()
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		svc.Stop()
		return nil
	case join := <-joined:
		// The wizard joined a server: this machine becomes a worker.
		logger.Info("joining server; this machine is switching to worker mode", "server", join.url)
		cancelBuiltin()
		stopAnnounce()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown of local server during join", "err", err)
		}
		svc.Stop()
		joinCfg := cfg
		joinCfg.Server = join.url
		return runWorkerFlow(ctx, joinCfg, logger, join.token)
	}
}

// runWorkerFlow pairs (when needed) and runs the worker loop until ctx is
// done. It blocks, so a docker or systemd unit that starts the default
// subcommand keeps a joined machine working across restarts. token, when
// given, overrides cfg.Token (the join request carries it explicitly).
func runWorkerFlow(ctx context.Context, cfg config.Config, logger *slog.Logger, token ...string) error {
	pairToken := cfg.Token
	if len(token) > 0 && token[0] != "" {
		pairToken = token[0]
	}
	serverURL, key, err := workerpkg.EnsurePaired(ctx, workerpkg.PairConfig{
		ServerURL: cfg.Server,
		Token:     pairToken,
		NodeName:  cfg.NodeName,
		DataDir:   cfg.DataDir,
		Discover:  discoverFn(cfg),
		Log:       logger,
	})
	if err != nil {
		return err
	}
	if serverURL == "" {
		return errors.New("worker: no server URL resolved; set SHRINK_SERVER")
	}
	logger.Info("worker starting",
		"server", serverURL,
		"node_name", cfg.NodeName,
		"scratch_dir", cfg.ScratchDir,
	)
	return workerpkg.Run(ctx, workerpkg.Config{
		ServerURL:   serverURL,
		Key:         key,
		ScratchDir:  cfg.ScratchDir,
		FFmpegPath:  cfg.FFmpegPath,
		FFprobePath: cfg.FFprobePath(),
		Log:         logger,
	})
}

// worker runs a remote worker: it pairs on first run (task 08) and then
// claims jobs from the server and encodes locally.
func worker(args []string) error {
	cfg, err := config.Load(config.Options{Args: args})
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWorkerFlow(ctx, cfg, logger)
}

func newLogger(cfg config.Config) *slog.Logger {
	level, err := cfg.SlogLevel()
	if err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
