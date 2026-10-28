// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ffprobeOutput is the raw JSON shape of
//
//	ffprobe -v quiet -print_format json -show_format -show_streams <file>
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeFormat struct {
	Filename       string `json:"filename"`
	FormatName     string `json:"format_name"`
	DurationString string `json:"duration"`
	SizeString     string `json:"size"`
}

type ffprobeStream struct {
	Index        int    `json:"index"`
	CodecName    string `json:"codec_name"`
	CodecType    string `json:"codec_type"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	PixFmt       string `json:"pix_fmt"`
	AvgFrameRate string `json:"avg_frame_rate"`
	RFrameRate   string `json:"r_frame_rate"`
	BitRate      string `json:"bit_rate"`
	// Audio-only field; some containers also put it in the tags.
	Channels int `json:"channels"`
	// Tags can contain non-string values in some containers, so they are
	// kept as any and converted with tagString.
	Disposition ffprobeDisposition `json:"disposition"`
	Tags        map[string]any     `json:"tags"`
	// Color information sits on the video stream itself.
	ColorTransfer string `json:"color_transfer"`
}

type ffprobeDisposition struct {
	Default     int `json:"default"`
	Forced      int `json:"forced"`
	AttachedPic int `json:"attached_pic"`
}

// Parse converts raw ffprobe JSON output into MediaInfo. It is exported so
// unit tests can run without ffprobe installed.
func Parse(data []byte) (MediaInfo, error) {
	var raw ffprobeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return MediaInfo{}, fmt.Errorf("probe: invalid ffprobe json: %w", err)
	}

	info := MediaInfo{
		Container:       normalizeContainer(raw.Format.FormatName, raw.Format.Filename),
		DurationSeconds: parseNumber(raw.Format.DurationString),
		SizeBytes:       int64(parseNumber(raw.Format.SizeString)),
	}

	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic == 1 {
				continue // cover art, not a real video stream
			}
			info.Video = append(info.Video, VideoStream{
				Codec:   s.CodecName,
				Width:   s.Width,
				Height:  s.Height,
				Bitrate: int64(parseNumber(s.BitRate)),
				FPS:     parseFrameRate(s.AvgFrameRate, s.RFrameRate),
				PixFmt:  s.PixFmt,
				HDR:     hdrTransfers[s.ColorTransfer],
			})
		case "audio":
			channels := s.Channels
			if channels == 0 {
				channels = tagInt(s.Tags, "channels")
			}
			info.Audio = append(info.Audio, AudioStream{
				Codec:    s.CodecName,
				Channels: channels,
				Language: tagString(s.Tags, "language"),
				Bitrate:  int64(parseNumber(s.BitRate)),
				Title:    tagString(s.Tags, "title"),
				Default:  s.Disposition.Default == 1,
			})
		case "subtitle":
			info.Subtitle = append(info.Subtitle, SubtitleStream{
				Codec:     s.CodecName,
				Language:  tagString(s.Tags, "language"),
				Forced:    s.Disposition.Forced == 1,
				Default:   s.Disposition.Default == 1,
				TextBased: !bitmapSubtitleCodecs[s.CodecName],
			})
		}
	}
	return info, nil
}

// Probe runs ffprobe against path and parses its JSON output. An empty
// ffprobePath falls back to "ffprobe" from PATH.
func Probe(ctx context.Context, ffprobePath, path string) (MediaInfo, error) {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return MediaInfo{}, fmt.Errorf("probe: ffprobe %s: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	return Parse(out)
}

// parseNumber parses an ffprobe numeric field that arrives as a string.
func parseNumber(s string) float64 {
	if s == "" || s == "N/A" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseFrameRate parses "num/denom" rates like "25/1"; "0/0" means unknown.
func parseFrameRate(rates ...string) float64 {
	for _, r := range rates {
		if r == "" || r == "0/0" {
			continue
		}
		num, denom, ok := strings.Cut(r, "/")
		if !ok {
			if v := parseNumber(num); v > 0 {
				return v
			}
			continue
		}
		d := parseNumber(denom)
		if d == 0 {
			continue
		}
		if v := parseNumber(num) / d; v > 0 {
			return v
		}
	}
	return 0
}

// tagString returns a tag value as a string. ffprobe mostly emits strings,
// but some containers emit numbers.
func tagString(tags map[string]any, key string) string {
	switch v := tags[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// tagInt returns a tag value as an int.
func tagInt(tags map[string]any, key string) int {
	if v, ok := tags[key].(float64); ok {
		return int(v)
	}
	return 0
}
