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

/* ------------------------------------------------------------------ */
/* Scale, rate and preset through the plan (task 54)                   */
/* ------------------------------------------------------------------ */

// tuningGraph is probe → if_codec_not hevc → scale → transcode → remux. The
// guard attaches to the scale (the next action), so the transcode line of
// Describe carries no guard prefix.
func tuningGraph(scaleParams, transcodeParams string) Graph {
	return Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "ifc", Type: StepIfCodecNot, Params: json.RawMessage(`{"codec":"hevc"}`)},
			{ID: "sc", Type: StepScale, Params: json.RawMessage(scaleParams)},
			{ID: "tv", Type: StepTranscodeVideo, Params: json.RawMessage(transcodeParams)},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{
			{From: "probe", To: "ifc"}, {From: "ifc", To: "sc"}, {From: "sc", To: "tv"}, {From: "tv", To: "remux"},
		},
	}
}

// TestResolvePlanCarriesScaleRateAndPreset: a scale placed before the encode
// survives the transcode (which owns codec, quality, rate and preset), and
// the resolved Target carries all of it.
func TestResolvePlanCarriesScaleRateAndPreset(t *testing.T) {
	g := tuningGraph(`{"height":1080}`, `{"codec":"hevc","quality":3,"rate_mode":"crf","crf":22,"max_kbps":9000,"preset":"slow"}`)
	plan, err := Compile(g)
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, infoH264TwoAudioOneSubtitle(), probe.KindVideo)
	require.NoError(t, err)
	v := eff.Target.Video
	require.Equal(t, CodecHEVC, v.Codec)
	require.Equal(t, &ScaleTarget{Height: 1080, OnlyIfLarger: true, AllowUpscale: false}, v.Scale)
	require.Equal(t, &RateTarget{Mode: RateModeCRF, CRF: 22, MaxKBPS: 9000}, v.Rate)
	require.Equal(t, "slow", v.Preset)
	require.Equal(t, plan.Target(), eff.Target, "Plan.Target reports the same thing as a resolved file")
}

// TestScaleDefaultsWhenOmitted: only_if_larger defaults to true and
// allow_upscale to false, both on the scale step and in the resolved Target.
func TestScaleDefaultsWhenOmitted(t *testing.T) {
	plan, err := Compile(tuningGraph(`{"height":720}`, `{"codec":"hevc","quality":3}`))
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, infoH264TwoAudioOneSubtitle(), probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, &ScaleTarget{Height: 720, OnlyIfLarger: true, AllowUpscale: false}, eff.Target.Video.Scale)

	plan, err = Compile(tuningGraph(`{"height":720,"only_if_larger":false,"allow_upscale":true}`, `{"codec":"hevc","quality":3}`))
	require.NoError(t, err)
	eff, err = ResolvePlan(plan, infoH264TwoAudioOneSubtitle(), probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, &ScaleTarget{Height: 720, OnlyIfLarger: false, AllowUpscale: true}, eff.Target.Video.Scale)
}

// TestScaleSkippedWhenEncodeSkipped: the guard that skips the encode also
// skips a scale placed behind the guard. A file already in the target codec
// keeps no scale.
func TestScaleSkippedWhenEncodeSkipped(t *testing.T) {
	g, err := TargetToFlow(withTuning(Default()))
	require.NoError(t, err)
	plan, err := Compile(g)
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, infoHEVC(), probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, CodecCopy, eff.Target.Video.Codec, "already HEVC: the encode is skipped")
	require.Nil(t, eff.Target.Video.Scale, "a skipped encode is never scaled")
	require.Empty(t, eff.Target.Video.Preset)
	require.Nil(t, eff.Target.Video.Rate)
}

// withTuning is Default() with scale, rate and preset set.
func withTuning(t Target) Target {
	t.Video.Scale = &ScaleTarget{Height: 1080, OnlyIfLarger: true}
	t.Video.Rate = &RateTarget{Mode: RateModeBitrate, BitrateKBPS: 4000, MaxKBPS: 6000}
	t.Video.Preset = "fast"
	return t
}

// TestTargetToFlowRoundTripsTuning: a Target with scale, rate and preset
// compiles to a graph and back to the same Target, guarded and unguarded.
func TestTargetToFlowRoundTripsTuning(t *testing.T) {
	for _, guarded := range []bool{true, false} {
		target := withTuning(Default())
		target.Video.ReencodeIfSameCodec = !guarded
		g, err := TargetToFlow(target)
		require.NoError(t, err)
		plan, err := Compile(g)
		require.NoError(t, err)
		require.Equal(t, target, plan.Target(), "guarded=%v", guarded)
	}
}

// TestTargetToFlowRepeatsGuardBeforeScale: when the encode is guarded, the
// scale step gets the same guard, so it never runs on its own.
func TestTargetToFlowRepeatsGuardBeforeScale(t *testing.T) {
	g, err := TargetToFlow(withTuning(Default()))
	require.NoError(t, err)
	types := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		types = append(types, n.Type)
	}
	require.Equal(t, []string{StepProbe, StepIfCodecNot, StepTranscodeVideo, StepIfCodecNot, StepScale, StepRemuxContainer, StepConvertAudio, StepSubtitles}, types)
	require.Empty(t, Validate(g))
}

// TestTargetToFlowDefaultUnchanged: a Target without the new fields generates
// the same graph JSON as before task 54 (no empty new keys on the step).
func TestTargetToFlowDefaultUnchanged(t *testing.T) {
	g, err := TargetToFlow(Default())
	require.NoError(t, err)
	raw, err := json.Marshal(g.Nodes[2])
	require.NoError(t, err)
	require.Equal(t, `{"id":"transcode_video","type":"transcode_video","params":{"codec":"hevc","quality":3}}`, string(raw))
}

// TestDescribeScaleAndTranscodeTuning: the new fields are described in plain
// language, one line per step, and the old two field line is unchanged.
func TestDescribeScaleAndTranscodeTuning(t *testing.T) {
	plan, err := Compile(tuningGraph(`{"height":1080}`, `{"codec":"hevc","quality":3,"rate_mode":"crf","crf":22,"max_kbps":9000,"preset":"slow","hardware":"qsv"}`))
	require.NoError(t, err)
	lines := plan.Describe()
	require.Equal(t, "Only when the video is not HEVC: Scale the video down to 1080p when it is taller; a smaller video is left as it is.", lines[1])
	require.Equal(t, "Encode the video as HEVC at a constant quality of CRF 22 (lower is better quality and a bigger file). Never more than 9000 kbps. Speed preset slow (slower presets save more space and take longer). Hardware: qsv.", lines[2])

	up := describeScale(&ScaleTarget{Height: 720, OnlyIfLarger: false, AllowUpscale: true})
	require.Equal(t, "Scale the video to 720p, enlarging it when it is smaller.", up)
	require.Equal(t, "Scale the video to 480p; a smaller video is not enlarged.", describeScale(&ScaleTarget{Height: 480}))

	plain, err := Compile(tuningGraph(`{"height":1080}`, `{"codec":"hevc","quality":3,"hardware":"auto"}`))
	require.NoError(t, err)
	require.Equal(t, "Encode the video as HEVC at quality 3 of 5 (lower saves more space).", plain.Describe()[2])
}

// TestStepTypesScaleIsPaletteEntry: the scale step is an action (not a guard)
// and carries no encoder terms on its face.
func TestStepTypesScaleIsPaletteEntry(t *testing.T) {
	for _, st := range StepTypes() {
		if st.Type != StepScale {
			continue
		}
		require.False(t, st.Condition)
		require.Equal(t, "Scale the video", st.Label)
		return
	}
	require.Fail(t, "scale is missing from StepTypes")
}

/* ------------------------------------------------------------------ */
/* Resolution, audio channel and HDR guards (task 56)                  */
/* ------------------------------------------------------------------ */

// guardedEncode is probe → guard → transcode (hevc) → remux (mkv): the one
// action sits behind the guard, the way a Moderate tree child does.
func guardedEncode(guardType, guardParams string) Graph {
	return Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "guard", Type: guardType, Params: json.RawMessage(guardParams)},
			{ID: "tv", Type: StepTranscodeVideo, Params: json.RawMessage(`{"codec":"hevc","quality":3}`)},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{
			{From: "probe", To: "guard"}, {From: "guard", To: "tv"}, {From: "tv", To: "remux"},
		},
	}
}

// encodesWith compiles the guarded graph and reports whether the transcode
// ran for the probed file (the video becomes the encode codec when it did).
func encodesWith(t *testing.T, g Graph, info probe.MediaInfo) bool {
	t.Helper()
	require.Empty(t, Validate(g))
	plan, err := Compile(g)
	require.NoError(t, err)
	eff, err := ResolvePlan(plan, info, probe.KindVideo)
	require.NoError(t, err)
	return eff.Target.Video.Codec == CodecHEVC
}

// TestResolutionGuardPassAndFail: a 1080p video passes "taller than 720"
// and fails "taller than 1080" (the threshold is exclusive, like bitrate).
func TestResolutionGuardPassAndFail(t *testing.T) {
	fhd := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 1080}}}
	sd := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 480}}}

	taller := guardedEncode(StepIfResolutionAbove, `{"height":720}`)
	assert.True(t, encodesWith(t, taller, fhd), "1080 is taller than 720: the child runs")
	assert.False(t, encodesWith(t, taller, sd), "480 is not taller than 720: the child is skipped")

	exact := guardedEncode(StepIfResolutionAbove, `{"height":1080}`)
	assert.False(t, encodesWith(t, exact, fhd), "1080 is not taller than 1080")
}

// TestAudioChannelsGuardPassAndFail: a 5.1 track passes "more than 2
// channels", a stereo-only file fails, and one surround track among stereo
// tracks is enough.
func TestAudioChannelsGuardPassAndFail(t *testing.T) {
	video := []probe.VideoStream{{Codec: CodecH264, Height: 1080}}
	stereo := probe.MediaInfo{Video: video, Audio: []probe.AudioStream{{Codec: "aac", Channels: 2}}}
	surround := probe.MediaInfo{Video: video, Audio: []probe.AudioStream{{Codec: "aac", Channels: 2}, {Codec: "eac3", Channels: 6}}}

	more := guardedEncode(StepIfAudioChannelsAbove, `{"channels":2}`)
	assert.False(t, encodesWith(t, more, stereo), "stereo is not more than 2 channels")
	assert.True(t, encodesWith(t, more, surround), "a 5.1 track is more than 2 channels")
}

// TestHDRGuardPassAndFail: is_hdr true runs only for HDR video, false only
// for video that is not HDR.
func TestHDRGuardPassAndFail(t *testing.T) {
	hdr := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 2160, HDR: true}}}
	sdr := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 2160, HDR: false}}}

	onlyHDR := guardedEncode(StepIfHDR, `{"is_hdr":true}`)
	assert.True(t, encodesWith(t, onlyHDR, hdr))
	assert.False(t, encodesWith(t, onlyHDR, sdr))

	onlySDR := guardedEncode(StepIfHDR, `{"is_hdr":false}`)
	assert.False(t, encodesWith(t, onlySDR, hdr))
	assert.True(t, encodesWith(t, onlySDR, sdr))
}

// TestGuardChildSkippedKeepsRestOfChain: the Moderate tree case. A false
// condition skips its indented child (the encode) and the rest of the chain
// (the container step) still runs; a true condition runs both.
func TestGuardChildSkippedKeepsRestOfChain(t *testing.T) {
	g := guardedEncode(StepIfHDR, `{"is_hdr":true}`)
	plan, err := Compile(g)
	require.NoError(t, err)
	sdr := probe.MediaInfo{Video: []probe.VideoStream{{Codec: CodecH264, Height: 1080}}}

	walk := FreshEffective()
	ran := map[string]bool{}
	for _, s := range plan.Steps {
		ok, err := ApplyStep(&walk, s, sdr, probe.KindVideo)
		require.NoError(t, err)
		ran[s.ID] = ok
	}
	assert.False(t, ran["tv"], "the child action is skipped when the condition fails")
	assert.True(t, ran["remux"], "the step after the skipped child still runs")
	assert.Equal(t, CodecCopy, walk.Target.Video.Codec, "the video stays copy")
	assert.Equal(t, ContainerMKV, walk.Target.Container)
}

// TestGuardedChildShapeMatchesBitrateGuard: a condition followed by a child
// action compiles to the same plan shape whichever guard type gates it, the
// shape if_bitrate_above has always produced.
func TestGuardedChildShapeMatchesBitrateGuard(t *testing.T) {
	shape := func(g Graph) []string {
		plan, err := Compile(g)
		require.NoError(t, err)
		out := make([]string, 0, len(plan.Steps))
		for _, s := range plan.Steps {
			out = append(out, fmt.Sprintf("%s/%s/when=%d", s.ID, s.Type, len(s.When)))
		}
		return out
	}
	want := []string{"probe/probe/when=0", "tv/transcode_video/when=1", "remux/remux_container/when=0"}
	require.Equal(t, want, shape(guardedEncode(StepIfBitrateAbove, `{"mbps":10}`)))
	require.Equal(t, want, shape(guardedEncode(StepIfResolutionAbove, `{"height":1080}`)))
	require.Equal(t, want, shape(guardedEncode(StepIfAudioChannelsAbove, `{"channels":2}`)))
	require.Equal(t, want, shape(guardedEncode(StepIfHDR, `{"is_hdr":true}`)))
}

// TestNewGuardsNeedMediaAndSkipNonMedia: the three guards read streams, so
// on a file that is not media they skip with the same rule as the others.
func TestNewGuardsNeedMediaAndSkipNonMedia(t *testing.T) {
	for _, typ := range []string{StepIfResolutionAbove, StepIfAudioChannelsAbove, StepIfHDR} {
		assert.True(t, StepNeedsMedia(typ), typ)
		assert.True(t, isGuard(typ), typ)
	}
	plan, err := Compile(guardedEncode(StepIfHDR, `{"is_hdr":false}`))
	require.NoError(t, err)
	eff := FreshEffective()
	ran, err := ApplyStep(&eff, plan.Steps[1], probe.MediaInfo{}, probe.KindImage)
	require.NoError(t, err)
	assert.False(t, ran)
}

// TestGuardClausesDescribeInPlainLanguage: each guard names its rule.
func TestGuardClausesDescribeInPlainLanguage(t *testing.T) {
	cases := map[string]struct {
		guard, params, want string
	}{
		"resolution": {StepIfResolutionAbove, `{"height":1080}`, "Only when the video is taller than 1080p: Encode the video as HEVC"},
		"channels":   {StepIfAudioChannelsAbove, `{"channels":6}`, "Only when the audio has more than 6 channels: Encode the video as HEVC"},
		"mono":       {StepIfAudioChannelsAbove, `{"channels":1}`, "Only when the audio has more than 1 channel: Encode the video as HEVC"},
		"hdr":        {StepIfHDR, `{"is_hdr":true}`, "Only when the video is HDR: Encode the video as HEVC"},
		"not hdr":    {StepIfHDR, `{"is_hdr":false}`, "Only when the video is not HDR: Encode the video as HEVC"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			plan, err := Compile(guardedEncode(c.guard, c.params))
			require.NoError(t, err)
			lines := plan.Describe()
			require.Len(t, lines, 3)
			require.Contains(t, lines[1], c.want)
		})
	}
}
