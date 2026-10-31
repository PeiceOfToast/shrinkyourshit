// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// PluginService is the v1 plugin support (task 31): install from a git
// URL or a directory, list, enable, collect settings, remove, and run
// the three hook phases around a job's life. The manifest lives on disk
// under <data dir>/plugins/<name>/; the store row is the index.
//
// Phases:
//
//   - preflight runs synchronously when a job is claimed (Queue.Claim);
//     a plugin that exits non-zero fails the job with its name and exit
//     code in the error. A plugin that cannot produce a verdict — a
//     missing required setting, a timeout, a cancelled caller, an
//     unreadable manifest — is skipped with a warn line in the job log
//     and never blocks the job.
//   - postprocess and notify run from the same outbound hook the webhook
//     dispatcher and the integration refresher hang off (one notifier
//     path): postprocess for a done job, notify for done and failed.
//     Their failures are warn lines and can never fail a job.
type PluginService struct {
	st      store.Store
	log     *slog.Logger
	runner  *plugin.Runner
	dataDir string

	// inline runs the postprocess/notify phases synchronously instead of
	// in a detached goroutine. Production leaves it false (a plugin must
	// never hold a completion response); phase tests set it true so the
	// log lines are deterministic.
	inline bool
}

// NewPluginService builds the service over the store. dataDir must be the
// server's data directory (the plugins root lives under it, next to the
// update cache and backups, so installed plugins survive a restart).
func NewPluginService(st store.Store, log *slog.Logger, dataDir string) *PluginService {
	if log == nil {
		log = slog.Default()
	}
	if dataDir == "" {
		// Only a test harness without a Config.DataDir can land here;
		// production always passes the data dir. Keep a per-process
		// directory so two servers never share a plugin root.
		dataDir = filepath.Join(os.TempDir(), "sys-plugins")
		_ = os.MkdirAll(dataDir, 0o755)
	}
	return &PluginService{
		st:      st,
		log:     log,
		runner:  plugin.NewRunner(dataDir),
		dataDir: dataDir,
	}
}

// Installed is the service-level shape of one installed plugin: the
// stored row plus what the manifest on disk says right now. When the
// directory was removed by hand the manifest fields are empty and
// ManifestError says so; the row itself stays removable.
type Installed struct {
	ID          string
	Name        string
	Version     string
	Type        string
	Enabled     bool
	Source      string
	Values      map[string]string
	InstalledAt time.Time
	Dir         string
	// From the manifest on disk; zero/empty when it cannot be read.
	Manifest      plugin.Manifest
	Description   string
	Author        string
	Timeout       int
	Env           []string
	Settings      []plugin.Setting
	ManifestError string
}

// InstalledFromRow reads a row and merges the manifest from disk.
func InstalledFromRow(row *store.Plugin, dataDir string) Installed {
	in := Installed{
		ID:          row.ID,
		Name:        row.Name,
		Version:     row.Version,
		Type:        row.Type,
		Enabled:     row.Enabled,
		Source:      row.Source,
		Values:      row.SettingValues(),
		InstalledAt: row.InstalledAt,
		Dir:         plugin.Dir(dataDir, row.Name),
	}
	if m, err := plugin.Load(in.Dir); err == nil {
		in.Manifest = m
		in.Version = m.Version // the manifest is the truth about the version
		in.Description = m.Description
		in.Author = m.Author
		in.Timeout = m.Timeout()
		in.Env = m.Env
		in.Settings = m.Settings
	} else {
		in.ManifestError = err.Error()
	}
	return in
}

// runnable builds the runner input of an installed plugin.
func (in Installed) runnable() plugin.Plugin {
	return plugin.Plugin{Manifest: in.Manifest, Dir: in.Dir, Settings: in.Values}
}

// List returns every installed plugin, oldest first.
func (s *PluginService) List(ctx context.Context) ([]Installed, error) {
	rows, err := s.st.ListPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: list: %w", err)
	}
	out := make([]Installed, 0, len(rows))
	for _, row := range rows {
		out = append(out, InstalledFromRow(row, s.dataDir))
	}
	return out, nil
}

// Get returns one installed plugin or ErrNotFound.
func (s *PluginService) Get(ctx context.Context, id string) (Installed, error) {
	row, err := s.st.GetPlugin(ctx, id)
	if err != nil {
		return Installed{}, err
	}
	return InstalledFromRow(row, s.dataDir), nil
}

// InstallRequest is one install: exactly one of GitURL (with an optional
// Ref) or Path.
type InstallRequest struct {
	GitURL string
	Ref    string
	Path   string
}

// Install installs a plugin (task 31). The new row is disabled — nothing
// runs until it is enabled on purpose — with no collected settings.
func (s *PluginService) Install(ctx context.Context, req InstallRequest) (Installed, error) {
	var (
		m      plugin.Manifest
		source string
		err    error
	)
	switch {
	case req.GitURL != "" && req.Path != "":
		return Installed{}, errors.New("plugins: install one source at a time: git_url or path, not both")
	case req.GitURL != "":
		source = req.GitURL
		m, err = plugin.InstallFromGit(ctx, req.GitURL, req.Ref, s.dataDir)
	case req.Path != "":
		source = req.Path
		m, err = plugin.InstallFromPath(req.Path, s.dataDir)
	default:
		return Installed{}, errors.New("plugins: install needs git_url or path")
	}
	if err != nil {
		return Installed{}, err
	}
	// The manifest names the plugin; a name that is already installed is
	// refused before the row is created (the directory check already ran
	// inside the install).
	if _, err := s.st.GetPluginByName(ctx, m.Name); err == nil {
		_ = plugin.Remove(s.dataDir, m.Name)
		return Installed{}, fmt.Errorf("%w: a plugin named %q is installed; remove it first", plugin.ErrAlreadyInstalled, m.Name)
	}
	row := &store.Plugin{Name: m.Name, Version: m.Version, Type: m.Type, Source: source}
	if err := s.st.CreatePlugin(ctx, row); err != nil {
		// The directory was created; a row failure must not leave a
		// half-install behind.
		_ = plugin.Remove(s.dataDir, m.Name)
		return Installed{}, fmt.Errorf("plugins: record install: %w", err)
	}
	return InstalledFromRow(row, s.dataDir), nil
}

// UpdateRequest is one PUT: the enabled switch and/or the collected
// settings values. A nil field is left unchanged.
type UpdateRequest struct {
	Enabled  *bool
	Settings map[string]string
}

// Update applies the enabled switch and/or new settings values. Unknown
// setting keys (not in the manifest on disk) are rejected; a manifest
// that cannot be read accepts any keys (nothing can run anyway).
func (s *PluginService) Update(ctx context.Context, id string, req UpdateRequest) (Installed, error) {
	row, err := s.st.GetPlugin(ctx, id)
	if err != nil {
		return Installed{}, err
	}
	if req.Settings != nil {
		declared := InstalledFromRow(row, s.dataDir).Settings
		for key := range req.Settings {
			known := false
			for _, def := range declared {
				if def.Key == key {
					known = true
					break
				}
			}
			if !known {
				return Installed{}, fmt.Errorf("plugins: unknown setting %q: the manifest declares %v", key, settingKeys(declared))
			}
		}
		values := row.SettingValues()
		for key, value := range req.Settings {
			values[key] = value
		}
		row.Settings = marshalSettings(values)
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if err := s.st.UpdatePlugin(ctx, row); err != nil {
		return Installed{}, fmt.Errorf("plugins: update: %w", err)
	}
	return InstalledFromRow(row, s.dataDir), nil
}

// Delete removes a plugin. An enabled plugin is refused (turn it off
// first); a disabled one loses its row and its directory.
func (s *PluginService) Delete(ctx context.Context, id string) error {
	row, err := s.st.GetPlugin(ctx, id)
	if err != nil {
		return err
	}
	if row.Enabled {
		return fmt.Errorf("%w: %q is enabled; turn it off before removing it", plugin.ErrEnabled, row.Name)
	}
	if err := s.st.DeletePlugin(ctx, row.ID); err != nil {
		return fmt.Errorf("plugins: delete: %w", err)
	}
	// The directory goes with the row; a failure here is a warn (the row
	// is already gone, so the UI no longer lists the plugin).
	if err := plugin.Remove(s.dataDir, row.Name); err != nil {
		s.log.Warn("plugins: remove directory", "plugin", row.Name, "err", err)
	}
	return nil
}

// TestResult is one test run of a plugin: the runner's result plus a
// plain ok flag (a run that executed and exited 0).
type TestResult struct {
	Result plugin.Result
	OK     bool
}

// Test runs the plugin once against a synthetic job (task 31) and
// returns its captured output and exit code. It works on disabled
// plugins too: an explicit diagnostic the admin asked for. There is no
// job, so nothing is appended to any log.
func (s *PluginService) Test(ctx context.Context, id string) (TestResult, error) {
	row, err := s.st.GetPlugin(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	in := InstalledFromRow(row, s.dataDir)
	if in.ManifestError != "" {
		return TestResult{}, fmt.Errorf("plugins: %s", in.ManifestError)
	}
	res := s.runner.Run(ctx, in.runnable(), SamplePluginJob(row.Type), row.Type)
	return TestResult{Result: res, OK: res.OK()}, nil
}

// settingKeys lists a manifest's setting keys for error messages.
func settingKeys(defs []plugin.Setting) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Key)
	}
	return out
}

// marshalSettings encodes the collected values the way the row stores
// them (a JSON object).
func marshalSettings(values map[string]string) string {
	raw, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

/* ------------------------------------------------------------------ */
/* The job phases (task 31)                                            */
/* ------------------------------------------------------------------ */

// Preflight runs every enabled preflight plugin for a freshly claimed
// job, synchronously, in install order. The first plugin that exits
// non-zero produces the job's failure error (its name and exit code are
// in it). Plugins that cannot produce a verdict are skipped with a warn
// line in the job log; the job proceeds.
func (s *PluginService) Preflight(ctx context.Context, job *store.Job, f *store.File) error {
	rows, err := s.st.ListEnabledPlugins(ctx, store.PluginTypePreflight)
	if err != nil {
		s.log.Error("plugins: list preflight", "err", err)
		return nil // a store failure must not fail jobs
	}
	if len(rows) == 0 {
		return nil
	}
	pc := plugin.JobContext{
		JobID:      job.ID,
		SourcePath: filePathOf(f),
		Status:     store.JobStatusClaimed,
	}
	if lib, lerr := s.st.GetLibrary(ctx, job.LibraryID); lerr == nil {
		pc.Library = lib.Name
	} else if !errors.Is(lerr, store.ErrNotFound) {
		s.log.Error("plugins: preflight library lookup", "job", job.ID, "err", lerr)
	}
	for _, row := range rows {
		res, ran := s.runOne(ctx, row, pc, plugin.TypePreflight)
		if !ran {
			continue // skipped; the reason is already in the log
		}
		switch {
		case res.OK():
			// The plugin accepted the job.
		case res.TimedOut:
			// No verdict: the job proceeds, loudly.
		default:
			return fmt.Errorf("preflight plugin %q failed: %s", row.Name, res.Error)
		}
	}
	return nil
}

// PostJob runs the postprocess and notify phases for a finished job.
// postprocess runs only for a done job; notify for done and failed. It
// returns as soon as the rows are listed: every plugin runs in a
// detached goroutine bounded by its own timeout, so a slow plugin can
// never hold a completion response, and a failing plugin is a warn line
// in the job log — it never fails a job that encoded fine.
func (s *PluginService) PostJob(ctx context.Context, pc plugin.JobContext) {
	var phases []string
	switch pc.Status {
	case store.JobStatusDone:
		phases = []string{plugin.TypePostprocess, plugin.TypeNotify}
	case store.JobStatusFailed:
		phases = []string{plugin.TypeNotify}
	default:
		return
	}

	type planned struct {
		row   *store.Plugin
		phase string
	}
	var plans []planned
	for _, phase := range phases {
		rows, err := s.st.ListEnabledPlugins(ctx, phase)
		if err != nil {
			s.log.Error("plugins: list", "phase", phase, "err", err)
			return
		}
		for _, row := range rows {
			plans = append(plans, planned{row: row, phase: phase})
		}
	}
	if len(plans) == 0 {
		return
	}

	for _, p := range plans {
		row, phase := p.row, p.phase
		run := func() {
			// The caller's context dies with the completion response; a
			// plugin must outlive it. Run clamps the run to the
			// manifest's timeout on its own.
			s.runOne(context.Background(), row, pc, phase)
		}
		if s.inline {
			run()
			continue
		}
		go run()
	}
}

// runOne loads the plugin's manifest, runs it for phase and appends the
// captured output plus the verdict to the job's structured log. ran is
// false when the plugin did not run (skipped, unreadable manifest); the
// reason is already in the log.
func (s *PluginService) runOne(ctx context.Context, row *store.Plugin, pc plugin.JobContext, phase string) (plugin.Result, bool) {
	in := InstalledFromRow(row, s.dataDir)
	if in.ManifestError != "" {
		s.logJobLine(ctx, pc.JobID, store.JobLogWarn, fmt.Sprintf("%s: skipped: %s", row.Name, in.ManifestError))
		return plugin.Result{}, false
	}
	res := s.runner.Run(ctx, in.runnable(), pc, phase)

	// Captured output into the job log (the task 27 viewer), the plugin
	// name leading the message; stdout is info, stderr warn.
	for _, line := range res.Output {
		level := store.JobLogInfo
		if line.Stderr {
			level = store.JobLogWarn
		}
		s.logJobLine(ctx, pc.JobID, level, row.Name+": "+line.Text)
	}

	// The verdict line.
	switch {
	case !res.Ran:
		s.logJobLine(ctx, pc.JobID, store.JobLogWarn, fmt.Sprintf("%s: skipped: %s", row.Name, res.SkipReason))
	case res.OK():
		s.logJobLine(ctx, pc.JobID, store.JobLogInfo, fmt.Sprintf("%s: exited 0", row.Name))
	default:
		// Failure, timeout or a killed process: warn. It never fails a
		// job here (the preflight rejection is decided by the caller).
		s.logJobLine(ctx, pc.JobID, store.JobLogWarn, fmt.Sprintf("%s: %s", row.Name, res.Error))
	}
	return res, res.Ran
}

// logJobLine appends one server-side line to a job's structured log. The
// source is "plugin" (task 31); seq is a millisecond stamp so the rows
// sort sensibly next to the worker's sequence numbers. Best effort: a
// failed append never fails the job.
func (s *PluginService) logJobLine(ctx context.Context, jobID, level, message string) {
	if jobID == "" || message == "" {
		return
	}
	line := store.JobLogLine{
		Seq:     time.Now().UnixMilli(),
		Level:   level,
		Source:  store.JobLogSourcePlugin,
		Message: message,
	}
	if err := s.st.AppendJobLog(ctx, jobID, []store.JobLogLine{line}); err != nil {
		s.log.Error("plugins: append job log", "job", jobID, "err", err)
	}
}

// SamplePluginJob is the synthetic job context the Test endpoint runs a
// plugin against (task 31): a synthetic done job with example numbers,
// mirroring the webhook SampleEvent.
func SamplePluginJob(phase string) plugin.JobContext {
	return plugin.JobContext{
		JobID:       "sample-job",
		Library:     "Sample Library",
		SourcePath:  "/media/movies/The Matrix (1999).mkv",
		OutputPath:  "/media/movies/The Matrix (1999).mkv",
		BytesBefore: 4_100_000_000,
		BytesAfter:  1_200_000_000,
		Encoder:     "sample",
		Status:      store.JobStatusDone,
		Phase:       phase,
	}
}

// libNameOf returns a library's name, tolerating a nil row.
func libNameOf(lib *store.Library) string {
	if lib == nil {
		return ""
	}
	return lib.Name
}

// filePathOf returns a file row's path, tolerating a nil row.
func filePathOf(f *store.File) string {
	if f == nil {
		return ""
	}
	return f.Path
}
