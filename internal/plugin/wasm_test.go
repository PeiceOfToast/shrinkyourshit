// SPDX-License-Identifier: AGPL-3.0-only

package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureDir is the committed testdata directory with the generated
// fixture modules (regenerate: go run ./internal/plugin/testdata/gen).
const fixtureDir = "testdata"

// wasmFixture builds a wasm plugin whose entry is a committed fixture.
// The manifest is validated the same way install does.
func wasmFixture(t *testing.T, name, file string, mods ...func(*Manifest)) Plugin {
	t.Helper()
	m := Manifest{
		Name:    name,
		Version: "1.0.0",
		Type:    TypeStep,
		Runtime: RuntimeWASM,
		Entry:   file,
	}
	for _, mod := range mods {
		mod(&m)
	}
	require.NoError(t, m.Validate())
	return Plugin{Manifest: m, Dir: fixtureDir}
}

// stepScratch lays out a scratch dir with a working file in a job
// subdirectory, the same shape a real job has.
func stepScratch(t *testing.T) (scratch, working string) {
	t.Helper()
	scratch = t.TempDir()
	working = filepath.Join(scratch, "job-j1", "source.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))
	return scratch, working
}

func runStep(t *testing.T, p Plugin, scratch, working string) StepOutcome {
	t.Helper()
	return NewRunner(t.TempDir()).RunStep(context.Background(), p, StepRequest{
		Job:       JobContext{JobID: "j1", Library: "Movies"},
		Scratch:   scratch,
		FilePath:  working,
		StepIndex: 1,
	})
}

// TestRunWasmHelloStdoutReachesJobLog (task 33): hello.wasm reads stdin,
// prints one line, exits 0. The line arrives as a stdout line — the job
// log writes these with source: plugin — and the missing trailing newline
// is flushed by the capture, not lost.
func TestRunWasmHelloStdoutReachesJobLog(t *testing.T) {
	p := wasmFixture(t, "hello-wasm", "hello.wasm")
	res := NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.Ran)
	require.True(t, res.OK(), res.Error)
	assert.Equal(t, 0, res.ExitCode)
	require.Len(t, res.Output, 1)
	assert.Equal(t, "hello from wasm", res.Output[0].Text)
	assert.False(t, res.Output[0].Stderr, "stdout lines reach the job log as source: plugin")
}

// TestRunWasmDefaultManifestIsNative (task 33): a manifest without
// runtime behaves exactly as before — the native runner sees it.
func TestRunWasmDefaultManifestIsNative(t *testing.T) {
	p := Plugin{Manifest: Manifest{
		Name:    "still-native",
		Version: "1.0.0",
		Type:    TypeNotify,
		Command: []string{"sh", "-c", "echo native"},
	}}
	res := NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.OK(), res.Error)
	assert.Equal(t, []string{"native"}, res.OutputText())
}

// TestRunStepWasmRewritesInPlace (task 33): rewrite.wasm reads the
// scratch-relative file_path from the stdin JSON, opens the file through
// the preopen and overwrites it. The change is on disk, the handover is
// the original path (in place), and the chain continues exactly as in the
// task 32 in-place test.
func TestRunStepWasmRewritesInPlace(t *testing.T) {
	scratch, working := stepScratch(t)
	p := wasmFixture(t, "wasm-rewriter", "rewrite.wasm")
	out := runStep(t, p, scratch, working)

	require.True(t, out.Result.Ran)
	require.True(t, out.Result.OK(), out.Result.Error)
	assert.Equal(t, working, out.File,
		"no result line: the working file continues down the chain")

	raw, err := os.ReadFile(working)
	require.NoError(t, err)
	assert.Equal(t, "WASM WAS HERE\n", string(raw), "the module rewrote the working file")
}

// TestRunStepWasmCannotOpenEtcPasswd (task 33): escape.wasm asks the WASI
// layer for /etc/passwd. The open fails (the only preopen is the job's
// scratch dir), the module reports the errno itself on stdout, the job
// still succeeds, and the report is what reaches the job log.
func TestRunStepWasmCannotOpenEtcPasswd(t *testing.T) {
	scratch, working := stepScratch(t)
	p := wasmFixture(t, "escape-attempt", "escape.wasm")
	out := runStep(t, p, scratch, working)

	require.True(t, out.Result.Ran)
	require.True(t, out.Result.OK(), out.Result.Error, "the failed open is the module's business, not a job failure")
	assert.Equal(t, working, out.File)

	lines := out.Result.OutputText()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "open /etc/passwd: errno=", "the module's own report is in the job log")
	assert.NotContains(t, lines[0], "errno=0", "the open must not succeed")
}

// TestRunWasmHookHasNoFilesystem (task 33): a hook phase has no scratch
// dir and the module gets no preopen at all — the same open attempt fails
// with a bad-fd errno instead.
func TestRunWasmHookHasNoFilesystem(t *testing.T) {
	p := wasmFixture(t, "escape-attempt", "escape.wasm")
	res := NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.OK(), res.Error)
	require.Len(t, res.Output, 1)
	assert.Contains(t, res.Output[0].Text, "open /etc/passwd: errno=")
	assert.NotContains(t, res.Output[0].Text, "errno=0")
}

// TestRunStepWasmBadFileFailsJob (task 33): badfile.wasm prints the
// handover result line naming /etc/passwd. The scratch-only check fails
// the step with the path in the message; the chain never continues.
func TestRunStepWasmBadFileFailsJob(t *testing.T) {
	scratch, working := stepScratch(t)
	p := wasmFixture(t, "bad-wasm", "badfile.wasm")
	out := runStep(t, p, scratch, working)

	require.True(t, out.Result.Ran)
	require.False(t, out.Result.OK())
	assert.Contains(t, out.Result.Error, "/etc/passwd")
	assert.Contains(t, out.Result.Error, "outside the job's scratch directory")
	assert.Empty(t, out.File, "a rejected result never continues the chain")
}

// TestRunStepWasmSpinTimeout (task 33): spin.wasm loops forever with no
// imports; only WithCloseOnContextDone makes the deadline interrupt the
// guest. The run ends within the deadline's neighbourhood and fails.
func TestRunStepWasmSpinTimeout(t *testing.T) {
	scratch, working := stepScratch(t)
	p := wasmFixture(t, "spin-wasm", "spin.wasm", func(m *Manifest) { m.TimeoutSeconds = 1 })

	start := time.Now()
	out := runStep(t, p, scratch, working)

	assert.Less(t, time.Since(start), 10*time.Second, "the deadline stopped the spinning module")
	require.True(t, out.Result.Ran)
	require.False(t, out.Result.OK())
	assert.True(t, out.Result.TimedOut)
	assert.Contains(t, out.Result.Error, "timed out after 1s")
	assert.Empty(t, out.File)
}

// TestRunStepWasmOomTrapsAtMemoryLimit (task 33): oom.wasm declares a
// memory max (64 pages) above the manifest's budget (memory_pages 1
// clamps to the 16-page floor). wazero rejects the module with its own
// limit error text and the job fails.
func TestRunStepWasmOomTrapsAtMemoryLimit(t *testing.T) {
	scratch, working := stepScratch(t)
	p := wasmFixture(t, "oom-wasm", "oom.wasm", func(m *Manifest) { m.MemoryPages = 1 })
	out := runStep(t, p, scratch, working)

	require.True(t, out.Result.Ran)
	require.False(t, out.Result.OK())
	assert.Equal(t, -1, out.Result.ExitCode)
	assert.Contains(t, out.Result.Error, "over limit", "the wazero error text is in the job log")
	assert.Empty(t, out.File)
}
