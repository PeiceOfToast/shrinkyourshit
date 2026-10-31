// SPDX-License-Identifier: AGPL-3.0-only

//go:build darwin

package encoders

import (
	"context"
	"fmt"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// videotoolboxEncoder drives Apple's VideoToolbox (task 30): the hardware
// path of the macOS binaries. Apple has no AV1 hardware encoder, so this
// backend reports h264 and hevc only.
type videotoolboxEncoder struct{}

// videotoolboxEncoderFor maps a target codec to its ffmpeg encoder name.
var videotoolboxEncoderFor = map[string]string{
	flow.CodecHEVC: "hevc_videotoolbox",
	flow.CodecH264: "h264_videotoolbox",
}

func (e *videotoolboxEncoder) Name() string { return "videotoolbox" }

func (e *videotoolboxEncoder) Codecs() []string {
	return []string{flow.CodecH264, flow.CodecHEVC}
}

// Detect self-tests each codec whose ffmpeg encoder exists in this build.
// There is no device node to check on macOS; the self-test is the gate. A
// codec fails when the Mac lacks the hardware (hevc needs a 6th-generation
// Intel core or Apple Silicon, macOS 10.13 and up — see README.md).
func (e *videotoolboxEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
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

// detectWithEnv runs detection against a prepared environment (see detect.go).
func (e *videotoolboxEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	var codecs []string
	for _, codec := range e.Codecs() {
		name := videotoolboxEncoderFor[codec]
		if !env.encoders[name] {
			continue
		}
		// Constant quality keeps the self-test short and needs no bitrate.
		err := selfTest(ctx, env.ffmpegPath, nil,
			[]string{"-c:v", name, "-q:v", "65", "-allow_sw", "0"})
		if err != nil {
			continue
		}
		codecs = append(codecs, codec)
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: videotoolbox self-test failed on all codecs")
	}
	return &EncoderInfo{Name: e.Name(), Codecs: codecs}, nil
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH.
func (e *videotoolboxEncoder) SelfTest(ctx context.Context) error {
	return selfTest(ctx, "ffmpeg", nil,
		[]string{"-c:v", videotoolboxEncoderFor[flow.CodecHEVC], "-q:v", "65", "-allow_sw", "0"})
}

// BuildArgs maps the quality step onto the VideoToolbox constant-quality
// scale (-q:v) with -allow_sw 0, so a software fallback cannot masquerade as
// hardware. video.codec=copy is handled centrally by buildCommand.
func (e *videotoolboxEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy {
		return buildCommand(spec, nil, nil)
	}
	name := videotoolboxEncoderFor[spec.Target.Video.Codec]
	q := qualityFor(spec.Target.Video.Quality)
	video := []string{"-c:v", name, "-q:v", itoa(q.VideoToolboxQ), "-allow_sw", "0"}
	return buildCommand(spec, nil, video)
}
