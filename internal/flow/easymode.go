// SPDX-License-Identifier: AGPL-3.0-only

package flow

import "fmt"

// EasyModeInput is the state of the easy screen (task 07): a quality slider
// with five stops, a "remove subtitles" checkbox and an audio slider with
// four positions. It is the JSON stored in the library's `easy` column so an
// easy-mode library reopens with its sliders exactly where they were.
type EasyModeInput struct {
	// Quality is 1 to 5; 1 saves the most space, 5 the least.
	Quality int `json:"quality"`
	// RemoveSubtitles selects subtitles.mode "remove" (true) or
	// "keep_all" (false).
	RemoveSubtitles bool `json:"remove_subtitles"`
	// Audio is the audio slider position, 1 to 4.
	Audio int `json:"audio"`
}

// Audio slider positions (ARCHITECTURE.md easy-mode mapping).
const (
	EasyAudioKeepAll       = 1 // keep everything
	EasyAudioKeepPreferred = 2 // keep my languages
	EasyAudioKeepBest      = 3 // keep the best track
	EasyAudioCompact       = 4 // keep the best track, make it small
)

// EasySettings carries the settings easy mode compiles with. Codec comes
// from the default_video_codec setting (the easy screen itself never names
// a codec), PreferredLanguages from the preferred_languages setting.
type EasySettings struct {
	Codec              string
	PreferredLanguages []string
}

// Validate rejects slider values outside their stops. CompileEasyMode
// requires a validated input; the API calls this to map bad values to 400s.
func (in EasyModeInput) Validate() error {
	if in.Quality < QualityMin || in.Quality > QualityMax {
		return fmt.Errorf("flow: invalid easy quality %d: want %d to %d", in.Quality, QualityMin, QualityMax)
	}
	if in.Audio < EasyAudioKeepAll || in.Audio > EasyAudioCompact {
		return fmt.Errorf("flow: invalid easy audio slider %d: want %d to %d", in.Audio, EasyAudioKeepAll, EasyAudioCompact)
	}
	return nil
}

// CompileEasyMode turns the easy screen into a Target, exactly per the
// easy-mode mapping table in ARCHITECTURE.md:
//
//	quality slider N  -> video.quality = N, video.codec from settings
//	remove subtitles  -> subtitles.mode remove, else keep_all
//	audio slider 1..4 -> keep_all, keep_preferred, keep_best, compact
//
// Everything else is fixed: container mkv, compact codec aac, no subtitle
// extraction, reencode_if_same_codec off (DECISIONS.md B1/B2). An empty
// settings codec falls back to hevc and empty languages to eng; a codec
// value easy mode cannot produce (copy or unknown) is an error.
func CompileEasyMode(in EasyModeInput, s EasySettings) (Target, error) {
	if err := in.Validate(); err != nil {
		return Target{}, err
	}
	codec := s.Codec
	if codec == "" {
		codec = CodecHEVC
	}
	switch codec {
	case CodecHEVC, CodecAV1, CodecH264:
	default:
		return Target{}, fmt.Errorf("flow: easy mode cannot use video codec %q: want hevc, av1 or h264", codec)
	}
	langs := s.PreferredLanguages
	if len(langs) == 0 {
		langs = []string{"eng"}
	}
	t := Target{
		Video: VideoTarget{
			Codec:   codec,
			Quality: in.Quality,
		},
		Audio: AudioTarget{
			Mode:               audioModeForSlider(in.Audio),
			PreferredLanguages: langs,
			CompactCodec:       CompactCodecAAC,
		},
		Subtitles: SubtitleTarget{
			Mode:               SubsKeepAll,
			PreferredLanguages: langs,
		},
		Container: ContainerMKV,
	}
	if in.RemoveSubtitles {
		t.Subtitles.Mode = SubsRemove
	}
	return t, nil
}

// audioModeForSlider maps the audio slider position to an audio mode.
func audioModeForSlider(audio int) string {
	switch audio {
	case EasyAudioKeepPreferred:
		return AudioKeepPreferred
	case EasyAudioKeepBest:
		return AudioKeepBest
	case EasyAudioCompact:
		return AudioCompact
	default:
		return AudioKeepAll
	}
}

// DescribeTarget is the reverse of CompileEasyMode: it reports whether a
// Target could have been produced by the easy screen and, when it could,
// the slider state that reproduces it. Preferred languages are not part of
// the round-trip (easy mode always takes them from the settings), so they
// are accepted as-is.
//
// Strict by design: a Target that easy mode cannot produce — video codec
// "copy", a hand-tuned reencode_if_same_codec, keep_preferred subtitles,
// subtitle extraction, an mp4 container, an opus compact codec — returns
// ok=false so those libraries open in the advanced editor instead, where
// every field is visible.
func DescribeTarget(t Target) (EasyModeInput, bool) {
	in := EasyModeInput{Quality: t.Video.Quality}

	switch t.Video.Codec {
	case CodecHEVC, CodecAV1, CodecH264:
	default:
		return EasyModeInput{}, false
	}
	if t.Video.ReencodeIfSameCodec {
		return EasyModeInput{}, false
	}
	if in.Quality < QualityMin || in.Quality > QualityMax {
		return EasyModeInput{}, false
	}

	switch t.Subtitles.Mode {
	case SubsKeepAll:
		in.RemoveSubtitles = false
	case SubsRemove:
		in.RemoveSubtitles = true
	default:
		return EasyModeInput{}, false
	}
	if t.Subtitles.ExtractRemoved {
		return EasyModeInput{}, false
	}

	switch t.Audio.Mode {
	case AudioKeepAll:
		in.Audio = EasyAudioKeepAll
	case AudioKeepPreferred:
		in.Audio = EasyAudioKeepPreferred
	case AudioKeepBest:
		in.Audio = EasyAudioKeepBest
	case AudioCompact:
		// Easy mode compacts to aac; an opus compact target was written
		// by hand (or a future editor) and is not easy-mode state.
		if t.Audio.CompactCodec != CompactCodecAAC {
			return EasyModeInput{}, false
		}
		in.Audio = EasyAudioCompact
	default:
		return EasyModeInput{}, false
	}

	if t.Container != ContainerMKV {
		return EasyModeInput{}, false
	}
	return in, true
}
