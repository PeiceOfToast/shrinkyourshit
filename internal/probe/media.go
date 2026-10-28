// SPDX-License-Identifier: AGPL-3.0-only

// Package probe wraps ffprobe and defines the MediaInfo types shared by the
// flow evaluation and the encoders. ffprobe is an external binary called as
// a subprocess; this package never reimplements it.
package probe

import "strings"

// MediaInfo is the probed state of one media file.
type MediaInfo struct {
	// Container is the normalized container kind: "mkv", "webm", "mp4" or,
	// for anything unrecognized, the raw ffprobe format_name. Target
	// containers are "mkv" and "mp4", so normalized values compare
	// directly against a Target.
	Container string `json:"container"`
	// DurationSeconds is 0 when unknown.
	DurationSeconds float64 `json:"duration_seconds"`
	// SizeBytes is the file size in bytes, 0 when unknown.
	SizeBytes int64 `json:"size_bytes"`
	// Video holds real video streams in stream order. Attached cover
	// pictures are excluded.
	Video []VideoStream `json:"video"`
	// Audio holds audio streams in stream order.
	Audio []AudioStream `json:"audio"`
	// Subtitle holds subtitle streams in stream order.
	Subtitle []SubtitleStream `json:"subtitle"`
}

// VideoStream describes one video stream.
type VideoStream struct {
	// Codec is the ffprobe codec_name, for example h264, hevc, av1.
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Bitrate is bits per second, 0 when unknown.
	Bitrate int64 `json:"bitrate"`
	// FPS is the average frame rate, 0 when unknown.
	FPS float64 `json:"fps"`
	// PixFmt is the pixel format, for example yuv420p or yuv420p10le.
	PixFmt string `json:"pix_fmt"`
	// HDR is true for HDR10 (color transfer smpte2084) and HLG
	// (arib-std-b67).
	HDR bool `json:"hdr"`
}

// AudioStream describes one audio stream.
type AudioStream struct {
	// Codec is the ffprobe codec_name, for example aac, eac3, flac.
	Codec string `json:"codec"`
	// Channels is the channel count, for example 2 for stereo, 6 for 5.1.
	Channels int `json:"channels"`
	// Language is the stream language tag, empty when unset. The raw
	// tag value is kept ("und" means undefined).
	Language string `json:"language"`
	// Bitrate is bits per second, 0 when unknown.
	Bitrate int64 `json:"bitrate"`
	// Title is the stream title tag, empty when unset.
	Title string `json:"title"`
	// Default is the disposition default flag.
	Default bool `json:"default"`
}

// SubtitleStream describes one subtitle stream.
type SubtitleStream struct {
	// Codec is the ffprobe codec_name, for example subrip, ass,
	// hdmv_pgs_subtitle.
	Codec string `json:"codec"`
	// Language is the stream language tag, empty when unset.
	Language string `json:"language"`
	// Forced is the disposition forced flag.
	Forced bool `json:"forced"`
	// Default is the disposition default flag.
	Default bool `json:"default"`
	// TextBased is true when the subtitle can be converted to SRT
	// (SubRip, ASS, ...). Bitmap subtitles (PGS, DVD, DVB) are false.
	TextBased bool `json:"text_based"`
}

// hdrTransfers are the color transfers that mark a stream as HDR.
var hdrTransfers = map[string]bool{
	"smpte2084":    true, // HDR10
	"arib-std-b67": true, // HLG
}

// bitmapSubtitleCodecs cannot be converted to text subtitles.
var bitmapSubtitleCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"dvd_subtitle":      true,
	"dvb_subtitle":      true,
	"arib_caption":      true,
}

// normalizeContainer maps an ffprobe format_name plus the source file
// extension to a container kind. matroska and webm share a format_name, so
// the extension decides: a .webm file is "webm" and must not satisfy a
// "mkv" target. QuickTime files (.mov) report the mp4 family and are
// treated as mp4; remuxing handles them.
func normalizeContainer(formatName, filename string) string {
	switch {
	case strings.Contains(formatName, "matroska"):
		if strings.HasSuffix(strings.ToLower(filename), ".webm") {
			return "webm"
		}
		return "mkv"
	case strings.Contains(formatName, "mp4"):
		return "mp4"
	case formatName == "":
		return ""
	default:
		return formatName
	}
}
