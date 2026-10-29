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
	"strings"
	"syscall"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/api"
	"github.com/PeiceOfToast/shrinkyourshit/internal/config"
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

// serve starts the HTTP server on the configured port. Role both also runs
// the built-in worker in-process, through the same worker API a remote
// worker uses (ARCHITECTURE.md).
func serve(args []string, version string) error {
	cfg, err := config.Load(config.Options{Args: args})
	if err != nil {
		return err
	}
	if cfg.Role == config.RoleWorker {
		return errors.New("serve does not run worker-only mode; use 'shrinkyourshit worker' with --server")
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

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
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// First serve start creates the built-in node and its key file; later
	// starts reuse both (server.NodeRegistry.EnsureBuiltinNode).
	nodeKey, _, err := svc.Nodes().EnsureBuiltinNode(ctx, cfg.DataDir, server.BuiltinNodeName(cfg.NodeName))
	if err != nil {
		return err
	}

	svc.Start(ctx)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           api.New(version, static, svc),
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
	// same HTTP API a remote worker uses.
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
			if err := workerpkg.Run(ctx, wcfg); err != nil {
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
	}
}

// worker runs a remote worker: it claims jobs from SHRINK_SERVER and encodes
// locally. Pairing arrives in task 08; for now the node key must already
// exist at <data-dir>/node.key.
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

	if cfg.Server == "" {
		return errors.New("worker mode needs a server: set SHRINK_SERVER or --server, e.g. http://192.168.1.10:8765")
	}
	keyPath := filepath.Join(cfg.DataDir, server.NodeKeyFile)
	key, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no node key at %s: join a server first (pairing arrives with task 08)", keyPath)
		}
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("worker starting",
		"server", cfg.Server,
		"node_name", cfg.NodeName,
		"scratch_dir", cfg.ScratchDir,
		"concurrency_hint", "from server settings",
	)
	return workerpkg.Run(ctx, workerpkg.Config{
		ServerURL:   cfg.Server,
		Key:         strings.TrimSpace(string(key)),
		ScratchDir:  cfg.ScratchDir,
		FFmpegPath:  cfg.FFmpegPath,
		FFprobePath: cfg.FFprobePath(),
		Log:         logger,
	})
}

func newLogger(cfg config.Config) *slog.Logger {
	level, err := cfg.SlogLevel()
	if err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
