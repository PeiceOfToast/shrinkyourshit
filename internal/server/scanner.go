// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// mediaExtensions is the list of file types the scanner considers media
// (task 03). Everything else in a library folder is ignored.
var mediaExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".ts": true, ".wmv": true, ".flv": true, ".webm": true,
	".mp3": true, ".flac": true, ".aac": true, ".m4a": true,
	".ogg": true, ".opus": true, ".wav": true,
}

// Prober probes one file. It is a function type so tests can replace
// ffprobe entirely; production uses probe.Probe.
type Prober func(ctx context.Context, ffprobePath, path string) (probe.MediaInfo, error)

func defaultProber(ctx context.Context, ffprobePath, path string) (probe.MediaInfo, error) {
	return probe.Probe(ctx, ffprobePath, path)
}

// Stability timing (task 03): a changed file must not grow for this long
// (and its mtime must be at least this old) before it is probed. The poll
// interval and max wait bound the wait.
const (
	StableDefault         = 5 * time.Second
	stabilityPollInterval = 250 * time.Millisecond
	stabilityMaxWait      = time.Hour
)

// ScanResult summarizes one library scan.
type ScanResult struct {
	Seen      int // media files found on disk
	Probed    int // ffprobe runs
	Reused    int // re-evaluated from the cached probe without ffprobe
	Skipped   int // fingerprint unchanged, nothing to do
	Queued    int // jobs newly queued
	Satisfied int // files (re)classified as satisfied
	Missing   int // DB rows marked missing (no longer on disk)
	Errors    int // probe or store failures for single files
}

// Scanner walks libraries, probes changed files, applies Evaluate and
// queues jobs for files that need work. Scans of one library serialize on a
// per-library mutex. Before probing a changed file it waits until the file
// is stable (size unchanged and mtime older than the stability window), so
// a file that is still being copied is never probed or queued part-way.
type Scanner struct {
	st          store.Store
	queue       *Queue
	events      *EventBus
	log         *slog.Logger
	ffprobePath string
	prober      Prober
	stable      time.Duration // stability window before probing
	procs       chan struct{} // bounds simultaneous fingerprint+probe work
	locks       *libraryLocks

	ctx     context.Context // lifecycle context for async scans
	scanMu  sync.Mutex
	running map[string]bool
	rerun   map[string]bool
}

// ScannerConfig tunes a Scanner. Zero values select the defaults.
type ScannerConfig struct {
	// FFprobePath is the ffprobe binary; empty means "on PATH".
	FFprobePath string
	// Prober overrides ffprobe (tests).
	Prober Prober
	// ProbeConcurrency bounds simultaneous probes; default 4.
	ProbeConcurrency int
	// StableWindow is how long a file must not change before probing;
	// default 5s (task 03).
	StableWindow time.Duration
}

// NewScanner builds a scanner.
func NewScanner(st store.Store, queue *Queue, events *EventBus, cfg ScannerConfig, log *slog.Logger) *Scanner {
	if log == nil {
		log = slog.Default()
	}
	prober := cfg.Prober
	if prober == nil {
		prober = defaultProber
	}
	procs := cfg.ProbeConcurrency
	if procs <= 0 {
		procs = 4
	}
	stable := cfg.StableWindow
	if stable <= 0 {
		stable = StableDefault
	}
	return &Scanner{
		st:          st,
		queue:       queue,
		events:      events,
		log:         log,
		ffprobePath: cfg.FFprobePath,
		prober:      prober,
		stable:      stable,
		procs:       make(chan struct{}, procs),
		locks:       newLibraryLocks(),
		ctx:         context.Background(),
		running:     map[string]bool{},
		rerun:       map[string]bool{},
	}
}

// setContext installs the lifecycle context used by async scans. Server
// startup calls this before any scan is scheduled.
func (s *Scanner) setContext(ctx context.Context) { s.ctx = ctx }

// finding is one examined file ready to be applied to the store.
type finding struct {
	path      string
	size      int64
	mtime     int64
	fp        string
	existing  *store.File
	info      probe.MediaInfo
	fromCache bool // re-evaluate the cached probe instead of info
}

// ScanLibrary scans one library: walk, probe changed files, evaluate, queue
// what needs work, mark vanished rows missing. Concurrent calls for the
// same library serialize.
func (s *Scanner) ScanLibrary(ctx context.Context, libraryID string) (ScanResult, error) {
	unlock := s.locks.lock(libraryID)
	defer unlock()

	lib, err := s.st.GetLibrary(ctx, libraryID)
	if err != nil {
		return ScanResult{}, err
	}
	target, err := flow.ParseTarget([]byte(lib.TargetJSON))
	if err != nil {
		return ScanResult{}, fmt.Errorf("scanner: library %s: %w", lib.ID, err)
	}

	paths, err := collectMediaPaths(lib.Path)
	if err != nil {
		return ScanResult{}, fmt.Errorf("scanner: walk %s: %w", lib.Path, err)
	}
	res := ScanResult{Seen: len(paths)}

	findings := make([]*finding, 0, len(paths))
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		skipped int
		failed  int
	)
	for _, p := range paths {
		if ctx.Err() != nil {
			break // shutdown; finish with what was examined
		}
		s.procs <- struct{}{} // bounds goroutines too
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			defer func() { <-s.procs }()
			f, outcome := s.examine(ctx, libraryID, p)
			mu.Lock()
			defer mu.Unlock()
			switch outcome {
			case examined:
				findings = append(findings, f)
			case probeFailed:
				failed++ // probe or fingerprint error on this file
			default:
				skipped++ // unchanged fingerprint, nothing to do
			}
		}(p)
	}
	wg.Wait()
	res.Skipped = skipped
	res.Errors += failed

	// Everything the walk saw is on disk: only DB rows whose path was not
	// seen (examined or skipped) are missing.
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		seen[p] = true
	}
	for _, f := range findings {
		queued, satisfied, err := s.apply(ctx, lib, target, f)
		if err != nil {
			s.log.Error("scanner: apply", "library", lib.ID, "path", f.path, "err", err)
			res.Errors++
			continue
		}
		if f.fromCache {
			res.Reused++
		} else {
			res.Probed++
		}
		if queued {
			res.Queued++
		}
		if satisfied {
			res.Satisfied++
		}
	}

	existing, err := s.st.ListFiles(ctx, libraryID)
	if err != nil {
		return res, fmt.Errorf("scanner: list files: %w", err)
	}
	for _, f := range existing {
		if seen[f.Path] || f.Status == store.FileStatusMissing {
			continue
		}
		f.Status = store.FileStatusMissing
		if err := s.st.UpdateFile(ctx, f); err != nil {
			s.log.Error("scanner: mark missing", "path", f.Path, "err", err)
			res.Errors++
			continue
		}
		res.Missing++
		s.log.Info("file missing", "library", lib.ID, "path", f.Path)
		s.publish(EventFileUpdated, NewFileDTO(f))
	}
	return res, nil
}

// ScanAsync schedules a scan of a library in the background. Bursts of
// triggers (watch events, API calls, schedule ticks) collapse into one run
// plus at most one follow-up, so a scan that starts does not queue a pile
// of duplicate work behind it.
func (s *Scanner) ScanAsync(libraryID string) {
	s.scanMu.Lock()
	if s.running[libraryID] {
		s.rerun[libraryID] = true // one follow-up after the current run
		s.scanMu.Unlock()
		return
	}
	s.running[libraryID] = true
	s.scanMu.Unlock()

	go func() {
		for {
			res, err := s.ScanLibrary(s.ctx, libraryID)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					s.log.Error("scanner: scan", "library", libraryID, "err", err)
				}
			} else {
				s.log.Info("scan finished", "library", libraryID,
					"seen", res.Seen, "probed", res.Probed, "reused", res.Reused,
					"queued", res.Queued, "satisfied", res.Satisfied,
					"missing", res.Missing, "errors", res.Errors)
			}
			s.scanMu.Lock()
			again := s.rerun[libraryID]
			delete(s.rerun, libraryID)
			if !again {
				delete(s.running, libraryID)
			}
			s.scanMu.Unlock()
			if !again {
				return
			}
		}
	}()
}

// examineOutcome says what examine did with a path.
type examineOutcome int

const (
	examined    examineOutcome = iota // a finding is ready to apply
	notRelevant                       // unchanged fingerprint, nothing to do
	probeFailed                       // fingerprint or probe error on this file
)

// examine fingerprints one path and probes it when needed.
func (s *Scanner) examine(ctx context.Context, libraryID, path string) (*finding, examineOutcome) {
	size, mtime, fp, err := statAndFingerprint(path)
	if err != nil {
		s.log.Debug("scanner: fingerprint failed", "path", path, "err", err)
		return nil, probeFailed
	}
	existing, err := s.st.GetFileByPath(ctx, libraryID, path)
	if err != nil && err != store.ErrNotFound {
		s.log.Error("scanner: file lookup", "path", path, "err", err)
		return nil, probeFailed
	}
	if existing != nil && existing.Fingerprint == fp {
		if existing.Status == store.FileStatusNew && existing.ProbeJSON != "" {
			// Classified as needing work before (e.g. a cancelled job) but
			// the file itself did not change: re-evaluate from the cache.
			return &finding{path: path, size: size, mtime: mtime, fp: fp,
				existing: existing, fromCache: true}, examined
		}
		return nil, notRelevant
	}

	// The file is new or changed: wait until it stops growing (a copy in
	// progress must never be probed part-way), then fingerprint again —
	// the size may have moved during the wait.
	if err := s.waitStable(ctx, path); err != nil {
		s.log.Debug("scanner: stability wait aborted", "path", path, "err", err)
		return nil, notRelevant
	}
	size, mtime, fp, err = statAndFingerprint(path)
	if err != nil {
		s.log.Debug("scanner: fingerprint failed", "path", path, "err", err)
		return nil, probeFailed
	}

	select {
	case <-ctx.Done():
		return nil, notRelevant
	default:
	}
	info, err := s.prober(ctx, s.ffprobePath, path)
	if err != nil {
		s.log.Warn("scanner: probe failed", "path", path, "err", err)
		return nil, probeFailed
	}
	return &finding{path: path, size: size, mtime: mtime, fp: fp, existing: existing, info: info}, examined
}

// waitStable blocks until path's size has not changed for the stability
// window and its mtime is at least that old. Bounded by stabilityMaxWait so
// a pathological writer cannot wedge a scan forever.
func (s *Scanner) waitStable(ctx context.Context, path string) error {
	deadline := time.Now().Add(stabilityMaxWait)
	var (
		lastSize   int64 = -1
		lastChange     = time.Now()
	)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		st, err := os.Stat(path)
		if err != nil {
			return nil // vanished mid-wait; the caller handles that
		}
		now := time.Now()
		if st.Size() != lastSize {
			lastSize = st.Size()
			lastChange = now
		}
		if now.Sub(st.ModTime()) >= s.stable && now.Sub(lastChange) >= s.stable {
			return nil
		}
		if now.After(deadline) {
			return nil // give up waiting; scan anyway
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stabilityPollInterval):
		}
	}
}

// apply stores one examined file and reports what happened: whether a job
// was queued and whether the file ended up satisfied.
func (s *Scanner) apply(ctx context.Context, lib *store.Library, target flow.Target, f *finding) (queued, satisfied bool, err error) {
	var (
		probeJSON string
		decision  flow.Decision
	)
	if f.fromCache {
		var cached probe.MediaInfo
		if err := json.Unmarshal([]byte(f.existing.ProbeJSON), &cached); err != nil {
			return false, false, fmt.Errorf("decode cached probe: %w", err)
		}
		decision = flow.Evaluate(cached, target)
		probeJSON = f.existing.ProbeJSON
	} else {
		data, err := json.Marshal(f.info)
		if err != nil {
			return false, false, fmt.Errorf("marshal probe: %w", err)
		}
		probeJSON = string(data)
		decision = flow.Evaluate(f.info, target)
	}

	file := &store.File{
		LibraryID:   lib.ID,
		Path:        f.path,
		Size:        f.size,
		Mtime:       f.mtime,
		Fingerprint: f.fp,
		ProbeJSON:   probeJSON,
	}
	if f.existing != nil {
		file.ID = f.existing.ID
		file.CreatedAt = f.existing.CreatedAt
		file.BytesSaved = f.existing.BytesSaved
	}

	if decision.Needs {
		file.Status = store.FileStatusNew
		file.SatisfiedReason = strings.Join(decision.Reasons, "; ")
		if err := s.st.UpsertFile(ctx, file); err != nil {
			return false, false, fmt.Errorf("upsert file: %w", err)
		}
		job, wasQueued, err := s.queue.QueueJob(ctx, lib, file, decision.Reasons)
		if err != nil {
			return false, false, fmt.Errorf("queue: %w", err)
		}
		if wasQueued {
			// QueueJob published the job.updated and file.updated events.
			return true, false, nil
		}
		// An active job already covers this file (e.g. the fingerprint
		// changed while it was queued). The upsert above is still news.
		_ = job
		s.publish(EventFileUpdated, NewFileDTO(file))
		return false, false, nil
	}

	file.Status = store.FileStatusSatisfied
	file.SatisfiedReason = ""
	if f.existing != nil &&
		f.existing.Status == store.FileStatusSatisfied &&
		f.existing.Fingerprint == f.fp {
		return false, true, nil // already recorded; no write, no event
	}
	if err := s.st.UpsertFile(ctx, file); err != nil {
		return false, false, fmt.Errorf("upsert file: %w", err)
	}
	s.publish(EventFileUpdated, NewFileDTO(file))
	return false, true, nil
}

// collectMediaPaths walks root and returns every regular, non-hidden media
// file path. The root itself may be a dotfolder — only entries inside are
// filtered.
func collectMediaPaths(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // missing root or unreadable dir fails the scan
		}
		if d.IsDir() {
			if path != root && isHidden(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || isHidden(d.Name()) {
			return nil // symlinks, devices and dotfiles are not library media
		}
		if !mediaExtensions[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// statAndFingerprint returns size, mtime (unix seconds) and the
// fingerprint. The fingerprint embeds both as "size:mtime:hash", so this
// avoids a second stat per file.
func statAndFingerprint(path string) (size, mtime int64, fp string, err error) {
	fp, err = probe.Fingerprint(path)
	if err != nil {
		return 0, 0, "", err
	}
	sizePart, rest, _ := strings.Cut(fp, ":")
	mtimePart, _, _ := strings.Cut(rest, ":")
	size, _ = strconv.ParseInt(sizePart, 10, 64)
	mtime, _ = strconv.ParseInt(mtimePart, 10, 64)
	return size, mtime, fp, nil
}

func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

// publish is a nil-safe event publish.
func (s *Scanner) publish(eventType string, payload any) {
	if s.events == nil {
		return
	}
	s.events.Publish(eventType, payload)
}
