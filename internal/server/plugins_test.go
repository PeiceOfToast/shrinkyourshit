// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shAvailable skips the phase tests when sh is missing (they exec shell
// scripts as the plugin process).
func shAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
}

// installPlugin writes a plugin directory containing a run.sh script and
// a manifest whose command runs it (no metacharacters reach the manifest,
// which the validator rejects), installs it and returns the shape.
func installPlugin(t *testing.T, h *harness, name, typ, script, extra string) Installed {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(src, 0o755))
	scriptPath := filepath.Join(src, "run.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script+"\n"), 0o755))
	raw := `name: ` + name + `
version: 1.0.0
type: ` + typ + `
command: ["sh", "` + scriptPath + `"]
` + extra
	require.NoError(t, os.WriteFile(filepath.Join(src, plugin.FileName), []byte(raw), 0o644))
	inst, err := h.svc.Plugins().Install(context.Background(), InstallRequest{Path: src})
	require.NoError(t, err)
	return inst
}

// writePluginSource writes a plugin directory with a manifest body
// verbatim and returns its path (the manifest-acceptance tests).
func writePluginSource(t *testing.T, name, typ, command, extra string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(src, 0o755))
	raw := `name: ` + name + `
version: 1.0.0
type: ` + typ + `
command: [` + command + `]
` + extra
	require.NoError(t, os.WriteFile(filepath.Join(src, plugin.FileName), []byte(raw), 0o644))
	if strings.Contains(command, `"run.sh"`) {
		require.NoError(t, os.WriteFile(filepath.Join(src, "run.sh"),
			[]byte("echo lazy ran\nexit 1\n"), 0o755))
	}
	return src
}

// enablePlugin turns an installed plugin on.
func enablePlugin(t *testing.T, h *harness, pluginID string) {
	t.Helper()
	yes := true
	_, err := h.svc.Plugins().Update(context.Background(), pluginID, UpdateRequest{Enabled: &yes})
	require.NoError(t, err)
}

// pluginLogLines returns the job's log rows with source "plugin".
func pluginLogLines(t *testing.T, h *harness, jobID string) []store.JobLogLine {
	t.Helper()
	lines, err := h.st.ListJobLog(context.Background(), jobID, 0, "", 0)
	require.NoError(t, err)
	var out []store.JobLogLine
	for _, l := range lines {
		if l.Source == store.JobLogSourcePlugin {
			out = append(out, l)
		}
	}
	return out
}

// TestPreflightPluginFailsJob (task 31): a preflight plugin that exits
// non-zero fails the job with the plugin's name and exit code in the
// error, and its output lands in the job log with source "plugin".
func TestPreflightPluginFailsJob(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)
	lib, _ := queueFixture(t, h)

	inst := installPlugin(t, h, "gate-guard", store.PluginTypePreflight,
		"echo gate says no\nexit 3", "")
	enablePlugin(t, h, inst.ID)

	_, err := h.svc.Queue().Claim(context.Background(), "node-x")
	require.ErrorIs(t, err, ErrPreflightRejected)

	// The job is failed with the plugin name and exit code in the error.
	fs := h.files(lib.ID)
	require.Len(t, fs, 1)
	var job *store.Job
	for _, f := range fs {
		jobs := h.jobsOf(f.ID)
		require.Len(t, jobs, 1)
		job = jobs[0]
	}
	require.Equal(t, store.JobStatusFailed, job.Status)
	assert.Contains(t, job.Error, "gate-guard")
	assert.Contains(t, job.Error, "exit status 3")

	// The plugin's output is in the job log with source "plugin".
	lines := pluginLogLines(t, h, job.ID)
	require.NotEmpty(t, lines)
	found := false
	for _, l := range lines {
		if strings.Contains(l.Message, "gate says no") {
			found = true
		}
		assert.Equal(t, store.JobLogSourcePlugin, l.Source)
	}
	assert.True(t, found, "the plugin's stdout line must be in the job log")
}

// TestPreflightPluginTimeoutContinuesJob (task 31): a plugin that sleeps
// past its timeout is killed and the job continues.
func TestPreflightPluginTimeoutContinuesJob(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)
	lib, _ := queueFixture(t, h)

	inst := installPlugin(t, h, "slow-gate", store.PluginTypePreflight,
		"sleep 30", "timeout_seconds: 1\n")
	enablePlugin(t, h, inst.ID)

	job, err := h.svc.Queue().Claim(context.Background(), "node-x")
	require.NoError(t, err, "a timed-out preflight plugin must not block the job")
	require.Equal(t, store.JobStatusClaimed, job.Status)
	require.Equal(t, lib.ID, job.LibraryID)

	// The timeout is in the job log as a warn line.
	lines := pluginLogLines(t, h, job.ID)
	require.NotEmpty(t, lines)
	var warn bool
	for _, l := range lines {
		if strings.Contains(l.Message, "slow-gate") && strings.Contains(l.Message, "timed out") {
			warn = true
			assert.Equal(t, store.JobLogWarn, l.Level)
		}
	}
	assert.True(t, warn, "the timeout must be logged as a warn line")
}

// TestPreflightMissingSettingSkipped (task 31): a plugin requiring a
// missing setting does not run, and the job log says why.
func TestPreflightMissingSettingSkipped(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)
	lib, _ := queueFixture(t, h)

	inst := installPlugin(t, h, "needs-token", store.PluginTypePreflight,
		"echo should not run",
		"settings:\n  - key: token\n    label: Token\n    required: true\n")
	enablePlugin(t, h, inst.ID)

	job, err := h.svc.Queue().Claim(context.Background(), "node-x")
	require.NoError(t, err, "a skipped preflight plugin must not block the job")
	require.Equal(t, store.JobStatusClaimed, job.Status)
	require.Equal(t, lib.ID, job.LibraryID)

	// The plugin never ran: its stdout is not in the log; the skip reason is.
	lines := pluginLogLines(t, h, job.ID)
	require.NotEmpty(t, lines)
	for _, l := range lines {
		assert.NotContains(t, l.Message, "should not run")
		assert.Equal(t, store.JobLogWarn, l.Level, "a skip is a warn line")
	}
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0].Message, "needs-token")
	assert.Contains(t, lines[0].Message, "token")
	assert.Contains(t, lines[0].Message, "not set")
}

// completeFixture runs one full replaced-output completion with the fake
// inspector and returns the job row.
func completeFixture(t *testing.T, h *harness, jobID string, output string) *store.Job {
	t.Helper()
	h.svc.Queue().SetInspector(func(_ context.Context, _, path string) (ResultInfo, error) {
		require.Equal(t, output, path)
		return inspectedOutput, nil
	}, "")
	done, err := h.svc.Queue().Complete(context.Background(), jobID, "node-1", CompleteParams{
		OutputPath: output, BytesAfter: 100, Encoder: "test",
	})
	require.NoError(t, err)
	return done
}

// TestPostprocessFailureLeavesJobDone (task 31): a failing postprocess
// plugin logs a warning and never fails a job that encoded fine.
func TestPostprocessFailureLeavesJobDone(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)
	_, f, job := claimJob(t, h, "node-1")

	inst := installPlugin(t, h, "crash-after", store.PluginTypePostprocess,
		"echo boom >&2\nexit 7", "")
	enablePlugin(t, h, inst.ID)
	h.svc.plugins.inline = true // deterministic phase runs for the test

	done := completeFixture(t, h, job.ID, f.Path)
	require.Equal(t, store.JobStatusDone, done.Status, "a postprocess failure must not fail the job")

	// The failure is a warn line in the job log, with the plugin name.
	lines := pluginLogLines(t, h, job.ID)
	require.NotEmpty(t, lines)
	var warn bool
	for _, l := range lines {
		if strings.Contains(l.Message, "boom") || strings.Contains(l.Message, "exit status 7") {
			warn = true
			assert.Equal(t, store.JobLogWarn, l.Level)
			assert.Contains(t, l.Message, "crash-after")
		}
	}
	assert.True(t, warn, "the postprocess failure must be logged as a warn line")
}

// TestNotifyPhaseRunsForDoneAndFailed (task 31): postprocess runs only
// for a done job; notify runs for done and failed.
func TestNotifyPhaseRunsForDoneAndFailed(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)

	// One notify plugin for both outcomes and one postprocess plugin that
	// only runs for a done job.
	enablePlugin(t, h, installPlugin(t, h, "bell", store.PluginTypeNotify,
		"echo status $SYS_PHASE", "").ID)
	enablePlugin(t, h, installPlugin(t, h, "afterwards", store.PluginTypePostprocess,
		"echo post ran", "").ID)
	h.svc.plugins.inline = true

	// A done job: both phases run.
	_, f, job := claimJob(t, h, "node-1")
	completeFixture(t, h, job.ID, f.Path)
	lines := pluginLogLines(t, h, job.ID)
	var postRan, notifyRan bool
	for _, l := range lines {
		if strings.Contains(l.Message, "post ran") {
			postRan = true
		}
		if strings.Contains(l.Message, "status notify") {
			notifyRan = true
		}
	}
	assert.True(t, postRan, "postprocess must run for a done job")
	assert.True(t, notifyRan, "notify must run for a done job")

	// A failed job: only notify runs. (The harness's two libraries share
	// one root, so the test picks fail.mkv by name.)
	lib2 := h.createLibrary(false)
	h.prober.mu.Lock()
	h.prober.byName["fail.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "fail.mkv", 2048)
	h.scan(lib2.ID)
	fs := h.files(lib2.ID)
	file2 := fs["fail.mkv"]
	require.NotNil(t, file2)
	_, queued, err := h.svc.Queue().QueueJob(context.Background(), lib2, file2, []string{"video is h264"})
	require.NoError(t, err)
	require.False(t, queued, "the scan already queued the job")
	_, err = h.svc.Queue().Claim(context.Background(), "node-1")
	require.NoError(t, err)
	jobs := h.jobsOf(file2.ID)
	require.Len(t, jobs, 1)
	require.Equal(t, store.JobStatusClaimed, jobs[0].Status)
	_, err = h.svc.Queue().Fail(context.Background(), jobs[0].ID, "node-1", FailParams{Error: "encoder exploded"})
	require.NoError(t, err)

	lines = pluginLogLines(t, h, jobs[0].ID)
	var notifyFailedRan bool
	for _, l := range lines {
		assert.NotContains(t, l.Message, "post ran", "postprocess must not run for a failed job")
		if strings.Contains(l.Message, "status notify") {
			notifyFailedRan = true
		}
	}
	assert.True(t, notifyFailedRan, "notify must run for a failed job")
}

// TestPluginInstalledDisabledDoesNothing (task 31): a plugin directory
// with a valid manifest installs, appears disabled, and does nothing
// until enabled — proven end to end through the service.
func TestPluginInstalledDisabledDoesNothing(t *testing.T) {
	shAvailable(t)
	h := newHarness(t)
	lib, _ := queueFixture(t, h)

	// Install from a directory (the manifest is valid).
	inst, err := h.svc.Plugins().Install(context.Background(),
		InstallRequest{Path: writePluginSource(t, "lazy-plugin", store.PluginTypePreflight,
			`"sh", "run.sh"`, "")})
	require.NoError(t, err)
	assert.False(t, inst.Enabled, "installed plugins are disabled by default")
	assert.Empty(t, inst.Values)

	// It does nothing until enabled: the claim succeeds and the log is empty.
	job, err := h.svc.Queue().Claim(context.Background(), "node-x")
	require.NoError(t, err)
	assert.Empty(t, pluginLogLines(t, h, job.ID))

	// Enabled: now the plugin's verdict applies to the next claim. (The
	// fixture's one job was already claimed above, so the test queues a
	// second file.)
	h.prober.mu.Lock()
	h.prober.byName["second.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "second.mkv", 2048)
	h.scan(lib.ID)
	fs := h.files(lib.ID)
	require.Len(t, fs, 2)
	file2 := fs["second.mkv"]
	require.NotNil(t, file2)
	enablePlugin(t, h, inst.ID)
	_, err = h.svc.Queue().Claim(context.Background(), "node-x")
	require.ErrorIs(t, err, ErrPreflightRejected)
	jobs := h.jobsOf(file2.ID)
	require.Len(t, jobs, 1)
	require.Equal(t, store.JobStatusFailed, jobs[0].Status)
	assert.Contains(t, jobs[0].Error, "lazy-plugin")
}
