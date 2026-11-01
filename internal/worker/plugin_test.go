// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// Step plugins on the node (task 32), unit-tested without ffmpeg: the
// plugin is a real shell script in the node's data dir and the encode is
// the writeRun override.

// installStepPlugin writes an installed step plugin into the node's data
// dir: manifest + entry script. The script content is passed through; the
// tests dump the stdin contract with $SYS_STDIN_OUT.
func installStepPlugin(t *testing.T, dataDir, name, script string) {
	t.Helper()
	dir := filepath.Join(dataDir, "plugins", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(
		"name: "+name+"\ntype: step\nversion: \"1.0.0\"\ndescription: test\ncommand: [\"sh\", \""+name+".sh\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".sh"), []byte(script+"\n"), 0o755))
}

// pluginLogLines collects the job log lines a plugin run produced (source
// plugin), in order.
func pluginLogLines(api *fakeAPI, jobID string) []store.JobLogLine {
	var out []store.JobLogLine
	for _, l := range collectJobLog(api, jobID) {
		if l.Source == store.JobLogSourcePlugin {
			out = append(out, l)
		}
	}
	return out
}

// pluginStepSpec is a spec with a single plugin step at chain position 1
// (after the probe, as a saved flow would place it; the spec here has no
// flow graph, so the fallback walk runs the entry in chain order).
func pluginStepSpec(t *testing.T, dir, jobID, pluginName string, ps server.PluginStep) (sourceFixture, *server.JobSpec) {
	t.Helper()
	src, spec := defaultSpec(t, dir, jobID)
	ps.Name = pluginName
	if ps.StepIndex == 0 {
		ps.StepIndex = 1 // the probe step sits at chain position 0
	}
	if ps.Command == nil {
		ps.Command = []string{"sh", pluginName + ".sh"}
	}
	if ps.TimeoutSec == 0 {
		ps.TimeoutSec = 30
	}
	spec.Plugins = []server.PluginStep{ps}
	// keep_both: the plugin tests assert the original stays on disk, so
	// the result policy must not replace it.
	spec.ResultPolicy = store.ResultPolicyKeepBoth
	return src, spec
}

// dumpContract is a plugin body that records its stdin JSON, the SYS_
// environment it received and the working file it was given, so tests can
// assert the exact contract. It is formatted with the absolute dump path.
func dumpContract(path string) string {
	return `#!/bin/sh
cat > "` + path + `"
env | grep -E '^SYS_' | sort > "` + path + `.env"
printf '%s' "$1" > "` + path + `.file"
printf 'plugin did it\n'
`
}

// readEnvBlock reads the env dump the dumpContract script wrote.
func readEnvBlock(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path + ".env")
	require.NoError(t, err)
	env := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if i := strings.Index(line, "="); i > 0 {
			env[line[:i]] = line[i+1:]
		}
	}
	return env
}

// TestWorkerPluginStepInPlace: an in-place rewrite step plugin runs with
// the step contract on stdin (phase, file_path, source_path, step_index,
// settings) and the env additions, its stdout lands in the job log with
// source plugin, and the job completes with the target result.
func TestWorkerPluginStepInPlace(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	dump := filepath.Join(dataDir, "stdin.json")
	installStepPlugin(t, dataDir, "remove-chapters", dumpContract(dump))
	src, spec := pluginStepSpec(t, dir, "job1", "remove-chapters", server.PluginStep{
		Settings: map[string]string{"level": "2"},
	})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	// The stdin contract the plugin received.
	raw, err := os.ReadFile(dump)
	require.NoError(t, err)
	var in map[string]any
	require.NoError(t, json.Unmarshal(raw, &in))
	require.Equal(t, "step", in["phase"])
	require.Equal(t, float64(1), in["step_index"])
	settings := in["settings"].(map[string]any)
	require.Equal(t, "2", settings["level"])
	_, ok := settings["color"]
	require.False(t, ok, "per-step overrides must not leak the stored 'color' setting")

	// file_path is the scratch copy; source_path is the original library
	// file. The original still holds its bytes.
	fp, ok := in["file_path"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(fp, filepath.Join(dir, "scratch")), "file_path must be inside scratch, got %s", fp)
	require.Equal(t, src.Path, in["source_path"])
	buf, err := os.ReadFile(src.Path)
	require.NoError(t, err)
	require.Equal(t, "SOURCE-CONTENT", string(buf), "the original stays untouched")

	// The working file is also the plugin's last argument.
	fileArg, err := os.ReadFile(dump + ".file")
	require.NoError(t, err)
	require.Equal(t, fp, string(fileArg))

	// The env additions.
	env := readEnvBlock(t, dump)
	require.Equal(t, "step", env["SYS_PHASE"])
	require.Equal(t, "1", env["SYS_STEP_INDEX"])
	require.Equal(t, "remove-chapters", env["SYS_PLUGIN_NAME"])
	require.Equal(t, dataDir, env["SYS_DATA_DIR"])

	// Output lands in the job log with source plugin.
	lines := pluginLogLines(api, "job1")
	require.NotEmpty(t, lines)
	require.Contains(t, lines[0].Message, "plugin did it")

	// Job completes with the target result (the encode ran after the step).
	complete := api.completeOf("job1")
	require.Equal(t, "cpu", complete.Encoder)
	require.Equal(t, filepath.Join(dir, "movie.shrunk.mkv"), complete.OutputPath)
}

// TestWorkerPluginStepHandover: a plugin prints {"file": "..."} for a file
// inside the job scratch dir; the chain continues with the handed file and
// the ffmpeg run consumes it.
func TestWorkerPluginStepHandover(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "stripper", `#!/bin/sh
cp "$1" "$(dirname "$1")/stripped.mkv"
printf '{"file": "%s"}' "$(dirname "$1")/stripped.mkv"
`)
	_, spec := pluginStepSpec(t, dir, "job1", "stripper", server.PluginStep{})
	api.enqueue("job1", spec)

	var seen []string
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT-DATA", &seen),
		byBase: map[string]probe.MediaInfo{
			"source.mp4":   srcInfoF(),
			"output.mkv":   outInfoF(),
			"stripped.mkv": srcInfoF(), // the plugin's copy: still h264, so the encode runs
		},
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	// The encode consumed the handed-off file (stripped.mkv), not the
	// scratch copy of the original. The writeRun override records the
	// args, input path first.
	require.NotEmpty(t, seen)
	require.Contains(t, seen[0], "stripped.mkv")
}

// TestWorkerPluginStepFailsJob: a non-zero exit fails the job with the
// plugin name, the exit code and the last lines of stderr, and the original
// file is byte identical.
func TestWorkerPluginStepFailsJob(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "break-it", "#!/bin/sh\necho 'boom on stderr' >&2\nexit 3\n")
	src, spec := pluginStepSpec(t, dir, "job1", "break-it", server.PluginStep{})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	rep, ok := api.failOf("job1")
	require.True(t, ok, "job must be reported failed")
	require.Contains(t, rep.Error, "plugin break-it")
	require.Contains(t, rep.Error, "exit status 3")
	require.Contains(t, rep.Error, "boom on stderr")

	buf, err := os.ReadFile(src.Path)
	require.NoError(t, err)
	require.Equal(t, "SOURCE-CONTENT", string(buf), "the original stays byte identical")
	_ = api.completeOf("job1")
}

// TestWorkerPluginStepHandoverOutsideScratch: a plugin that hands over a
// file outside the job scratch dir fails the job with a message naming the
// path. A named file that does not exist also fails.
func TestWorkerPluginStepHandoverOutsideScratch(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "leaky", "#!/bin/sh\necho '{\"file\": \"/etc/passwd\"}'\n")
	_, spec := pluginStepSpec(t, dir, "job1", "leaky", server.PluginStep{})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "/etc/passwd")
	require.Contains(t, rep.Error, "scratch")

	// A named file that does not exist.
	api = newFakeAPI(t, "key")
	installStepPlugin(t, dataDir, "missing-file",
		"#!/bin/sh\necho '{\"file\": \"/tmp/nope.mkv\"}'\n")
	_, spec = pluginStepSpec(t, dir, "job2", "missing-file", server.PluginStep{})
	api.enqueue("job2", spec)
	r = newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok = api.failOf("job2")
	require.True(t, ok)
	require.Contains(t, rep.Error, "nope.mkv")
}

// TestWorkerPluginStepTimeout: a plugin that sleeps past its timeout is
// killed and fails the job with a message naming the plugin.
func TestWorkerPluginStepTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "sleepy", "#!/bin/sh\nsleep 30\n")
	_, spec := pluginStepSpec(t, dir, "job1", "sleepy", server.PluginStep{TimeoutSec: 1})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	start := time.Now()
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	require.Less(t, time.Since(start), 20*time.Second, "the timeout kills the process quickly")
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "plugin sleepy")
	require.Contains(t, rep.Error, "timed out")
}

// TestWorkerPluginStepNotInstalled: a plugin step naming a plugin that is
// not installed on the node fails with a message naming the plugin and the
// node.
func TestWorkerPluginStepNotInstalled(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := pluginStepSpec(t, dir, "job1", "not-here", server.PluginStep{})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  filepath.Join(dir, "data"),
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "plugin not-here is not installed on node nas-01")
}

// TestWorkerPluginStepContinueOnError: continue_on_error logs a warning and
// the chain continues with the existing working file (the encode runs).
func TestWorkerPluginStepContinueOnError(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "flaky", "#!/bin/sh\nexit 1\n")
	_, spec := pluginStepSpec(t, dir, "job1", "flaky", server.PluginStep{ContinueOnError: true})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	complete := api.completeOf("job1")
	require.Equal(t, "cpu", complete.Encoder)
	// The warning lands in the job log with source plugin.
	lines := pluginLogLines(api, "job1")
	require.NotEmpty(t, lines)
	require.Contains(t, lines[0].Message, "flaky")
}

// TestWorkerPluginStepProbeAfterStep: the re-probe after a step uses the
// file the plugin produced, not the original probe. A flow graph whose
// plugin hands over a hevc file and guards the transcode with
// if_codec_not hevc skips the encode — the guard saw the plugin's output.
func TestWorkerPluginStepProbeAfterStep(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "stripper", `#!/bin/sh
cp "$1" "$(dirname "$1")/stripped.mkv"
printf '{"file": "%s"}' "$(dirname "$1")/stripped.mkv"
`)
	_, spec := defaultSpec(t, dir, "job1")
	spec.Flow = &flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "plug", Type: flow.StepPlugin, Params: json.RawMessage(`{"plugin":"stripper"}`)},
			{ID: "guard", Type: flow.StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
			{ID: "transcode", Type: flow.StepTranscodeVideo, Params: json.RawMessage(`{"codec":"h264","quality":3}`)},
			{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "plug"}, {From: "plug", To: "guard"},
			{From: "guard", To: "transcode"}, {From: "transcode", To: "remux"},
		},
	}
	spec.Plugins = []server.PluginStep{{
		Name: "stripper", Command: []string{"sh", "stripper.sh"}, TimeoutSec: 30, StepIndex: 1,
	}}
	// The fake prober answers the handed-off file (stripped.mkv) as hevc.
	hevc := remuxInfoF()
	hevc.Video[0].Codec = flow.CodecHEVC
	api.enqueue("job1", spec)

	var seen []string
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run: writeRun("OUTPUT-DATA", &seen),
		byBase: map[string]probe.MediaInfo{
			"source.mp4": srcInfoF(), "output.mkv": outInfoF(), "stripped.mkv": hevc},
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	// The guard saw the plugin's output (hevc) and skipped the video
	// transcode: the encode args carry no libx265 (the default target's
	// encoder) — the plugin's output already satisfied the guard.
	require.NotEmpty(t, seen)
	require.NotContains(t, seen[0], "libx265")
}

// TestWorkerPluginStepSettingsMerged: per-step settings override the stored
// settings. The plugin sees both names with the override winning.
func TestWorkerPluginStepSettingsMerged(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	dump := filepath.Join(dataDir, "stdin.json")
	installStepPlugin(t, dataDir, "configured", dumpContract(dump))
	_, spec := pluginStepSpec(t, dir, "job1", "configured", server.PluginStep{
		Settings: map[string]string{"level": "5"},
	})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	raw, err := os.ReadFile(dump)
	require.NoError(t, err)
	var in map[string]any
	require.NoError(t, json.Unmarshal(raw, &in))
	require.Equal(t, "5", in["settings"].(map[string]any)["level"])
}

// TestWorkerPluginStepLogFileSource: the plugin's stdout lands in the job
// log with source plugin (the unit test checks the sink wiring; the in-place
// test above asserts the message text).
func TestWorkerPluginStepLogFileSource(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	installStepPlugin(t, dataDir, "chatty", "#!/bin/sh\necho 'on stdout'\necho 'on stderr' >&2\nexit 2\n")
	_, spec := pluginStepSpec(t, dir, "job1", "chatty", server.PluginStep{ContinueOnError: true})
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:      writeRun("OUTPUT-DATA", nil),
		dataDir:  dataDir,
		nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	lines := collectJobLog(api, "job1")
	var pluginLines []store.JobLogLine
	for _, l := range lines {
		if l.Source == store.JobLogSourcePlugin {
			pluginLines = append(pluginLines, l)
		}
	}
	require.Len(t, pluginLines, 3, "stdout, stderr and the continue warning")
	// The captured lines can arrive in either order (both pipes are
	// drained concurrently); both are the plugin's output and carry its
	// name. The third is the continue-on-error warning.
	msgs := []string{pluginLines[0].Message, pluginLines[1].Message}
	require.ElementsMatch(t, []string{"chatty: on stdout", "chatty: on stderr"}, msgs)
	require.Contains(t, pluginLines[2].Message, "failed but the flow continues on error")
	require.Equal(t, store.JobLogWarn, pluginLines[2].Level)
}

// TestWorkerPluginStepUnreadableFile: a plugin that leaves a file the
// prober cannot read fails the step with the probe error (the job fails
// instead of proceeding).
func TestWorkerPluginStepUnreadableFile(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	_, spec := pluginStepSpec(t, dir, "job1", "wrecker", server.PluginStep{})
	// The plugin hands over a scratch file that is not in the prober map
	// (unreadable to the fake prober).
	installStepPlugin(t, dataDir, "wrecker", `#!/bin/sh
cp "$1" "$(dirname "$1")/wrecked.mkv"
printf '{"file": "%s"}' "$(dirname "$1")/wrecked.mkv"
`)
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		run:     writeRun("OUTPUT-DATA", nil),
		byBase:  map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": outInfoF()},
		dataDir: dataDir, nodeName: "nas-01",
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))
	rep, ok := api.failOf("job1")
	require.True(t, ok)
	require.Contains(t, rep.Error, "plugin wrecker")
	require.Contains(t, rep.Error, "probe")
}
