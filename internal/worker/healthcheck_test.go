// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// hcSpec returns a health check spec, defaulting zero fields like the
// compiler does.
func hcSpec(mode string, minutes int, tolerance float64) *flow.HealthCheckSpec {
	return &flow.HealthCheckSpec{Mode: mode, Minutes: minutes, DurationTolerance: tolerance}
}

// srcWith builds a source probe with the given stream layout.
func srcWith(video, audio, subs int, duration float64) probe.MediaInfo {
	info := probe.MediaInfo{DurationSeconds: duration}
	for i := 0; i < video; i++ {
		info.Video = append(info.Video, probe.VideoStream{Codec: flow.CodecH264})
	}
	for i := 0; i < audio; i++ {
		info.Audio = append(info.Audio, probe.AudioStream{Codec: "aac"})
	}
	for i := 0; i < subs; i++ {
		info.Subtitle = append(info.Subtitle, probe.SubtitleStream{Codec: "subrip", TextBased: true})
	}
	return info
}

// TestVerifyHealthOutput: the stream types the source had must survive (a
// removed subtitle policy is legitimate; dropped video or audio is not) and
// the duration must survive the tolerance (task 24).
func TestVerifyHealthOutput(t *testing.T) {
	hc := hcSpec(flow.HealthCheckSample, 2, 0.02)

	// Healthy: same streams, duration within tolerance.
	require.NoError(t, verifyHealthOutput(hc, srcWith(1, 2, 1, 100), srcWith(1, 2, 1, 99)))

	// Dropped video fails.
	err := verifyHealthOutput(hc, srcWith(1, 1, 0, 10), srcWith(0, 1, 0, 10))
	require.ErrorContains(t, err, "video stream")

	// Dropped audio fails, naming the track count.
	err = verifyHealthOutput(hc, srcWith(1, 2, 0, 10), srcWith(1, 0, 0, 10))
	require.ErrorContains(t, err, "2 audio track")

	// Removed subtitles are a policy, not a defect.
	require.NoError(t, verifyHealthOutput(hc, srcWith(1, 1, 3, 10), srcWith(1, 1, 0, 10)))

	// A duration under the tolerance fails, naming both durations.
	err = verifyHealthOutput(hc, srcWith(1, 1, 0, 10), srcWith(1, 1, 0, 9))
	require.ErrorContains(t, err, "9.00s")
	require.ErrorContains(t, err, "10.00s")

	// Within the tolerance it passes.
	require.NoError(t, verifyHealthOutput(hc, srcWith(1, 1, 0, 10), srcWith(1, 1, 0, 9.9)))

	// A custom tolerance moves the line.
	require.NoError(t, verifyHealthOutput(hcSpec(flow.HealthCheckSample, 2, 0.5),
		srcWith(1, 1, 0, 10), srcWith(1, 1, 0, 5.5)))

	// An unknown source duration skips the duration check entirely.
	require.NoError(t, verifyHealthOutput(hc, srcWith(1, 1, 0, 0), srcWith(1, 1, 0, 0)))
}

// TestSampleWindow: the sample decode takes minutes from the middle of the
// file; a shorter file decodes whole and an unknown duration decodes the
// window from the start (task 24).
func TestSampleWindow(t *testing.T) {
	ss, tt, ok := sampleWindow(2, 3*3600)
	require.True(t, ok)
	require.InDelta(t, (3*3600-120)/2, ss, 0.001)
	require.InDelta(t, 120, tt, 0.001)

	// A file shorter than the window: decode all of it, no seek.
	ss, tt, ok = sampleWindow(2, 90)
	require.False(t, ok)
	require.Equal(t, 0.0, ss)
	require.Equal(t, 0.0, tt)

	// Unknown duration: the window from the start.
	ss, tt, ok = sampleWindow(2, 0)
	require.True(t, ok)
	require.Equal(t, 0.0, ss)
	require.InDelta(t, 120, tt, 0.001)
}

// TestHealthCheckArgs: the decode command carries -xerror and narrows to
// the sample window when the mode is sample (task 24).
func TestHealthCheckArgs(t *testing.T) {
	output := "/scratch/output.mkv"

	full := healthCheckArgs(hcSpec(flow.HealthCheckFull, 2, 0.02), output, 10800)
	require.Equal(t, []string{"-v", "error", "-xerror", "-nostdin", "-i", output, "-f", "null", "-"}, full)

	sample := healthCheckArgs(hcSpec(flow.HealthCheckSample, 2, 0.02), output, 3*3600)
	require.Equal(t, []string{"-v", "error", "-xerror", "-nostdin",
		"-ss", "5340.000", "-t", "120.000", "-i", output, "-f", "null", "-"}, sample)

	// A file shorter than the window decodes whole.
	whole := healthCheckArgs(hcSpec(flow.HealthCheckSample, 2, 0.02), output, 90)
	require.Equal(t, []string{"-v", "error", "-xerror", "-nostdin", "-i", output, "-f", "null", "-"}, whole)
}

// TestTailExcerpt: the error message carries the last ffmpeg lines.
func TestTailExcerpt(t *testing.T) {
	tail := newTailBuffer(1 << 16)
	require.Empty(t, tailExcerpt(tail))
	_, _ = tail.Write([]byte("line one\nline two\nline three\nline four\n"))
	require.Equal(t, "line two | line three | line four", tailExcerpt(tail))
}

// TestRunHealthCheckDecodeFailure: a failing decode surfaces the ffmpeg
// error with its tail in the message (task 24).
func TestRunHealthCheckDecodeFailure(t *testing.T) {
	r := &runner{runFn: func(context.Context, []string) error {
		return errors.New("exit status 183")
	}}
	tail := newTailBuffer(1 << 16)
	_, _ = tail.Write([]byte("[h264] Invalid data found when processing input\n"))

	err := r.runHealthCheck(context.Background(), hcSpec(flow.HealthCheckFull, 2, 0.02),
		"/scratch/output.mkv", srcWith(1, 1, 0, 10), srcWith(1, 1, 0, 10), tail)
	require.ErrorContains(t, err, "decode check failed")
	require.ErrorContains(t, err, "exit status 183")
	require.ErrorContains(t, err, "Invalid data")
}
