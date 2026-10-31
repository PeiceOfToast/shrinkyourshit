// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// The health_check step (task 24): the produced file is verified before
// anything is reported complete — the probed stream types and duration must
// survive, then a decode pass must run clean. A failed check fails the job
// before the result policy touches the library, exactly like an encode
// failure.

// runHealthCheck verifies one output. The returned error carries the reason;
// the caller prefixes it and reports the job failed with the ffmpeg tail.
func (r *runner) runHealthCheck(ctx context.Context, hc *flow.HealthCheckSpec, output string, src, out probe.MediaInfo, tail *tailBuffer, sink *jobLogSink) error {
	if err := verifyHealthOutput(hc, src, out); err != nil {
		return err
	}
	args := healthCheckArgs(hc, output, out.DurationSeconds)
	if err := r.execDecode(ctx, args, tail, sink); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("the decode check failed: %v: %s", err, tailExcerpt(tail))
	}
	return nil
}

// verifyHealthOutput checks the probed output against the source: every
// stream type the source had must still be there, and the duration must
// survive the step's tolerance. Subtitle tracks are excepted — removing
// them is a legitimate subtitle policy; video and audio cannot be dropped
// by any policy, so their absence means a broken encode.
func verifyHealthOutput(hc *flow.HealthCheckSpec, src, out probe.MediaInfo) error {
	if len(src.Video) > 0 && len(out.Video) == 0 {
		return errors.New("the source has a video stream but the output has none")
	}
	if len(src.Audio) > 0 && len(out.Audio) == 0 {
		return fmt.Errorf("the source has %d audio track(s) but the output has none", len(src.Audio))
	}
	if src.DurationSeconds > 0 {
		minDuration := src.DurationSeconds * (1 - hc.DurationTolerance)
		if out.DurationSeconds < minDuration {
			return fmt.Errorf("output duration %.2fs is shorter than the source %.2fs (allowed loss %.1f%%)",
				out.DurationSeconds, src.DurationSeconds, hc.DurationTolerance*100)
		}
	}
	return nil
}

// healthCheckArgs builds the decode command for the output:
// ffmpeg -v error -xerror -nostdin -i <output> -f null -, narrowed to the
// sample window when the mode is sample. -xerror is what makes it a real
// check rather than a probe.
func healthCheckArgs(hc *flow.HealthCheckSpec, output string, outDuration float64) []string {
	args := []string{"-v", "error", "-xerror", "-nostdin"}
	if hc.Mode == flow.HealthCheckSample {
		if ss, t, ok := sampleWindow(hc.Minutes, outDuration); ok {
			if ss > 0 {
				args = append(args, "-ss", secondsArg(ss))
			}
			if t > 0 {
				args = append(args, "-t", secondsArg(t))
			}
		}
	}
	return append(args, "-i", output, "-f", "null", "-")
}

// sampleWindow returns the seek and length of the sample decode (task 24):
// minutes from the middle of the file. The whole file needs no window when
// it is shorter than the decode length; an unknown duration decodes the
// window from the start.
func sampleWindow(minutes int, duration float64) (ss, t float64, windowed bool) {
	window := float64(minutes) * 60
	switch {
	case duration <= 0:
		return 0, window, true
	case duration <= window:
		return 0, 0, false
	default:
		return (duration - window) / 2, window, true
	}
}

// secondsArg formats seconds for an ffmpeg argument (decimal seconds).
func secondsArg(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

// tailExcerpt condenses the ffmpeg stderr tail to its last lines for an
// error message; the full tail is stored on the job record.
func tailExcerpt(tail *tailBuffer) string {
	raw := strings.TrimSpace(tail.Tail())
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	const keep = 3
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	return strings.Join(lines, " | ")
}

// execDecode runs one decode verification pass. Like the encode and the
// extracts it goes through the test override, so tests can fake a clean
// (or failing) decode without ffmpeg. Its stderr feeds the structured log.
func (r *runner) execDecode(ctx context.Context, args []string, tail *tailBuffer,
	sink *jobLogSink) error {
	if r.runFn != nil {
		return r.runFn(ctx, args)
	}
	tee := newStderrTee(tail, sink)
	defer tee.close()
	return execFFmpeg(ctx, r.cfg.FFmpegPath, args, io.Discard, tee)
}
