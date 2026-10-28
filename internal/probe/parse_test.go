// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkvJSON mimics real ffprobe output for an mkv with h264 video, a cover
// art attachment, two audio languages and one text subtitle.
const mkvJSON = `{
  "streams": [
    {"index": 0, "codec_name": "h264", "codec_type": "video",
     "width": 1920, "height": 1080, "pix_fmt": "yuv420p",
     "avg_frame_rate": "24000/1001", "r_frame_rate": "24000/1001",
     "bit_rate": "4500000", "disposition": {"default": 1, "forced": 0, "attached_pic": 0},
     "tags": {"language": "und"}},
    {"index": 1, "codec_name": "mjpeg", "codec_type": "video", "width": 640, "height": 360,
     "disposition": {"default": 0, "forced": 0, "attached_pic": 1},
     "tags": {}},
    {"index": 2, "codec_name": "aac", "codec_type": "audio", "channels": 2,
     "bit_rate": "192000",
     "disposition": {"default": 1, "forced": 0, "attached_pic": 0},
     "tags": {"language": "eng", "title": "English 2.0"}},
    {"index": 3, "codec_name": "eac3", "codec_type": "audio", "channels": 6,
     "bit_rate": "640000",
     "disposition": {"default": 0, "forced": 0, "attached_pic": 0},
     "tags": {"language": "ger", "title": "Deutsch 5.1"}},
    {"index": 4, "codec_name": "subrip", "codec_type": "subtitle",
     "disposition": {"default": 1, "forced": 0, "attached_pic": 0},
     "tags": {"language": "eng"}}
  ],
  "format": {
    "filename": "/media/movie.mkv",
    "format_name": "matroska,webm",
    "duration": "7200.046000",
    "size": "4632603385"
  }
}`

func TestParseMKV(t *testing.T) {
	info, err := Parse([]byte(mkvJSON))
	require.NoError(t, err)

	assert.Equal(t, "mkv", info.Container)
	assert.InDelta(t, 7200.046, info.DurationSeconds, 0.001)
	assert.Equal(t, int64(4632603385), info.SizeBytes)

	// Cover art is excluded, the real stream is kept with parsed fields.
	require.Len(t, info.Video, 1)
	v := info.Video[0]
	assert.Equal(t, "h264", v.Codec)
	assert.Equal(t, 1920, v.Width)
	assert.Equal(t, 1080, v.Height)
	assert.Equal(t, int64(4500000), v.Bitrate)
	assert.InDelta(t, 24000.0/1001.0, v.FPS, 0.0001)
	assert.Equal(t, "yuv420p", v.PixFmt)
	assert.False(t, v.HDR)

	require.Len(t, info.Audio, 2)
	assert.Equal(t, "aac", info.Audio[0].Codec)
	assert.Equal(t, 2, info.Audio[0].Channels)
	assert.Equal(t, "eng", info.Audio[0].Language)
	assert.Equal(t, "English 2.0", info.Audio[0].Title)
	assert.True(t, info.Audio[0].Default)
	assert.Equal(t, int64(192000), info.Audio[0].Bitrate)
	assert.Equal(t, "eac3", info.Audio[1].Codec)
	assert.Equal(t, 6, info.Audio[1].Channels)
	assert.Equal(t, "ger", info.Audio[1].Language)
	assert.False(t, info.Audio[1].Default)

	require.Len(t, info.Subtitle, 1)
	s := info.Subtitle[0]
	assert.Equal(t, "subrip", s.Codec)
	assert.Equal(t, "eng", s.Language)
	assert.True(t, s.TextBased)
	assert.True(t, s.Default)
	assert.False(t, s.Forced)
}

func TestParseHDR(t *testing.T) {
	for _, tc := range []struct {
		transfer string
		want     bool
	}{
		{"smpte2084", true},    // HDR10
		{"arib-std-b67", true}, // HLG
		{"bt709", false},
		{"", false},
	} {
		t.Run(tc.transfer, func(t *testing.T) {
			data := strings.ReplaceAll(mkvJSON, `"pix_fmt": "yuv420p",`, `"pix_fmt": "yuv420p", "color_transfer": "`+tc.transfer+`",`)
			info, err := Parse([]byte(data))
			require.NoError(t, err)
			require.Len(t, info.Video, 1)
			assert.Equal(t, tc.want, info.Video[0].HDR)
		})
	}
}

func TestParseBitmapSubtitle(t *testing.T) {
	data := strings.ReplaceAll(mkvJSON, `"codec_name": "subrip"`, `"codec_name": "hdmv_pgs_subtitle"`)
	info, err := Parse([]byte(data))
	require.NoError(t, err)
	require.Len(t, info.Subtitle, 1)
	assert.False(t, info.Subtitle[0].TextBased, "PGS is bitmap, not convertible to srt")
}

func TestNormalizeContainer(t *testing.T) {
	for _, tc := range []struct {
		formatName string
		filename   string
		want       string
	}{
		{"matroska,webm", "/media/movie.mkv", "mkv"},
		{"matroska,webm", "/media/movie.webm", "webm"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "/media/movie.mp4", "mp4"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "/media/movie.mov", "mp4"},
		{"avi", "/media/movie.avi", "avi"},
		{"", "", ""},
	} {
		assert.Equal(t, tc.want, normalizeContainer(tc.formatName, tc.filename), tc.formatName)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse([]byte("not json"))
	assert.Error(t, err)
}

func TestParseFrameRate(t *testing.T) {
	assert.InDelta(t, 25.0, parseFrameRate("25/1", ""), 0.0001)
	assert.InDelta(t, 29.97, parseFrameRate("30000/1001"), 0.01)
	assert.Equal(t, 0.0, parseFrameRate("0/0", "0/0"))
	assert.InDelta(t, 30.0, parseFrameRate("0/0", "30/1"), 0.0001)
}
