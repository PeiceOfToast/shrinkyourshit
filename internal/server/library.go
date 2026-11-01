// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// LibraryParams are the editable fields of a library, used by both create
// and update. TargetJSON may be nil on create (the default Target applies);
// on update it must be present (PUT semantics). EasyJSON is the easy-screen
// input that compiled to TargetJSON (task 07); empty means the library was
// not saved from the easy screen. The flow graph (task 14) is always
// generated from the compiled Target here; a hand-edited flow is stored
// through UpdateFlow instead. On update an unchanged Target keeps the
// stored flow, so a name/watch-only save cannot destroy manual steps.
// FileKinds is what the library holds (task 34): a non-empty list of valid
// probe.Kinds with no duplicates.
type LibraryParams struct {
	Name         string
	Path         string
	TargetJSON   []byte // raw Target JSON
	EasyJSON     []byte // raw EasyModeInput JSON, empty to clear
	Watch        bool
	Schedule     string
	ResultPolicy string
	FileKinds    []string
}

// LibraryManager owns library CRUD and the target-driven re-evaluation of a
// library's files. Deleting a library removes DB records only — files on
// disk are never touched.
type LibraryManager struct {
	st      store.Store
	queue   *Queue
	events  *EventBus
	log     *slog.Logger
	locks   *libraryLocks
	watcher *Watcher
	// scan kicks off an initial scan of a newly created library; set by
	// the server wiring, nil-safe elsewhere.
	scan func(libraryID string)
	// pluginCheck gates a saved flow's plugin steps (task 32): the
	// plugin service's CheckGraph, set by the server wiring; nil-safe
	// (tests without a plugin service).
	pluginCheck func(ctx context.Context, g flow.Graph) error
}

// NewLibraryManager builds the manager. watcher may be nil (watch state is
// then left to the caller).
func NewLibraryManager(st store.Store, queue *Queue, events *EventBus, watcher *Watcher, log *slog.Logger) *LibraryManager {
	if log == nil {
		log = slog.Default()
	}
	return &LibraryManager{
		st:      st,
		queue:   queue,
		events:  events,
		log:     log,
		locks:   newLibraryLocks(),
		watcher: watcher,
	}
}

// SetScanHook wires the initial-scan trigger for newly created libraries.
// The server wiring installs Scanner.ScanAsync; tests may install a no-op.
func (m *LibraryManager) SetScanHook(scan func(libraryID string)) { m.scan = scan }

// SetPluginCheck wires the plugin-step gate (task 32): the plugin
// service's CheckGraph. A saved flow that names a missing, disabled or
// non-step plugin is refused with ErrPluginStep (the API answers 422).
func (m *LibraryManager) SetPluginCheck(check func(ctx context.Context, g flow.Graph) error) {
	m.pluginCheck = check
}

// Create validates and stores a new library.
func (m *LibraryManager) Create(ctx context.Context, p LibraryParams) (*store.Library, error) {
	lib, err := m.build(nil, p)
	if err != nil {
		return nil, err
	}
	if err := m.st.CreateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("library: create: %w", err)
	}
	m.log.Info("library created", "library", lib.ID, "name", lib.Name, "path", lib.Path)
	m.publish(EventLibraryUpdated, NewLibraryDTO(lib))

	if m.watcher != nil {
		m.watcher.Add(lib)
	}
	if m.scan != nil {
		m.scan(lib.ID) // initial scan of the new library
	}
	return lib, nil
}

// Update replaces the editable fields and re-evaluates every file in the
// library against the new Target (ARCHITECTURE.md): files that now need
// work are queued, files that now satisfy it are marked satisfied, and
// queued jobs that are no longer needed are cancelled. Claimed/running jobs
// are left alone; the worker re-reads the Target when it starts.
func (m *LibraryManager) Update(ctx context.Context, id string, p LibraryParams) (*store.Library, error) {
	old, err := m.st.GetLibrary(ctx, id)
	if err != nil {
		return nil, err
	}
	lib, err := m.build(old, p)
	if err != nil {
		return nil, err
	}
	if err := m.st.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("library: update: %w", err)
	}
	m.log.Info("library updated", "library", lib.ID, "name", lib.Name)
	m.publish(EventLibraryUpdated, NewLibraryDTO(lib))

	m.reevaluate(ctx, lib)

	if m.watcher != nil {
		m.watcher.Add(lib) // re-sync watch state (may have flipped off)
	}
	return lib, nil
}

// Delete removes the library and its files/jobs from the database. The
// filesystem is never touched.
func (m *LibraryManager) Delete(ctx context.Context, id string) error {
	if _, err := m.st.GetLibrary(ctx, id); err != nil {
		return err
	}
	if err := m.st.DeleteLibrary(ctx, id); err != nil {
		return fmt.Errorf("library: delete: %w", err)
	}
	m.log.Info("library deleted", "library", id)
	if m.watcher != nil {
		m.watcher.Remove(id)
	}
	m.publish(EventLibraryUpdated, LibraryDTO{ID: id})
	return nil
}

// Get returns one library.
func (m *LibraryManager) Get(ctx context.Context, id string) (*store.Library, error) {
	return m.st.GetLibrary(ctx, id)
}

// List returns all libraries.
func (m *LibraryManager) List(ctx context.Context) ([]*store.Library, error) {
	return m.st.ListLibraries(ctx)
}

// build validates params into a store.Library. old is nil for create.
func (m *LibraryManager) build(old *store.Library, p LibraryParams) (*store.Library, error) {
	if p.Name == "" {
		return nil, fmt.Errorf("library name must not be empty")
	}
	if p.Path == "" || !filepath.IsAbs(p.Path) {
		return nil, fmt.Errorf("library path %q must be an absolute path", p.Path)
	}
	if p.Schedule != "" {
		if _, err := ParseSchedule(p.Schedule); err != nil {
			return nil, err
		}
	}
	policy := p.ResultPolicy
	if policy == "" {
		policy = store.ResultPolicyReplaceIfSmaller
	}
	valid := false
	for _, v := range store.ResultPolicyValues() {
		if v == policy {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("invalid result_policy %q: want one of %v", policy, store.ResultPolicyValues())
	}

	// FileKinds (task 34): a missing list defaults to ["video"] so
	// programmatic callers (the tests, the manager itself) keep working
	// and a default library still reports ["video"]; an explicitly empty
	// list is rejected. Every value must be a valid Kind and duplicates
	// are rejected. Every error names the field and the bad value.
	if p.FileKinds == nil {
		p.FileKinds = []string{string(probe.KindVideo)}
	}
	if len(p.FileKinds) == 0 {
		return nil, fmt.Errorf("file_kinds %v must not be empty: want at least one of %v", p.FileKinds, probe.KindValues())
	}
	seen := map[string]bool{}
	for _, k := range p.FileKinds {
		valid := false
		for _, v := range probe.KindValues() {
			if string(v) == k {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("file_kinds: invalid file kind %q: want one of %v", k, probe.KindValues())
		}
		if seen[k] {
			return nil, fmt.Errorf("file_kinds: duplicate file kind %q", k)
		}
		seen[k] = true
	}

	target := flow.Default()
	if p.TargetJSON != nil {
		var err error
		target, err = flow.ParseTarget(p.TargetJSON)
		if err != nil {
			return nil, fmt.Errorf("library: %w", err)
		}
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("library: marshal target: %w", err)
	}

	lib := &store.Library{
		Name:         p.Name,
		Path:         filepath.Clean(p.Path),
		TargetJSON:   string(canonical),
		EasyJSON:     string(p.EasyJSON),
		Watch:        p.Watch,
		Schedule:     p.Schedule,
		ResultPolicy: policy,
		FileKinds:    p.FileKinds,
	}
	if old != nil {
		lib.ID = old.ID
		lib.CreatedAt = old.CreatedAt
		lib.FlowJSON = old.FlowJSON
		lib.FlowSource = old.FlowSource
	}
	// The flow must match the rule this save produced: regenerate it from
	// the compiled Target when the rule changed. An easy save always
	// regenerates (the easy tier owns the flow then); a target-only save
	// keeps the stored flow when the Target is unchanged, so a name or
	// watch-only update never destroys hand-built steps.
	switch {
	case p.EasyJSON != nil || old == nil || old.TargetJSON != string(canonical):
		graph, err := flow.TargetToFlow(target)
		if err != nil {
			return nil, fmt.Errorf("library: %w", err)
		}
		raw, err := json.Marshal(graph)
		if err != nil {
			return nil, fmt.Errorf("library: marshal flow: %w", err)
		}
		lib.FlowJSON = string(raw)
		if p.EasyJSON != nil {
			lib.FlowSource = flow.FlowSourceEasy
		} else {
			lib.FlowSource = flow.FlowSourceAdvanced
		}
	case old.FlowJSON == "":
		// Legacy row kept but never backfilled: store the generated flow.
		graph, err := flow.TargetToFlow(target)
		if err == nil {
			if raw, err := json.Marshal(graph); err == nil {
				lib.FlowJSON = string(raw)
				lib.FlowSource = flow.FlowSourceAdvanced
			}
		}
	}
	return lib, nil
}

// reevaluate applies the library's flow to every file from its cached
// probe, queueing or satisfying as needed. Serialized against scans of the
// same library. Each file is resolved against the flow with its own probe,
// so a guarded encode (for example "only when the video is not HEVC")
// satisfies files that already match without queueing them.
func (m *LibraryManager) reevaluate(ctx context.Context, lib *store.Library) {
	unlock := m.locks.lock(lib.ID)
	defer unlock()

	plan, _, err := LibraryPlan(lib)
	if err != nil {
		m.log.Error("library: re-evaluate flow", "library", lib.ID, "err", err)
		return
	}
	logFlowTargetMismatch(m.log, lib, plan)
	files, err := m.st.ListFiles(ctx, lib.ID)
	if err != nil {
		m.log.Error("library: re-evaluate list", "library", lib.ID, "err", err)
		return
	}

	for _, f := range files {
		// Missing rows are dormant until the file reappears; active work is
		// left alone. A file whose kind the library does not look for is
		// not part of this library's rules (task 34): the scan records it
		// as skipped, re-evaluation leaves it alone.
		if f.Status == store.FileStatusMissing || f.Status == store.FileStatusProcessing {
			continue
		}
		kind := probe.KindForPath(f.Path)
		if !libraryWantsKind(lib, kind) {
			continue
		}
		var info probe.MediaInfo
		if f.ProbeJSON != "" {
			if err := json.Unmarshal([]byte(f.ProbeJSON), &info); err != nil {
				m.log.Error("library: re-evaluate probe", "file", f.Path, "err", err)
				continue
			}
		} else if probe.IsMedia(kind) {
			// Media without a probe waits for the next scan; a file that is
			// not media (task 34) re-evaluates from its identity alone.
			continue
		}

		eff, err := flow.ResolvePlan(plan, info, kind)
		if err != nil {
			m.log.Error("library: re-evaluate resolve", "file", f.Path, "err", err)
			continue
		}

		var decision flow.Decision
		if probe.IsMedia(kind) {
			decision = flow.Evaluate(info, eff.Target)
		} else if plan.HasKindAgnosticWork() {
			// A file that is not media is queued only when the flow carries
			// kind-agnostic steps (rename, move, plugin) — task 34.
			decision.Needs = true
			decision.Reasons = plan.KindAgnosticReasons()
		}

		if decision.Needs {
			if _, queued, err := m.queue.QueueJob(ctx, lib, f, decision.Reasons); err != nil {
				m.log.Error("library: re-evaluate queue", "file", f.Path, "err", err)
			} else if queued {
				m.log.Debug("re-evaluation queued job", "file", f.Path)
			}
			continue
		}

		// Satisfied now. Cancel a queued job that is no longer needed; a
		// claimed/running job is left to finish.
		if job := m.queuedJobFor(ctx, f.ID); job != nil {
			f.Status = store.FileStatusSatisfied
			f.SatisfiedReason = eff.Reason
			if err := m.st.UpdateFile(ctx, f); err != nil {
				m.log.Error("library: re-evaluate file", "file", f.Path, "err", err)
				continue
			}
			m.publish(EventFileUpdated, NewFileDTO(f))
			if _, err := m.queue.Cancel(ctx, job.ID); err != nil && !errors.Is(err, ErrJobNotActive) {
				m.log.Error("library: cancel unneeded job", "job", job.ID, "err", err)
			}
			continue
		}
		if f.Status == store.FileStatusSatisfied {
			continue // nothing changed
		}
		f.Status = store.FileStatusSatisfied
		f.SatisfiedReason = eff.Reason
		if err := m.st.UpdateFile(ctx, f); err != nil {
			m.log.Error("library: re-evaluate file", "file", f.Path, "err", err)
			continue
		}
		m.publish(EventFileUpdated, NewFileDTO(f))
	}
}

// queuedJobFor returns the queued job for a file, if any.
func (m *LibraryManager) queuedJobFor(ctx context.Context, fileID string) *store.Job {
	jobs, err := m.st.ListJobs(ctx, store.JobFilter{FileID: fileID})
	if err != nil {
		return nil
	}
	for _, j := range jobs {
		if j.Status == store.JobStatusQueued {
			return j
		}
	}
	return nil
}

// libraryWantsKind reports whether the library's file kinds include kind
// (task 34). An empty list reads as video-only, the same default the
// manager applies on create.
func libraryWantsKind(lib *store.Library, kind probe.Kind) bool {
	kinds := lib.FileKinds
	if len(kinds) == 0 {
		kinds = []string{string(probe.KindVideo)}
	}
	for _, k := range kinds {
		if k == string(kind) {
			return true
		}
	}
	return false
}

// publish is a nil-safe event publish.
func (m *LibraryManager) publish(eventType string, payload any) {
	if m.events == nil {
		return
	}
	m.events.Publish(eventType, payload)
}
