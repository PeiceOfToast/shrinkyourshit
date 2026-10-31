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
type JobContext struct {
	JobID       string `json:"job_id"`
	Library     string `json:"library,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	OutputPath  string `json:"output_path,omitempty"`
	BytesBefore int64  `json:"bytes_before"`
	BytesAfter  int64  `json:"bytes_after,omitempty"`
	Encoder     string `json:"encoder,omitempty"`
	Status      string `json:"status"`
	Phase       string `json:"phase"`
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

	cmd := exec.CommandContext(runCtx, p.Manifest.Command[0], p.Manifest.Command[1:]...)
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
		o.mu.Lock()
		if len(o.lines) < MaxOutputLines {
			o.lines = append(o.lines, OutputLine{Text: sc.Text(), Stderr: stderr})
		}
		o.mu.Unlock()
	}
}
