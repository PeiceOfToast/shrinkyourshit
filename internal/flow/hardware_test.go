// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// TestEffectiveCarriesHardwareChoice: the transcode step's hardware value and
// step ID reach the Effective the runner resolves (task 57). A graph without a
// transcode step carries no choice at all.
func TestEffectiveCarriesHardwareChoice(t *testing.T) {
	info := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 2160}}}

	for _, h := range []string{HardwareQSV, HardwareSoftware, HardwareAuto} {
		plan, err := Compile(tuningGraph(`{"height":1080}`, `{"codec":"hevc","quality":3,"hardware":"`+h+`"}`))
		require.NoError(t, err)
		eff, err := ResolvePlan(plan, info, probe.KindVideo)
		require.NoError(t, err)
		require.Equal(t, h, eff.Hardware)
		require.Equal(t, "tv", eff.HardwareStep)
	}

	// No hardware param at all: the choice is empty, which the runner reads
	// as the global policy (auto).
	plan, err := Compile(tuningGraph(`{"height":1080}`, `{"codec":"hevc","quality":3}`))
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, info, probe.KindVideo)
	require.NoError(t, err)
	require.Empty(t, eff.Hardware)
	require.Equal(t, "tv", eff.HardwareStep)
}

// TestEffectiveNoTranscodeNoHardware: a plan with no transcode step (a remux
// only) resolves with no hardware choice and no step.
func TestEffectiveNoTranscodeNoHardware(t *testing.T) {
	g := Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "remux", Type: StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
		},
		Edges: []Edge{{From: "probe", To: "remux"}},
	}
	plan, err := Compile(g)
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, probe.MediaInfo{}, probe.KindVideo)
	require.NoError(t, err)
	require.Empty(t, eff.Hardware)
	require.Empty(t, eff.HardwareStep)
}
