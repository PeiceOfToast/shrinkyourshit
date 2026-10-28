// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// nvencEncoder drives NVIDIA NVENC (DECISIONS.md 14). One hardware engine
// serves the three ffmpeg encoders h264_nvenc, hevc_nvenc and av1_nvenc;
// support for AV1 depends on the GPU generation, so each codec is
// self-tested on its own.
type nvencEncoder struct{}

// nvencEncoderFor maps a target codec to its ffmpeg encoder name.
var nvencEncoderFor = map[string]string{
	flow.CodecHEVC: "hevc_nvenc",
	flow.CodecAV1:  "av1_nvenc",
	flow.CodecH264: "h264_nvenc",
}

func (e *nvencEncoder) Name() string { return "nvenc" }
func (e *nvencEncoder) Codecs() []string {
	return []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264}
}

// Detect checks NVIDIA device presence and self-tests each codec whose
// ffmpeg encoder exists in this build.
func (e *nvencEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
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
func (e *nvencEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	if err := requireDevices(env.nvidia, "NVIDIA"); err != nil {
		return nil, err
	}
	var codecs []string
	for _, codec := range e.Codecs() {
		name := nvencEncoderFor[codec]
		if !env.encoders[name] {
			continue
		}
		// p1 is the fastest NVENC preset; the self-test only proves the
		// encoder session can be created, so speed matters more than
		// quality. Real encodes use p5 (quality step table).
		err := selfTest(ctx, env.ffmpegPath, nil,
			[]string{"-c:v", name, "-preset", "p1"})
		if err != nil {
			continue
		}
		codecs = append(codecs, codec)
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: nvenc self-test failed on all codecs")
	}
	return &EncoderInfo{Name: e.Name(), Codecs: codecs}, nil
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH.
func (e *nvencEncoder) SelfTest(ctx context.Context) error {
	return selfTest(ctx, "ffmpeg", nil,
		[]string{"-c:v", nvencEncoderFor[flow.CodecHEVC], "-preset", "p1"})
}

// BuildArgs maps the quality step onto hevc_nvenc/av1_nvenc/h264_nvenc
// (constant quality, preset p5). video.codec=copy is handled centrally by
// buildCommand.
func (e *nvencEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy {
		return buildCommand(spec, nil, nil)
	}
	name := nvencEncoderFor[spec.Target.Video.Codec]
	q := qualityFor(spec.Target.Video.Quality)
	video := []string{"-c:v", name, "-cq", itoa(q.NVENCCQ), "-preset", "p5"}
	return buildCommand(spec, nil, video)
}
