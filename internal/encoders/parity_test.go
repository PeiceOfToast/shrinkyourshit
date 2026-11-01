// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The parity gate (task 14): for every row of the easy mode table, the
// compiled flow resolves to a Target whose command arguments are byte
// identical to what the direct Target path produces. If the flow engine
// ever regresses, this test fails before a user sees a wrong encode.

// parityProbes covers the two interesting video states: a file that needs
// the transcode (h264 source) and a file the satisfied rule already skips
// (hevc source, so the if_codec_not guard fails and the video is copied).
var parityProbes = []probe.MediaInfo{
	{
		Container: flow.ContainerMKV,
		Video:     []probe.VideoStream{{Codec: flow.CodecH264, Bitrate: 8_000_000, FPS: 24}},
		Audio: []probe.AudioStream{
			{Codec: "eac3", Channels: 6, Language: "jpn", Bitrate: 768_000},
			{Codec: "aac", Channels: 2, Language: "eng", Bitrate: 192_000},
		},
		Subtitle: []probe.SubtitleStream{
			{Codec: "subrip", Language: "eng", TextBased: true},
			{Codec: "subrip", Language: "ger", TextBased: true},
		},
	},
	{
		Container: flow.ContainerMKV,
		Video:     []probe.VideoStream{{Codec: flow.CodecHEVC, Bitrate: 8_000_000, FPS: 24}},
		Audio: []probe.AudioStream{
			{Codec: "eac3", Channels: 6, Language: "jpn", Bitrate: 768_000},
			{Codec: "aac", Channels: 2, Language: "eng", Bitrate: 192_000},
		},
		Subtitle: []probe.SubtitleStream{
			{Codec: "subrip", Language: "eng", TextBased: true},
			{Codec: "subrip", Language: "ger", TextBased: true},
		},
	},
}

// parityEncoders are the backends whose BuildArgs the parity gate covers;
// all of them are pure functions of the spec, so no ffmpeg is needed.
var parityEncoders = []Encoder{CPU, NVENC, QSV, VAAPI}

func TestEasyFlowParity(t *testing.T) {
	// Detection state the singletons carry would change BuildArgs; pin the
	// defaults so the goldens are independent of test order.
	resetQSVVariants()
	resetVAAPIVariants()
	defer resetVAAPIVariants()
	for _, info := range parityProbes {
		for quality := flow.QualityMin; quality <= flow.QualityMax; quality++ {
			for audio := flow.EasyAudioKeepAll; audio <= flow.EasyAudioCompact; audio++ {
				for _, removeSubs := range []bool{false, true} {
					easy := flow.EasyModeInput{
						Quality:         quality,
						RemoveSubtitles: removeSubs,
						Audio:           audio,
					}
					target, err := flow.CompileEasyMode(easy, flow.EasySettings{
						Codec:              flow.CodecHEVC,
						PreferredLanguages: []string{"eng"},
					})
					require.NoError(t, err)

					graph, err := flow.TargetToFlow(target)
					require.NoError(t, err)
					plan, err := flow.Compile(graph)
					require.NoError(t, err)
					eff, err := flow.ResolvePlan(plan, info, probe.KindVideo)
					require.NoError(t, err)

					direct := NewSpec("/in.mkv", "/out.mkv", info, target)
					viaFlow := NewSpec("/in.mkv", "/out.mkv", info, eff.Target)
					for _, enc := range parityEncoders {
						want := enc.BuildArgs(direct)
						got := enc.BuildArgs(viaFlow)
						require.Equal(t, want, got,
							"flow args diverge from the builder (encoder %s, easy %v)", enc.Name(), easy)
					}

					// The extract commands (subtitles.extract_removed)
					// must match too; easy mode keeps extraction off, so
					// both paths produce none.
					require.Equal(t, ExtractCommands(direct), ExtractCommands(viaFlow))
				}
			}
		}
	}
}

// TestFlowCopyParity: a hand-written flow without a transcode step (video
// copy) matches a copy Target.
func TestFlowCopyParity(t *testing.T) {
	info := parityProbes[0]
	target := flow.Default()
	target.Video.Codec = flow.CodecCopy

	graph, err := flow.TargetToFlow(target)
	require.NoError(t, err)
	plan, err := flow.Compile(graph)
	require.NoError(t, err)
	eff, err := flow.ResolvePlan(plan, info, probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, flow.CodecCopy, eff.Target.Video.Codec)

	direct := NewSpec("/in.mkv", "/out.mkv", info, target)
	viaFlow := NewSpec("/in.mkv", "/out.mkv", info, eff.Target)
	for _, enc := range parityEncoders {
		require.Equal(t, enc.BuildArgs(direct), enc.BuildArgs(viaFlow))
	}
}

// TestFlowAlwaysReencode: a transcode step without a guard really runs
// even when the file already has the target codec (reencode_if_same_codec).
func TestFlowAlwaysReencode(t *testing.T) {
	info := parityProbes[1] // hevc source

	graph := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "transcode_video", Type: flow.StepTranscodeVideo, Params: []byte(`{"codec":"hevc","quality":3}`)},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "transcode_video"},
			{From: "transcode_video", To: "remux_container"},
		},
	}
	plan, err := flow.Compile(graph)
	require.NoError(t, err)
	eff, err := flow.ResolvePlan(plan, info, probe.KindVideo)
	require.NoError(t, err)
	require.True(t, eff.Target.Video.ReencodeIfSameCodec, "an unguarded transcode always runs")

	// The direct equivalent is the same Target with the flag on.
	target := flow.Default()
	target.Video.ReencodeIfSameCodec = true
	direct := NewSpec("/in.mkv", "/out.mkv", info, target)
	viaFlow := NewSpec("/in.mkv", "/out.mkv", info, eff.Target)
	require.Equal(t, CPU.BuildArgs(direct), CPU.BuildArgs(viaFlow))
}

// TestFlowBitrateGuard: an if_bitrate_above guard runs the transcode for a
// high-bitrate file (even same codec) and skips a low-bitrate one.
func TestFlowBitrateGuard(t *testing.T) {
	graph := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "bitrate", Type: flow.StepIfBitrateAbove, Params: []byte(`{"mbps":10}`)},
			{ID: "transcode_video", Type: flow.StepTranscodeVideo, Params: []byte(`{"codec":"hevc","quality":3}`)},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "bitrate"},
			{From: "bitrate", To: "transcode_video"},
			{From: "transcode_video", To: "remux_container"},
		},
	}
	plan, err := flow.Compile(graph)
	require.NoError(t, err)

	mkProbe := func(bitrate int64) probe.MediaInfo {
		p := parityProbes[0]
		p.Video = []probe.VideoStream{{Codec: flow.CodecH264, Bitrate: bitrate, FPS: 24}}
		return p
	}
	low := mkProbe(4_000_000)
	high := mkProbe(12_000_000)

	skipped, err := flow.ResolvePlan(plan, low, probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, flow.CodecCopy, skipped.Target.Video.Codec)

	runs, err := flow.ResolvePlan(plan, high, probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, flow.CodecHEVC, runs.Target.Video.Codec)
	require.True(t, runs.Target.Video.ReencodeIfSameCodec,
		"a bitrate-gated transcode must really run even on same-codec files")
}

// TestQSVPrefsOffParity (task 35): with both QSV settings off — whatever
// the capability probe found — the qsv backend's argument vector is byte
// identical to the behaviour before task 35. This is the regression that
// matters: a machine that opted into nothing must encode exactly as it
// did before.
func TestQSVPrefsOffParity(t *testing.T) {
	resetQSVVariants()
	resetQSVFeaturesAndPrefs()
	defer func() {
		resetQSVVariants()
		resetQSVFeaturesAndPrefs()
	}()

	// Every feature probed true: still nothing in the vector without the
	// user's opt-in.
	setQSVFeatures(map[string]qsvFeatures{
		flow.CodecHEVC: {LookAhead: true, ExtBRC: true, AsyncDepth: true, LowPower: true},
		flow.CodecAV1:  {LookAhead: true, ExtBRC: true, AsyncDepth: true, LowPower: true},
		flow.CodecH264: {LookAhead: true, ExtBRC: true, AsyncDepth: true, LowPower: true},
	})

	base := func(encoder string) string {
		return join(
			"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
			"-i", "in.mkv",
			keepAllMaps,
			"-c:v", encoder, "-global_quality", "26",
			keepAllAudioCodecs, keepAllSubCodecs,
			"out.mkv")
	}
	for _, tc := range []struct{ codec, encoder string }{
		{flow.CodecHEVC, "hevc_qsv"},
		{flow.CodecAV1, "av1_qsv"},
		{flow.CodecH264, "h264_qsv"},
	} {
		assertArgs(t, QSV, qsvSpec(tc.codec, 3), base(tc.encoder))
	}

	// The vaapi variant is byte identical too (it never carried the
	// extras).
	assertArgs(t, QSV, vaapiSpec(flow.CodecHEVC, 3, "/dev/dri/renderD128"), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD128",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "hevc_vaapi", "-global_quality", "26", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestQSVRequestedUnsupportedParity (task 35): with every probe false the
// settings produce no flags at all — one warn per requested feature, no
// error, and the same byte-identical vector as with the settings off.
func TestQSVRequestedUnsupportedParity(t *testing.T) {
	resetQSVVariants()
	resetQSVFeaturesAndPrefs()
	defer func() {
		resetQSVVariants()
		resetQSVFeaturesAndPrefs()
	}()

	setQSVFeatures(map[string]qsvFeatures{}) // every probe rejected
	setQSVPrefsForTest(QSVPrefs{ExtraQuality: true, LowPower: true})

	spec := qsvSpec(flow.CodecH264, 3)
	args := strings.Join(QSV.BuildArgs(spec), " ")
	assert.Equal(t, join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_qsv", "-global_quality", "26",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"), args)

	for _, flag := range []string{"-look_ahead", "-extbrc", "-low_power", "-async_depth"} {
		assert.NotContains(t, args, flag)
	}
}

// TestFlowSkipAndRename: skip_if_smaller and rename surface outside the
// Target, so the job can override the library policy and name the output.
func TestFlowSkipAndRename(t *testing.T) {
	graph := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: []byte(`{"container":"mkv"}`)},
			{ID: "rename", Type: flow.StepRename, Params: []byte(`{"template":"{name}.shrunk"}`)},
			{ID: "skip", Type: flow.StepSkipIfSmaller},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "remux_container"},
			{From: "remux_container", To: "rename"},
			{From: "rename", To: "skip"},
		},
	}
	plan, err := flow.Compile(graph)
	require.NoError(t, err)
	require.True(t, plan.HasSkipIfSmaller())
	require.Equal(t, "{name}.shrunk", plan.RenameTemplate())

	eff, err := flow.ResolvePlan(plan, parityProbes[0], probe.KindVideo)
	require.NoError(t, err)
	require.True(t, eff.SkipIfSmaller)
	require.Equal(t, "{name}.shrunk", eff.Rename)
}
