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
