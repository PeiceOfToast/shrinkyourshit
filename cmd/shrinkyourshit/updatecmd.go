// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/PeiceOfToast/shrinkyourshit/internal/config"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
)

// updateCmd is `shrinkyourshit update` (task 16): report the latest
// release (--check), roll back to the previous binary (--rollback), or
// download, verify and swap in the latest one. Every path honours the
// container refusal: a container install updates by pulling the image.
//
// After a successful swap the process restarts the new binary — exit 0
// under systemd (Restart=always brings it back, see restart.go), exec
// anywhere else.
func updateCmd(args []string, info buildInfo, stdout io.Writer) error {
	flags := config.NewFlags("shrinkyourshit update")
	check := flags.Bool("check", false, "report the latest release and exit")
	rollback := flags.Bool("rollback", false, "restore the previous binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromFlags(flags, nil, "")
	if err != nil {
		return err
	}

	opts := update.Options{
		DataDir:    cfg.DataDir,
		ScratchDir: cfg.ScratchDir,
		Current: update.Info{
			Version: info.version,
			Commit:  info.commit,
			Date:    info.date,
		},
	}

	if *rollback {
		exe, err := update.Rollback(opts)
		if err != nil {
			return err
		}
		sayf(stdout, "restored %s from %s", exe, exe+".prev")
		return restartSelf(exe)
	}

	// The feed and the backup switch live in the settings; a fresh
	// install without a database yet falls back to the defaults.
	opts.Channel, opts.SkipBackup = readUpdateSettings(cfg.DataDir)

	// Interruptible: a cancelled check or download leaves the running
	// binary and the library untouched.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *check {
		return updateCheckCmd(ctx, opts, info, stdout)
	}

	applied, err := update.Apply(ctx, opts)
	if err != nil {
		if errors.Is(err, update.ErrUpToDate) {
			sayf(stdout, "already on the latest release (%s)", opts.Current.Version)
			return nil
		}
		return err // includes the typed container refusal
	}

	sayf(stdout, "updated %s -> %s", applied.From, applied.To)
	sayf(stdout, "installed: %s", applied.Path)
	sayf(stdout, "previous version kept at %s", applied.Prev)
	if applied.BackupPath != "" {
		sayf(stdout, "database backup: %s", applied.BackupPath)
	}
	return restartSelf(applied.Path)
}

// sayf prints one progress line; write errors to a dying stdout are not
// actionable.
func sayf(stdout io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stdout, format+"\n", args...)
}

// updateCheckCmd prints current and latest, changed or not, and exits 0.
func updateCheckCmd(ctx context.Context, opts update.Options, info buildInfo, stdout io.Writer) error {
	rel, err := update.Latest(ctx, opts)
	if err != nil {
		return err
	}
	sayf(stdout, "current: %s", info.version)
	sayf(stdout, "latest:  %s (%s channel)", rel.Tag, opts.Channel)
	if rel.Tag == info.version {
		sayf(stdout, "no update available")
	} else {
		sayf(stdout, "update available; run `shrinkyourshit update` to apply it")
	}
	return nil
}

// readUpdateSettings reads the self-update settings from the database
// (channel, backup switch). Defaults apply when the database does not
// exist yet or cannot be opened; a settings read failure never blocks the
// update command.
func readUpdateSettings(dataDir string) (channel string, skipBackup bool) {
	channel = update.ChannelStable
	skipBackup = false // backup_before_update defaults to true
	db := filepath.Join(dataDir, dbFile)
	if _, err := os.Stat(db); err != nil {
		return channel, skipBackup
	}
	st, err := store.New(db)
	if err != nil {
		return channel, skipBackup
	}
	defer func() { _ = st.Close() }()
	raw, err := st.ListSettings(context.Background())
	if err != nil {
		return channel, skipBackup
	}
	switch raw[store.SettingUpdateChannel] {
	case update.ChannelStable, update.ChannelPrerelease:
		channel = raw[store.SettingUpdateChannel]
	}
	if raw[store.SettingBackupBeforeUpdate] == "false" {
		skipBackup = true
	}
	return channel, skipBackup
}
