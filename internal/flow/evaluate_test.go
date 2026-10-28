// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// hevcTarget is the default hevc/mkv target used by most cases.
func hevcTarget() Target {
	return Default()
}

func infoHEVC() probe.MediaInfo {
	return probe.MediaInfo{
		Container: "mkv",
		Video:     []probe.VideoStream{{Codec: "hevc", Width: 1920, Height: 1080, FPS: 24}},
		Audio:     []probe.AudioStream{{Codec: "aac", Channels: 2, Language: "eng"}},
		Subtitle:  []probe.SubtitleStream{{Codec: "subrip", Language: "eng"}},
	}
}

// infoHevcTwoAudioOneSubtitle is like infoH264TwoAudioOneSubtitle but with
// video that already matches the hevc target, so audio and subtitle
// assertions are not polluted by a video reason.
func infoHevcTwoAudioOneSubtitle() probe.MediaInfo {
	info := infoH264TwoAudioOneSubtitle()
	info.Video[0].Codec = "hevc"
	return info
}

func infoH264TwoAudioOneSubtitle() probe.MediaInfo {
	return probe.MediaInfo{
		Container: "mkv",
		Video:     []probe.VideoStream{{Codec: "h264", Width: 1920, Height: 1080, FPS: 24}},
		Audio: []probe.AudioStream{
			{Codec: "aac", Channels: 2, Language: "eng"},
			{Codec: "eac3", Channels: 6, Language: "ger"},
		},
		Subtitle: []probe.SubtitleStream{{Codec: "subrip", Language: "eng"}},
	}
}

func TestEvaluateVideoCodecDifference(t *testing.T) {
	d := Evaluate(infoH264TwoAudioOneSubtitle(), hevcTarget())
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "video is h264, target is hevc")
}

func TestEvaluateVideoAlreadyTargetSatisfied(t *testing.T) {
	d := Evaluate(infoHEVC(), hevcTarget())
	assert.False(t, d.Needs)
	assert.Empty(t, d.Reasons)
}

func TestEvaluateReencodeIfSameCodec(t *testing.T) {
	// B2 override: same codec but reencode_if_same_codec forces work.
	target := hevcTarget()
	target.Video.ReencodeIfSameCodec = true
	d := Evaluate(infoHEVC(), target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "video is hevc, target is hevc (reencode_if_same_codec)")
}

func TestEvaluateVideoCopyNeverNeedsWork(t *testing.T) {
	target := hevcTarget()
	target.Video.Codec = CodecCopy
	d := Evaluate(infoH264TwoAudioOneSubtitle(), target)
	assert.False(t, d.Needs, "copy never re-encodes video")
	assert.Empty(t, d.Reasons)
}

func TestEvaluateNoVideoStream(t *testing.T) {
	// An audio-only file has no video to satisfy; the video clause passes.
	info := infoHEVC()
	info.Video = nil
	d := Evaluate(info, hevcTarget())
	assert.False(t, d.Needs)
}

func TestEvaluateAudioKeepAll(t *testing.T) {
	target := hevcTarget()
	target.Audio.Mode = AudioKeepAll
	d := Evaluate(infoH264TwoAudioOneSubtitle(), target)
	require.True(t, d.Needs)
	assert.Equal(t, []string{"video is h264, target is hevc"}, d.Reasons,
		"keep_all never adds audio reasons")
}

func TestEvaluateAudioKeepPreferred(t *testing.T) {
	// eng+ger with preferred eng: dropping ger needs work.
	target := hevcTarget()
	target.Audio.Mode = AudioKeepPreferred
	target.Audio.PreferredLanguages = []string{"eng"}
	d := Evaluate(infoHevcTwoAudioOneSubtitle(), target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio mode keep_preferred would drop 1 track(s)")

	// Both tracks eng: nothing to drop, satisfied.
	info := infoHevcTwoAudioOneSubtitle()
	info.Audio[1].Language = "eng"
	d = Evaluate(info, target)
	assert.False(t, d.Needs)
}

func TestEvaluateAudioKeepPreferredNoMatchKeepsAll(t *testing.T) {
	// No matching language: the rule says keep all, so satisfied.
	target := hevcTarget()
	target.Audio.Mode = AudioKeepPreferred
	target.Audio.PreferredLanguages = []string{"fre"}
	d := Evaluate(infoHevcTwoAudioOneSubtitle(), target)
	assert.False(t, d.Needs)
}

func TestEvaluateAudioKeepPreferredUndNeverMatches(t *testing.T) {
	// "und" and empty tags never match a preferred language.
	target := hevcTarget()
	target.Audio.Mode = AudioKeepPreferred
	target.Audio.PreferredLanguages = []string{"eng"}
	info := infoHevcTwoAudioOneSubtitle()
	info.Audio[0].Language = "und"
	info.Audio[1].Language = ""
	d := Evaluate(info, target)
	assert.False(t, d.Needs, "no tagged track matched, keep all")
}

func TestEvaluateAudioKeepBest(t *testing.T) {
	target := hevcTarget()
	target.Audio.Mode = AudioKeepBest
	target.Audio.PreferredLanguages = []string{"eng"}

	// Two tracks: dropping one needs work.
	d := Evaluate(infoHevcTwoAudioOneSubtitle(), target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio mode keep_best would drop 1 track(s)")

	// Same file with 1 track: satisfied (acceptance case).
	info := infoHevcTwoAudioOneSubtitle()
	info.Audio = info.Audio[:1]
	d = Evaluate(info, target)
	assert.False(t, d.Needs)
}

func TestEvaluateAudioKeepBestPrefersHighestChannels(t *testing.T) {
	// Preferred language appears twice: the highest channel count wins.
	target := hevcTarget()
	target.Audio.Mode = AudioKeepBest
	target.Audio.PreferredLanguages = []string{"eng"}
	info := probe.MediaInfo{
		Container: "mkv",
		Video:     []probe.VideoStream{{Codec: "hevc"}},
		Audio: []probe.AudioStream{
			{Codec: "aac", Channels: 2, Language: "eng"},
			{Codec: "eac3", Channels: 6, Language: "eng"},
		},
	}
	assert.Equal(t, []int{1}, SelectAudio(info, target), "6 channels beats 2")
}

func TestEvaluateAudioKeepBestFallsBackToFirstTrack(t *testing.T) {
	target := hevcTarget()
	target.Audio.Mode = AudioKeepBest
	target.Audio.PreferredLanguages = []string{"fre"}
	assert.Equal(t, []int{0}, SelectAudio(infoH264TwoAudioOneSubtitle(), target))
}

func TestEvaluateAudioCompact(t *testing.T) {
	target := hevcTarget()
	target.Audio.Mode = AudioCompact
	target.Audio.CompactCodec = CompactCodecAAC

	// Two aac tracks: track selection alone needs work.
	info := infoHevcTwoAudioOneSubtitle()
	info.Audio[0].Codec = "aac"
	info.Audio[1].Codec = "aac"
	d := Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio mode compact would drop 1 track(s)")

	// Single eac3 track: must be transcoded to aac, needs work.
	info = infoHEVC()
	info.Audio[0].Codec = "eac3"
	d = Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio track is eac3, target compact codec is aac")

	// Single aac track: satisfied.
	info.Audio[0].Codec = "aac"
	d = Evaluate(info, target)
	assert.False(t, d.Needs)

	// Opus target: aac must be transcoded.
	target.Audio.CompactCodec = CompactCodecOpus
	d = Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio track is aac, target compact codec is opus")
}

func TestEvaluateSubtitleRemove(t *testing.T) {
	target := hevcTarget()
	target.Subtitles.Mode = SubsRemove

	// hevc with a subtitle track: needs work (acceptance case).
	d := Evaluate(infoHEVC(), target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "subtitle mode remove would drop 1 track(s)")

	// Same file without subtitles: satisfied.
	info := infoHEVC()
	info.Subtitle = nil
	d = Evaluate(info, target)
	assert.False(t, d.Needs)
}

func TestEvaluateSubtitleKeepPreferred(t *testing.T) {
	target := hevcTarget()
	target.Subtitles.Mode = SubsKeepPreferred
	target.Subtitles.PreferredLanguages = []string{"eng"}

	// Two subtitle tracks (eng + ger): dropping ger needs work.
	info := infoHEVC()
	info.Subtitle = []probe.SubtitleStream{
		{Codec: "subrip", Language: "eng"},
		{Codec: "subrip", Language: "ger"},
	}
	d := Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "subtitle mode keep_preferred would drop 1 track(s)")

	// No matching language: keep all, satisfied.
	info.Subtitle[0].Language = "fre"
	info.Subtitle[1].Language = "ger"
	d = Evaluate(info, target)
	assert.False(t, d.Needs)
}

func TestEvaluateContainer(t *testing.T) {
	target := hevcTarget()
	target.Container = ContainerMKV
	info := infoHEVC()
	info.Container = "mp4"

	d := Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "container is mp4, target is mkv")
}

func TestEvaluateWebmDoesNotSatisfyMKV(t *testing.T) {
	// A .webm file reports matroska internally; it must not satisfy a
	// mkv target or it would never be remuxed.
	target := hevcTarget()
	info := infoHEVC()
	info.Container = "webm"
	d := Evaluate(info, target)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "container is webm, target is mkv")
}

func TestEvaluateEverythingSatisfied(t *testing.T) {
	// A fully conforming file needs nothing.
	info := infoHEVC()
	d := Evaluate(info, hevcTarget())
	assert.False(t, d.Needs)
	assert.Empty(t, d.Reasons)
}

func TestEvaluateAllReasonsTogether(t *testing.T) {
	target := hevcTarget()
	target.Audio.Mode = AudioKeepBest
	target.Subtitles.Mode = SubsRemove
	d := Evaluate(infoH264TwoAudioOneSubtitle(), target)
	require.True(t, d.Needs)
	assert.Len(t, d.Reasons, 3)
}

func TestSelectAudioAllModes(t *testing.T) {
	info := infoHevcTwoAudioOneSubtitle()

	keepAll := Default()
	assert.Equal(t, []int{0, 1}, SelectAudio(info, keepAll))

	keepPref := Default()
	keepPref.Audio.Mode = AudioKeepPreferred
	keepPref.Audio.PreferredLanguages = []string{"ger"}
	assert.Equal(t, []int{1}, SelectAudio(info, keepPref))

	keepBest := Default()
	keepBest.Audio.Mode = AudioKeepBest
	keepBest.Audio.PreferredLanguages = []string{"eng"}
	assert.Equal(t, []int{0}, SelectAudio(info, keepBest))
}

func TestSelectSubtitlesAllModes(t *testing.T) {
	info := probe.MediaInfo{
		Subtitle: []probe.SubtitleStream{
			{Codec: "subrip", Language: "eng"},
			{Codec: "subrip", Language: "ger"},
		},
	}

	assert.Equal(t, []int{0, 1}, SelectSubtitles(info, Default()))

	keepPref := Default()
	keepPref.Subtitles.Mode = SubsKeepPreferred
	keepPref.Subtitles.PreferredLanguages = []string{"GER"}
	assert.Equal(t, []int{1}, SelectSubtitles(info, keepPref), "language matching is case-insensitive")

	remove := Default()
	remove.Subtitles.Mode = SubsRemove
	assert.Empty(t, SelectSubtitles(info, remove))
}

func TestSelectAudioEmptyFile(t *testing.T) {
	info := probe.MediaInfo{}
	assert.Empty(t, SelectAudio(info, Default()))
	assert.Empty(t, SelectSubtitles(info, Default()))
}
