// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileEasyModeMappingTable walks every slider and checkbox
// combination of the easy-mode mapping table (ARCHITECTURE.md): 5 quality
// stops x 2 subtitle states x 4 audio stops, asserting the exact Target
// fields each combination must produce.
func TestCompileEasyModeMappingTable(t *testing.T) {
	settings := EasySettings{Codec: CodecHEVC, PreferredLanguages: []string{"eng"}}

	for quality := QualityMin; quality <= QualityMax; quality++ {
		for _, removeSubs := range []bool{false, true} {
			for audio := EasyAudioKeepAll; audio <= EasyAudioCompact; audio++ {
				in := EasyModeInput{Quality: quality, RemoveSubtitles: removeSubs, Audio: audio}
				name := fmt.Sprintf("quality %d / subs %s / audio %d", quality,
					map[bool]string{false: "keep", true: "remove"}[removeSubs], audio)
				t.Run(name, func(t *testing.T) {
					got, err := CompileEasyMode(in, settings)
					require.NoError(t, err)

					// Video: quality from the slider, codec from settings,
					// skip-when-already-target stays off (B2).
					require.Equal(t, quality, got.Video.Quality)
					require.Equal(t, CodecHEVC, got.Video.Codec)
					require.False(t, got.Video.ReencodeIfSameCodec)

					// Subtitles: checkbox only.
					if removeSubs {
						require.Equal(t, SubsRemove, got.Subtitles.Mode)
					} else {
						require.Equal(t, SubsKeepAll, got.Subtitles.Mode)
					}
					require.False(t, got.Subtitles.ExtractRemoved)
					require.Equal(t, []string{"eng"}, got.Subtitles.PreferredLanguages)

					// Audio: slider position 1..4 -> the four modes.
					wantMode := map[int]string{
						EasyAudioKeepAll:       AudioKeepAll,
						EasyAudioKeepPreferred: AudioKeepPreferred,
						EasyAudioKeepBest:      AudioKeepBest,
						EasyAudioCompact:       AudioCompact,
					}[audio]
					require.Equal(t, wantMode, got.Audio.Mode)
					require.Equal(t, CompactCodecAAC, got.Audio.CompactCodec)
					require.Equal(t, []string{"eng"}, got.Audio.PreferredLanguages)

					// Container is always mkv in easy mode.
					require.Equal(t, ContainerMKV, got.Container)

					// The compiled Target must itself be valid.
					require.NoError(t, got.Validate())
				})
			}
		}
	}
}

// TestCompileEasyModeExactJSON pins the exact stored JSON of one
// representative combination (quality 3, remove subtitles, audio 2).
func TestCompileEasyModeExactJSON(t *testing.T) {
	got, err := CompileEasyMode(
		EasyModeInput{Quality: 3, RemoveSubtitles: true, Audio: EasyAudioKeepPreferred},
		EasySettings{Codec: CodecHEVC, PreferredLanguages: []string{"eng"}},
	)
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"video": {"codec": "hevc", "quality": 3, "reencode_if_same_codec": false},
		"audio": {"mode": "keep_preferred", "preferred_languages": ["eng"], "compact_codec": "aac"},
		"subtitles": {"mode": "remove", "preferred_languages": ["eng"], "extract_removed": false},
		"container": "mkv"
	}`, string(raw))
}

// TestCompileEasyModeSettings covers how the settings flow in: the codec
// choice and preferred languages, plus the safe defaults.
func TestCompileEasyModeSettings(t *testing.T) {
	for _, codec := range []string{CodecHEVC, CodecAV1, CodecH264} {
		got, err := CompileEasyMode(
			EasyModeInput{Quality: 2, Audio: EasyAudioKeepBest},
			EasySettings{Codec: codec, PreferredLanguages: []string{"deu", "eng"}},
		)
		require.NoError(t, err)
		require.Equal(t, codec, got.Video.Codec)
		require.Equal(t, []string{"deu", "eng"}, got.Audio.PreferredLanguages)
		require.Equal(t, []string{"deu", "eng"}, got.Subtitles.PreferredLanguages)
	}

	// Empty settings fall back to the defaults (DECISIONS.md B1).
	got, err := CompileEasyMode(
		EasyModeInput{Quality: 1, Audio: EasyAudioKeepAll},
		EasySettings{},
	)
	require.NoError(t, err)
	require.Equal(t, CodecHEVC, got.Video.Codec)
	require.Equal(t, []string{"eng"}, got.Audio.PreferredLanguages)

	// Codecs easy mode cannot produce are rejected.
	_, err = CompileEasyMode(
		EasyModeInput{Quality: 3, Audio: EasyAudioKeepAll},
		EasySettings{Codec: CodecCopy},
	)
	require.ErrorContains(t, err, "copy")
}

// TestCompileEasyModeInvalidInput: bad slider values must fail validation
// (the API turns this into a 400), not silently produce a broken Target.
func TestCompileEasyModeInvalidInput(t *testing.T) {
	for _, in := range []EasyModeInput{
		{Quality: 0, Audio: EasyAudioKeepAll},
		{Quality: 6, Audio: EasyAudioKeepAll},
		{Quality: 3, Audio: 0},
		{Quality: 3, Audio: 5},
	} {
		_, err := CompileEasyMode(in, EasySettings{Codec: CodecHEVC})
		require.Error(t, err)
	}
}

// TestDescribeTargetRoundTrip: every combination the compiler produces
// describes back to the exact slider state it came from.
func TestDescribeTargetRoundTrip(t *testing.T) {
	settings := EasySettings{Codec: CodecHEVC, PreferredLanguages: []string{"eng"}}
	for quality := QualityMin; quality <= QualityMax; quality++ {
		for _, removeSubs := range []bool{false, true} {
			for audio := EasyAudioKeepAll; audio <= EasyAudioCompact; audio++ {
				in := EasyModeInput{Quality: quality, RemoveSubtitles: removeSubs, Audio: audio}
				target, err := CompileEasyMode(in, settings)
				require.NoError(t, err)
				got, ok := DescribeTarget(target)
				require.True(t, ok, "easy-produced target must describe back")
				require.Equal(t, in, got)
			}
		}
	}

	// The codec choice survives the round-trip check too (describe does
	// not know which codec settings had, but it accepts all three).
	for _, codec := range []string{CodecHEVC, CodecAV1, CodecH264} {
		target, err := CompileEasyMode(
			EasyModeInput{Quality: 4, Audio: EasyAudioCompact},
			EasySettings{Codec: codec, PreferredLanguages: []string{"eng"}},
		)
		require.NoError(t, err)
		_, ok := DescribeTarget(target)
		require.True(t, ok)
	}
}

// TestDescribeTargetRejectsHandEdited covers Targets easy mode cannot
// produce: the describe must refuse them so those libraries open in the
// advanced editor instead.
func TestDescribeTargetRejectsHandEdited(t *testing.T) {
	base, err := CompileEasyMode(
		EasyModeInput{Quality: 3, Audio: EasyAudioCompact},
		EasySettings{Codec: CodecHEVC, PreferredLanguages: []string{"eng"}},
	)
	require.NoError(t, err)

	handEdited := []struct {
		name   string
		mutate func(*Target)
	}{
		{"video codec copy", func(t *Target) { t.Video.Codec = CodecCopy }},
		{"video codec unknown", func(t *Target) { t.Video.Codec = "vp9" }},
		{"reencode_if_same_codec on", func(t *Target) { t.Video.ReencodeIfSameCodec = true }},
		{"quality out of range low", func(t *Target) { t.Video.Quality = 0 }},
		{"quality out of range high", func(t *Target) { t.Video.Quality = 6 }},
		{"subtitles keep_preferred", func(t *Target) { t.Subtitles.Mode = SubsKeepPreferred }},
		{"subtitles unknown mode", func(t *Target) { t.Subtitles.Mode = "strip" }},
		{"extract_removed on", func(t *Target) { t.Subtitles.ExtractRemoved = true }},
		{"audio unknown mode", func(t *Target) { t.Audio.Mode = "keep_dts" }},
		{"compact codec opus", func(t *Target) { t.Audio.CompactCodec = CompactCodecOpus }},
		{"container mp4", func(t *Target) { t.Container = ContainerMP4 }},
	}
	for _, tc := range handEdited {
		t.Run(tc.name, func(t *testing.T) {
			mutated := base
			tc.mutate(&mutated)
			_, ok := DescribeTarget(mutated)
			require.False(t, ok)
		})
	}

	// The all-zero Target (a corrupt row) describes to nothing.
	_, ok := DescribeTarget(Target{})
	require.False(t, ok)
}
