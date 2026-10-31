// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// qsvEncoder drives Intel Quick Sync Video (DECISIONS.md 14). The primary
// path uses the qsv ffmpeg encoders (h264_qsv, hevc_qsv, av1_qsv). On Linux,
// when the QSV session cannot be initialized (for example on older drivers
// or when only the VAAPI stack is present), the VAAPI equivalent encoders
// are tried instead; the working variant per codec is chosen during
// detection and used by BuildArgs.
type qsvEncoder struct {
	// variants maps a target codec to the ffmpeg encoder chosen during
	// detection ("hevc_qsv" or "hevc_vaapi" style) plus the VAAPI device
	// node for the vaapi variant. Guarded by mu; written by detection,
	// read by BuildArgs. A codec without an entry defaults to the qsv
	// encoder.
	mu       sync.Mutex
	variants map[string]qsvVariant
}

// qsvVariant is the ffmpeg encoder setup chosen for one codec.
type qsvVariant struct {
	// encoderName is the ffmpeg -c:v value.
	encoderName string
	// device is the VAAPI device node; empty for the qsv variant.
	device string
}

// qsvEncoderFor and vaapiEncoderFor map a target codec to the ffmpeg
// encoder names of each variant.
var (
	qsvEncoderFor = map[string]string{
		flow.CodecHEVC: "hevc_qsv",
		flow.CodecAV1:  "av1_qsv",
		flow.CodecH264: "h264_qsv",
	}
	vaapiEncoderFor = map[string]string{
		flow.CodecHEVC: "hevc_vaapi",
		flow.CodecAV1:  "av1_vaapi",
		flow.CodecH264: "h264_vaapi",
	}
)

func (e *qsvEncoder) Name() string { return "qsv" }

func (e *qsvEncoder) Codecs() []string {
	return []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264}
}

// Detect checks Intel device presence and self-tests each codec: first the
// qsv encoder, then (on Linux) the VAAPI equivalent when QSV init fails.
func (e *qsvEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
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
func (e *qsvEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	hasDRI := len(env.driEntries) > 0
	if err := requireDevices(hasDRI, "Intel (VAAPI/QSV)"); err != nil {
		return nil, err
	}

	var codecs []string
	for _, codec := range e.Codecs() {
		// Primary path: the qsv encoder. QSV accepts software frames and
		// creates its own device, so no pre-input options are needed.
		name := qsvEncoderFor[codec]
		if env.encoders[name] {
			if err := selfTest(ctx, env.ffmpegPath, nil,
				[]string{"-c:v", name}); err == nil {
				e.setVariant(codec, qsvVariant{encoderName: name})
				codecs = append(codecs, codec)
				continue
			}
		}
		// Fallback path: the VAAPI equivalent. It needs a device node
		// (prefer the render node) and hardware frames, uploaded with an
		// explicit filter chain. Only tried on Linux, where the VAAPI
		// stack lives.
		if runtime.GOOS != "linux" || !env.hwaccels["vaapi"] {
			continue
		}
		vname := vaapiEncoderFor[codec]
		device := env.renderNode()
		if !env.encoders[vname] || device == "" {
			continue
		}
		if err := selfTest(ctx, env.ffmpegPath,
			[]string{"-vaapi_device", device},
			[]string{"-vf", "format=nv12,hwupload", "-c:v", vname}); err == nil {
			e.setVariant(codec, qsvVariant{encoderName: vname, device: device})
			codecs = append(codecs, codec)
		}
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: qsv self-test failed on all codecs")
	}
	return &EncoderInfo{Name: e.Name(), Codecs: codecs}, nil
}

// setVariant records the working ffmpeg encoder for a codec.
func (e *qsvEncoder) setVariant(codec string, v qsvVariant) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.variants == nil {
		e.variants = map[string]qsvVariant{}
	}
	e.variants[codec] = v
}

// variantFor returns the ffmpeg encoder for a codec, defaulting to the qsv
// variant when detection has not run (or has not chosen a variant).
func (e *qsvEncoder) variantFor(codec string) qsvVariant {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.variants[codec]; ok {
		return v
	}
	return qsvVariant{encoderName: qsvEncoderFor[codec]}
}

// claimedDevices returns the VAAPI device nodes the qsv backend took for its
// own vaapi variants (task 30). The vaapi backend skips these so one GPU is
// never reported by both backends; devices qsv does not hold remain free for
// vaapi — a machine can legitimately report both qsv and vaapi.
func (e *qsvEncoder) claimedDevices() map[string]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]bool{}
	for _, v := range e.variants {
		if v.device != "" {
			out[v.device] = true
		}
	}
	return out
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH, using the variant detection chose.
func (e *qsvEncoder) SelfTest(ctx context.Context) error {
	v := e.variantFor(flow.CodecHEVC)
	if v.device == "" {
		return selfTest(ctx, "ffmpeg", nil, []string{"-c:v", v.encoderName})
	}
	return selfTest(ctx, "ffmpeg", []string{"-vaapi_device", v.device},
		[]string{"-vf", "format=nv12,hwupload", "-c:v", v.encoderName})
}

// BuildArgs maps the quality step onto the variant chosen during detection.
// The qsv variant takes -global_quality; the vaapi variant additionally
// needs the device node and a hwupload filter chain. A copied video (explicit
// copy, or the satisfied rule) needs no device, so the args are the plain
// remux shape from every backend.
func (e *qsvEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy || videoShouldCopy(spec) {
		return buildCommand(spec, nil, nil)
	}
	codec := spec.Target.Video.Codec
	v := e.variantFor(codec)
	q := qualityFor(spec.Target.Video.Quality)
	if v.device == "" {
		video := []string{"-c:v", v.encoderName, "-global_quality", itoa(q.QSVGQ)}
		return buildCommand(spec, nil, video)
	}
	video := []string{"-c:v", v.encoderName, "-global_quality", itoa(q.QSVGQ), "-vf", "format=nv12,hwupload"}
	// h264_vaapi has no global_quality mode; -qp sets the fixed quantizer.
	if codec == flow.CodecH264 {
		video = []string{"-c:v", v.encoderName, "-qp", itoa(q.QSVGQ), "-vf", "format=nv12,hwupload"}
	}
	return buildCommand(spec, []string{"-vaapi_device", v.device}, video)
}
