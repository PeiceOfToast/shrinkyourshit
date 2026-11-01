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

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	r := NewRunner(t.TempDir())
	p := Plugin{Manifest: Manifest{
		Name:    "echo-plugin",
		Version: "1.0.0",
		Type:    TypeNotify,
		Command: []string{"sh", "-c", "echo hello; echo oops >&2"},
	}}
	res := r.Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.Ran)
	require.True(t, res.OK(), res.Error)
	assert.Equal(t, 0, res.ExitCode)
	// The two lines arrive concurrently from two stream readers, so the
	// test asserts on membership, not order.
	assert.ElementsMatch(t, []string{"hello", "oops"}, res.OutputText())
	for _, line := range res.Output {
		switch line.Text {
		case "hello":
			assert.False(t, line.Stderr, "stdout lines are not tagged stderr")
		case "oops":
			assert.True(t, line.Stderr, "stderr lines are tagged")
		}
	}
}

// TestRunCapturesOutputBeyondThePipeBuffer (task 31 regression): the
// runner must drain both pipes before Wait, because Wait closes them the
// moment the command exits and any lines still buffered in the pipe would
// be lost. A run whose output exceeds the 64 KiB pipe buffer (and the
// 1000-line capture cap) proves the drain: exactly the capped number of
// lines arrives, ending at the cap's last line.
func TestRunCapturesOutputBeyondThePipeBuffer(t *testing.T) {
	r := NewRunner(t.TempDir())
	p := Plugin{Manifest: Manifest{
		Name:    "chatty-plugin",
		Version: "1.0.0",
		Type:    TypeNotify,
		Command: []string{"sh", "-c", `i=0; while [ $i -lt 3000 ]; do echo "line $i"; i=$((i+1)); done`},
	}}
	res := r.Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.Ran)
	require.True(t, res.OK(), res.Error)
	lines := res.OutputText()
	require.Len(t, lines, MaxOutputLines, "every line up to the cap must arrive")
	assert.Equal(t, "line 0", lines[0])
	assert.Equal(t, "line 999", lines[MaxOutputLines-1],
		"the tail must not be lost to Wait closing the pipes early")
}

func TestRunFailureCarriesExitCode(t *testing.T) {
	r := NewRunner(t.TempDir())
	p := Plugin{Manifest: Manifest{
		Name:    "fail-plugin",
		Type:    TypePreflight,
		Command: []string{"sh", "-c", "echo nope; exit 3"},
	}}
	res := r.Run(context.Background(), p, JobContext{JobID: "j1"}, TypePreflight)
	require.True(t, res.Ran)
	require.False(t, res.OK())
	assert.Equal(t, 3, res.ExitCode)
	assert.Contains(t, res.Error, "exit status 3")
	assert.Equal(t, []string{"nope"}, res.OutputText())
}

func TestRunTimeoutKillsAndJobContextReachesStdin(t *testing.T) {
	dir := t.TempDir()
	// The script proves the stdin JSON arrives: it reads one line and
	// writes it to a file, then sleeps past the timeout.
	script := `cat > ` + filepath.Join(dir, "stdin.json") + `; sleep 30`
	p := Plugin{
		Manifest: Manifest{
			Name:           "sleeper",
			Type:           TypePostprocess,
			Command:        []string{"sh", "-c", script},
			TimeoutSeconds: 1,
		},
		Dir: dir,
	}
	r := NewRunner(dir)

	start := time.Now()
	res := r.Run(context.Background(), p, JobContext{JobID: "j9", Status: "done"}, TypePostprocess)
	elapsed := time.Since(start)

	require.True(t, res.Ran)
	require.False(t, res.OK())
	assert.True(t, res.TimedOut, "the run must be marked timed out")
	assert.Equal(t, -1, res.ExitCode)
	assert.Contains(t, res.Error, "timed out after 1s")
	assert.Less(t, elapsed, 10*time.Second, "the timeout killed the process group quickly")

	// The stdin JSON reached the process with phase and job id.
	raw, err := os.ReadFile(filepath.Join(dir, "stdin.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"job_id":"j9"`)
	assert.Contains(t, string(raw), `"phase":"postprocess"`)
}

func TestRunPassesEnvAndWorkingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYS_FROM_SERVER", "from-server")
	script := `pwd > pwd.txt; env | grep -E '^(SYS_PLUGIN_NAME|SYS_PHASE|SYS_DATA_DIR|MY_TOKEN|SYS_FROM_SERVER)=' | sort > env.txt`
	p := Plugin{
		Manifest: Manifest{
			Name:    "env-plugin",
			Type:    TypeNotify,
			Command: []string{"sh", "-c", script},
			Env:     []string{"MY_TOKEN", "SYS_FROM_SERVER"},
		},
		Dir:      dir,
		Settings: map[string]string{"MY_TOKEN": "sekret"},
	}
	res := NewRunner(dir).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.OK(), res.Error)

	pwd, err := os.ReadFile(filepath.Join(dir, "pwd.txt"))
	require.NoError(t, err)
	assert.Equal(t, dir+"\n", string(pwd), "the command runs in the plugin's directory")

	env, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	require.NoError(t, err)
	text := string(env)
	assert.Contains(t, text, "SYS_PLUGIN_NAME=env-plugin")
	assert.Contains(t, text, "SYS_PHASE=notify")
	assert.Contains(t, text, "SYS_DATA_DIR="+dir)
	assert.Contains(t, text, "MY_TOKEN=sekret", "an env name resolves from the plugin settings")
	assert.Contains(t, text, "SYS_FROM_SERVER=from-server", "an env name falls back to the server environment")
}

func TestRunEnvOnlyDeclaredNamesAreInherited(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHRINK_UNRELATED", "nope")
	script := `env | grep -c '^SHRINK_UNRELATED=' > found.txt; true`
	p := Plugin{
		Manifest: Manifest{
			Name:    "env-strict",
			Type:    TypeNotify,
			Command: []string{"sh", "-c", script},
		},
		Dir: dir,
	}
	res := NewRunner(dir).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.OK(), res.Error)
	raw, err := os.ReadFile(filepath.Join(dir, "found.txt"))
	require.NoError(t, err)
	assert.Equal(t, "0\n", string(raw), "undeclared server env must not reach the plugin")
}

func TestRunMissingRequiredSettingDoesNotRun(t *testing.T) {
	p := Plugin{
		Manifest: Manifest{
			Name:    "needs-token",
			Type:    TypeNotify,
			Command: []string{"sh", "-c", "echo should not run"},
			Settings: []Setting{
				{Key: "chat_id", Label: "Chat id", Required: true},
			},
		},
		Settings: map[string]string{},
	}
	res := NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.False(t, res.Ran)
	require.False(t, res.OK())
	assert.Contains(t, res.SkipReason, "chat_id")
	assert.Empty(t, res.Output)

	// With the value set the plugin runs.
	p.Settings = map[string]string{"chat_id": "123"}
	res = NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.True(t, res.OK())
}

func TestRunEmptyCommandDoesNotRun(t *testing.T) {
	p := Plugin{Manifest: Manifest{Name: "no-cmd", Type: TypeNotify}}
	res := NewRunner(t.TempDir()).Run(context.Background(), p, JobContext{JobID: "j1"}, TypeNotify)
	require.False(t, res.Ran)
	assert.Contains(t, res.SkipReason, "command")
}

func TestRunCancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := Plugin{Manifest: Manifest{
		Name:    "slow",
		Type:    TypePostprocess,
		Command: []string{"sh", "-c", "sleep 30"},
	}}
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	res := NewRunner(t.TempDir()).Run(ctx, p, JobContext{JobID: "j1"}, TypePostprocess)
	require.False(t, res.OK())
	assert.False(t, res.TimedOut, "a caller cancellation is not the manifest's timeout")
	assert.Contains(t, res.SkipReason, "cancelled")
}

/* ------------------------------------------------------------------ */
/* Step runs (task 32)                                                 */
/* ------------------------------------------------------------------ */

// stepContext dumps the stdin JSON, the step environment and the command
// line to files, so the test can assert the exact contract a step plugin
// stepContext dumps the stdin JSON, the step environment and the command
// line to files, so the test can assert the exact contract a step plugin
// receives. It is written to a script file and invoked as
// ["sh", scriptPath] — the same shape a real manifest uses.
const stepContextScript = `cat > stdin.json; env | grep -E '^SYS_' | sort > env.txt; echo "$#" > argc.txt; echo "$1" > filearg.txt`

func TestRunStepHandsTheStepContract(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	working := filepath.Join(scratch, "job-j1", "source.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))

	scriptPath := filepath.Join(dir, "sidecar.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(stepContextScript), 0o755))
	p := Plugin{Manifest: Manifest{
		Name:    "sidecar",
		Type:    TypeStep,
		Command: []string{"sh", scriptPath},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job:       JobContext{JobID: "j1", Library: "Movies"},
		Scratch:   scratch,
		FilePath:  working,
		StepIndex: 3,
		Settings:  map[string]string{"level": "quiet"},
		SourcePath: "/library/Movies/movie.mkv",
	})
	require.True(t, out.Result.OK(), out.Result.Error)
	require.Equal(t, working, out.File, "no result line: the working file continues down the chain")

	raw, err := os.ReadFile(filepath.Join(dir, "stdin.json"))
	require.NoError(t, err)
	text := string(raw)
	assert.Contains(t, text, `"job_id":"j1"`)
	assert.Contains(t, text, `"phase":"step"`)
	assert.Contains(t, text, `"file_path":"`+working+`"`)
	assert.Contains(t, text, `"source_path":"/library/Movies/movie.mkv"`)
	assert.Contains(t, text, `"step_index":3`)
	assert.Contains(t, text, `"settings":{"level":"quiet"}`)

	env, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	require.NoError(t, err)
	envText := string(env)
	assert.Contains(t, envText, "SYS_PHASE=step")
	assert.Contains(t, envText, "SYS_STEP_INDEX=3")
	assert.Contains(t, envText, "SYS_PLUGIN_NAME=sidecar")
	assert.Contains(t, envText, "SYS_DATA_DIR="+dir)

	// The working file is also the plugin's last argument, so a script
	// never has to parse JSON to find it.
	argc, err := os.ReadFile(filepath.Join(dir, "argc.txt"))
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(argc))
	fileArg, err := os.ReadFile(filepath.Join(dir, "filearg.txt"))
	require.NoError(t, err)
	assert.Equal(t, working+"\n", string(fileArg))
}

// TestRunStepResultFileInsideScratch: the {"file": ...} result protocol
// hands the rest of the chain a different file inside the job's scratch
// directory.
func TestRunStepResultFileInsideScratch(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	jobDir := filepath.Join(scratch, "job-j1")
	require.NoError(t, os.MkdirAll(jobDir, 0o755))
	working := filepath.Join(jobDir, "source.mkv")
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))
	stripped := filepath.Join(jobDir, "stripped.mkv")
	require.NoError(t, os.WriteFile(stripped, []byte("stripped"), 0o644))

	p := Plugin{Manifest: Manifest{
		Name:    "stripper",
		Type:    TypeStep,
		Command: []string{"sh", "-c", `echo "{\"file\": \"$1\"}"`, "stripper", stripped},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job:       JobContext{JobID: "j1"},
		Scratch:   scratch,
		FilePath:  working,
		StepIndex: 1,
	})
	require.True(t, out.Result.OK(), out.Result.Error)
	assert.Equal(t, stripped, out.File, "the named file continues down the chain")
}

// TestRunStepRejectsResultOutsideScratch (task 32): {"file": "/etc/passwd"}
// is a failure naming the path — no exceptions, no symlinks out of scratch.
func TestRunStepRejectsResultOutsideScratch(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	working := filepath.Join(scratch, "source.mkv")
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))

	p := Plugin{Manifest: Manifest{
		Name:    "escapee",
		Type:    TypeStep,
		Command: []string{"sh", "-c", `echo '{"file": "/etc/passwd"}'`},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: scratch, FilePath: working, StepIndex: 1,
	})
	require.True(t, out.Result.Ran)
	require.False(t, out.Result.OK())
	assert.Contains(t, out.Result.Error, "/etc/passwd")
	assert.Contains(t, out.Result.Error, scratch, "the message names both paths")
	assert.Empty(t, out.File, "a rejected result never continues the chain")
}

// TestRunStepRejectsSymlinkedResultOutsideScratch: a symlink that points
// out of scratch is followed and rejected (task 32: "no following symlinks
// out of scratch").
func TestRunStepRejectsSymlinkedResultOutsideScratch(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	jobDir := filepath.Join(scratch, "job-j1")
	require.NoError(t, os.MkdirAll(jobDir, 0o755))
	working := filepath.Join(jobDir, "source.mkv")
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))
	outside := filepath.Join(dir, "outside.mkv")
	require.NoError(t, os.WriteFile(outside, []byte("sneaky"), 0o644))
	link := filepath.Join(jobDir, "link.mkv")
	require.NoError(t, os.Symlink(outside, link))

	p := Plugin{Manifest: Manifest{
		Name:    "symlink-escape",
		Type:    TypeStep,
		Command: []string{"sh", "-c", `echo "{\"file\": \"$1\"}"`, "x", link},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: scratch, FilePath: working, StepIndex: 1,
	})
	require.False(t, out.Result.OK())
	assert.Contains(t, out.Result.Error, "outside the job's scratch directory")
	assert.Empty(t, out.File)
}

// TestRunStepRejectsMissingResultFile: a named file that does not exist is
// a failure with the path in the message (task 32).
func TestRunStepRejectsMissingResultFile(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	working := filepath.Join(scratch, "source.mkv")
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))

	p := Plugin{Manifest: Manifest{
		Name:    "ghost",
		Type:    TypeStep,
		Command: []string{"sh", "-c", `echo '{"file": "` + filepath.Join(scratch, "nope.mkv") + `"}'`},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: scratch, FilePath: working, StepIndex: 1,
	})
	require.False(t, out.Result.OK())
	assert.Contains(t, out.Result.Error, "nope.mkv")
	assert.Empty(t, out.File)
}

// TestRunStepResultLineAmongLogOutput: log lines before (and after) the
// final {"file": ...} object do not confuse the protocol; the last result
// line wins.
func TestRunStepResultLineAmongLogOutput(t *testing.T) {
	dir := t.TempDir()
	scratch := t.TempDir()
	jobDir := filepath.Join(scratch, "job-j1")
	require.NoError(t, os.MkdirAll(jobDir, 0o755))
	working := filepath.Join(jobDir, "source.mkv")
	require.NoError(t, os.WriteFile(working, []byte("media"), 0o644))
	next := filepath.Join(jobDir, "next.mkv")
	require.NoError(t, os.WriteFile(next, []byte("next"), 0o644))

	p := Plugin{Manifest: Manifest{
		Name:    "logger",
		Type:    TypeStep,
		Command: []string{"sh", "-c", `echo starting; echo progress 1; echo '{"file": "'"$1"'"}'`,
			"logger", next},
	}, Dir: dir}
	out := NewRunner(dir).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: scratch, FilePath: working, StepIndex: 2,
	})
	require.True(t, out.Result.OK(), out.Result.Error)
	assert.Equal(t, next, out.File)
	require.Len(t, out.Result.Output, 3, "the log lines are still captured for the job log")
}

// TestRunStepFailureCarriesExitCode (task 32): a non-zero exit is not OK;
// the caller fails the job with the name and exit code.
func TestRunStepFailureCarriesExitCode(t *testing.T) {
	p := Plugin{Manifest: Manifest{
		Name:    "angry",
		Type:    TypeStep,
		Command: []string{"sh", "-c", "echo nope; echo oops >&2; exit 4"},
	}}
	out := NewRunner(t.TempDir()).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: t.TempDir(),
		FilePath: "/nowhere/x.mkv", StepIndex: 1,
	})
	require.True(t, out.Result.Ran)
	require.False(t, out.Result.OK())
	assert.Equal(t, 4, out.Result.ExitCode)
	assert.Empty(t, out.File)
}

// TestRunStepTimeoutKillsProcessGroup (task 32): exceeding timeout_seconds
// marks the run timed out; the caller fails the job.
func TestRunStepTimeoutKillsProcessGroup(t *testing.T) {
	p := Plugin{Manifest: Manifest{
		Name:           "sleepy",
		Type:           TypeStep,
		Command:        []string{"sh", "-c", "sleep 30"},
		TimeoutSeconds: 1,
	}}
	start := time.Now()
	out := NewRunner(t.TempDir()).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: t.TempDir(),
		FilePath: "/nowhere/x.mkv", StepIndex: 1,
	})
	assert.Less(t, time.Since(start), 10*time.Second, "the group kill ends the run quickly")
	require.False(t, out.Result.OK())
	assert.True(t, out.Result.TimedOut)
	assert.Empty(t, out.File)
}

// TestRunStepMissingRequiredSettingDoesNotRun: a step whose required
// setting has no value does not run; the caller fails the step (never a
// silent continue on the critical path).
func TestRunStepMissingRequiredSettingDoesNotRun(t *testing.T) {
	p := Plugin{Manifest: Manifest{
		Name:    "needs-setting",
		Type:    TypeStep,
		Command: []string{"sh", "-c", "echo should not run"},
		Settings: []Setting{
			{Key: "level", Label: "Level", Required: true},
		},
	}}
	out := NewRunner(t.TempDir()).RunStep(context.Background(), p, StepRequest{
		Job: JobContext{JobID: "j1"}, Scratch: t.TempDir(),
		FilePath: "/nowhere/x.mkv", StepIndex: 1,
	})
	require.False(t, out.Result.Ran)
	assert.Contains(t, out.Result.SkipReason, "level")
	assert.Empty(t, out.File)
}
