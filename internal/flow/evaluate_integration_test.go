// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package flow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
)

// probeFixture ffprobes a generated fixture. The test skips before any
// ffmpeg work when ffprobe is missing.
func probeFixture(t *testing.T, path string) probe.MediaInfo {
	t.Helper()
	_, ffprobe := testutil.RequireFFmpeg(t)
	info, err := probe.Probe(context.Background(), ffprobe, path)
	require.NoError(t, err)
	return info
}

func TestEvaluateAgainstRealFiles(t *testing.T) {
	ffmpeg, _ := testutil.RequireFFmpeg(t)
	fixtures := testutil.GenerateFixtures(t, ffmpeg, t.TempDir())

	// h264+aac+2 audio langs+1 srt in mkv vs hevc target: needs work with
	// the exact reason the UI shows.
	h264Info := probeFixture(t, fixtures.H264TwoAudioSubtitleMKV)
	d := Evaluate(h264Info, Default())
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "video is h264, target is hevc")

	// hevc+aac+1 srt in mkv vs hevc target: fully satisfied, never
	// re-processed.
	hevcInfo := probeFixture(t, fixtures.HEVCAudioSubtitleMKV)
	d = Evaluate(hevcInfo, Default())
	assert.False(t, d.Needs, "got reasons: %v", d.Reasons)

	// hevc with subtitles.mode=remove and a subtitle track: needs work.
	removeSubs := Default()
	removeSubs.Subtitles.Mode = SubsRemove
	d = Evaluate(hevcInfo, removeSubs)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "subtitle mode remove would drop 1 track(s)")

	// hevc with audio.mode=keep_best and 2 audio tracks: needs work.
	keepBest := Default()
	keepBest.Audio.Mode = AudioKeepBest
	keepBest.Audio.PreferredLanguages = []string{"eng"}
	d = Evaluate(h264Info, keepBest)
	require.True(t, d.Needs)
	assert.Contains(t, d.Reasons, "audio mode keep_best would drop 1 track(s)")

	// The same file with 1 track is satisfied.
	d = Evaluate(hevcInfo, keepBest)
	assert.False(t, d.Needs, "got reasons: %v", d.Reasons)

	// mp4 vs container=mkv needs work. Video codec "copy" isolates the
	// container rule.
	remuxOnly := Default()
	remuxOnly.Video.Codec = CodecCopy
	mp4Info := probeFixture(t, fixtures.H264AudioMP4)
	d = Evaluate(mp4Info, remuxOnly)
	require.True(t, d.Needs)
	assert.Equal(t, []string{"container is mp4, target is mkv"}, d.Reasons)
}
