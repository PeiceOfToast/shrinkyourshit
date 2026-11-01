// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// fullGraph is a graph with every shaping step plus health_check and move
// (task 24), chained probe → guards → transcode → remux → audio → subs →
// rename → skip → health → move.
func fullGraph() Graph {
	return Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "ifc", Type: StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
			{ID: "transcode", Type: StepTranscodeVideo, Params: json.RawMessage(`{"codec":"hevc","quality":3}`)},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
			{ID: "audio", Type: StepConvertAudio, Params: json.RawMessage(`{"mode":"keep_best","languages":["eng"]}`)},
			{ID: "subs", Type: StepSubtitles, Params: json.RawMessage(`{"mode":"remove"}`)},
			{ID: "rename", Type: StepRename, Params: json.RawMessage(`{"template":"{name}.shrunk"}`)},
			{ID: "skip", Type: StepSkipIfSmaller},
			{ID: "hc", Type: StepHealthCheck, Params: json.RawMessage(`{"mode":"full","minutes":7,"duration_tolerance":0.1}`)},
			{ID: "mv", Type: StepMove, Params: json.RawMessage(`{"destination":"/data/shrunk/","keep_tree":true}`)},
		},
		Edges: []Edge{
			{From: "probe", To: "ifc"}, {From: "ifc", To: "transcode"},
			{From: "transcode", To: "remux"}, {From: "remux", To: "audio"},
			{From: "audio", To: "subs"}, {From: "subs", To: "rename"},
			{From: "rename", To: "skip"}, {From: "skip", To: "hc"},
			{From: "hc", To: "mv"},
		},
	}
}

// TestCompileAndResolveCarryNewSteps: both new steps compile into the plan
// and resolve into the effective outcome (task 24).
func TestCompileAndResolveCarryNewSteps(t *testing.T) {
	g := fullGraph()
	require.Empty(t, Validate(g))
	plan, err := Compile(g)
	require.NoError(t, err)

	require.NotNil(t, plan.HealthCheck)
	require.Equal(t, HealthCheckSpec{Mode: HealthCheckFull, Minutes: 7, DurationTolerance: 0.1}, *plan.HealthCheck)
	require.Equal(t, "/data/shrunk", plan.MoveTo, "trailing slashes trimmed")
	require.True(t, plan.MoveKeepTree)

	eff, err := ResolvePlan(plan, probe.MediaInfo{}, probe.KindVideo)
	require.NoError(t, err)
	require.NotNil(t, eff.HealthCheck)
	require.Equal(t, *plan.HealthCheck, *eff.HealthCheck)
	require.Equal(t, plan.MoveTo, eff.MoveTo)
	require.Equal(t, plan.MoveKeepTree, eff.MoveKeepTree)

	// The plain-language lines cover every compiled step, new ones included.
	lines := plan.Describe()
	require.Len(t, lines, len(plan.Steps))
}

// TestHealthCheckDoesNotChangeTarget: health_check and move never touch the
// Target the encode runs with (task 24) — the resolved and collapsed
// targets are exactly those of the same graph without them.
func TestHealthCheckDoesNotChangeTarget(t *testing.T) {
	with := fullGraph()
	without := fullGraph()
	without.Nodes = without.Nodes[:len(without.Nodes)-2] // drop health_check + move
	without.Edges = without.Edges[:len(without.Edges)-2]
	require.Empty(t, Validate(without))

	pWith, err := Compile(with)
	require.NoError(t, err)
	pWithout, err := Compile(without)
	require.NoError(t, err)

	require.Equal(t, pWithout.Target(), pWith.Target())

	effWith, err := ResolvePlan(pWith, probe.MediaInfo{}, probe.KindVideo)
	require.NoError(t, err)
	effWithout, err := ResolvePlan(pWithout, probe.MediaInfo{}, probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, effWithout.Target, effWith.Target)
}

// TestResolvePlanHealthCheckDefaults: an empty params object resolves to the
// documented defaults (task 24).
func TestResolvePlanHealthCheckDefaults(t *testing.T) {
	g := graphWith(Step{ID: "hc", Type: StepHealthCheck})
	plan, err := Compile(g)
	require.NoError(t, err)
	require.Equal(t, HealthCheckSpec{
		Mode:              HealthCheckSample,
		Minutes:           HealthCheckMinutesDefault,
		DurationTolerance: HealthCheckToleranceDefault,
	}, *plan.HealthCheck)
	require.Empty(t, plan.MoveTo)
	require.False(t, plan.MoveKeepTree)

	// A graph without the new steps resolves with nothing set.
	plain, err := Compile(graphBase())
	require.NoError(t, err)
	eff, err := ResolvePlan(plain, probe.MediaInfo{}, probe.KindVideo)
	require.NoError(t, err)
	require.Nil(t, eff.HealthCheck)
	require.Empty(t, eff.MoveTo)
}

// TestStepTypesIncludeNewSteps: the palette carries both steps with their
// parameter schemas, matching the validation rules (task 24).
func TestStepTypesIncludeNewSteps(t *testing.T) {
	types := StepTypes()
	byType := map[string]StepTypeInfo{}
	for _, st := range types {
		byType[st.Type] = st
	}
	hc, ok := byType[StepHealthCheck]
	require.True(t, ok, "health_check must be in the palette")
	require.Equal(t, "Verify the result", hc.Label)
	require.False(t, hc.Condition)
	require.Len(t, hc.Params, 3)
	require.Equal(t, "mode", hc.Params[0].Name)
	require.Equal(t, HealthCheckSample, hc.Params[0].Default)
	require.Equal(t, "minutes", hc.Params[1].Name)
	require.NotNil(t, hc.Params[1].Min)
	require.Equal(t, float64(HealthCheckMinutesMin), *hc.Params[1].Min)
	require.Equal(t, "duration_tolerance", hc.Params[2].Name)
	require.Equal(t, float64(HealthCheckToleranceDefault), hc.Params[2].Default)

	mv, ok := byType[StepMove]
	require.True(t, ok, "move must be in the palette")
	require.False(t, mv.Condition)
	require.Len(t, mv.Params, 2)
	require.Equal(t, "destination", mv.Params[0].Name)
	require.Equal(t, "keep_tree", mv.Params[1].Name)
}
