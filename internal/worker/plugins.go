// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// Step plugins on the node (task 32). The spec carries one resolved
// PluginStep per plugin step of the flow (manifest facts, merged settings
// and the chain position); the node runs each entry at its step_index
// point in the chain, from its own <data dir>/plugins/<name>/ directory.
// The server never ships plugin files in v1: a single install with role
// `both` is the common case, and the not-installed message tells a
// distributed user what to do.

// pluginNameOf returns a plugin plan step's plugin name, or the step id
// when its parameters cannot be read (error messages stay readable).
func pluginNameOf(step flow.PlanStep) string {
	if p, err := flow.DecodePluginStep(step); err == nil {
		return p.Plugin
	}
	return step.ID
}

// execPluginStep runs one resolved plugin step (from the job spec) on the
// current working file and returns the file the rest of the chain continues
// with. The returned error is the job's failure message (it names the
// plugin and, on non-zero exit, carries the exit code and the last 20 lines
// of stderr).
//
// Failure rules, deliberately different from the notify/postprocess hooks
// (task 32): a non-zero exit fails the job; a timeout kills the process
// group and fails the job; a rejected result file fails the job. The
// result policy never runs on a failed job, so the original stays byte
// identical. Not installed on this node is unconditional — even with
// continue_on_error, because it is an installation error, not a plugin
// verdict.
func (r *runner) execPluginStep(ctx context.Context, spec *server.JobSpec, ps server.PluginStep,
	working, scratchDir string, sink *jobLogSink) (string, error) {

	name := ps.Name

	// The node's own install of the plugin. Missing: fail naming the
	// plugin and the node.
	nodeName := r.cfg.NodeName
	if nodeName == "" {
		nodeName = "this node"
	}
	dir := ""
	if r.cfg.DataDir != "" {
		dir = plugin.Dir(r.cfg.DataDir, name)
		if st, serr := os.Stat(dir); serr != nil || !st.IsDir() {
			return "", fmt.Errorf("plugin %s is not installed on node %s", name, nodeName)
		}
	} else {
		return "", fmt.Errorf("plugin %s is not installed on node %s (no data directory configured)", name, nodeName)
	}

	if r.plugs == nil {
		return "", fmt.Errorf("plugin %s cannot run: this worker has no plugin support", name)
	}
	out := r.plugs.RunStep(ctx, plugin.Plugin{
		Manifest: plugin.Manifest{
			Name:           ps.Name,
			Command:        ps.Command,
			Env:            ps.EnvNames,
			TimeoutSeconds: ps.TimeoutSec,
			// Runtime facts (task 33): how the step executes and, for a
			// wasm plugin, its entry module file and memory budget.
			Runtime:     ps.Runtime,
			Entry:       ps.Entry,
			MemoryPages: ps.MemoryPages,
		},
		Dir:      dir,
		Settings: ps.Settings,
	}, plugin.StepRequest{
		Job: plugin.JobContext{
			JobID:       spec.ID,
			SourcePath:  spec.SourcePath,
			BytesBefore: spec.BytesBefore,
			Status:      "running",
		},
		Scratch:    scratchDir,
		FilePath:   working,
		StepIndex:  ps.StepIndex,
		Settings:   ps.Settings,
		SourcePath: spec.SourcePath,
	})

	// Captured output into the job log (the task 27 viewer), the plugin
	// name leading the message; stdout is info, stderr warn.
	for _, line := range out.Result.Output {
		if line.Stderr {
			sink.pluginWarnf(name, "%s", line.Text)
			continue
		}
		sink.pluginInfof(name, "%s", line.Text)
	}

	// verdict turns a non-OK run into the job's failure message.
	verdict := func(res plugin.Result) error {
		switch {
		case !res.Ran:
			// A skipped run (missing required setting, cancelled caller)
			// is not a verdict: on the critical path the step must fail.
			sink.pluginWarnf(name, "skipped: %s", res.SkipReason)
			return fmt.Errorf("plugin %s did not run: %s", name, res.SkipReason)
		case res.TimedOut:
			// Exceeding timeout_seconds kills the process group and fails
			// the job (task 32; unlike the hook phases).
			return fmt.Errorf("plugin %s %s", name, res.Error)
		case strings.HasPrefix(res.Error, "could not start"):
			// The manifest declares an entry point that is missing or not
			// runnable on this node: the same not-installed message.
			return fmt.Errorf("plugin %s is not installed on node %s (%s)", name, nodeName, res.Error)
		default:
			// Non-zero exit (or a rejected result file): the job's error
			// carries the plugin name, the exit code and the last 20
			// lines of stderr.
			return fmt.Errorf("plugin %s failed: %s", name, stderrTail(res, 20))
		}
	}
	if out.Result.OK() {
		sink.pluginInfof(name, "exited 0; the chain continues with %s", out.File)
		return out.File, nil
	}
	failErr := verdict(out.Result)
	if ps.ContinueOnError && !strings.HasPrefix(out.Result.Error, "could not start") {
		// continue_on_error exists for logging and sidecar style steps
		// (docs/plugins.md): a failing step is a warning and the chain
		// continues. An entry point that cannot even start is an
		// installation problem, not a plugin verdict, and fails the job
		// regardless.
		sink.pluginWarnf(name, "failed but the flow continues on error: %v", failErr)
		return working, nil
	}
	return "", failErr
}

// walkPluginFallback runs a spec's resolved plugin steps in chain order
// against the working file — the pre-task-14 fallback for specs that lack a
// flow graph. Each step is re-probed so later guards (in the encode's own
// skip-if-satisfied logic) and the encode see what the plugin produced.
func (r *runner) walkPluginFallback(ctx context.Context, spec *server.JobSpec,
	working *string, cur *probe.MediaInfo, dir string, sink *jobLogSink) error {
	for _, ps := range spec.Plugins {
		next, err := r.execPluginStep(ctx, spec, ps, *working, dir, sink)
		if err != nil {
			return err
		}
		info, perr := r.prober(ctx, r.cfg.FFprobePath, next)
		if perr != nil {
			return fmt.Errorf("plugin %s: probe the working file after the step: %v", ps.Name, perr)
		}
		*working = next
		*cur = info
	}
	return nil
}

// pluginStepFor finds the spec's resolved PluginStep for the plan step at
// chain position index.
func pluginStepFor(plugins []server.PluginStep, index int) (server.PluginStep, bool) {
	for _, ps := range plugins {
		if ps.StepIndex == index {
			return ps, true
		}
	}
	return server.PluginStep{}, false
}

// stderrTail builds the error suffix of a failed run: the exit code plus
// the last n stderr lines, capped so a job's error message stays
// readable. A rejected result file (validation) surfaces through res.Error.
func stderrTail(res plugin.Result, n int) string {
	if res.TimedOut {
		return res.Error
	}
	var lines []string
	for _, l := range res.Output {
		if l.Stderr {
			lines = append(lines, l.Text)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 0 {
		return res.Error
	}
	return fmt.Sprintf("exit status %d: %s", res.ExitCode, strings.Join(lines, " | "))
}
