// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// buildCommand assembles the full ffmpeg argument list for a Spec around
// encoder-specific video options. Shape:
//
//	<global flags> [hw pre-input flags] -i input -map ... <video options>
//	<audio options> <subtitle options> [container flags] output
//
// The runner executes this with the ffmpeg binary prefixed and parses
// "key=value" progress lines from stdout (STACK.md: always pass
// -progress pipe:1 -nostats for encodes).
func buildCommand(spec Spec, preInputArgs, videoArgs []string) []string {
	args := []string{"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y"}
	args = append(args, preInputArgs...)
	args = append(args, "-i", spec.Input)

	// Stream maps. The uppercase V specifier matches only real video
	// streams, matching probe.MediaInfo.Video which excludes attached
	// cover pictures; audio and subtitle indexes are positions within
	// their stream type, exactly the indexes in the Spec. Mapping any
	// stream disables ffmpeg's automatic selection, so only the selected
	// tracks end up in the output.
	if len(spec.Info.Video) > 0 {
		args = append(args, "-map", "0:V:0")
	}
	for _, i := range spec.KeepAudio {
		args = append(args, "-map", "0:a:"+itoa(i))
	}
	for _, i := range spec.KeepSubs {
		args = append(args, "-map", "0:s:"+itoa(i))
	}

	if videoShouldCopy(spec) {
		// copy never re-encodes video (remux and track changes only),
		// either because video.codec is copy or because the file's video
		// already is the target codec and reencode_if_same_codec is off
		// (DECISIONS.md B2 satisfied rule).
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, videoArgs...)
	}
	args = append(args, audioArgs(spec)...)
	args = append(args, subtitleArgs(spec)...)

	if spec.Target.Container == flow.ContainerMP4 {
		// Put the index at the front of the file so it streams before the
		// whole file is downloaded.
		args = append(args, "-movflags", "+faststart")
	}
	return append(args, spec.Output)
}

// videoShouldCopy reports whether the video must be remuxed instead of
// re-encoded: an explicit copy target, or the satisfied rule (DECISIONS.md
// B2) — the file's video already is the target codec and
// reencode_if_same_codec is off. In both cases the output only needs track
// and container changes, and re-encoding would add generational loss for
// nothing.
func videoShouldCopy(spec Spec) bool {
	if spec.Target.Video.Codec == flow.CodecCopy {
		return true
	}
	if len(spec.Info.Video) == 0 {
		return false
	}
	// 0:V:0 maps the first real video stream, which is info.Video[0].
	return spec.Info.Video[0].Codec == spec.Target.Video.Codec &&
		!spec.Target.Video.ReencodeIfSameCodec
}

// audioArgs picks a codec per kept audio stream: copy, or the compact
// transcode (aac 160k stereo or opus 128k stereo, ARCHITECTURE.md) when the
// mode is compact and the track is not already in the compact codec. The
// per-stream specifiers are output stream positions in map order.
func audioArgs(spec Spec) []string {
	var args []string
	for k, idx := range spec.KeepAudio {
		if idx < 0 || idx >= len(spec.Info.Audio) {
			continue // inconsistent spec; never panic
		}
		sel := ":a:" + itoa(k)
		if spec.Target.Audio.Mode == flow.AudioCompact &&
			spec.Info.Audio[idx].Codec != spec.Target.Audio.CompactCodec {
			switch spec.Target.Audio.CompactCodec {
			case flow.CompactCodecOpus:
				args = append(args, "-c"+sel, "libopus", "-b"+sel, "128k", "-ac"+sel, "2")
			default: // aac
				args = append(args, "-c"+sel, "aac", "-b"+sel, "160k", "-ac"+sel, "2")
			}
			continue
		}
		args = append(args, "-c"+sel, "copy")
	}
	return args
}

// subtitleArgs picks a codec per kept subtitle stream. Subtitles are copied
// except where the container cannot hold the codec:
//
//   - to mkv: mov_text (the mp4 text codec) becomes srt;
//   - to mp4: every other text subtitle becomes mov_text, the only text
//     codec mp4 reliably holds.
//
// Bitmap subtitles (PGS and friends) are copied everywhere; they are not
// convertible and not valid in mp4, which the output verification (task 05)
// reports rather than silently dropping user data.
func subtitleArgs(spec Spec) []string {
	var args []string
	for k, idx := range spec.KeepSubs {
		if idx < 0 || idx >= len(spec.Info.Subtitle) {
			continue // inconsistent spec; never panic
		}
		codec := spec.Info.Subtitle[idx].Codec
		out := "copy"
		switch {
		case spec.Target.Container == flow.ContainerMKV && codec == "mov_text":
			out = "srt"
		case spec.Target.Container == flow.ContainerMP4 && isTextSubtitle(codec) && codec != "mov_text":
			out = "mov_text"
		}
		args = append(args, "-c:s:"+itoa(k), out)
	}
	return args
}

// isTextSubtitle reports whether a subtitle codec can be converted to SRT
// (and mov_text). Bitmap codecs cannot.
func isTextSubtitle(codec string) bool {
	switch codec {
	case "", "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "arib_caption":
		return false
	}
	return true
}

// ExtractCommand is one ffmpeg command that writes a removed text subtitle
// as an .srt file beside the output (subtitles.extract_removed, task 05
// executes these after the main encode).
type ExtractCommand struct {
	// SubIndex is the subtitle stream index within the file's subtitle
	// streams (the same index space as Spec.KeepSubs).
	SubIndex int
	// Language is the tag used in the output file name; "und" when the
	// track has no language.
	Language string
	// Args is the full ffmpeg argument list (binary path prefixed by the
	// runner).
	Args []string
}

// ExtractCommands returns one command per removed text subtitle. Nothing is
// returned when extract_removed is off, when no subtitle was removed, or
// when only bitmap subtitles were removed (those cannot be converted).
//
// Files are named <output-base>.<lang>.srt (for example
// movie.eng.srt) beside the output, which is in scratch at job time —
// nothing is written into the library folder until the result is verified.
// Repeated languages get an occurrence suffix (movie.eng.srt,
// movie.eng.2.srt) so one track cannot overwrite another.
func ExtractCommands(spec Spec) []ExtractCommand {
	if !spec.Target.Subtitles.ExtractRemoved {
		return nil
	}
	kept := map[int]bool{}
	for _, i := range spec.KeepSubs {
		kept[i] = true
	}
	base := strings.TrimSuffix(spec.Output, filepath.Ext(spec.Output))
	seen := map[string]int{}

	var out []ExtractCommand
	for i, s := range spec.Info.Subtitle {
		if kept[i] || !s.TextBased {
			continue
		}
		lang := s.Language
		if lang == "" {
			lang = "und"
		}
		seen[lang]++
		name := base + "." + lang + ".srt"
		if n := seen[lang]; n > 1 {
			name = base + "." + lang + "." + strconv.Itoa(n) + ".srt"
		}
		args := []string{
			"-hide_banner", "-nostdin", "-v", "error", "-y",
			"-i", spec.Input,
			"-map", "0:s:" + itoa(i),
			"-c:s", "srt",
			name,
		}
		out = append(out, ExtractCommand{SubIndex: i, Language: lang, Args: args})
	}
	return out
}
