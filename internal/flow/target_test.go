// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetJSONRoundTrip(t *testing.T) {
	// The exact shape from ARCHITECTURE.md.
	source := `{
	  "video": {"codec": "hevc", "quality": 3, "reencode_if_same_codec": false},
	  "audio": {"mode": "keep_all", "preferred_languages": ["eng"], "compact_codec": "aac"},
	  "subtitles": {"mode": "keep_all", "preferred_languages": ["eng"], "extract_removed": false},
	  "container": "mkv"
	}`
	target, err := ParseTarget([]byte(source))
	require.NoError(t, err)
	assert.Equal(t, CodecHEVC, target.Video.Codec)
	assert.Equal(t, 3, target.Video.Quality)
	assert.Equal(t, AudioKeepAll, target.Audio.Mode)
	assert.Equal(t, []string{"eng"}, target.Audio.PreferredLanguages)
	assert.Equal(t, SubsKeepAll, target.Subtitles.Mode)
	assert.Equal(t, ContainerMKV, target.Container)

	// Round-trips through marshal/unmarshal.
	encoded, err := json.Marshal(target)
	require.NoError(t, err)
	decoded, err := ParseTarget(encoded)
	require.NoError(t, err)
	assert.Equal(t, target, decoded)
}

func TestTargetDefault(t *testing.T) {
	def := Default()
	require.NoError(t, def.Validate())
	assert.Equal(t, CodecHEVC, def.Video.Codec, "B1: default codec is hevc")
	assert.Equal(t, ContainerMKV, def.Container)
	assert.Equal(t, AudioKeepAll, def.Audio.Mode)
	assert.Equal(t, SubsKeepAll, def.Subtitles.Mode)
}

func TestTargetValidate(t *testing.T) {
	valid := Default()
	require.NoError(t, valid.Validate())

	for name, mutate := range map[string]func(*Target){
		"bad video codec":     func(t *Target) { t.Video.Codec = "mpeg2" },
		"empty video codec":   func(t *Target) { t.Video.Codec = "" },
		"quality zero":        func(t *Target) { t.Video.Quality = 0 },
		"quality six":         func(t *Target) { t.Video.Quality = 6 },
		"bad audio mode":      func(t *Target) { t.Audio.Mode = "keep_none" },
		"compact bad codec":   func(t *Target) { t.Audio.Mode = AudioCompact; t.Audio.CompactCodec = "mp3" },
		"compact empty codec": func(t *Target) { t.Audio.Mode = AudioCompact; t.Audio.CompactCodec = "" },
		"bad subtitle mode":   func(t *Target) { t.Subtitles.Mode = "extract" },
		"bad container":       func(t *Target) { t.Container = "avi" },
		"empty container":     func(t *Target) { t.Container = "" },
		"empty audio mode":    func(t *Target) { t.Audio.Mode = "" },
		"empty subtitle mode": func(t *Target) { t.Subtitles.Mode = "" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			assert.Error(t, candidate.Validate())
		})
	}
}

func TestTargetValidateAccepts(t *testing.T) {
	for name, mutate := range map[string]func(*Target){
		"opus compact":      func(t *Target) { t.Audio.Mode = AudioCompact; t.Audio.CompactCodec = CompactCodecOpus },
		"aac compact":       func(t *Target) { t.Audio.Mode = AudioCompact; t.Audio.CompactCodec = CompactCodecAAC },
		"quality 1":         func(t *Target) { t.Video.Quality = 1 },
		"quality 5":         func(t *Target) { t.Video.Quality = 5 },
		"reencode flag":     func(t *Target) { t.Video.ReencodeIfSameCodec = true },
		"extract flag":      func(t *Target) { t.Subtitles.Mode = SubsRemove; t.Subtitles.ExtractRemoved = true },
		"copy codec":        func(t *Target) { t.Video.Codec = CodecCopy },
		"empty langs":       func(t *Target) { t.Audio.PreferredLanguages = nil; t.Subtitles.PreferredLanguages = nil },
		"leftover codec ok": func(t *Target) { t.Audio.CompactCodec = "flac" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := Default()
			mutate(&candidate)
			assert.NoError(t, candidate.Validate())
		})
	}
}

func TestParseTargetRejectsUnknownEnums(t *testing.T) {
	for name, source := range map[string]string{
		"video codec":   `{"video":{"codec":"h265","quality":3},"audio":{"mode":"keep_all"},"subtitles":{"mode":"keep_all"},"container":"mkv"}`,
		"audio mode":    `{"video":{"codec":"hevc","quality":3},"audio":{"mode":"strip"},"subtitles":{"mode":"keep_all"},"container":"mkv"}`,
		"subtitle mode": `{"video":{"codec":"hevc","quality":3},"audio":{"mode":"keep_all"},"subtitles":{"mode":"drop_all"},"container":"mkv"}`,
		"container":     `{"video":{"codec":"hevc","quality":3},"audio":{"mode":"keep_all"},"subtitles":{"mode":"keep_all"},"container":"avi"}`,
		"quality":       `{"video":{"codec":"hevc","quality":9},"audio":{"mode":"keep_all"},"subtitles":{"mode":"keep_all"},"container":"mkv"}`,
		"compact codec": `{"video":{"codec":"hevc","quality":3},"audio":{"mode":"compact","compact_codec":"wma"},"subtitles":{"mode":"keep_all"},"container":"mkv"}`,
		"malformed":     `{not json}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTarget([]byte(source))
			assert.Error(t, err)
		})
	}
}
