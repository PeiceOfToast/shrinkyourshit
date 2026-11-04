// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// cpuEncoder drives the software encoders libx265, libsvtav1 and libx264.
// It is the final fallback: every registry ends with it and its failure
// ends the job.
type cpuEncoder struct{}

// cpuEncoderFor maps a target codec to its ffmpeg encoder name.
var cpuEncoderFor = map[string]string{
	flow.CodecHEVC: "libx265",
	flow.CodecAV1:  "libsvtav1",
	flow.CodecH264: "libx264",
}

func (e *cpuEncoder) Name() string { return "cpu" }

func (e *cpuEncoder) Codecs() []string {
	return []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264}
}

// Detect self-tests each codec whose ffmpeg encoder exists in this build.
// CPU encoding has no hardware prerequisites; a codec is missing only when
// the ffmpeg build lacks the library (for example a build without
// libsvtav1).
func (e *cpuEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
	env, err := processDetector.probeEnv(ctx, ffmpegPath)
	if err != nil {
		return Capability{}, err
	}
	entry, err := e.detectWithEnv(ctx, env)
	if err != nil {
		return Capability{}, err
	}
	return Capability{Encoders: []EncoderInfo{*entry}}, nil
}

// cpuSelfTestArgs are speed-optimized settings per ffmpeg encoder for the
// self-test (libsvtav1 uses numeric presets, x265/x264 the named ones).
var cpuSelfTestArgs = map[string][]string{
	"libx265":   {"-preset", "ultrafast", "-crf", "40"},
	"libsvtav1": {"-preset", "12", "-crf", "55"},
	"libx264":   {"-preset", "ultrafast", "-crf", "40"},
}

// detectWithEnv runs detection against a prepared environment (see detect.go).
func (e *cpuEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	var codecs []string
	for _, codec := range e.Codecs() {
		name := cpuEncoderFor[codec]
		if !env.encoders[name] {
			continue
		}
		// Fast settings keep the self-test short; real encodes use the
		// quality step table.
		err := selfTest(ctx, env.ffmpegPath, nil,
			append([]string{"-c:v", name}, cpuSelfTestArgs[name]...))
		if err != nil {
			continue
		}
		codecs = append(codecs, codec)
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: no CPU encoder available in %s", env.ffmpegPath)
	}
	return &EncoderInfo{Name: e.Name(), Codecs: codecs}, nil
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH.
func (e *cpuEncoder) SelfTest(ctx context.Context) error {
	return selfTest(ctx, "ffmpeg", nil,
		[]string{"-c:v", cpuEncoderFor[flow.CodecH264], "-preset", "ultrafast", "-crf", "40"})
}

// BuildArgs maps the quality step onto the x265/SVT-AV1/x264 columns of the
// quality table in quality mode. crf mode passes the Target's CRF, bitrate
// mode passes -b:v, and the bitrate cap and speed preset apply to all three.
// video.codec=copy is handled centrally by buildCommand.
func (e *cpuEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy {
		return buildCommand(spec, nil, nil)
	}
	codec := spec.Target.Video.Codec
	q := qualityFor(spec.Target.Video.Quality)
	var video []string
	switch codec {
	case flow.CodecHEVC:
		video = cpuVideoArgs(spec, "libx265", q.X265CRF, presetFor(spec, x26xPresets))
	case flow.CodecAV1:
		video = cpuVideoArgs(spec, "libsvtav1", q.SVTAV1CRF, presetFor(spec, svtAV1Presets))
	case flow.CodecH264:
		video = cpuVideoArgs(spec, "libx264", q.X264CRF, presetFor(spec, x26xPresets))
	}
	return buildCommand(spec, nil, video)
}

// x26xPresets maps the named Target presets onto the libx264/libx265 names,
// which are the same words; medium is the balanced default.
var x26xPresets = map[string]string{
	"veryfast": "veryfast", "faster": "faster", "fast": "fast", "medium": "medium",
	"slow": "slow", "slower": "slower", "veryslow": "veryslow",
}

// cpuVideoArgs is the software encoder's video option list: the rate mode's
// option (quality CRF from the table, the Target CRF, or -b:v), the bitrate
// cap, then the preset. In quality mode the list is the pre-task-55 vector.
func cpuVideoArgs(spec Spec, name string, qualityCRF int, preset string) []string {
	args := []string{"-c:v", name}
	switch rateMode(spec.Rate) {
	case flow.RateModeCRF:
		args = append(args, "-crf", crfString(spec))
	case flow.RateModeBitrate:
		args = append(args, bitrateArgs(spec)...)
	default:
		args = append(args, "-crf", itoa(qualityCRF))
	}
	args = append(args, capArgs(spec)...)
	return append(args, "-preset", preset)
}
