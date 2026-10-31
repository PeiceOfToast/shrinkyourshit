// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
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
