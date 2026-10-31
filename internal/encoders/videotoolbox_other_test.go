// SPDX-License-Identifier: AGPL-3.0-only

//go:build !darwin

package encoders

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVideotoolboxRegistration proves the per-platform registration off
// macOS (task 30 acceptance: a darwin build registers videotoolbox while a
// linux build does not): the stub is registered for detection but errors on
// detect, so no non-darwin capability list ever carries it.
func TestVideotoolboxRegistration(t *testing.T) {
	found := false
	for _, enc := range allEncoders {
		if enc.Name() == "videotoolbox" {
			found = true
			assert.Equal(t, []string{"h264", "hevc"}, enc.Codecs())
		}
	}
	require.True(t, found, "the stub is registered so the registry compiles everywhere")

	// Even an ffmpeg build listing the videotoolbox encoders cannot make a
	// non-darwin build report the backend: detection errors first.
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...),
		"h264_videotoolbox", "hevc_videotoolbox"), []string{"videotoolbox"}, nil)
	cap, err := VideoToolbox.Detect(context.Background(), script)
	require.Error(t, err)
	assert.Empty(t, cap.Encoders)

	// Full detection with the same fake ffmpeg reports only cpu.
	d := fakeDevices(t, false, false)
	cap, err = d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.NotContains(t, names(cap), "videotoolbox")
	assert.Contains(t, names(cap), "cpu")
}
