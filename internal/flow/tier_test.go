// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tierBase is the basic-representable chain every case extends: probe,
// the satisfied-rule guard, a quality-only encode to mkv with the easy audio
// and subtitle policies. Nothing in it may be outside the basic tier.
func tierBase() []Step {
	return []Step{
		{ID: "probe", Type: StepProbe},
		{ID: "if_codec_not", Type: StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
		{ID: "transcode_video", Type: StepTranscodeVideo, Params: json.RawMessage(`{"codec":"hevc","quality":3}`)},
		{ID: "remux_container", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		{ID: "convert_audio", Type: StepConvertAudio, Params: json.RawMessage(`{"mode":"keep_all","languages":["eng"],"compact_codec":"aac"}`)},
		{ID: "subtitles", Type: StepSubtitles, Params: json.RawMessage(`{"mode":"keep_all","languages":["eng"]}`)},
	}
}

// tierGraph is the base chain with extra steps appended in order.
func tierGraph(extra ...Step) Graph {
	nodes := append(tierBase(), extra...)
	var edges []Edge
	for i := 1; i < len(nodes); i++ {
		edges = append(edges, Edge{From: nodes[i-1].ID, To: nodes[i].ID})
	}
	return Graph{Schema: FlowSchema, Nodes: nodes, Edges: edges}
}

// stepFor builds one step of the given type with the given id and params.
func stepFor(id, typ, params string) Step {
	s := Step{ID: id, Type: typ}
	if params != "" {
		s.Params = json.RawMessage(params)
	}
	return s
}

// flaggedAt reports whether RepresentableAt names the step id in tier.
func flaggedAt(g Graph, tier Tier, id string) bool {
	for _, e := range RepresentableAt(g, tier) {
		if strings.Contains(e.Message, fmt.Sprintf("step %q (", id)) {
			return true
		}
	}
	return false
}

// TestRepresentableAtStepSet checks every step type of the palette in every
// tier: the step set crossed with the three tiers. A step is listed as not
// representable exactly when the tier cannot edit it.
func TestRepresentableAtStepSet(t *testing.T) {
	cases := []struct {
		name     string
		step     Step
		basic    bool
		moderate bool
	}{
		{"probe", stepFor("extra", StepProbe, ""), true, true},
		{"if_codec_not on the encode codec", stepFor("extra", StepIfCodecNot, `{"codec":"hevc"}`), true, true},
		{"if_bitrate_above", stepFor("extra", StepIfBitrateAbove, `{"mbps":8}`), false, true},
		{"if_resolution_above", stepFor("extra", StepIfResolutionAbove, `{"height":1080}`), false, true},
		{"if_audio_channels_above", stepFor("extra", StepIfAudioChannelsAbove, `{"channels":2}`), false, true},
		{"if_hdr", stepFor("extra", StepIfHDR, `{"is_hdr":true}`), false, true},
		{"transcode_video quality only", stepFor("extra", StepTranscodeVideo, `{"codec":"av1","quality":2}`), true, true},
		{"transcode_video with crf", stepFor("extra", StepTranscodeVideo, `{"codec":"hevc","quality":3,"rate_mode":"crf","crf":22}`), false, true},
		{"transcode_video with a preset", stepFor("extra", StepTranscodeVideo, `{"codec":"hevc","quality":3,"preset":"slow"}`), false, true},
		{"transcode_video with hardware qsv", stepFor("extra", StepTranscodeVideo, `{"codec":"hevc","quality":3,"hardware":"qsv"}`), false, true},
		{"scale", stepFor("extra", StepScale, `{"height":1080}`), false, true},
		{"remux_container mp4", stepFor("extra", StepRemuxContainer, `{"container":"mp4"}`), false, true},
		{"convert_audio compact", stepFor("extra", StepConvertAudio, `{"mode":"compact","compact_codec":"aac"}`), true, true},
		{"convert_audio opus", stepFor("extra", StepConvertAudio, `{"mode":"compact","compact_codec":"opus"}`), false, true},
		{"subtitles remove", stepFor("extra", StepSubtitles, `{"mode":"remove"}`), true, true},
		{"subtitles keep_preferred", stepFor("extra", StepSubtitles, `{"mode":"keep_preferred","languages":["eng"]}`), false, true},
		{"subtitles extract", stepFor("extra", StepSubtitles, `{"mode":"remove","extract_removed":true}`), false, true},
		{"rename", stepFor("extra", StepRename, `{"template":"{name}.{ext}"}`), false, true},
		{"skip_if_smaller", stepFor("extra", StepSkipIfSmaller, ""), false, true},
		{"health_check", stepFor("extra", StepHealthCheck, `{"mode":"sample","minutes":2}`), false, true},
		{"move", stepFor("extra", StepMove, `{"destination":"/media/out","keep_tree":true}`), false, true},
		{"plugin", stepFor("extra", StepPlugin, `{"plugin":"x"}`), false, false},
		{"raw_args", stepFor("extra", StepRawArgs, `{"stage":"output","args":"-threads 2"}`), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tierGraph(tc.step)
			require.Equal(t, !tc.basic, flaggedAt(g, TierBasic, "extra"), "basic")
			require.Equal(t, !tc.moderate, flaggedAt(g, TierModerate, "extra"), "moderate")
			require.False(t, flaggedAt(g, TierAdvanced, "extra"), "advanced expresses every step")
		})
	}
}

// TestRepresentableAtBaseIsBasic: the base chain is the easy shape, so the
// basic tier edits it without a single error.
func TestRepresentableAtBaseIsBasic(t *testing.T) {
	require.Empty(t, RepresentableAt(tierGraph(), TierBasic))
	require.Empty(t, RepresentableAt(tierGraph(), TierModerate))
	require.Empty(t, RepresentableAt(tierGraph(), TierAdvanced))
}

// TestRepresentableAtAdvancedIsEverything: advanced never reports anything,
// even a graph holding every escape hatch.
func TestRepresentableAtAdvancedIsEverything(t *testing.T) {
	g := tierGraph(
		stepFor("raw", StepRawArgs, `{"stage":"output","args":"-threads 2"}`),
		stepFor("plug", StepPlugin, `{"plugin":"x"}`),
	)
	require.Empty(t, RepresentableAt(g, TierAdvanced))
	require.NotEmpty(t, RepresentableAt(g, TierModerate))
}

// TestRepresentableAtNamesLowestTier: the message names the tier that can
// edit the step, so the banner can say which editor to open.
func TestRepresentableAtNamesLowestTier(t *testing.T) {
	errs := RepresentableAt(tierGraph(stepFor("cond", StepIfBitrateAbove, `{"mbps":8}`)), TierBasic)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, `step "cond"`)
	require.Contains(t, errs[0].Message, "the moderate tier can edit it")

	errs = RepresentableAt(tierGraph(stepFor("raw", StepRawArgs, `{"stage":"output","args":"-threads 2"}`)), TierModerate)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "the advanced tier can edit it")
}

// TestRepresentableAtPathsAreNodeIndexes: each error points at the node that
// owns the step, so the UI attaches it to the right row.
func TestRepresentableAtPathsAreNodeIndexes(t *testing.T) {
	g := tierGraph(stepFor("cond", StepIfHDR, `{"is_hdr":true}`), stepFor("scale1", StepScale, `{"height":720}`))
	errs := RepresentableAt(g, TierBasic)
	require.Len(t, errs, 2)
	require.Equal(t, "nodes[6].type", errs[0].Path)
	require.Equal(t, "nodes[7].type", errs[1].Path)
}

// TestRepresentableAtBasicGraphRules: the basic tier also needs the shape the
// easy screen writes: an encode, and a guard on that same codec.
func TestRepresentableAtBasicGraphRules(t *testing.T) {
	t.Run("no encode", func(t *testing.T) {
		g := tierGraph()
		g.Nodes = removeNode(g.Nodes, "transcode_video")
		errs := RepresentableAt(g, TierBasic)
		require.NotEmpty(t, errs)
		require.Equal(t, "graph", errs[0].Path)
		require.Contains(t, errs[0].Message, "transcode_video")
	})
	t.Run("no guard", func(t *testing.T) {
		g := tierGraph()
		g.Nodes = removeNode(g.Nodes, "if_codec_not")
		errs := RepresentableAt(g, TierBasic)
		require.Len(t, errs, 1)
		require.Equal(t, "graph", errs[0].Path)
		require.Contains(t, errs[0].Message, "if_codec_not")
	})
	t.Run("guard on another codec", func(t *testing.T) {
		g := tierGraph()
		g.Nodes[1] = stepFor("if_codec_not", StepIfCodecNot, `{"codec":"av1"}`)
		errs := RepresentableAt(g, TierBasic)
		require.Len(t, errs, 1)
		require.Equal(t, "graph", errs[0].Path)
	})
	t.Run("moderate does not need the guard", func(t *testing.T) {
		g := tierGraph()
		g.Nodes = removeNode(g.Nodes, "if_codec_not")
		require.Empty(t, RepresentableAt(g, TierModerate))
	})
}

// TestRepresentableAtBasicCodecsAndModes: the basic rule accepts only the
// easy codecs, containers and modes, however the step is written.
func TestRepresentableAtBasicCodecsAndModes(t *testing.T) {
	g := tierGraph()
	g.Nodes[2] = stepFor("transcode_video", StepTranscodeVideo, `{"codec":"hevc","quality":3,"rate_mode":"quality","hardware":"auto"}`)
	require.Empty(t, RepresentableAt(g, TierBasic), "quality rate and auto hardware are the easy shape")
}

// TestParseTier: the three tiers parse, anything else is an error.
func TestParseTier(t *testing.T) {
	for _, want := range TierValues() {
		got, err := ParseTier(string(want))
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	_, err := ParseTier("expert")
	require.Error(t, err)
	_, err = ParseTier("")
	require.Error(t, err)
}

// TestRepresentableAtUnknownTier: a value outside the three tiers is reported
// rather than treated as the most permissive tier.
func TestRepresentableAtUnknownTier(t *testing.T) {
	errs := RepresentableAt(tierGraph(), Tier("expert"))
	require.Len(t, errs, 1)
	require.Equal(t, "tier", errs[0].Path)
}

func removeNode(nodes []Step, id string) []Step {
	out := nodes[:0:0]
	for _, n := range nodes {
		if n.ID != id {
			out = append(out, n)
		}
	}
	return out
}
