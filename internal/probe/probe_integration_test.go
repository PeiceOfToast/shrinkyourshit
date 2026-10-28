// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package probe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
)

func TestProbeRealFiles(t *testing.T) {
	ffmpeg, ffprobe := testutil.RequireFFmpeg(t)
	fixtures := testutil.GenerateFixtures(t, ffmpeg, t.TempDir())

	// A: h264 + eng/ger audio + srt subtitle in mkv.
	info, err := Probe(context.Background(), ffprobe, fixtures.H264TwoAudioSubtitleMKV)
	require.NoError(t, err)
	assert.Equal(t, "mkv", info.Container)
	require.Len(t, info.Video, 1)
	assert.Equal(t, "h264", info.Video[0].Codec)
	assert.Equal(t, 160, info.Video[0].Width)
	assert.Equal(t, 120, info.Video[0].Height)
	assert.Greater(t, info.Video[0].FPS, 0.0)
	assert.False(t, info.Video[0].HDR)
	require.Len(t, info.Audio, 2)
	assert.Equal(t, "eng", info.Audio[0].Language)
	assert.Equal(t, "ger", info.Audio[1].Language)
	assert.Equal(t, "aac", info.Audio[0].Codec)
	assert.Equal(t, 2, info.Audio[0].Channels)
	assert.Greater(t, info.SizeBytes, int64(0))
	assert.Greater(t, info.DurationSeconds, 0.0)
	require.Len(t, info.Subtitle, 1)
	assert.Equal(t, "subrip", info.Subtitle[0].Codec)
	assert.Equal(t, "eng", info.Subtitle[0].Language)
	assert.True(t, info.Subtitle[0].TextBased)

	// B: hevc in mkv.
	info, err = Probe(context.Background(), ffprobe, fixtures.HEVCAudioSubtitleMKV)
	require.NoError(t, err)
	assert.Equal(t, "mkv", info.Container)
	require.Len(t, info.Video, 1)
	assert.Equal(t, "hevc", info.Video[0].Codec)
	require.Len(t, info.Audio, 1)

	// C: h264 in mp4.
	info, err = Probe(context.Background(), ffprobe, fixtures.H264AudioMP4)
	require.NoError(t, err)
	assert.Equal(t, "mp4", info.Container)
	require.Len(t, info.Video, 1)
	assert.Equal(t, "h264", info.Video[0].Codec)
	require.Len(t, info.Audio, 1)
	assert.Empty(t, info.Subtitle)

	// D: HDR10 signaling is detected.
	info, err = Probe(context.Background(), ffprobe, fixtures.HDRH264MKV)
	require.NoError(t, err)
	require.Len(t, info.Video, 1)
	assert.True(t, info.Video[0].HDR, "color_trc smpte2084 must mark the stream HDR")
}

func TestProbeMissingFile(t *testing.T) {
	_, ffprobe := testutil.RequireFFmpeg(t)
	_, err := Probe(context.Background(), ffprobe, filepath.Join(t.TempDir(), "nope.mkv"))
	assert.Error(t, err)
}

func TestFingerprintRealMedia(t *testing.T) {
	ffmpeg, _ := testutil.RequireFFmpeg(t)
	fixtures := testutil.GenerateFixtures(t, ffmpeg, t.TempDir())

	first, err := Fingerprint(fixtures.H264AudioMP4)
	require.NoError(t, err)
	second, err := Fingerprint(fixtures.H264AudioMP4)
	require.NoError(t, err)
	assert.Equal(t, first, second, "fingerprint is stable across runs")
	assert.NotEqual(t, first, mustFingerprint(t, fixtures.HEVCAudioSubtitleMKV))
}

func mustFingerprint(t *testing.T, path string) string {
	t.Helper()
	fp, err := Fingerprint(path)
	require.NoError(t, err)
	return fp
}
