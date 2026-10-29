// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"errors"
	"fmt"
	"math"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// VerifyOutput checks that the produced file is a faithful result of the
// spec before anything in the library is touched (task 05): the file must
// be non-empty, keep the video codec the target asked for (or the source
// codec for copy), keep exactly the audio and subtitle tracks the spec
// selected, and stay within 1% or 2s of the source duration. A failed
// verification fails the job; the original is never modified by a failed
// run.
func VerifyOutput(spec Spec, srcDuration float64, out probe.MediaInfo, outSize int64) error {
	if outSize <= 0 {
		return errors.New("output file is empty")
	}

	if len(spec.Info.Video) > 0 {
		if len(out.Video) == 0 {
			return errors.New("output has no video stream")
		}
		expected := spec.Target.Video.Codec
		if expected == flow.CodecCopy {
			// Copy preserves the source codec; the target says nothing.
			expected = spec.Info.Video[0].Codec
		}
		if got := out.Video[0].Codec; got != expected {
			return fmt.Errorf("output video is %s, want %s", orUnknown(got), expected)
		}
	}

	if got, want := len(out.Audio), len(spec.KeepAudio); got != want {
		return fmt.Errorf("output has %d audio track(s), want %d", got, want)
	}
	if got, want := len(out.Subtitle), len(spec.KeepSubs); got != want {
		return fmt.Errorf("output has %d subtitle track(s), want %d", got, want)
	}

	if srcDuration > 0 {
		outDuration := out.DurationSeconds
		if outDuration <= 0 {
			return fmt.Errorf("output duration is unknown, source is %.2fs", srcDuration)
		}
		tolerance := math.Max(srcDuration*0.01, 2)
		if math.Abs(outDuration-srcDuration) > tolerance {
			return fmt.Errorf("output duration %.2fs is too far from source %.2fs", outDuration, srcDuration)
		}
	}
	return nil
}

func orUnknown(codec string) string {
	if codec == "" {
		return "unknown"
	}
	return codec
}
