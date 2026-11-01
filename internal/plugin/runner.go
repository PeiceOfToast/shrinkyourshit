// SPDX-License-Identifier: AGPL-3.0-only

package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxOutputLines caps one run's captured output (the store's per-job log
// cap of 2000 rows still applies on append; this keeps one plugin's
// result bounded on its own).
const MaxOutputLines = 1000

// JobContext is the JSON document every plugin process receives on stdin
// (task 31). The exact shape is a documented contract (docs/plugins.md):
// job_id, library, source_path, output_path, bytes_before, bytes_after,
// encoder, status, phase. output_path is empty when no output was applied
// (the result policy kept the original, or the output could not be
// verified); the plugin decides what that means for it.
//
// A step plugin (task 32) receives three more fields, filled only for a
// step run: file_path (the working file the plugin operates on),
// step_index (its position in the executed chain) and settings (the
// merged settings for this step).
type JobContext struct {
	JobID       string            `json:"job_id"`
	Library     string            `json:"library,omitempty"`
	SourcePath  string            `json:"source_path,omitempty"`
	OutputPath  string            `json:"output_path,omitempty"`
	BytesBefore int64             `json:"bytes_before"`
	BytesAfter  int64             `json:"bytes_after,omitempty"`
	Encoder     string            `json:"encoder,omitempty"`
	Status      string            `json:"status"`
	Phase       string            `json:"phase"`
	FilePath    string            `json:"file_path,omitempty"`
	StepIndex   int               `json:"step_index,omitempty"` // hook contexts leave it unset
	Settings    map[string]string `json:"settings,omitempty"`
}

// StepResult is one JSON object a step plugin may print as its final
// stdout line to hand the rest of the chain a different file (task 32).
// The exact shape is the documented contract: {"file": "<absolute path>"}.
type StepResult struct {
	File string `json:"file"`
}

// Plugin is one installed plugin ready to run: its manifest, the
// directory it lives in (the command's working directory) and the
// settings values collected in the UI.
type Plugin struct {
	Manifest Manifest
	Dir      string
	Settings map[string]string
}

// OutputLine is one captured output line of a run. Stderr marks the
// stream, which the job log mirrors as the line's level.
type OutputLine struct {
	Text   string
	Stderr bool
}

// Result is the outcome of one plugin run.
type Result struct {
	// Ran is false when the plugin did not run at all (a required
	// setting has no value); SkipReason then says why. It is also false
	// for an empty command, which a validated manifest never has.
	Ran        bool
	SkipReason string
	// ExitCode is the process's exit code; -1 when it was killed or
	// could not be started.
	ExitCode int
	// Error is "" on success, otherwise a short reason (exit status,
	// timeout, start failure).
	Error    string
	TimedOut bool
	// Output holds the captured stdout and stderr lines in arrival
	// order, capped at MaxOutputLines.
	Output []OutputLine
}

// OK reports a run that executed and exited 0.
func (r Result) OK() bool { return r.Ran && r.Error == "" }

// OutputText returns just the lines' texts, in order (the test
// endpoint's payload).
func (r Result) OutputText() []string {
	out := make([]string, 0, len(r.Output))
	for _, l := range r.Output {
		out = append(out, l.Text)
	}
	return out
}

// Runner executes plugin processes (task 31). One instance is shared by
// the server; it is safe for concurrent use.
type Runner struct {
	// DataDir is reported to the plugin as SYS_DATA_DIR.
	DataDir string
}

// NewRunner builds a runner. An empty dataDir is acceptable for tests;
// the process then sees an empty SYS_DATA_DIR.
func NewRunner(dataDir string) *Runner {
	return &Runner{DataDir: dataDir}
}

// Run executes the plugin once for phase. The job context goes to the
// process's stdin as JSON with phase set; the returned Result carries the
// captured output and the verdict. Run never panics and never blocks
// beyond the manifest's timeout: the process group is killed on timeout.
func (r *Runner) Run(ctx context.Context, p Plugin, job JobContext, phase string) Result {
	// A wasm plugin (task 33) runs in the sandbox instead of a subprocess;
	// a hook phase has no scratch dir, so the module gets no filesystem.
	if p.Manifest.Runtime == RuntimeWASM {
		job.Phase = phase
		res, _ := r.RunWASM(ctx, p, job, "")
		return res
	}
	return r.exec(ctx, p, job, phase, nil, nil)
}

// StepRequest is one step-plugin run (task 32). Scratch is the job's
// scratch directory — the only place a result file may live; FilePath is
// the working file the plugin operates on; StepIndex is its zero-based
// position in the executed chain; Settings are the merged settings for
// this step (per-step overrides over the plugin's stored values);
// SourcePath is the original library file, reported on stdin and never a
// writable path for the plugin.
type StepRequest struct {
	Job        JobContext
	Scratch    string
	FilePath   string
	StepIndex  int
	Settings   map[string]string
	SourcePath string
}

// StepOutcome is the outcome of one step-plugin run: the raw run result
// plus the file the rest of the chain continues with ("" when the run did
// not succeed; the caller fails the job).
type StepOutcome struct {
	Result Result
	// File is the working file the chain continues with. It is req's
	// FilePath when the plugin left the file in place, the named result
	// file when it handed a different one over, and "" when the run
	// failed or its result was rejected.
	File string
}

// RunStep runs a plugin as a flow step (task 32). It reuses the task 31
// run with the same captured stdout, stderr and timeout handling; the
// stdin JSON gains file_path, step_index and settings, and the
// environment gains SYS_STEP_INDEX (SYS_PHASE is "step").
//
// Result protocol: the plugin may modify FilePath in place (no result
// line needed), or print one final JSON object {"file": "<absolute
// path>"} on stdout to hand the rest of the chain a different file. A
// named file must resolve inside the job's scratch directory (symlinks
// followed — anything resolving outside is a failure naming both paths)
// and must exist. A non-zero exit, a timeout, a rejected result and a
// skipped run all surface as a non-OK Result with the reason in Error;
// the caller decides (a step failure fails the job).
func (r *Runner) RunStep(ctx context.Context, p Plugin, req StepRequest) StepOutcome {
	if req.Scratch == "" {
		return StepOutcome{Result: Result{SkipReason: "the step has no scratch directory"}} // defensive; a job always has one
	}
	job := req.Job
	job.Phase = TypeStep
	job.FilePath = req.FilePath
	job.SourcePath = req.SourcePath
	job.StepIndex = req.StepIndex
	job.Settings = req.Settings

	// A wasm plugin (task 33) runs in the sandbox: the job scratch dir is
	// preopened at guest "." and the same stdin JSON and env contract
	// apply, with SYS_STEP_INDEX included. The result protocol below is
	// identical for both runtimes.
	var res Result
	if p.Manifest.Runtime == RuntimeWASM {
		res, _ = r.RunWASM(ctx, p, job, req.Scratch)
	} else {
		res = r.exec(ctx, p, job, TypeStep, []string{fmt.Sprintf("SYS_STEP_INDEX=%d", req.StepIndex)},
			[]string{req.FilePath})
	}
	out := StepOutcome{Result: res}
	if !res.OK() {
		return out
	}

	// The result line: the last stdout line that is exactly a
	// {"file": "<absolute path>"} object. Anything else on stdout is
	// plugin log output and lands in the job log as before.
	named := ""
	for _, line := range res.Output {
		if line.Stderr {
			continue
		}
		var sr StepResult
		dec := json.NewDecoder(strings.NewReader(line.Text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&sr); err == nil && !dec.More() && sr.File != "" {
			named = sr.File
		}
	}
	if named == "" {
		out.File = req.FilePath // the plugin worked in place
		return out
	}
	// A wasm module (task 33) sees only the scratch dir, so a result line
	// it prints names the file relative to the preopen; join before the
	// absolute-path validation, which still enforces the scratch-only rule.
	if p.Manifest.Runtime == RuntimeWASM && !filepath.IsAbs(named) {
		named = filepath.Join(req.Scratch, named)
	}
	if err := validateStepFile(named, req.Scratch); err != nil {
		// A rejected result is a failed step: the caller fails the job
		// with the offending path(s) in the message.
		out.Result.Error = fmt.Sprintf("rejected result file: %s", err)
		return out
	}
	out.File = named
	return out
}

// validateStepFile enforces the scratch-only contract (task 32): the
// named file must be an absolute path that exists and — symlinks
// followed — resolve inside the job's scratch directory. Anything else
// fails with a message naming the offending path(s).
func validateStepFile(name, scratch string) error {
	switch {
	case name == "":
		return fmt.Errorf("the result file path is empty")
	case !filepath.IsAbs(name):
		return fmt.Errorf("the result file %q is not an absolute path", name)
	}
	root, err := filepath.EvalSymlinks(scratch)
	if err != nil {
		return fmt.Errorf("the job scratch directory %q is unreadable: %v", scratch, err)
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("the result file %q does not exist", name)
		}
		return fmt.Errorf("the result file %q is unreadable: %v", name, err)
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return fmt.Errorf("the result file %q resolves to %q which is outside the job's scratch directory %q", name, resolved, scratch)
	}
	return nil
}

// exec is the shared process run (task 31): required settings gate the
// run, the stdin JSON is the job context, the environment is exactly
// SYS_PLUGIN_NAME, SYS_PHASE, SYS_DATA_DIR, the manifest's declared names
// and (task 32, step runs) extraEnv's variables. extraArgs are appended
// after the manifest's command (task 32, step runs: the working file, so a
// plugin script never has to parse JSON to find it). The process group is
// killed at the manifest's timeout.
func (r *Runner) exec(ctx context.Context, p Plugin, job JobContext, phase string, extraEnv, extraArgs []string) Result {
	job.Phase = phase

	// Required settings gate the run: a missing value means the plugin
	// does not run and the reason is logged by the caller (task 31: it
	// never falls back to a guess).
	for _, s := range p.Manifest.Settings {
		if !s.Required {
			continue
		}
		if strings.TrimSpace(p.Settings[s.Key]) == "" {
			return Result{SkipReason: fmt.Sprintf("required setting %q is not set", s.Key)}
		}
	}
	if len(p.Manifest.Command) == 0 {
		return Result{SkipReason: "the manifest has no command"}
	}

	env := []string{
		"SYS_PLUGIN_NAME=" + p.Manifest.Name,
		"SYS_PHASE=" + phase,
		"SYS_DATA_DIR=" + r.DataDir,
	}
	env = append(env, extraEnv...)
	// Declared env names only, resolved from the plugin settings first
	// and the server environment second. Nothing else is inherited: a
	// plugin sees exactly the variables its manifest declared.
	for _, name := range p.Manifest.Env {
		if v := p.Settings[name]; strings.TrimSpace(v) != "" {
			env = append(env, name+"="+v)
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}

	raw, err := json.Marshal(job)
	if err != nil {
		return Result{SkipReason: "could not build the job context: " + err.Error()}
	}

	timeout := time.Duration(p.Manifest.Timeout()) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append(append([]string{}, p.Manifest.Command[1:]...), extraArgs...)
	cmd := exec.CommandContext(runCtx, p.Manifest.Command[0], args...)
	cmd.Dir = p.Dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(raw)
	setProcessGroup(cmd) // unix: the timeout kills the whole process group

	out := &outputCapture{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{SkipReason: "could not open the plugin's stdout: " + err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{SkipReason: "could not open the plugin's stderr: " + err.Error()}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go out.read(&wg, stdout, false)
	go out.read(&wg, stderr, true)

	// Start, then drain both pipes fully, and only then Wait. Wait closes
	// the pipe descriptors as soon as it sees the command exit, so reading
	// after (or during) it can lose the lines still buffered in the pipe —
	// the output of a fast plugin would vanish from the job log under
	// load. The readers see EOF once the process (or the timeout's group
	// kill) has closed its ends; the drain is then complete and Wait only
	// reaps.
	startErr := cmd.Start()
	if startErr != nil {
		// Nothing will ever write the pipes; unblock the readers (a
		// failed Start may leave them open) and collect what there is.
		_ = stdout.Close()
		_ = stderr.Close()
		wg.Wait()
		return Result{Ran: true, ExitCode: -1, Output: out.lines,
			Error: "could not start: " + startErr.Error()}
	}
	wg.Wait()
	startErr = cmd.Wait()

	res := Result{Ran: true, Output: out.lines}
	switch {
	case ctx.Err() != nil:
		// The caller is going away; report the run as not happened
		// rather than as a plugin failure.
		res.Ran = false
		res.SkipReason = "cancelled"
	case runCtx.Err() == context.DeadlineExceeded:
		res.TimedOut = true
		res.ExitCode = -1
		res.Error = fmt.Sprintf("timed out after %s and was killed", timeout)
	case startErr != nil:
		if ee, ok := startErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			res.Error = fmt.Sprintf("exit status %d", ee.ExitCode())
		} else {
			res.ExitCode = -1
			res.Error = "could not start: " + startErr.Error()
		}
	}
	return res
}

// outputCapture collects stdout and stderr lines under a mutex, in
// arrival order, capped at MaxOutputLines.
type outputCapture struct {
	mu    sync.Mutex
	lines []OutputLine
}

// read scans one stream until EOF, tagging the lines with their stream.
func (o *outputCapture) read(wg *sync.WaitGroup, stream io.Reader, stderr bool) {
	defer wg.Done()
	sc := bufio.NewScanner(stream)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024) // lines up to 64 KiB
	for sc.Scan() {
		o.addLine(sc.Text(), stderr)
	}
}

// addLine appends one captured line, capped at MaxOutputLines.
func (o *outputCapture) addLine(text string, stderr bool) {
	o.mu.Lock()
	if len(o.lines) < MaxOutputLines {
		o.lines = append(o.lines, OutputLine{Text: text, Stderr: stderr})
	}
	o.mu.Unlock()
}
