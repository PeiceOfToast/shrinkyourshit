// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// TestScaleApplies pins the one scale rule the encoders and the satisfied
// rule share (task 55).
func TestScaleApplies(t *testing.T) {
	down := &ScaleTarget{Height: 1080}
	up := &ScaleTarget{Height: 2160, AllowUpscale: true}

	cases := []struct {
		name   string
		src    int
		target *ScaleTarget
		want   bool
	}{
		{"nil scale never applies", 2160, nil, false},
		{"taller source is scaled down", 2160, down, true},
		{"equal height is left alone", 1080, down, false},
		{"shorter source is not enlarged by default", 720, down, false},
		{"shorter source is enlarged with allow_upscale", 1080, up, true},
		{"equal height with allow_upscale is still left alone", 2160, up, false},
		{"unknown height never scales", 0, down, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, ScaleApplies(c.src, c.target))
		})
	}
}

// TestEvaluateScaleClause: a file already in the target codec is satisfied
// (DECISIONS.md B2), unless a scale would change its picture. The scale
// clause only applies to video that is re-encoded; copy never re-encodes.
func TestEvaluateScaleClause(t *testing.T) {
	info := func(height int) probe.MediaInfo {
		return probe.MediaInfo{
			Container: "mkv",
			Video:     []probe.VideoStream{{Codec: "hevc", Width: 3840, Height: height}},
		}
	}
	target := func(scale *ScaleTarget) Target {
		tg := Default()
		tg.Container = ContainerMKV
		tg.Video.Scale = scale
		return tg
	}

	t.Run("4K hevc with a 1080 scale needs work", func(t *testing.T) {
		d := Evaluate(info(2160), target(&ScaleTarget{Height: 1080, OnlyIfLarger: true}))
		require.True(t, d.Needs)
		assert.Equal(t, []string{"video is 2160p, scale target is 1080p"}, d.Reasons)
	})

	t.Run("1080 hevc with a 1080 scale is satisfied", func(t *testing.T) {
		d := Evaluate(info(1080), target(&ScaleTarget{Height: 1080, OnlyIfLarger: true}))
		assert.False(t, d.Needs, "reasons: %v", d.Reasons)
	})

	t.Run("1080 hevc with no scale is satisfied", func(t *testing.T) {
		d := Evaluate(info(1080), target(nil))
		assert.False(t, d.Needs, "reasons: %v", d.Reasons)
	})

	t.Run("copy never re-encodes, so a scale does not make it need work", func(t *testing.T) {
		tg := target(&ScaleTarget{Height: 1080, OnlyIfLarger: true})
		tg.Video.Codec = CodecCopy
		d := Evaluate(info(2160), tg)
		assert.False(t, d.Needs, "reasons: %v", d.Reasons)
	})
}
