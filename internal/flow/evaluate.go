// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"fmt"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// Decision is the result of Evaluate: whether a file needs work and why.
// Reasons are shown in the UI, for example "video is h264, target is hevc".
type Decision struct {
	// Needs is true when the file does not yet satisfy the Target.
	Needs bool
	// Reasons lists every rule that is not met.
	Reasons []string
}

// Evaluate applies the satisfied rule (ARCHITECTURE.md): a file needs work
// when any of video, audio, subtitles or container does not already match
// the Target. A file that already matches is never re-processed.
func Evaluate(info probe.MediaInfo, t Target) Decision {
	d := Decision{}
	if r := videoReasons(info, t); len(r) > 0 {
		d.Needs = true
		d.Reasons = append(d.Reasons, r...)
	}
	if r := audioReasons(info, t); len(r) > 0 {
		d.Needs = true
		d.Reasons = append(d.Reasons, r...)
	}
	if r := subtitleReasons(info, t); len(r) > 0 {
		d.Needs = true
		d.Reasons = append(d.Reasons, r...)
	}
	if info.Container != t.Container {
		d.Needs = true
		d.Reasons = append(d.Reasons, fmt.Sprintf("container is %s, target is %s", or(info.Container, "unknown"), t.Container))
	}
	return d
}

// videoReasons checks the video clause of the satisfied rule.
func videoReasons(info probe.MediaInfo, t Target) []string {
	if t.Video.Codec == CodecCopy {
		return nil // never re-encode video
	}
	var reasons []string
	for _, v := range info.Video {
		alreadyTarget := v.Codec == t.Video.Codec
		if alreadyTarget && !t.Video.ReencodeIfSameCodec {
			continue // satisfied (DECISIONS.md B2)
		}
		r := fmt.Sprintf("video is %s, target is %s", or(v.Codec, "unknown"), t.Video.Codec)
		if alreadyTarget {
			r += " (reencode_if_same_codec)"
		}
		reasons = append(reasons, r)
	}
	if len(info.Video) == 0 {
		// No video stream: nothing to re-encode, the video clause passes.
		return nil
	}
	return dedupe(reasons)
}

// audioReasons checks the audio clause: the file's audio tracks must
// already match what the audio mode would produce.
func audioReasons(info probe.MediaInfo, t Target) []string {
	keep := SelectAudio(info, t)
	switch t.Audio.Mode {
	case AudioKeepAll:
		return nil
	case AudioKeepPreferred:
		if len(keep) == len(info.Audio) {
			return nil // selection keeps everything
		}
		return []string{fmt.Sprintf("audio mode keep_preferred would drop %d track(s)", len(info.Audio)-len(keep))}
	case AudioKeepBest, AudioCompact:
		if len(keep) != 1 {
			// keep_best always selects exactly one track, so this only
			// happens for files with no audio at all.
			return nil
		}
		if len(info.Audio) != 1 {
			return []string{fmt.Sprintf("audio mode %s would drop %d track(s)", t.Audio.Mode, len(info.Audio)-1)}
		}
		if t.Audio.Mode == AudioCompact && info.Audio[keep[0]].Codec != t.Audio.CompactCodec {
			return []string{fmt.Sprintf("audio track is %s, target compact codec is %s",
				or(info.Audio[keep[0]].Codec, "unknown"), t.Audio.CompactCodec)}
		}
		return nil
	default:
		return nil
	}
}

// subtitleReasons checks the subtitle clause.
func subtitleReasons(info probe.MediaInfo, t Target) []string {
	keep := SelectSubtitles(info, t)
	switch t.Subtitles.Mode {
	case SubsKeepAll:
		return nil
	case SubsKeepPreferred:
		if len(keep) == len(info.Subtitle) {
			return nil
		}
		return []string{fmt.Sprintf("subtitle mode keep_preferred would drop %d track(s)", len(info.Subtitle)-len(keep))}
	case SubsRemove:
		if len(info.Subtitle) == 0 {
			return nil
		}
		return []string{fmt.Sprintf("subtitle mode remove would drop %d track(s)", len(info.Subtitle))}
	default:
		return nil
	}
}

// SelectAudio returns the indexes (into info.Audio, which is in stream
// order) of the audio tracks the Target keeps. Task 04 reuses this to build
// ffmpeg track mappings.
//
//   - keep_all: every track.
//   - keep_preferred: tracks whose language is in PreferredLanguages; if
//     none match, keep all.
//   - keep_best / compact: one track — the first match in
//     PreferredLanguages by highest channel count, else the first track.
func SelectAudio(info probe.MediaInfo, t Target) []int {
	switch t.Audio.Mode {
	case AudioKeepAll, "":
		return allIndexes(len(info.Audio))
	case AudioKeepPreferred:
		keep := matchingIndexes(info.Audio, func(a probe.AudioStream) bool {
			return languageMatches(a.Language, t.Audio.PreferredLanguages)
		})
		if len(keep) > 0 {
			return keep
		}
		return allIndexes(len(info.Audio))
	case AudioKeepBest, AudioCompact:
		if len(info.Audio) == 0 {
			return nil
		}
		// First match in PreferredLanguages by highest channel count.
		best := -1
		for i, a := range info.Audio {
			if !languageMatches(a.Language, t.Audio.PreferredLanguages) {
				continue
			}
			if best == -1 || a.Channels > info.Audio[best].Channels {
				best = i
			}
		}
		if best != -1 {
			return []int{best}
		}
		// Else the first track.
		return []int{0}
	default:
		return allIndexes(len(info.Audio))
	}
}

// SelectSubtitles returns the indexes (into info.Subtitle) of the subtitle
// tracks the Target keeps.
//
//   - keep_all: every track.
//   - keep_preferred: tracks whose language is in PreferredLanguages; if
//     none match, keep all.
//   - remove: none.
func SelectSubtitles(info probe.MediaInfo, t Target) []int {
	switch t.Subtitles.Mode {
	case SubsRemove:
		return nil
	case SubsKeepPreferred:
		keep := matchingIndexes(info.Subtitle, func(s probe.SubtitleStream) bool {
			return languageMatches(s.Language, t.Subtitles.PreferredLanguages)
		})
		if len(keep) > 0 {
			return keep
		}
		return allIndexes(len(info.Subtitle))
	default: // keep_all
		return allIndexes(len(info.Subtitle))
	}
}

// languageMatches reports whether a stream language tag matches any of the
// preferred languages, case-insensitively. Unset ("und" or empty) tags
// never match: a file with no language tags falls back to "keep all".
func languageMatches(tag string, preferred []string) bool {
	if tag == "" || strings.EqualFold(tag, "und") {
		return false
	}
	for _, p := range preferred {
		if strings.EqualFold(tag, p) {
			return true
		}
	}
	return false
}

func allIndexes(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func matchingIndexes[T any](items []T, match func(T) bool) []int {
	var out []int
	for i, item := range items {
		if match(item) {
			out = append(out, i)
		}
	}
	return out
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// dedupe drops duplicate reasons while keeping order.
func dedupe(reasons []string) []string {
	seen := map[string]bool{}
	out := reasons[:0]
	for _, r := range reasons {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}
