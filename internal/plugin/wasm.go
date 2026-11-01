// SPDX-License-Identifier: AGPL-3.0-only

// The wasm sandbox (task 33). A plugin with runtime: wasm runs as a WASI
// module inside wazero instead of as a native subprocess. The sandbox is
// explicit: the module gets stdin (the job JSON), stdout and stderr (both
// captured into the job log), exactly one directory preopened read-write
// (the job's scratch dir, or nothing for a hook phase) and the env names
// its manifest declared. No other host path, no network, no subprocesses,
// a memory ceiling and a wall-clock deadline.
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	wzsys "github.com/tetratelabs/wazero/sys"
)

// wasmMagic is the \0asm four-byte prefix of every wasm binary; the
// install-time entry check uses it to reject a non-module before wazero
// even parses one.
var wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}

// RunWASM executes a WASI module with the plugin contract. The module gets
// stdin, stdout, stderr, the job scratch dir preopened read-write at the
// guest path "." and the env names from the manifest. No other directory,
// no clock precision guarantees, no network, no subprocesses. The Result
// protocol is the same as a native plugin's: the captured output carries
// the verdict, and a step's file handover is validated by the caller.
func (r *Runner) RunWASM(ctx context.Context, p Plugin, job JobContext, scratch string) (Result, error) {
	// Required settings gate the run: a missing value means the plugin
	// does not run and the reason is logged by the caller (task 31: it
	// never falls back to a guess).
	for _, s := range p.Manifest.Settings {
		if !s.Required {
			continue
		}
		if strings.TrimSpace(p.Settings[s.Key]) == "" {
			return Result{SkipReason: fmt.Sprintf("required setting %q is not set", s.Key)}, nil
		}
	}
	if p.Manifest.Entry == "" {
		return Result{SkipReason: "the manifest has no entry"}, nil
	}

	module, err := os.ReadFile(filepath.Join(p.Dir, p.Manifest.Entry))
	if err != nil {
		return Result{Ran: true, ExitCode: -1,
			Error: "could not start: read " + p.Manifest.Entry + ": " + err.Error()}, nil
	}

	env := pluginEnv(r.DataDir, p, job)
	// A wasm module sees the scratch dir as its root: the WASI layer
	// rejects absolute paths (fs.ValidPath rejects a leading "/"), so the
	// guest's file_path is the path relative to the preopen, not the
	// absolute host path (task 33). A hook phase has no scratch dir and
	// the module cannot touch any file at all.
	if scratch != "" && job.FilePath != "" {
		if rel, err := filepath.Rel(scratch, job.FilePath); err == nil {
			job.FilePath = rel
		}
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return Result{Ran: true, ExitCode: -1,
			Error: "could not build the job context: " + err.Error()}, nil
	}

	timeout := time.Duration(p.Manifest.Timeout()) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		// The manifest's memory budget, in 64 KiB pages (task 33),
		// clamped by the manifest's own limits.
		WithMemoryLimitPages(uint32(p.Manifest.MemoryLimit())).
		// Without this a module spinning in an endless loop ignores the
		// context deadline (wazero only polls the context between host
		// calls); with it the deadline interrupts the guest CPU directly.
		WithCloseOnContextDone(true))
	// Close on a context that survives the run's cancellation, so a
	// cancelled run still frees the compiled code.
	defer func() { _ = rt.Close(context.WithoutCancel(ctx)) }()

	if _, err := wasiInstantiate(runCtx, rt); err != nil {
		return Result{Ran: true, ExitCode: -1,
			Error: "could not start: " + err.Error()}, nil
	}

	out := outputCapture{}
	stdoutW := &captureWriter{cap: &out}
	stderrW := &captureWriter{cap: &out, stderr: true}
	cfg := wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(raw)).
		WithStdout(stdoutW).
		WithStderr(stderrW).
		WithArgs(p.Manifest.Name)
	// WithEnv is per-pair in this wazero version (not variadic over
	// KEY=VALUE strings), so each entry is split once here.
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			cfg = cfg.WithEnv(kv[:i], kv[i+1:])
		}
	}
	if scratch != "" {
		// Preopen only the job's scratch dir, read-write, at guest path
		// ".". Nothing else: no "." from the host cwd, no data dir, no
		// library path. A hook phase passes no scratch dir and the module
		// has no filesystem at all — its stdin JSON is the whole contract.
		cfg = cfg.WithFSConfig(wazero.NewFSConfig().WithFSMount(jailedFS{root: scratch}, "."))
	}

	_, err = rt.InstantiateWithConfig(runCtx, module, cfg)
	// A module's last line may have no trailing newline; flush it into the
	// capture either way (the native runner's scanner does the same).
	stdoutW.flush()
	stderrW.flush()

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
	case err == nil || wasmExitCode(err) == 0:
		// _start returned normally or called proc_exit(0): success.
		res.ExitCode = 0
	default:
		// Only a real WASI proc_exit carries an exit status; anything else
		// is a trap (the memory limit, out of bounds memory, a missing
		// export, a malformed module) and the wazero error text goes to
		// the job log as-is.
		var ee *wzsys.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = int(ee.ExitCode())
			res.Error = fmt.Sprintf("exit status %d", ee.ExitCode())
		} else {
			res.ExitCode = -1
			res.Error = err.Error()
		}
	}
	return res, nil
}

// wasiInstantiate registers the WASI imports under the module's own
// (package-private) name so the import of wazero/imports/... stays in one
// place in this file.
func wasiInstantiate(ctx context.Context, rt wazero.Runtime) (api.Closer, error) {
	return wasi_snapshot_preview1.Instantiate(ctx, rt)
}

// wasmExitCode extracts the WASI exit code from an error, or -1 when the
// error is not a proc_exit (a trap, a deadline, anything else).
func wasmExitCode(err error) int32 {
	if err == nil {
		return 0
	}
	var ee *wzsys.ExitError
	if errors.As(err, &ee) {
		return int32(ee.ExitCode())
	}
	return -1
}

// pluginEnv builds the environment a plugin sees (task 31's contract,
// shared by the native and wasm runners): exactly SYS_PLUGIN_NAME,
// SYS_PHASE, SYS_DATA_DIR, the manifest's declared names resolved from the
// plugin settings first and the server environment second, and (task 32)
// SYS_STEP_INDEX for a step run. Nothing else is inherited.
func pluginEnv(dataDir string, p Plugin, job JobContext) []string {
	env := []string{
		"SYS_PLUGIN_NAME=" + p.Manifest.Name,
		"SYS_PHASE=" + job.Phase,
		"SYS_DATA_DIR=" + dataDir,
	}
	if job.Phase == TypeStep {
		env = append(env, fmt.Sprintf("SYS_STEP_INDEX=%d", job.StepIndex))
	}
	for _, name := range p.Manifest.Env {
		if v := p.Settings[name]; strings.TrimSpace(v) != "" {
			env = append(env, name+"="+v)
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// captureWriter adapts the module's stdout/stderr streams into the run's
// outputCapture, splitting raw bytes into lines like the native runner's
// scanner does. A final line without a newline is flushed at the end.
type captureWriter struct {
	cap    *outputCapture
	stderr bool
	buf    []byte
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.cap.addLine(strings.TrimRight(string(w.buf[:i]), "\r"), w.stderr)
		w.buf = w.buf[i+1:]
	}
	return len(b), nil
}

// flush emits the trailing partial line, if any.
func (w *captureWriter) flush() {
	if len(w.buf) > 0 {
		w.cap.addLine(strings.TrimRight(string(w.buf), "\r"), w.stderr)
		w.buf = nil
	}
}

// jailedFS is the only filesystem a wasm plugin sees: the job's scratch
// directory, rooted at the guest path ".". It implements fs.FS, which
// wazero's FSConfig.WithFSMount adapts into the WASI layer. Every path the
// guest names is resolved inside root and rejected when it would escape;
// symlinks inside root are followed only to targets still inside root, so
// a planted link cannot point the module outside.
//
// The adapter carries no open flags down to fs.FS (only the O_DIRECTORY
// check happens above us), so Open decides the mode itself: a regular file
// is opened read-write (the module needs write access for the step
// handover, and a read-only file fails at the host on write anyway), a
// missing file is created read-write when its parent chain stays inside
// root (the guest's O_CREAT requests are served here), and a directory is
// opened read-only. There is deliberately no O_NOFOLLOW: the symlink
// containment check below is the real guard, and it covers O_CREAT
// smuggling through a parent link too.
type jailedFS struct{ root string }

// Open implements fs.FS. It opens the guest path inside the jail, refusing
// anything that would leave root: an invalid path (fs.ValidPath rejects
// absolute paths and ".." elements), a symlink chain that resolves outside,
// or a create request whose parent chain resolves outside.
func (j jailedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	host := filepath.Join(j.root, name)
	resolved, err := filepath.EvalSymlinks(host)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		// The entry does not exist (yet): the guest asked for O_CREAT.
		// The parent chain must stay inside root, so a create cannot be
		// smuggled through a symlink.
		parent := filepath.Dir(host)
		parentResolved, perr := filepath.EvalSymlinks(parent)
		if perr != nil || !withinRoot(j.root, parentResolved) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return os.OpenFile(host, os.O_RDWR|os.O_CREATE, 0o600)
	}
	if !withinRoot(j.root, resolved) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if st.IsDir() {
		return os.Open(resolved)
	}
	// Regular file: read-write. A file the host made read-only fails at
	// the host on write, which is the same result the WASI layer would
	// give.
	return os.OpenFile(resolved, os.O_RDWR, 0)
}

// withinRoot reports whether p is root or a path inside it.
func withinRoot(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}
