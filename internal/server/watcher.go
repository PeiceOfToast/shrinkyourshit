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

// Watch timing (task 03): debounce event bursts 2s before scanning; the
// scanner itself waits for file stability before probing. When fsnotify
// cannot watch a path (network mounts), fall back to a poll scan every 5
// minutes.
const (
	DebounceDefault  = 2 * time.Second
	WatchPollDefault = 5 * time.Minute
)

// Watcher watches library folders with fsnotify and triggers scans. One
// watcher state per library; Add replaces any previous watch for the same
// library (path changes restart the watch).
type Watcher struct {
	scanner *Scanner
	log     *slog.Logger

	debounce  time.Duration
	pollEvery time.Duration

	mu      sync.Mutex
	watches map[string]context.CancelFunc
}

// NewWatcher builds a watcher. Durations of zero select the defaults
// (shrink them in tests).
func NewWatcher(scanner *Scanner, log *slog.Logger, debounce, pollEvery time.Duration) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	if debounce == 0 {
		debounce = DebounceDefault
	}
	if pollEvery == 0 {
		pollEvery = WatchPollDefault
	}
	return &Watcher{
		scanner:   scanner,
		log:       log,
		debounce:  debounce,
		pollEvery: pollEvery,
		watches:   map[string]context.CancelFunc{},
	}
}

// Add starts (or restarts) the watch for a library. Libraries with
// watch=false are removed instead. The fsnotify setup happens synchronously
// here, so once Add returns, events for the library path are guaranteed to
// be seen — writes that land a microsecond later cannot fall into a setup
// gap.
func (w *Watcher) Add(lib *store.Library) {
	if lib == nil {
		return
	}
	w.Remove(lib.ID)
	if !lib.Watch {
		return
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.log.Warn("watcher: fsnotify unavailable, polling instead",
			"library", lib.ID, "err", err)
		ctx, cancel := context.WithCancel(context.Background())
		w.mu.Lock()
		w.watches[lib.ID] = cancel
		w.mu.Unlock()
		go w.pollLoop(ctx, lib.ID)
		return
	}
	if err := w.addRecursive(fsw, lib.Path); err != nil {
		// Network mounts and some exotic filesystems cannot be watched.
		w.log.Warn("watcher: cannot watch path, polling instead",
			"library", lib.ID, "path", lib.Path, "err", err)
		_ = fsw.Close()
		ctx, cancel := context.WithCancel(context.Background())
		w.mu.Lock()
		w.watches[lib.ID] = cancel
		w.mu.Unlock()
		go w.pollLoop(ctx, lib.ID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.watches[lib.ID] = cancel
	w.mu.Unlock()
	go w.run(ctx, lib.ID, fsw)
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

// Watching reports whether a watch is registered for the library.
func (w *Watcher) Watching(libraryID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.watches[libraryID]
	return ok
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

// run is the event loop for one library watch. fsw arrives with the watch
// already registered (set up synchronously by Add).
func (w *Watcher) run(ctx context.Context, libraryID string, fsw *fsnotify.Watcher) {
	defer func() { _ = fsw.Close() }()

	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

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
				// A directory appeared (mkdir -p): watch below it too and
				// scan twice — once now, once after the stability window.
				// Files written between the mkdir and the new watch's
				// first event would otherwise be missed.
				if err := w.addRecursive(fsw, ev.Name); err != nil {
					w.log.Debug("watcher: add subdir", "path", ev.Name, "err", err)
				}
				w.scanner.ScanAsync(libraryID)
				time.AfterFunc(w.debounce+StableDefault, func() {
					if ctx.Err() == nil {
						w.scanner.ScanAsync(libraryID)
					}
				})
				continue // directory create alone is not a file change
			}
			// Any file-level change restarts the debounce window; the
			// scan fires once the folder has been quiet for a while. The
			// scanner skips files that are still growing.
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
			// The scanner itself waits for file stability before probing,
			// so a debounced burst goes straight to a scan.
			w.scanner.ScanAsync(libraryID)
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
