// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// testInfo is a realistic probe result: vp9 video (a codec no golden
// targets, so those goldens always exercise the re-encode path), a ger 5.1
// eac3 track plus an eng stereo aac track, and three subtitles (srt eng,
// mov_text ger, pgs eng).
func testInfo() probe.MediaInfo {
	return probe.MediaInfo{
		Container: "mkv",
		Video:     []probe.VideoStream{{Codec: "vp9", Width: 1920, Height: 1080, FPS: 24, PixFmt: "yuv420p"}},
		Audio: []probe.AudioStream{
			{Codec: "eac3", Channels: 6, Language: "ger"},
			{Codec: "aac", Channels: 2, Language: "eng"},
		},
		Subtitle: []probe.SubtitleStream{
			{Codec: "subrip", Language: "eng", TextBased: true},
			{Codec: "mov_text", Language: "ger", TextBased: true},
			{Codec: "hdmv_pgs_subtitle", Language: "eng"},
		},
	}
}

// testSpec is the default spec: hevc step 3 in mkv, all tracks kept.
func testSpec() Spec {
	return NewSpec("in.mkv", "out.mkv", testInfo(), flow.Default())
}

func withTarget(tg flow.Target) Spec {
	return NewSpec("in.mkv", "out.mkv", testInfo(), tg)
}

func withTargetOut(tg flow.Target, output string) Spec {
	return NewSpec("in.mkv", output, testInfo(), tg)
}

func codecTarget(codec string, step int) flow.Target {
	tg := flow.Default()
	tg.Video.Codec = codec
	tg.Video.Quality = step
	return tg
}

func copyTarget() flow.Target {
	tg := flow.Default()
	tg.Video.Codec = flow.CodecCopy
	return tg
}

func compactTarget(compactCodec, videoCodec, preferred string) flow.Target {
	tg := flow.Default()
	tg.Video.Codec = videoCodec
	tg.Audio.Mode = flow.AudioCompact
	tg.Audio.CompactCodec = compactCodec
	tg.Audio.PreferredLanguages = []string{preferred}
	return tg
}

func removeSubsTarget() flow.Target {
	tg := flow.Default()
	tg.Subtitles.Mode = flow.SubsRemove
	return tg
}

func mp4Target() flow.Target {
	tg := flow.Default()
	tg.Container = flow.ContainerMP4
	return tg
}

// resetQSVVariants clears the variant cache the qsv singleton accumulates
// across tests, so golden expectations stay independent of test order.
func resetQSVVariants() {
	qsv := QSV.(*qsvEncoder)
	qsv.mu.Lock()
	defer qsv.mu.Unlock()
	qsv.variants = nil
}

// resetQSVFeaturesAndPrefs clears the feature probe and the applied opt-ins
// (task 35), so tests that depend on the default argument vector stay
// independent of test order.
func resetQSVFeaturesAndPrefs() {
	qsv := QSV.(*qsvEncoder)
	qsv.mu.Lock()
	defer qsv.mu.Unlock()
	qsv.probed = false
	qsv.features = nil
	qsv.prefs = QSVPrefs{}
	qsv.warned = nil
}

// setQSVFeatures pins per-codec probe results as if detection had run
// (task 35 tests).
func setQSVFeatures(features map[string]qsvFeatures) {
	QSV.(*qsvEncoder).setFeatures(features)
}

// setQSVPrefs pins the applied opt-ins without warns (task 35 tests).
func setQSVPrefsForTest(p QSVPrefs) {
	QSV.(*qsvEncoder).setPrefs(p, "test", nil)
}

func setQSVVariant(codec string, v qsvVariant) {
	QSV.(*qsvEncoder).setVariant(codec, v)
}

// qsvSpec pins the qsv (primary) variant for a codec.
func qsvSpec(codec string, step int) Spec {
	setQSVVariant(codec, qsvVariant{encoderName: qsvEncoderFor[codec]})
	return withTarget(codecTarget(codec, step))
}

// vaapiSpec pins the VAAPI variant (as detection chooses it when QSV init
// fails) for a codec.
func vaapiSpec(codec string, step int, device string) Spec {
	setQSVVariant(codec, qsvVariant{encoderName: vaapiEncoderFor[codec], device: device})
	return withTarget(codecTarget(codec, step))
}

// keepAllMaps is the map block of the default spec: the video stream, both
// audio tracks and all three subtitles.
const keepAllMaps = "-map 0:V:0 -map 0:a:0 -map 0:a:1 -map 0:s:0 -map 0:s:1 -map 0:s:2"

// keepAllSubCodecs is the codec block for the default spec in mkv: copy
// everywhere except the ger mov_text track, which becomes srt.
const keepAllSubCodecs = "-c:s:0 copy -c:s:1 srt -c:s:2 copy"

// keepAllAudioCodecs is the codec block for keep_all audio: plain copy.
const keepAllAudioCodecs = "-c:a:0 copy -c:a:1 copy"

func join(args ...string) string {
	return strings.Join(args, " ")
}

// assertArgs compares the built args against an exact golden string.
func assertArgs(t *testing.T, enc Encoder, spec Spec, want string) {
	t.Helper()
	got := strings.Join(enc.BuildArgs(spec), " ")
	assert.Equal(t, want, got)
}

// TestBuildArgsCPUQualitySteps: the x265/x264/SVT-AV1 CRF columns of the
// quality table, one golden per step.
func TestBuildArgsCPUQualitySteps(t *testing.T) {
	crfs := []struct{ step, crf string }{
		{"1", "28"}, {"2", "26"}, {"3", "24"}, {"4", "22"}, {"5", "20"},
	}
	for _, c := range crfs {
		t.Run("hevc step "+c.step, func(t *testing.T) {
			tg := flow.Default()
			tg.Video.Quality = mustStep(t, c.step)
			assertArgs(t, CPU, withTarget(tg), join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "libx265", "-crf", c.crf, "-preset", "medium",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"))
		})
	}

	// The other CPU codecs at step 3.
	assertArgs(t, CPU, withTarget(codecTarget(flow.CodecH264, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "libx264", "-crf", "22", "-preset", "medium",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
	assertArgs(t, CPU, withTarget(codecTarget(flow.CodecAV1, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "libsvtav1", "-crf", "32", "-preset", "6",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsNVENCQualitySteps: the NVENC CQ column, one golden per step.
func TestBuildArgsNVENCQualitySteps(t *testing.T) {
	cqs := []struct{ step, cq string }{
		{"1", "32"}, {"2", "30"}, {"3", "28"}, {"4", "26"}, {"5", "24"},
	}
	for _, c := range cqs {
		t.Run("hevc step "+c.step, func(t *testing.T) {
			tg := flow.Default()
			tg.Video.Quality = mustStep(t, c.step)
			assertArgs(t, NVENC, withTarget(tg), join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "hevc_nvenc", "-cq", c.cq, "-preset", "p5",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"))
		})
	}

	assertArgs(t, NVENC, withTarget(codecTarget(flow.CodecH264, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_nvenc", "-cq", "28", "-preset", "p5",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
	assertArgs(t, NVENC, withTarget(codecTarget(flow.CodecAV1, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "av1_nvenc", "-cq", "28", "-preset", "p5",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsQSVQualitySteps: the QSV global_quality column, one golden
// per step (primary qsv variant).
func TestBuildArgsQSVQualitySteps(t *testing.T) {
	resetQSVVariants()
	gqs := []struct{ step, gq string }{
		{"1", "30"}, {"2", "28"}, {"3", "26"}, {"4", "24"}, {"5", "22"},
	}
	for _, c := range gqs {
		t.Run("hevc step "+c.step, func(t *testing.T) {
			tg := flow.Default()
			tg.Video.Quality = mustStep(t, c.step)
			setQSVVariant(flow.CodecHEVC, qsvVariant{encoderName: "hevc_qsv"})
			assertArgs(t, QSV, withTarget(tg), join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "hevc_qsv", "-global_quality", c.gq,
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"))
		})
	}
	resetQSVVariants()

	assertArgs(t, QSV, qsvSpec(flow.CodecH264, 3), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_qsv", "-global_quality", "26",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
	assertArgs(t, QSV, qsvSpec(flow.CodecAV1, 3), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "av1_qsv", "-global_quality", "26",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
	resetQSVVariants()
}

// TestBuildArgsQSVVAAPIVariant: when detection fell back to the VAAPI
// encoders, args carry the device node and the hwupload filter chain.
func TestBuildArgsQSVVAAPIVariant(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	assertArgs(t, QSV, vaapiSpec(flow.CodecHEVC, 3, "/dev/dri/renderD128"), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD128",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "hevc_vaapi", "-global_quality", "26", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))

	// h264_vaapi has no global_quality mode; -qp sets the fixed quantizer.
	assertArgs(t, QSV, vaapiSpec(flow.CodecH264, 3, "/dev/dri/renderD128"), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD128",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_vaapi", "-qp", "26", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsCopyVideo: video.codec=copy emits -c:v copy from every
// encoder (remux and track changes only).
func TestBuildArgsCopyVideo(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	want := join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "copy",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv")
	assertArgs(t, CPU, withTarget(copyTarget()), want)
	assertArgs(t, NVENC, withTarget(copyTarget()), want)
	assertArgs(t, QSV, withTarget(copyTarget()), want)
}

// TestBuildArgsCompactAudio: compact transcodes the kept track when it is
// not already in the compact codec, and copies it when it is.
func TestBuildArgsCompactAudio(t *testing.T) {
	// keep_best with ger preference selects the eac3 ger track (index 0).
	assertArgs(t, CPU, withTarget(compactTarget(flow.CompactCodecAAC, flow.CodecHEVC, "ger")), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		"-map", "0:V:0", "-map", "0:a:0",
		"-map", "0:s:0", "-map", "0:s:1", "-map", "0:s:2",
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		"-c:a:0", "aac", "-b:a:0", "160k", "-ac:a:0", "2",
		keepAllSubCodecs,
		"out.mkv"))

	assertArgs(t, CPU, withTarget(compactTarget(flow.CompactCodecOpus, flow.CodecHEVC, "ger")), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		"-map", "0:V:0", "-map", "0:a:0",
		"-map", "0:s:0", "-map", "0:s:1", "-map", "0:s:2",
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		"-c:a:0", "libopus", "-b:a:0", "128k", "-ac:a:0", "2",
		keepAllSubCodecs,
		"out.mkv"))

	// keep_best with eng preference selects the aac eng track (index 1),
	// already in the compact codec: plain copy.
	assertArgs(t, CPU, withTarget(compactTarget(flow.CompactCodecAAC, flow.CodecHEVC, "eng")), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		"-map", "0:V:0", "-map", "0:a:1",
		"-map", "0:s:0", "-map", "0:s:1", "-map", "0:s:2",
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		"-c:a:0", "copy",
		keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsSubtitleRemoval: removed subtitles are neither mapped nor
// given codec options.
func TestBuildArgsSubtitleRemoval(t *testing.T) {
	assertArgs(t, CPU, withTarget(removeSubsTarget()), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		"-map", "0:V:0", "-map", "0:a:0", "-map", "0:a:1",
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		keepAllAudioCodecs,
		"out.mkv"))
}

// TestBuildArgsMP4Output: faststart for mp4 and text subtitles converted to
// mov_text (the only text codec mp4 reliably holds).
func TestBuildArgsMP4Output(t *testing.T) {
	assertArgs(t, CPU, withTargetOut(mp4Target(), "out.mp4"), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		keepAllAudioCodecs,
		"-c:s:0", "mov_text", "-c:s:1", "copy", "-c:s:2", "copy",
		"-movflags", "+faststart",
		"out.mp4"))
}

// TestBuildArgsFileWithoutSubtitles: no subtitle maps when the file has none.
func TestBuildArgsFileWithoutSubtitles(t *testing.T) {
	info := testInfo()
	info.Subtitle = nil
	assertArgs(t, CPU, NewSpec("in.mkv", "out.mkv", info, flow.Default()), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		"-map", "0:V:0", "-map", "0:a:0", "-map", "0:a:1",
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		keepAllAudioCodecs,
		"out.mkv"))
}

// TestBuildArgsQualityOutOfRange: a bad quality clamps instead of panicking.
func TestBuildArgsQualityOutOfRange(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	tg := flow.Default()
	tg.Video.Quality = 99
	spec := withTarget(tg)
	args := CPU.BuildArgs(spec)
	assert.Contains(t, strings.Join(args, " "), "-crf 20", "quality 99 clamps to step 5")
}

// TestBuildArgsAlreadyTargetCodec: a file whose video already is the target
// codec is remuxed, not re-encoded (DECISIONS.md B2 satisfied rule) — but
// reencode_if_same_codec forces the encoder.
func TestBuildArgsAlreadyTargetCodec(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	alreadyHEVC := testInfo()
	alreadyHEVC.Video[0].Codec = "hevc"

	// Video already hevc, target hevc: remux and track changes only.
	spec := NewSpec("in.mkv", "out.mkv", alreadyHEVC, flow.Default())
	assertArgs(t, CPU, spec, join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "copy",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))

	// Same file, but the override flag forces a re-encode.
	tg := flow.Default()
	tg.Video.ReencodeIfSameCodec = true
	spec = NewSpec("in.mkv", "out.mkv", alreadyHEVC, tg)
	assertArgs(t, CPU, spec, join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "libx265", "-crf", "24", "-preset", "medium",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestExtractCommandsGolden covers the extract_removed side commands.
func TestExtractCommandsGolden(t *testing.T) {
	target := flow.Default()
	target.Subtitles.Mode = flow.SubsRemove
	target.Subtitles.ExtractRemoved = true
	spec := NewSpec("in.mkv", "/scratch/job/output.mkv", testInfo(), target)

	cmds := ExtractCommands(spec)
	require.Len(t, cmds, 2, "pgs is bitmap and cannot be extracted")

	assert.Equal(t, ExtractCommand{
		SubIndex: 0,
		Language: "eng",
		Args: []string{
			"-hide_banner", "-nostdin", "-v", "error", "-y",
			"-i", "in.mkv",
			"-map", "0:s:0",
			"-c:s", "srt",
			"/scratch/job/output.eng.srt",
		},
	}, cmds[0])
	assert.Equal(t, ExtractCommand{
		SubIndex: 1,
		Language: "ger",
		Args: []string{
			"-hide_banner", "-nostdin", "-v", "error", "-y",
			"-i", "in.mkv",
			"-map", "0:s:1",
			"-c:s", "srt",
			"/scratch/job/output.ger.srt",
		},
	}, cmds[1])
}

// TestExtractCommandsOffAndDuplicateLangs: nothing when the flag is off,
// and repeated languages get an occurrence suffix instead of overwriting.
func TestExtractCommandsOffAndDuplicateLangs(t *testing.T) {
	// Flag off: nothing, even with removed subtitles.
	target := flow.Default()
	target.Subtitles.Mode = flow.SubsRemove
	spec := NewSpec("in.mkv", "out.mkv", testInfo(), target)
	assert.Empty(t, ExtractCommands(spec))

	// Two removed eng text tracks: the second becomes out.eng.2.srt.
	dup := testInfo()
	dup.Subtitle = []probe.SubtitleStream{
		{Codec: "subrip", Language: "eng", TextBased: true},
		{Codec: "ass", Language: "eng", TextBased: true},
	}
	target.Subtitles.ExtractRemoved = true
	spec = NewSpec("in.mkv", "out.mkv", dup, target)
	cmds := ExtractCommands(spec)
	require.Len(t, cmds, 2)
	assert.Equal(t, "out.eng.srt", cmds[0].Args[len(cmds[0].Args)-1])
	assert.Equal(t, "out.eng.2.srt", cmds[1].Args[len(cmds[1].Args)-1])

	// An unset language falls back to "und".
	nolang := testInfo()
	nolang.Subtitle = []probe.SubtitleStream{{Codec: "subrip", TextBased: true}}
	spec = NewSpec("in.mkv", "out.mkv", nolang, target)
	cmds = ExtractCommands(spec)
	require.Len(t, cmds, 1)
	assert.Equal(t, "out.und.srt", cmds[0].Args[len(cmds[0].Args)-1])
	assert.Equal(t, "und", cmds[0].Language)
}

// TestBuildArgsQSVFeatureFlags (task 35): the argument vector as a table
// over the applied opt-ins and the probed features — including the case
// where nothing is supported, which must equal the pre-task-35 vector.
func TestBuildArgsQSVFeatureFlags(t *testing.T) {
	// Per-codec probe results used across the cases: h264 accepts
	// everything (look-ahead is h264-only), hevc accepts everything but
	// look-ahead, av1 nothing.
	probes := map[string]qsvFeatures{
		flow.CodecH264: {LookAhead: true, ExtBRC: true, AsyncDepth: true, LowPower: true},
		flow.CodecHEVC: {ExtBRC: true, AsyncDepth: true, LowPower: true},
	}

	tests := []struct {
		name   string
		prefs  QSVPrefs
		probes map[string]qsvFeatures
		codec  string
		step   int
		want   string
	}{
		{
			name:   "settings off with every feature supported",
			probes: probes,
			codec:  flow.CodecH264, step: 3,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "h264_qsv", "-global_quality", "26",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:   "settings on but nothing supported",
			prefs:  QSVPrefs{ExtraQuality: true, LowPower: true},
			probes: map[string]qsvFeatures{},
			codec:  flow.CodecH264, step: 3,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "h264_qsv", "-global_quality", "26",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:  "extra quality on with look-ahead and extbrc (h264)",
			prefs: QSVPrefs{ExtraQuality: true},
			// Async depth probed off, so this case isolates the look-ahead
			// pair.
			probes: map[string]qsvFeatures{
				flow.CodecH264: {LookAhead: true, ExtBRC: true},
			},
			codec: flow.CodecH264, step: 3,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "h264_qsv", "-global_quality", "26",
				"-look_ahead", "1", "-extbrc", "1",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:   "extra quality on cannot give hevc the h264-only look-ahead",
			prefs:  QSVPrefs{ExtraQuality: true},
			probes: probes,
			codec:  flow.CodecHEVC, step: 3,
			// hevc gets no look-ahead (h264-only option), but it does get
			// the async pipeline depth the opt-in carries.
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "hevc_qsv", "-global_quality", "26", "-async_depth", "3",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:   "low power on with async depth (hevc, step 3)",
			prefs:  QSVPrefs{LowPower: true},
			probes: probes,
			codec:  flow.CodecHEVC, step: 3,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "hevc_qsv", "-global_quality", "26",
				"-low_power", "1", "-async_depth", "3",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:   "both on, everything supported, quality 5 (shallowest pipeline)",
			prefs:  QSVPrefs{ExtraQuality: true, LowPower: true},
			probes: probes,
			codec:  flow.CodecH264, step: 5,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "h264_qsv", "-global_quality", "22",
				"-look_ahead", "1", "-extbrc", "1", "-low_power", "1", "-async_depth", "2",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
		{
			name:   "both on, everything supported, quality 1 (deepest pipeline)",
			prefs:  QSVPrefs{ExtraQuality: true, LowPower: true},
			probes: probes,
			codec:  flow.CodecH264, step: 1,
			want: join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "h264_qsv", "-global_quality", "30",
				"-look_ahead", "1", "-extbrc", "1", "-low_power", "1", "-async_depth", "4",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetQSVFeaturesAndPrefs()
			defer resetQSVFeaturesAndPrefs()
			setQSVFeatures(tc.probes)
			setQSVPrefsForTest(tc.prefs)
			assertArgs(t, QSV, qsvSpec(tc.codec, tc.step), tc.want)
		})
	}

	// The vaapi variant never carries the QSV extras (the *_vaapi encoders
	// have none of those options), whatever the settings and probes say.
	t.Run("vaapi variant ignores the extras", func(t *testing.T) {
		resetQSVFeaturesAndPrefs()
		defer resetQSVFeaturesAndPrefs()
		setQSVFeatures(probes)
		setQSVPrefsForTest(QSVPrefs{ExtraQuality: true, LowPower: true})
		assertArgs(t, QSV, vaapiSpec(flow.CodecH264, 3, "/dev/dri/renderD128"), join(
			"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
			"-vaapi_device", "/dev/dri/renderD128",
			"-i", "in.mkv",
			keepAllMaps,
			"-c:v", "h264_vaapi", "-qp", "26", "-vf", "format=nv12,hwupload",
			keepAllAudioCodecs, keepAllSubCodecs,
			"out.mkv"))
	})

	// A copied video (explicit copy or the satisfied rule) never gains the
	// extras either.
	t.Run("copy stays copy", func(t *testing.T) {
		resetQSVFeaturesAndPrefs()
		defer resetQSVFeaturesAndPrefs()
		setQSVFeatures(probes)
		setQSVPrefsForTest(QSVPrefs{ExtraQuality: true, LowPower: true})
		assertArgs(t, QSV, withTarget(copyTarget()), join(
			"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
			"-i", "in.mkv",
			keepAllMaps,
			"-c:v", "copy",
			keepAllAudioCodecs, keepAllSubCodecs,
			"out.mkv"))
	})
}

// TestBuildArgsQSVICQAndFallbackRangeEnds (task 35): the ICQ path
// (hevc_qsv, av1_qsv — -global_quality is Intel's ICQ quality) and the
// fallback path (h264_qsv keeps the existing mapping, because ICQ support
// for h264 varies by Intel generation) pinned at the quality range ends.
func TestBuildArgsQSVICQAndFallbackRangeEnds(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	cases := []struct {
		codec, gq1, gq5 string
	}{
		{flow.CodecHEVC, "30", "22"}, // ICQ path
		{flow.CodecAV1, "30", "22"},  // ICQ path
		{flow.CodecH264, "30", "22"}, // existing mapping, kept as the fallback
	}
	for _, c := range cases {
		for _, step := range []struct {
			n  int
			gq string
		}{{1, c.gq1}, {5, c.gq5}} {
			t.Run(c.codec+" step "+itoa(step.n), func(t *testing.T) {
				assertArgs(t, QSV, qsvSpec(c.codec, step.n), join(
					"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
					"-i", "in.mkv",
					keepAllMaps,
					"-c:v", qsvEncoderFor[c.codec], "-global_quality", step.gq,
					keepAllAudioCodecs, keepAllSubCodecs,
					"out.mkv"))
			})
		}
	}
}

func mustStep(t *testing.T, s string) int {
	t.Helper()
	switch s {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	case "5":
		return 5
	}
	t.Fatalf("bad step %q", s)
	return 0
}
