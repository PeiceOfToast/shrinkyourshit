// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// h264Source is a probed source: h264 + 2 audio + 1 subtitle.
var h264Source = probe.MediaInfo{
	Container:       flow.ContainerMKV,
	DurationSeconds: 10,
	Video:           []probe.VideoStream{{Codec: flow.CodecH264, Width: 160}},
	Audio:           []probe.AudioStream{{Codec: "eac3", Channels: 6}, {Codec: "aac", Channels: 2}},
	Subtitle:        []probe.SubtitleStream{{Codec: "subrip", TextBased: true}},
}

func hevcOutput() probe.MediaInfo {
	return probe.MediaInfo{
		Container:       flow.ContainerMKV,
		DurationSeconds: 10,
		Video:           []probe.VideoStream{{Codec: flow.CodecHEVC}},
		Audio:           []probe.AudioStream{{Codec: "eac3", Channels: 6}, {Codec: "aac", Channels: 2}},
		Subtitle:        []probe.SubtitleStream{{Codec: "subrip", TextBased: true}},
	}
}

func specFor(t *testing.T, target flow.Target) Spec {
	t.Helper()
	require.NoError(t, target.Validate())
	return NewSpec("in.mkv", "out.mkv", h264Source, target)
}

func TestVerifyOutputHappy(t *testing.T) {
	target := flow.Default() // hevc, keep_all
	sp := specFor(t, target)
	require.NoError(t, VerifyOutput(sp, h264Source.DurationSeconds, hevcOutput(), 1234))
	require.NoError(t, VerifyOutput(sp, 0, hevcOutput(), 1234)) // unknown source duration: skip check
}

func TestVerifyOutputEmptyFile(t *testing.T) {
	sp := specFor(t, flow.Default())
	err := VerifyOutput(sp, 10, hevcOutput(), 0)
	require.ErrorContains(t, err, "empty")
	err = VerifyOutput(sp, 10, hevcOutput(), -1)
	require.ErrorContains(t, err, "empty")
}

func TestVerifyOutputWrongVideoCodec(t *testing.T) {
	sp := specFor(t, flow.Default())
	out := hevcOutput()
	out.Video[0].Codec = flow.CodecH264
	err := VerifyOutput(sp, 10, out, 1234)
	require.ErrorContains(t, err, "output video is h264, want hevc")
}

func TestVerifyOutputCopyKeepsSourceCodec(t *testing.T) {
	target := flow.Default()
	target.Video.Codec = flow.CodecCopy
	target.Subtitles.Mode = flow.SubsRemove // copy + track change still verifies
	sp := specFor(t, target)
	// Video codec stays h264 (copy), subtitles removed.
	out := hevcOutput()
	out.Video[0].Codec = flow.CodecH264
	out.Subtitle = nil
	require.NoError(t, VerifyOutput(sp, 10, out, 1234))

	// A different codec than the source fails.
	broken := out
	broken.Video[0].Codec = flow.CodecHEVC
	err := VerifyOutput(sp, 10, broken, 1234)
	require.ErrorContains(t, err, "output video is hevc, want h264")
}

func TestVerifyOutputTrackCounts(t *testing.T) {
	sp := specFor(t, flow.Default()) // keeps both audio tracks and the subtitle

	droppedAudio := hevcOutput()
	droppedAudio.Audio = droppedAudio.Audio[:1]
	err := VerifyOutput(sp, 10, droppedAudio, 1234)
	require.ErrorContains(t, err, "1 audio track(s), want 2")

	droppedSubs := hevcOutput()
	droppedSubs.Subtitle = nil
	err = VerifyOutput(sp, 10, droppedSubs, 1234)
	require.ErrorContains(t, err, "0 subtitle track(s), want 1")

	// A selection that keeps one track verifies with one.
	target := flow.Default()
	target.Audio.Mode = flow.AudioKeepBest
	spBest := specFor(t, target)
	best := hevcOutput()
	best.Audio = []probe.AudioStream{{Codec: "aac", Channels: 2}}
	require.NoError(t, VerifyOutput(spBest, 10, best, 1234))
}

func TestVerifyOutputNoVideoStream(t *testing.T) {
	sp := specFor(t, flow.Default())
	out := hevcOutput()
	out.Video = nil
	err := VerifyOutput(sp, 10, out, 1234)
	require.ErrorContains(t, err, "no video stream")
}

func TestVerifyOutputDuration(t *testing.T) {
	sp := specFor(t, flow.Default())

	// 1% tolerance: 10s source allows 9.9s..10.1s, plus the 2s floor.
	out := hevcOutput()
	out.DurationSeconds = 9.0
	require.NoError(t, VerifyOutput(sp, 10, out, 1234)) // within the 2s floor

	out.DurationSeconds = 7.5
	require.ErrorContains(t, VerifyOutput(sp, 10, out, 1234), "too far")

	// Long sources use the 1% rule (1% of 300s = 3s > the 2s floor).
	out.DurationSeconds = 298
	require.NoError(t, VerifyOutput(sp, 300, out, 1234))
	out.DurationSeconds = 296
	require.ErrorContains(t, VerifyOutput(sp, 300, out, 1234), "too far")

	// Unknown output duration fails when the source duration is known.
	out.DurationSeconds = 0
	require.ErrorContains(t, VerifyOutput(sp, 10, out, 1234), "unknown")
}
