// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/fsnotify/fsnotify"
)

// Watch timing defaults (task 03): debounce events 2s, then wait until a
// file's size has been stable for 5s before scanning (files may still be
// mid-copy). When fsnotify cannot watch a path (network mounts), fall back
// to a poll scan every 5 minutes.
const (
	DebounceDefault       = 2 * time.Second
	StableDefault         = 5 * time.Second
	WatchPollDefault      = 5 * time.Minute
	stabilityPollInterval = 500 * time.Millisecond
	stabilityMaxWait      = time.Hour // runaway copy guard
)

// Watcher watches library folders with fsnotify and triggers scans. One
// watcher state per library; Add replaces any previous watch for the same
// library (path changes restart the watch).
type Watcher struct {
	scanner *Scanner
	log     *slog.Logger

	debounce  time.Duration
	stable    time.Duration
	pollEvery time.Duration

	mu      sync.Mutex
	watches map[string]context.CancelFunc
}

// NewWatcher builds a watcher. Durations of zero select the defaults
// (shrink them in tests).
func NewWatcher(scanner *Scanner, log *slog.Logger, debounce, stable, pollEvery time.Duration) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	if debounce == 0 {
		debounce = DebounceDefault
	}
	if stable == 0 {
		stable = StableDefault
	}
	if pollEvery == 0 {
		pollEvery = WatchPollDefault
	}
	return &Watcher{
		scanner:   scanner,
		log:       log,
		debounce:  debounce,
		stable:    stable,
		pollEvery: pollEvery,
		watches:   map[string]context.CancelFunc{},
	}
}

// Add starts (or restarts) the watch for a library. Libraries with
// watch=false are removed instead.
func (w *Watcher) Add(lib *store.Library) {
	if lib == nil {
		return
	}
	w.Remove(lib.ID)
	if !lib.Watch {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.watches[lib.ID] = cancel
	w.mu.Unlock()
	go w.run(ctx, lib.ID, lib.Path)
}

// Remove stops the watch for a library. Missing watches are fine.
func (w *Watcher) Remove(libraryID string) {
	w.mu.Lock()
	cancel, ok := w.watches[libraryID]
	if ok {
		delete(w.watches, libraryID)
	}
	w.mu.Unlock()
	if ok {
		cancel()
	}
}

// Stop tears down every watch.
func (w *Watcher) Stop() {
	w.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(w.watches))
	for id, cancel := range w.watches {
		cancels = append(cancels, cancel)
		delete(w.watches, id)
	}
	w.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// run is the event loop for one library watch.
func (w *Watcher) run(ctx context.Context, libraryID, root string) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.log.Warn("watcher: fsnotify unavailable, polling instead",
			"library", libraryID, "err", err)
		w.pollLoop(ctx, libraryID)
		return
	}
	defer func() { _ = fsw.Close() }()

	pollOnly := false
	if err := w.addRecursive(fsw, root); err != nil {
		// Network mounts and some exotic filesystems cannot be watched.
		w.log.Warn("watcher: cannot watch path, polling instead",
			"library", libraryID, "path", root, "err", err)
		pollOnly = true
	}

	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		pending map[string]struct{}
	)
	if pollOnly {
		w.pollLoop(ctx, libraryID)
		return
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return // watcher closed
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if st, err := os.Stat(ev.Name); err == nil && st.IsDir() && ev.Has(fsnotify.Create) {
				// A directory appeared (mkdir -p): watch below it too.
				if err := w.addRecursive(fsw, ev.Name); err != nil {
					w.log.Debug("watcher: add subdir", "path", ev.Name, "err", err)
				}
				continue // directory create alone is not a file change
			}
			if pending == nil {
				pending = map[string]struct{}{}
			}
			pending[ev.Name] = struct{}{}
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(w.debounce)
			timerC = timer.C
		case fe, ok := <-fsw.Errors:
			if !ok {
				return
			}
			w.log.Debug("watcher: error", "library", libraryID, "err", fe)
		case <-timerC:
			timer = nil
			timerC = nil
			paths := make([]string, 0, len(pending))
			for p := range pending {
				paths = append(paths, p)
			}
			pending = nil
			go w.stableThenScan(ctx, libraryID, paths)
		}
	}
}

// pollLoop is the fallback when the path cannot be watched: scan on a
// fixed interval until stopped.
func (w *Watcher) pollLoop(ctx context.Context, libraryID string) {
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scanner.ScanAsync(libraryID)
		}
	}
}

// stableThenScan waits until every changed path has a stable size (5s of no
// growth and an mtime older than the stability window), then scans. Files
// that vanish while waiting are dropped (removed or renamed).
func (w *Watcher) stableThenScan(ctx context.Context, libraryID string, paths []string) {
	deadline := time.Now().Add(stabilityMaxWait)
	sizes := map[string]int64{}
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Now().After(deadline) {
			break // give up waiting; scan anyway
		}
		stable := true
		for _, p := range paths {
			st, err := os.Stat(p)
			if err != nil {
				continue // gone: rename/remove, nothing to wait for
			}
			if st.IsDir() {
				continue
			}
			last, seen := sizes[p]
			if !seen || last != st.Size() || time.Since(st.ModTime()) < w.stable {
				sizes[p] = st.Size()
				stable = false
			}
		}
		if stable {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(stabilityPollInterval):
		}
	}
	w.scanner.ScanAsync(libraryID)
}

// addRecursive watches root and every subdirectory (hidden dirs skipped,
// matching the scanner's walk).
func (w *Watcher) addRecursive(fsw *fsnotify.Watcher, root string) error {
	if err := fsw.Add(root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subdirs are not fatal for watching
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isHidden(d.Name()) {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		if err := fsw.Add(path); err != nil {
			w.log.Debug("watcher: add", "path", path, "err", err)
		}
		return nil
	})
}
