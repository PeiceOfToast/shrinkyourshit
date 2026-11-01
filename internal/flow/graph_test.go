// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// graphBase is a minimal valid graph the step tests extend.
func graphBase() Graph {
	return Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{{From: "probe", To: "remux"}},
	}
}

// graphWith splices a step into the middle of graphBase (probe → step → remux).
func graphWith(s Step) Graph {
	g := graphBase()
	g.Nodes = append(g.Nodes[:1], append([]Step{s}, g.Nodes[1:]...)...)
	g.Edges = []Edge{{From: "probe", To: s.ID}, {From: s.ID, To: "remux"}}
	return g
}

// TestValidateHealthCheckDefaults: every zero field is the documented
// default, on the graph and on the compiled plan (task 24).
func TestValidateHealthCheckDefaults(t *testing.T) {
	g := graphWith(Step{ID: "hc", Type: StepHealthCheck})
	require.Empty(t, Validate(g), "empty params must be the defaults")

	plan, err := Compile(g)
	require.NoError(t, err)
	require.NotNil(t, plan.HealthCheck)
	require.Equal(t, HealthCheckSpec{
		Mode:              HealthCheckSample,
		Minutes:           HealthCheckMinutesDefault,
		DurationTolerance: HealthCheckToleranceDefault,
	}, *plan.HealthCheck)

	eff, err := ResolvePlan(plan, probe.MediaInfo{})
	require.NoError(t, err)
	require.NotNil(t, eff.HealthCheck)
	require.Equal(t, *plan.HealthCheck, *eff.HealthCheck)
}

// TestValidateHealthCheckRanges: out-of-range values are FieldErrors on the
// right path (task 24).
func TestValidateHealthCheckRanges(t *testing.T) {
	cases := []struct {
		name   string
		params string
		path   string
	}{
		{"bad mode", `{"mode":"paranoid"}`, "nodes[1].params.mode"},
		{"minutes below", `{"minutes":0.5}`, ""}, // decode error, not a field error
		{"minutes too few", `{"minutes":-1}`, "nodes[1].params.minutes"},
		{"minutes too many", `{"minutes":121}`, "nodes[1].params.minutes"},
		{"tolerance too wide", `{"duration_tolerance":0.6}`, "nodes[1].params.duration_tolerance"},
		{"tolerance negative", `{"duration_tolerance":-0.01}`, "nodes[1].params.duration_tolerance"},
		{"unknown field", `{"wat":1}`, "nodes[1].params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graphWith(Step{ID: "hc", Type: StepHealthCheck, Params: json.RawMessage(tc.params)})
			errs := Validate(g)
			require.NotEmpty(t, errs)
			if tc.path != "" {
				found := false
				for _, e := range errs {
					if e.Path == tc.path {
						found = true
					}
				}
				require.True(t, found, "want a field error on %s, got %v", tc.path, errs)
			}
		})
	}

	// In-range values validate and compile verbatim.
	g := graphWith(Step{ID: "hc", Type: StepHealthCheck,
		Params: json.RawMessage(`{"mode":"full","minutes":90,"duration_tolerance":0.25}`)})
	require.Empty(t, Validate(g))
	plan, err := Compile(g)
	require.NoError(t, err)
	require.Equal(t, HealthCheckSpec{Mode: HealthCheckFull, Minutes: 90, DurationTolerance: 0.25}, *plan.HealthCheck)
}

// TestValidateHealthCheckSingle: a second health_check step is rejected
// (task 24: it always runs last, two of them can only conflict).
func TestValidateHealthCheckSingle(t *testing.T) {
	g := graphWith(Step{ID: "hc", Type: StepHealthCheck})
	g.Nodes = append(g.Nodes, Step{ID: "hc2", Type: StepHealthCheck})
	g.Edges = append(g.Edges, Edge{From: "remux", To: "hc2"})
	errs := Validate(g)
	require.NotEmpty(t, errs)
	require.Contains(t, errsText(errs), "only one health_check step is allowed")
}

// TestValidateMoveDestination: the destination is required and must be
// absolute; trailing slashes are trimmed for the compiled plan (task 24).
func TestValidateMoveDestination(t *testing.T) {
	g := graphWith(Step{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":""}`)})
	errs := Validate(g)
	require.NotEmpty(t, errs)
	require.Contains(t, errsText(errs), "nodes[1].params.destination")

	g = graphWith(Step{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":"shrunk"}`)})
	errs = Validate(g)
	require.NotEmpty(t, errs)
	require.Contains(t, errsText(errs), "absolute path")

	g = graphWith(Step{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":"/data/shrunk/"}`)})
	require.Empty(t, Validate(g), "trailing slashes are trimmed, not rejected")
	plan, err := Compile(g)
	require.NoError(t, err)
	require.Equal(t, "/data/shrunk", plan.MoveTo)
	require.False(t, plan.MoveKeepTree)

	plan, err = Compile(graphWith(Step{ID: "mv", Type: StepMove,
		Params: json.RawMessage(`{"destination":"/data/shrunk","keep_tree":true}`)}))
	require.NoError(t, err)
	require.Equal(t, "/data/shrunk", plan.MoveTo)
	require.True(t, plan.MoveKeepTree)
}

// TestValidateMoveSingle: a second move step is rejected (task 24).
func TestValidateMoveSingle(t *testing.T) {
	g := graphWith(Step{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":"/a"}`)})
	g.Nodes = append(g.Nodes, Step{ID: "mv2", Type: StepMove, Params: json.RawMessage(`{"destination":"/b"}`)})
	g.Edges = append(g.Edges, Edge{From: "remux", To: "mv2"})
	errs := Validate(g)
	require.NotEmpty(t, errs)
	require.Contains(t, errsText(errs), "only one move step is allowed")
}

// TestValidateMoveNoParams: params on a step that takes none are rejected,
// and the new steps take parameters (a null params object is fine).
func TestValidateMoveNoParams(t *testing.T) {
	g := graphWith(Step{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":"/a","wat":1}`)})
	require.Contains(t, errsText(Validate(g)), "unknown field")

	hc := graphWith(Step{ID: "hc", Type: StepHealthCheck, Params: json.RawMessage(`null`)})
	require.Empty(t, Validate(hc))
}

// errsText joins field errors for substring assertions.
func errsText(errs []FieldError) string {
	out := ""
	for _, e := range errs {
		out += fmt.Sprintf("%s: %s; ", e.Path, e.Message)
	}
	return out
}

/* ------------------------------------------------------------------ */
/* The plugin step (task 32)                                           */
/* ------------------------------------------------------------------ */

// pluginGraph is graphBase with a plugin step between probe and remux.
func pluginGraph() Graph {
	return graphWith(Step{ID: "plug", Type: StepPlugin,
		Params: json.RawMessage(`{"plugin":"remove-chapters"}`)})
}

// TestValidatePluginStep (task 32): a valid plugin step validates; several
// plugin steps may chain; multiple plugin steps are not "shaping" (no
// one-per-graph rule).
func TestValidatePluginStep(t *testing.T) {
	require.Empty(t, Validate(pluginGraph()))

	// Two plugin steps in a row are fine.
	g := pluginGraph()
	g.Nodes = append(g.Nodes, Step{ID: "plug2", Type: StepPlugin,
		Params: json.RawMessage(`{"plugin":"sidecar","continue_on_error":true}`)})
	g.Edges = []Edge{
		{From: "probe", To: "plug"}, {From: "plug", To: "plug2"}, {From: "plug2", To: "remux"},
	}
	require.Empty(t, Validate(g))

	// Per-step settings and continue_on_error are accepted.
	require.Empty(t, Validate(graphWith(Step{ID: "plug", Type: StepPlugin,
		Params: json.RawMessage(`{"plugin":"sidecar","settings":{"LOG_LEVEL":"debug"},"continue_on_error":true}`)})))
}

// TestValidatePluginParamErrors (task 32): a missing name, a bad name and
// an unknown-field or bad-key settings map each produce a FieldError on
// the parameter path.
func TestValidatePluginParamErrors(t *testing.T) {
	cases := []struct {
		name   string
		params string
		path   string
	}{
		{"missing name", `{}`, "nodes[1].params.plugin"},
		{"empty name", `{"plugin":""}`, "nodes[1].params.plugin"},
		{"bad name shape", `{"plugin":"Remove Chapters"}`, "nodes[1].params.plugin"},
		{"bad settings key", `{"plugin":"sidecar","settings":{"not a key":"x"}}`, "nodes[1].params.settings.not a key"},
		{"empty settings key", `{"plugin":"sidecar","settings":{"":"x"}}`, "nodes[1].params.settings"},
		{"unknown field", `{"plugin":"sidecar","wat":1}`, "nodes[1].params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graphWith(Step{ID: "plug", Type: StepPlugin, Params: json.RawMessage(tc.params)})
			errs := Validate(g)
			require.NotEmpty(t, errs)
			require.Contains(t, errsText(errs), tc.path)
		})
	}

	// Settings values may be empty strings.
	require.Empty(t, Validate(graphWith(Step{ID: "plug", Type: StepPlugin,
		Params: json.RawMessage(`{"plugin":"sidecar","settings":{"LOG_LEVEL":""}}`)})))
}

// TestValidatePluginAheadOfProbe (task 32): a plugin step may not run
// before the probe step hands it the probe output. The entry must be the
// probe; a plugin entry produces the position error on the step's type.
func TestValidatePluginAheadOfProbe(t *testing.T) {
	g := Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "plug", Type: StepPlugin, Params: json.RawMessage(`{"plugin":"remove-chapters"}`)},
			{ID: "probe", Type: StepProbe},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{{From: "plug", To: "probe"}, {From: "probe", To: "remux"}},
	}
	errs := Validate(g)
	require.NotEmpty(t, errs)
	require.Contains(t, errsText(errs), "nodes[0].type")
	require.Contains(t, errsText(errs), "probe step before it")

	// And a graph whose only steps are probe → plugin (probe first) is
	// valid apart from the missing container step; the plugin position is
	// not the problem.
	g2 := Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "plug", Type: StepPlugin, Params: json.RawMessage(`{"plugin":"remove-chapters"}`)},
		},
		Edges: []Edge{{From: "probe", To: "plug"}},
	}
	require.Contains(t, errsText(Validate(g2)), "the flow needs exactly one remux_container step")
}

// TestCompileCarriesPluginAsChainStep (task 32): a plugin step is a
// PlanStep in order (like remux_container), not a side spec.
func TestCompileCarriesPluginAsChainStep(t *testing.T) {
	plan, err := Compile(pluginGraph())
	require.NoError(t, err)
	require.Len(t, plan.Steps, 3)
	require.Equal(t, StepProbe, plan.Steps[0].Type)
	require.Equal(t, StepPlugin, plan.Steps[1].Type)
	require.Equal(t, StepRemuxContainer, plan.Steps[2].Type)

	// A guard before the plugin attaches to it like to any step.
	g := Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "guard", Type: StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
			{ID: "plug", Type: StepPlugin, Params: json.RawMessage(`{"plugin":"remove-chapters"}`)},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{
			{From: "probe", To: "guard"}, {From: "guard", To: "plug"}, {From: "plug", To: "remux"},
		},
	}
	plan, err = Compile(g)
	require.NoError(t, err)
	require.Equal(t, StepPlugin, plan.Steps[1].Type)
	require.Len(t, plan.Steps[1].When, 1, "the guard is carried as a condition")
	require.Equal(t, StepRemuxContainer, plan.Steps[2].Type)
}

// TestResolvePlanIgnoresPluginStep (task 32): a plugin step changes
// nothing in the resolved Target — the worker runs it and re-probes.
func TestResolvePlanIgnoresPluginStep(t *testing.T) {
	plan, err := Compile(pluginGraph())
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264}}})
	require.NoError(t, err)
	assert.Equal(t, CodecCopy, eff.Target.Video.Codec, "the plugin step never shapes the Target")
	assert.False(t, eff.SkipIfSmaller)
	assert.Empty(t, eff.Rename)

	// The step-walk equivalent: ApplyStep on each step with the same
	// probe resolves to the same Effective (the worker's walk parity).
	walk := FreshEffective()
	for _, s := range plan.Steps {
		ran, err := ApplyStep(&walk, s, probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264}}})
		require.NoError(t, err)
		require.True(t, ran)
	}
	require.Equal(t, eff, walk)
}

// TestApplyStepSkipsOnGuard (task 32): a guarded step is not applied when
// the condition fails, and ApplyStep reports it.
func TestApplyStepSkipsOnGuard(t *testing.T) {
	plan, err := Compile(Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "guard", Type: StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
			{ID: "transcode", Type: StepTranscodeVideo, Params: json.RawMessage(`{"codec":"h264","quality":3}`)},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{
			{From: "probe", To: "guard"}, {From: "guard", To: "transcode"}, {From: "transcode", To: "remux"},
		},
	})
	require.NoError(t, err)
	// hevc video: the codec_not hevc guard fails → the transcode is
	// skipped, the video stays copy.
	eff, err := ResolvePlan(plan, probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecHEVC}}})
	require.NoError(t, err)
	assert.Equal(t, CodecCopy, eff.Target.Video.Codec)

	// Same result through the walk.
	walk := FreshEffective()
	for _, s := range plan.Steps {
		ran, err := ApplyStep(&walk, s, probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecHEVC}}})
		require.NoError(t, err)
		if s.Type == StepTranscodeVideo {
			assert.False(t, ran, "the guarded transcode does not run")
		}
	}
	assert.Equal(t, eff, walk)
}

// TestDescribePluginStep (task 32): the plain-language line names the
// plugin, in the same voice as its neighbours.
func TestDescribePluginStep(t *testing.T) {
	plan, err := Compile(pluginGraph())
	require.NoError(t, err)
	lines := plan.Describe()
	require.Contains(t, lines[1], "Run plugin remove-chapters")

	// continue_on_error says so.
	g := graphWith(Step{ID: "plug", Type: StepPlugin,
		Params: json.RawMessage(`{"plugin":"sidecar","continue_on_error":true}`)})
	plan, err = Compile(g)
	require.NoError(t, err)
	require.Contains(t, plan.Describe()[1], "carries on anyway")
}

// TestStepTypesIncludePlugin (task 32): the palette carries the plugin
// entry with a kind-"plugin" select and the continue-on-error toggle. The
// option list is filled by the API from the installed plugins.
func TestStepTypesIncludePlugin(t *testing.T) {
	types := StepTypes()
	byType := map[string]StepTypeInfo{}
	for _, st := range types {
		byType[st.Type] = st
	}
	pl, ok := byType[StepPlugin]
	require.True(t, ok, "plugin must be in the palette")
	require.Equal(t, "Run a plugin", pl.Label)
	require.False(t, pl.Condition)
	require.Len(t, pl.Params, 2)
	require.Equal(t, "plugin", pl.Params[0].Name)
	require.Equal(t, "plugin", pl.Params[0].Kind, "the editor renders a select of installed step plugins")
	require.True(t, pl.Params[0].Required)
	require.Equal(t, "continue_on_error", pl.Params[1].Name)
	require.Equal(t, "toggle", pl.Params[1].Kind)
}
