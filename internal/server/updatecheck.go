// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
)

// UpdateOptions tunes the update-check service. Zero values are fine; the
// overrides exist for tests (and main wires the build identity).
type UpdateOptions struct {
	// DataDir holds the release cache and the pre-update backups; empty
	// falls back to the OS temp dir so a mis-wired server never writes a
	// cache file into the working directory.
	DataDir string
	// ScratchDir receives downloads before verification; empty falls back
	// to the OS temp dir.
	ScratchDir string
	// Current is the running binary's build identity.
	Current update.Info
	// Restart runs after a successful apply, once the events have had a
	// moment to flush (main execs the freshly installed binary). Nil means
	// no restart happens.
	Restart func(exe string) error
	// RestartDelay is how long a successful apply waits before restarting,
	// so the SSE events and the 202 answer reach the UI first. Default 2s.
	RestartDelay time.Duration
	// BackgroundDelay is how long StartBackgroundCheck waits before its
	// first check, so startup is never blocked on the network.
	// Default 3s.
	BackgroundDelay time.Duration
	// CheckWindow is the minimum gap between two release refreshes (the
	// "Check now" rate limit). Default 1 minute.
	CheckWindow time.Duration

	// Test overrides.
	IsContainer func() bool
	HTTP        *http.Client
	APIBase     string
	Executable  func() (string, error)
	Now         func() time.Time
}

// checkResult is one finished release check: the release on the configured
// channel, or the failure text when the check could not complete.
type checkResult struct {
	release update.Release
	errText string
}

// ApplyProgress is the live state of an in-app update apply, surfaced on
// the status endpoint (task 17) so the About card can render progress.
type ApplyProgress struct {
	Active bool   `json:"active"`
	Stage  string `json:"stage,omitempty"`
	Error  string `json:"error,omitempty"`
}

// UpdateStatus is the GET /api/v1/update/status body (API.md, task 17).
// latest, release_url, release_notes and published_at are empty until a
// check has succeeded; reason explains a "no" that is not simply
// up-to-date.
type UpdateStatus struct {
	Current             string         `json:"current"`
	Latest              string         `json:"latest,omitempty"`
	UpdateAvailable     bool           `json:"update_available"`
	ReleaseURL          string         `json:"release_url,omitempty"`
	ReleaseNotes        string         `json:"release_notes,omitempty"`
	PublishedAt         string         `json:"published_at,omitempty"`
	SelfUpdateSupported bool           `json:"self_update_supported"`
	Reason              string         `json:"reason,omitempty"`
	Progress            *ApplyProgress `json:"progress,omitempty"`
}

// UpdateCheck is the in-app update service (task 17). It checks the
// release feed on demand and once in the background, and applies an update
// only on an explicit call — nothing here applies anything by itself.
type UpdateCheck struct {
	bus      *EventBus
	settings *Settings
	st       store.Store
	opts     UpdateOptions
	log      *slog.Logger

	mu          sync.Mutex
	last        *checkResult
	lastRefresh time.Time
	applying    bool
	progress    *ApplyProgress
}

// NewUpdateCheck builds the service.
func NewUpdateCheck(bus *EventBus, settings *Settings, st store.Store, opts UpdateOptions, log *slog.Logger) *UpdateCheck {
	if log == nil {
		log = slog.Default()
	}
	if opts.DataDir == "" {
		opts.DataDir = os.TempDir()
	}
	if opts.ScratchDir == "" {
		opts.ScratchDir = os.TempDir()
	}
	if opts.RestartDelay <= 0 {
		opts.RestartDelay = 2 * time.Second
	}
	if opts.BackgroundDelay <= 0 {
		opts.BackgroundDelay = 3 * time.Second
	}
	if opts.CheckWindow <= 0 {
		opts.CheckWindow = time.Minute
	}
	return &UpdateCheck{bus: bus, settings: settings, st: st, opts: opts, log: log}
}

func (u *UpdateCheck) now() time.Time {
	if u.opts.Now != nil {
		return u.opts.Now()
	}
	return time.Now()
}

func (u *UpdateCheck) isContainer() bool {
	if u.opts.IsContainer != nil {
		return u.opts.IsContainer()
	}
	return update.IsContainer()
}

// Runtime names how this install runs (the version endpoint reports it):
// container or binary, honouring the test override when set.
func (u *UpdateCheck) Runtime() string {
	if u.isContainer() {
		return update.RuntimeContainer
	}
	return update.RuntimeBinary
}

// stageNames are the apply stages the About card renders in plain words;
// stageLabel maps an update stage to its label.
var stageNames = map[string]string{
	update.StageChecking:    "Looking up the latest release…",
	update.StageBackup:      "Backing up the database…",
	update.StageDownloading: "Downloading the new version…",
	update.StageVerifying:   "Verifying the download…",
	update.StageSwapping:    "Installing…",
	progressStarting:        "Starting the update…",
	progressDone:            "Installed. The app is restarting…",
}

const (
	progressStarting = "starting"
	progressDone     = "done"
)

// StageLabel returns the plain-language label of an update stage, for the
// About card's progress line.
func StageLabel(stage string) string {
	if label, ok := stageNames[stage]; ok {
		return label
	}
	return stage
}

// SelfUpdateUnsupported is the typed error behind the 409
// self_update_unsupported answer; Reason is the human-facing text that
// names the fix (the docker commands for a container, the permission or
// build note otherwise).
type SelfUpdateUnsupported struct{ Reason string }

func (e *SelfUpdateUnsupported) Error() string { return e.Reason }

// ErrAlreadyApplying marks a second apply while one is still running
// (409 update_in_progress).
var ErrAlreadyApplying = errors.New("an update is already running")

// ErrUpdateRateLimited marks a refresh refused by the once-per-minute
// window (429 rate_limited).
var ErrUpdateRateLimited = errors.New("update was checked less than a minute ago")

// support reports whether this install can update itself, and the reason
// it cannot when it must not (container, dev build, read-only install dir).
func (u *UpdateCheck) support() (bool, string) {
	if u.isContainer() {
		return false, update.ContainerReason()
	}
	if u.opts.Current.Version == "" || u.opts.Current.Version == "v0.0.0" {
		return false, "This build was made without release information, so it cannot update itself. Install a released build to get in-app updates."
	}
	exe, err := u.executablePath()
	if err != nil {
		return false, fmt.Sprintf("The install location could not be determined (%v), so the app cannot update itself.", err)
	}
	if !writableDir(filepath.Dir(exe)) {
		return false, fmt.Sprintf("The install folder (%s) is not writable, so the app cannot replace its own binary. Check the folder's permissions.", filepath.Dir(exe))
	}
	return true, ""
}

// writableDir reports whether a file can be created in dir (the install
// dir must accept the staged binary for an update to be possible).
func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".shrink-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func (u *UpdateCheck) executablePath() (string, error) {
	if u.opts.Executable != nil {
		return u.opts.Executable()
	}
	return os.Executable()
}

// Snapshot builds the current status without touching the network.
func (u *UpdateCheck) Snapshot() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.snapshotLocked()
}

// snapshotLocked builds the status from the current state. The caller
// holds mu.
func (u *UpdateCheck) snapshotLocked() UpdateStatus {
	supported, reason := u.support()
	out := UpdateStatus{
		Current:             u.opts.Current.Version,
		SelfUpdateSupported: supported,
	}
	if u.progress != nil {
		p := *u.progress
		out.Progress = &p
	}
	last := u.last
	switch {
	// The fix-naming reason (container, dev build, read-only install
	// dir) outranks everything: it is what the operator must act on.
	case reason != "":
		out.Reason = reason
	case last == nil:
		// No check has finished yet (the background one may still be on
		// its delay): answer immediately with the not-checked reason
		// (API.md, task 17).
		out.Reason = "not checked yet"
	case last.errText != "":
		// A quiet failure: no banner, the About card shows this text.
		out.Reason = last.errText
	default:
		out.Latest = last.release.Tag
		out.ReleaseURL = last.release.URL
		out.ReleaseNotes = last.release.Notes
		if !last.release.Published.IsZero() {
			out.PublishedAt = last.release.Published.UTC().Format(time.RFC3339)
		}
		out.UpdateAvailable = last.release.Tag != u.opts.Current.Version
		if !out.UpdateAvailable {
			out.Reason = "already on the latest release"
		}
	}
	return out
}

// Check forces a release refresh (the "Check now" button). Rate limited to
// one refresh per window (a minute): past it the call answers
// ErrUpdateRateLimited and the handler maps that to 429. A failed network
// check is not an error here — it lands in the status as a quiet reason,
// so the About card can show it and nothing else changes.
func (u *UpdateCheck) Check(ctx context.Context) (UpdateStatus, error) {
	u.mu.Lock()
	if !u.lastRefresh.IsZero() && u.now().Sub(u.lastRefresh) < u.opts.CheckWindow {
		status := u.snapshotLocked()
		u.mu.Unlock()
		return status, ErrUpdateRateLimited
	}
	u.lastRefresh = u.now()
	u.mu.Unlock()

	channel, err := u.currentChannel(ctx)
	if err != nil {
		return UpdateStatus{}, err
	}

	rel, checkErr := update.Latest(ctx, u.latestOptions(channel))
	result := &checkResult{}
	if checkErr != nil {
		result.errText = fmt.Sprintf("Could not check for updates: %v", checkErr)
		u.log.Warn("update check failed", "err", checkErr)
	} else {
		result.release = rel
	}

	u.mu.Lock()
	u.last = result
	status := u.snapshotLocked()
	u.mu.Unlock()
	return status, nil
}

// currentChannel reads the configured release feed from settings.
func (u *UpdateCheck) currentChannel(ctx context.Context) (string, error) {
	values, err := u.settings.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("update: read settings: %w", err)
	}
	if values.UpdateChannel == "" {
		return update.ChannelStable, nil
	}
	return values.UpdateChannel, nil
}

// latestOptions builds the update-package options for a check.
func (u *UpdateCheck) latestOptions(channel string) update.Options {
	return update.Options{
		DataDir:    u.opts.DataDir,
		ScratchDir: u.opts.ScratchDir,
		Channel:    channel,
		Current:    u.opts.Current,
		HTTP:       u.opts.HTTP,
		APIBase:    u.opts.APIBase,
		Executable: u.opts.Executable,
		Now:        u.opts.Now,
		Log:        u.log,
	}
}

// StartBackgroundCheck runs one release check in the background after a
// short delay, unless the update_check setting turned checks off. It never
// blocks startup and never applies anything; failures stay in the log and
// the status endpoint's reason.
func (u *UpdateCheck) StartBackgroundCheck(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(u.opts.BackgroundDelay):
		}
		values, err := u.settings.Get(ctx)
		if err != nil {
			u.log.Warn("update: background check skipped, settings unreadable", "err", err)
			return
		}
		if !values.UpdateCheck {
			u.log.Info("update: automatic checks are off")
			return
		}
		if _, err := u.Check(ctx); err != nil && !errors.Is(err, ErrUpdateRateLimited) {
			u.log.Info("update: background check did not complete", "err", err)
		}
	}()
}

// Apply starts an in-app update on a binary install and reports progress
// as update.* events on the bus. It refuses container installs (and any
// other unsupported shape) synchronously so the caller can answer 409 with
// the fix. The returned error types:
//
//   - *SelfUpdateUnsupported → 409 self_update_unsupported
//   - ErrAlreadyApplying     → 409 update_in_progress
func (u *UpdateCheck) Apply(ctx context.Context) error {
	if supported, reason := u.support(); !supported {
		return &SelfUpdateUnsupported{Reason: reason}
	}
	u.mu.Lock()
	if u.applying {
		u.mu.Unlock()
		return ErrAlreadyApplying
	}
	u.applying = true
	u.setProgressLocked(&ApplyProgress{Active: true, Stage: progressStarting})
	u.mu.Unlock()
	u.publishProgress(progressStarting)

	go u.runApply()
	return nil
}

// setProgressLocked stores the progress snapshot. The caller holds mu.
func (u *UpdateCheck) setProgressLocked(p *ApplyProgress) {
	u.progress = p
}

// publishProgress emits one update.progress event.
func (u *UpdateCheck) publishProgress(stage string) {
	u.bus.Publish(EventUpdateProgress, map[string]string{"stage": stage})
}

// runApply performs the update outside any request context (the request
// returns 202 immediately). Progress stages stream as events; the final
// state is one update.succeeded or update.failed event.
func (u *UpdateCheck) runApply() {
	ctx := context.Background()
	opts := u.applyOptions()
	opts.Progress = func(stage string) {
		u.mu.Lock()
		u.setProgressLocked(&ApplyProgress{Active: true, Stage: stage})
		u.mu.Unlock()
		u.publishProgress(stage)
	}

	applied, err := update.Apply(ctx, opts)
	if err != nil {
		u.mu.Lock()
		u.setProgressLocked(&ApplyProgress{Active: false, Error: err.Error()})
		u.applying = false
		u.mu.Unlock()
		u.log.Warn("in-app update failed", "err", err)
		u.bus.Publish(EventUpdateFailed, map[string]string{"error": err.Error()})
		return
	}

	u.mu.Lock()
	u.setProgressLocked(&ApplyProgress{Active: false, Stage: progressDone})
	u.applying = false
	u.mu.Unlock()
	u.log.Info("in-app update installed", "from", applied.From, "to", applied.To)
	u.bus.Publish(EventUpdateSucceeded, map[string]string{
		"from": applied.From,
		"to":   applied.To,
	})

	if u.opts.Restart == nil {
		return
	}
	// Give the events and the 202 answer a moment to reach the UI before
	// the process hands itself over to the new binary.
	time.Sleep(u.opts.RestartDelay)
	if err := u.opts.Restart(applied.Path); err != nil {
		u.log.Error("update: restart after apply", "err", err)
	}
}

// applyOptions builds the update-package options for an apply: the current
// channel and the store's consistent database snapshot.
func (u *UpdateCheck) applyOptions() update.Options {
	channel := update.ChannelStable
	if values, err := u.settings.Get(context.Background()); err == nil && values.UpdateChannel != "" {
		channel = values.UpdateChannel
	}
	opts := u.latestOptions(channel)
	if u.st != nil {
		opts.Backup = u.st.Backup
	}
	return opts
}

// Applying reports whether an in-app update is currently running.
func (u *UpdateCheck) Applying() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.applying
}
