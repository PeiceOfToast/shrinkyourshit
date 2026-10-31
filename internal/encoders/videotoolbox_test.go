// SPDX-License-Identifier: AGPL-3.0-only

//go:build darwin

package encoders

import (
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVideotoolboxRegistration proves the per-platform registration on
// macOS: the build registers videotoolbox (h264 and hevc — Apple has no AV1
// hardware encoder) in the detection list, before cpu.
func TestVideotoolboxRegistration(t *testing.T) {
	found := -1
	for i, enc := range allEncoders {
		if enc.Name() == "videotoolbox" {
			found = i
			assert.Equal(t, []string{"h264", "hevc"}, enc.Codecs())
		}
	}
	require.Greater(t, found, -1, "a darwin build registers videotoolbox")
	require.NotEqual(t, len(allEncoders)-1, found,
		"cpu stays last in the registration order")
}

// TestBuildArgsVideotoolbox: the quality step maps onto the VideoToolbox
// constant-quality scale with -allow_sw 0, so a software fallback cannot
// masquerade as hardware (darwin only — the backend does not exist
// elsewhere).
func TestBuildArgsVideotoolbox(t *testing.T) {
	assertArgs(t, VideoToolbox, withTarget(codecTarget(flow.CodecHEVC, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "hevc_videotoolbox", "-q:v", "60", "-allow_sw", "0",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))

	assertArgs(t, VideoToolbox, withTarget(codecTarget(flow.CodecH264, 5)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_videotoolbox", "-q:v", "90", "-allow_sw", "0",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))

	// copy never re-encodes, like every backend.
	assertArgs(t, VideoToolbox, withTarget(copyTarget()), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "copy",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}
