// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// specRecorder is a backend that records the encoder spec it was asked to
// build and delegates the argv to the real cpu builder, so the test sees both
// the spec the worker built and the argv ffmpeg would get.
type specRecorder struct {
	encoders.Encoder
	got *encoders.Spec
}

func (s specRecorder) BuildArgs(spec encoders.Spec) []string {
	*s.got = spec
	return s.Encoder.BuildArgs(spec)
}

// rawArgsGraph is probe → transcode (hevc) → one input raw_args step, one
// output raw_args step, remux. The raw steps sit after the encode in chain
// order, which is where a user puts them.
func rawArgsGraph() *flow.Graph {
	return &flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "encode", Type: flow.StepTranscodeVideo, Params: json.RawMessage(`{"codec":"hevc","quality":3}`)},
			{ID: "in_arg", Type: flow.StepRawArgs, Params: json.RawMessage(`{"stage":"input","args":"-ss 1","note":"skip intro"}`)},
			{ID: "out_arg", Type: flow.StepRawArgs, Params: json.RawMessage(`{"stage":"output","args":"-metadata \"comment=shrink me\" -map_metadata -1"}`)},
			{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "encode"},
			{From: "encode", To: "in_arg"},
			{From: "in_arg", To: "out_arg"},
			{From: "out_arg", To: "remux"},
		},
	}
}

// TestRunnerRawArgsReachEncoderSpec: the raw_args tokens of a compiled flow
// reach the encoder spec split by stage, and the argv ffmpeg runs carries them
// at the right place.
func TestRunnerRawArgsReachEncoderSpec(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	spec.Flow = rawArgsGraph()
	api.enqueue("job1", spec)

	var got encoders.Spec
	var seen []string
	rec := specRecorder{Encoder: encoders.CPU, got: &got}
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		byBase: map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": outInfoF()},
		encs:   []encoders.Encoder{rec},
		run:    writeRun("OUTPUT-DATA", &seen),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	require.Equal(t, []string{"-ss", "1"}, got.InputArgs, "input stage tokens")
	require.Equal(t, []string{"-metadata", "comment=shrink me", "-map_metadata", "-1"}, got.OutputArgs,
		"output stage tokens, quoted word kept whole")

	require.Len(t, seen, 1, "one encode ran")
	argv := seen[0]
	require.Contains(t, argv, "-ss 1 -i ", "input stage token sits before -i")
	require.Contains(t, argv, `-metadata comment=shrink me -map_metadata -1 `, "output stage tokens sit before the output")
}

// TestRunnerRawArgsLogLinePerStep: one job log line per raw_args step, naming
// the step, its stage and the exact tokens, so task 63 can assert on it.
func TestRunnerRawArgsLogLinePerStep(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	spec.Flow = rawArgsGraph()
	api.enqueue("job1", spec)

	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		byBase: map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": outInfoF()},
		run:    writeRun("OUTPUT-DATA", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	text := logText(api, "job1")
	require.Contains(t, text, `raw_args step in_arg (input stage): "-ss" "1"`)
	require.Contains(t, text, `raw_args step out_arg (output stage): "-metadata" "comment=shrink me" "-map_metadata" "-1"`)
	require.Equal(t, 2, strings.Count(text, "raw_args step "), "exactly one line per raw_args step")
}

// TestRunnerNoRawArgsNoLogLine: a flow without raw_args adds no raw_args line
// and passes no raw tokens, so existing jobs are unchanged.
func TestRunnerNoRawArgsNoLogLine(t *testing.T) {
	api := newFakeAPI(t, "key")
	dir := t.TempDir()
	_, spec := defaultSpec(t, dir, "job1")
	spec.Flow = hardwareGraph("auto")
	api.enqueue("job1", spec)

	var got encoders.Spec
	rec := specRecorder{Encoder: encoders.CPU, got: &got}
	r := newRunner(t, api, filepath.Join(dir, "scratch"), runnerOpts{
		byBase: map[string]probe.MediaInfo{"source.mp4": srcInfoF(), "output.mkv": outInfoF()},
		encs:   []encoders.Encoder{rec},
		run:    writeRun("OUTPUT-DATA", nil),
	})
	require.NoError(t, r.run(context.Background(), spec, newCancelFlag()))

	require.Empty(t, got.InputArgs)
	require.Empty(t, got.OutputArgs)
	require.NotContains(t, logText(api, "job1"), "raw_args step")
}
