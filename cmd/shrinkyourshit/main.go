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
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/webui"
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

// serve starts the HTTP server on the configured port.
func serve(args []string, version string) error {
	cfg, err := config.Load(config.Options{Args: args})
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	static, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embedded web UI: %w", err)
	}

	// The store lives in the data dir; tasks beyond 03 keep everything in
	// it (settings, node key).
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

// worker is the remote worker mode; the claim loop arrives in task 05.
func worker(args []string) error {
	flags := config.NewFlags("shrinkyourshit worker")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return errors.New("worker mode is not implemented yet")
}

func newLogger(cfg config.Config) *slog.Logger {
	level, err := cfg.SlogLevel()
	if err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
