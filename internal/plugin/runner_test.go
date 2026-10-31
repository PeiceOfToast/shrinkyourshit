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
