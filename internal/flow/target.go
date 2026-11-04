// SPDX-License-Identifier: AGPL-3.0-only

// Package flow holds the Target type, Evaluate (the skip-if-satisfied rule)
// and the easy-mode compiler. The flow graph engine itself is post-MVP.
package flow

import (
	"encoding/json"
	"fmt"
)

// Codec values for Target.Video.Codec.
const (
	CodecHEVC = "hevc"
	CodecAV1  = "av1"
	CodecH264 = "h264"
	CodecCopy = "copy"
)

// Container values for Target.Container.
const (
	ContainerMKV = "mkv"
	ContainerMP4 = "mp4"
)

// Audio modes for Target.Audio.Mode.
const (
	AudioKeepAll       = "keep_all"
	AudioKeepPreferred = "keep_preferred"
	AudioKeepBest      = "keep_best"
	AudioCompact       = "compact"
)

// Subtitle modes for Target.Subtitles.Mode.
const (
	SubsKeepAll       = "keep_all"
	SubsKeepPreferred = "keep_preferred"
	SubsRemove        = "remove"
)

// Audio compact codecs for Target.Audio.CompactCodec.
const (
	CompactCodecAAC  = "aac"
	CompactCodecOpus = "opus"
)

// QualityMin and QualityMax bound Target.Video.Quality (1 to 5; 1 saves the
// most space, 5 the least).
const (
	QualityMin = 1
	QualityMax = 5
)

// Rate modes for RateTarget.Mode. A nil Target.Video.Rate means quality, the
// existing scalar meaning (Target.Video.Quality).
const (
	RateModeQuality = "quality"
	RateModeCRF     = "crf"
	RateModeBitrate = "bitrate"
)

// Speed presets for VideoTarget.Preset, fastest to slowest. The empty string
// means the balanced default (encoders map it to their medium preset).
var presetValues = []string{"veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"}

// Presets lists the valid VideoTarget.Preset values, fastest to slowest.
func Presets() []string { return append([]string(nil), presetValues...) }

// Scale heights for ScaleTarget.Height, largest first.
var scaleHeights = []int{2160, 1440, 1080, 720, 480}

// ScaleHeights lists the valid ScaleTarget.Height values, largest first.
func ScaleHeights() []int { return append([]int(nil), scaleHeights...) }

// CRF bounds for RateTarget.CRF. Zero is not a valid CRF in crf mode.
const (
	CRFMin = 0.0
	CRFMax = 51.0
)

// Target is the desired end state for files in a library. Its JSON shape is
// fixed by ARCHITECTURE.md; changing it is a schema migration.
type Target struct {
	Video     VideoTarget    `json:"video"`
	Audio     AudioTarget    `json:"audio"`
	Subtitles SubtitleTarget `json:"subtitles"`
	Container string         `json:"container"`
}

// VideoTarget describes the video codec and quality.
type VideoTarget struct {
	// Codec is hevc, av1, h264 or copy. "copy" never re-encodes video.
	Codec string `json:"codec"`
	// Quality is 1 to 5; 1 saves the most space, 5 the least.
	Quality int `json:"quality"`
	// ReencodeIfSameCodec overrides the skip-when-already-target rule
	// (DECISIONS.md B2). Off by default.
	ReencodeIfSameCodec bool `json:"reencode_if_same_codec"`
	// Scale resizes the video. Nil means the resolution is left alone.
	// Task 54: stored and resolved; the encoders apply it in task 55.
	Scale *ScaleTarget `json:"scale,omitempty"`
	// Rate is the rate control. Nil means quality (Quality above).
	Rate *RateTarget `json:"rate,omitempty"`
	// Preset is the encoder speed preset: veryfast..veryslow. Empty means
	// balanced.
	Preset string `json:"preset,omitempty"`
}

// ScaleTarget resizes the video by its height and keeps the aspect ratio.
type ScaleTarget struct {
	// Height is 2160, 1440, 1080, 720 or 480.
	Height int `json:"height"`
	// OnlyIfLarger scales only when the source is taller than Height. The
	// scale step defaults it to true; a stored Target always writes it.
	OnlyIfLarger bool `json:"only_if_larger"`
	// AllowUpscale permits enlarging a source shorter than Height. Default
	// false: a video is never enlarged.
	AllowUpscale bool `json:"allow_upscale"`
}

// RateTarget is one of three rate control modes. Crf and bitrate are
// mutually exclusive; MaxKBPS caps either one (zero means uncapped).
type RateTarget struct {
	// Mode is quality (reserved: a nil Rate is quality), crf or bitrate.
	Mode string `json:"mode"`
	// CRF is 0 to 51 and must be above zero in crf mode.
	CRF float64 `json:"crf"`
	// BitrateKBPS is the target video bitrate, required in bitrate mode.
	BitrateKBPS int `json:"bitrate_kbps"`
	// MaxKBPS caps the bitrate. In bitrate mode it must not be below
	// BitrateKBPS.
	MaxKBPS int `json:"max_kbps"`
}

// rateRuleError checks the rate fields of a transcode step or a RateTarget
// against their mode. It returns the offending field name and a plain
// message, or empty strings when the combination is valid. An empty mode
// means quality. The rules are the cross field rules of spec 5.2; a value
// set outside its mode is an error, never silently ignored.
func rateRuleError(mode string, crf float64, bitrateKBPS, maxKBPS int) (field, msg string) {
	switch mode {
	case "", RateModeQuality:
		switch {
		case crf != 0:
			return "crf", "crf only applies when rate_mode is crf"
		case bitrateKBPS != 0:
			return "bitrate_kbps", "bitrate_kbps only applies when rate_mode is bitrate"
		case maxKBPS != 0:
			return "max_kbps", "max_kbps only applies when rate_mode is crf or bitrate"
		}
	case RateModeCRF:
		switch {
		case crf <= CRFMin || crf > CRFMax:
			return "crf", "rate_mode crf needs a crf above 0 and up to 51"
		case bitrateKBPS != 0:
			return "bitrate_kbps", "bitrate_kbps only applies when rate_mode is bitrate"
		}
	case RateModeBitrate:
		switch {
		case bitrateKBPS <= 0:
			return "bitrate_kbps", "rate_mode bitrate needs a bitrate above 0 kbps"
		case crf != 0:
			return "crf", "crf only applies when rate_mode is crf"
		case maxKBPS != 0 && maxKBPS < bitrateKBPS:
			return "max_kbps", "max_kbps must not be below bitrate_kbps"
		}
	default:
		return "rate_mode", fmt.Sprintf("want quality, crf or bitrate, got %q", mode)
	}
	return "", ""
}

// validScaleHeight reports whether h is one of the scale heights.
func validScaleHeight(h int) bool {
	for _, v := range scaleHeights {
		if v == h {
			return true
		}
	}
	return false
}

// validPreset reports whether p is "" or one of the speed presets.
func validPreset(p string) bool {
	if p == "" {
		return true
	}
	for _, v := range presetValues {
		if v == p {
			return true
		}
	}
	return false
}

// AudioTarget describes how audio tracks are selected and compacted.
type AudioTarget struct {
	// Mode is keep_all, keep_preferred, keep_best or compact.
	Mode string `json:"mode"`
	// PreferredLanguages are ISO 639 language tags matched
	// case-insensitively; used by keep_preferred, keep_best and compact.
	PreferredLanguages []string `json:"preferred_languages"`
	// CompactCodec is aac or opus; only used by mode "compact".
	CompactCodec string `json:"compact_codec"`
}

// SubtitleTarget describes how subtitle tracks are handled.
type SubtitleTarget struct {
	// Mode is keep_all, keep_preferred or remove.
	Mode string `json:"mode"`
	// PreferredLanguages are ISO 639 language tags matched
	// case-insensitively; used by keep_preferred.
	PreferredLanguages []string `json:"preferred_languages"`
	// ExtractRemoved writes removed text subtitles as .srt beside the
	// file (engine support in task 04; UI exposes it post-MVP).
	ExtractRemoved bool `json:"extract_removed"`
}

// Default returns the default Target: HEVC in MKV, quality 3, keep all
// audio and subtitles (DECISIONS.md B1 and the ARCHITECTURE.md example).
func Default() Target {
	return Target{
		Video:     VideoTarget{Codec: CodecHEVC, Quality: 3},
		Audio:     AudioTarget{Mode: AudioKeepAll, PreferredLanguages: []string{"eng"}, CompactCodec: CompactCodecAAC},
		Subtitles: SubtitleTarget{Mode: SubsKeepAll, PreferredLanguages: []string{"eng"}},
		Container: ContainerMKV,
	}
}

// Validate reports Target values that are not usable.
func (t Target) Validate() error {
	switch t.Video.Codec {
	case CodecHEVC, CodecAV1, CodecH264, CodecCopy:
	default:
		return fmt.Errorf("invalid video codec %q: want hevc, av1, h264 or copy", t.Video.Codec)
	}
	if t.Video.Quality < QualityMin || t.Video.Quality > QualityMax {
		return fmt.Errorf("invalid video quality %d: want %d to %d", t.Video.Quality, QualityMin, QualityMax)
	}
	if t.Video.Scale != nil && !validScaleHeight(t.Video.Scale.Height) {
		return fmt.Errorf("invalid scale height %d: want 2160, 1440, 1080, 720 or 480", t.Video.Scale.Height)
	}
	if !validPreset(t.Video.Preset) {
		return fmt.Errorf("invalid preset %q: want veryfast, faster, fast, medium, slow, slower or veryslow", t.Video.Preset)
	}
	if t.Video.Rate != nil {
		r := t.Video.Rate
		if field, msg := rateRuleError(r.Mode, r.CRF, r.BitrateKBPS, r.MaxKBPS); field != "" {
			return fmt.Errorf("invalid rate %s: %s", field, msg)
		}
	}
	switch t.Audio.Mode {
	case AudioKeepAll, AudioKeepPreferred, AudioKeepBest, AudioCompact:
	default:
		return fmt.Errorf("invalid audio mode %q: want keep_all, keep_preferred, keep_best or compact", t.Audio.Mode)
	}
	if t.Audio.Mode == AudioCompact {
		switch t.Audio.CompactCodec {
		case CompactCodecAAC, CompactCodecOpus:
		default:
			return fmt.Errorf("invalid compact codec %q: want aac or opus", t.Audio.CompactCodec)
		}
	}
	switch t.Subtitles.Mode {
	case SubsKeepAll, SubsKeepPreferred, SubsRemove:
	default:
		return fmt.Errorf("invalid subtitle mode %q: want keep_all, keep_preferred or remove", t.Subtitles.Mode)
	}
	switch t.Container {
	case ContainerMKV, ContainerMP4:
	default:
		return fmt.Errorf("invalid container %q: want mkv or mp4", t.Container)
	}
	return nil
}

// ParseTarget unmarshals a Target from JSON and validates it. It is the
// single entry point for Targets read from the database or API.
func ParseTarget(data []byte) (Target, error) {
	var t Target
	if err := json.Unmarshal(data, &t); err != nil {
		return Target{}, fmt.Errorf("flow: invalid target json: %w", err)
	}
	if err := t.Validate(); err != nil {
		return Target{}, err
	}
	return t, nil
}
