// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// vaapiEncoder drives VAAPI encoding (task 30): the encoder path AMD cards
// and Intel Arc both take when the qsv stack is not usable (see
// README.md for the qsv/vaapi precedence and the AMF non-goal). Each codec
// is self-tested against each candidate render device until one works; the
// working device is recorded per codec and used by BuildArgs, exactly like
// the qsv backend's variant map.
//
// Devices the qsv backend already claimed for its own vaapi variants are
// skipped so one GPU is not reported twice.
type vaapiEncoder struct {
	// variants maps a target codec to the ffmpeg encoder (always the
	// <codec>_vaapi name) plus the device node detection chose. Guarded
	// by mu; written by detection, read by BuildArgs. A codec without an
	// entry defaults to the first render node.
	mu       sync.Mutex
	variants map[string]vaapiVariant
}

// vaapiVariant is the ffmpeg encoder setup chosen for one codec.
type vaapiVariant struct {
	// encoderName is the ffmpeg -c:v value.
	encoderName string
	// device is the VAAPI device node.
	device string
}

// defaultVaapiDevice is the device variantFor reports before detection has
// run. Real encodes always run after detection; this keeps BuildArgs
// deterministic for tests and standalone checks.
const defaultVaapiDevice = "/dev/dri/renderD128"

// vaapiEncoderFor, the codec-to-ffmpeg-encoder map, is shared with the qsv
// backend's vaapi fallback variant and lives in qsv.go.

func (e *vaapiEncoder) Name() string { return "vaapi" }

func (e *vaapiEncoder) Codecs() []string {
	return []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264}
}

// Detect probes the environment itself (the standalone entry point; batch
// detection calls detectWithEnv directly).
func (e *vaapiEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
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
// It requires the vaapi hwaccel, then self-tests each codec against each
// candidate render device until one works.
func (e *vaapiEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("encoders: vaapi is only usable on Linux")
	}
	if !env.hwaccels["vaapi"] {
		return nil, fmt.Errorf("encoders: ffmpeg does not list the vaapi hwaccel")
	}
	devices := e.candidateDevices(env)
	if len(devices) == 0 {
		return nil, fmt.Errorf("encoders: no VAAPI render device found")
	}

	claimed := QSV.(*qsvEncoder).claimedDevices()
	var usable []string
	for _, dev := range devices {
		if claimed[dev] {
			// The qsv backend already encodes on this device; reporting
			// vaapi here too would double count one GPU (README.md).
			continue
		}
		usable = append(usable, dev)
	}

	var codecs []string
	for _, codec := range e.Codecs() {
		name := vaapiEncoderFor[codec]
		if !env.encoders[name] {
			continue
		}
		for _, dev := range usable {
			if err := selfTest(ctx, env.ffmpegPath,
				[]string{"-vaapi_device", dev},
				[]string{"-vf", "format=nv12,hwupload", "-c:v", name}); err != nil {
				continue
			}
			e.setVariant(codec, vaapiVariant{encoderName: name, device: dev})
			codecs = append(codecs, codec)
			break
		}
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: vaapi self-test failed on all codecs")
	}
	return &EncoderInfo{Name: e.Name(), Codecs: codecs}, nil
}

// candidateDevices returns the render nodes to try, renderD128, renderD129
// and renderD130 in order (the numbering the kernel hands out in practice).
func (e *vaapiEncoder) candidateDevices(env detectEnv) []string {
	want := map[string]bool{"renderD128": true, "renderD129": true, "renderD130": true}
	var out []string
	for _, name := range env.driEntries {
		if want[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	res := make([]string, len(out))
	for i, name := range out {
		res[i] = filepath.Join(env.driDir, name)
	}
	return res
}

// setVariant records the working ffmpeg encoder and device for a codec.
func (e *vaapiEncoder) setVariant(codec string, v vaapiVariant) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.variants == nil {
		e.variants = map[string]vaapiVariant{}
	}
	e.variants[codec] = v
}

// variantFor returns the ffmpeg encoder and device for a codec, defaulting
// to the first render node when detection has not run.
func (e *vaapiEncoder) variantFor(codec string) vaapiVariant {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.variants[codec]; ok {
		return v
	}
	return vaapiVariant{encoderName: vaapiEncoderFor[codec], device: defaultVaapiDevice}
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH, using the device detection chose.
func (e *vaapiEncoder) SelfTest(ctx context.Context) error {
	v := e.variantFor(flow.CodecHEVC)
	return selfTest(ctx, "ffmpeg", []string{"-vaapi_device", v.device},
		[]string{"-vf", "format=nv12,hwupload", "-c:v", v.encoderName})
}

// BuildArgs maps the quality step onto the <codec>_vaapi encoder with the
// device detection chose. hevc_vaapi and av1_vaapi take the quality value as
// -global_quality; h264_vaapi has no global_quality mode, so -qp sets the
// fixed quantizer — the same rule the qsv backend's vaapi variant applies.
// A copied video (explicit copy, or the satisfied rule) needs no device, so
// the args are the plain remux shape from every backend.
func (e *vaapiEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy || videoShouldCopy(spec) {
		return buildCommand(spec, nil, nil)
	}
	codec := spec.Target.Video.Codec
	v := e.variantFor(codec)
	q := qualityFor(spec.Target.Video.Quality)
	qualityFlag := "-global_quality"
	if codec == flow.CodecH264 {
		qualityFlag = "-qp"
	}
	video := []string{
		"-c:v", v.encoderName, qualityFlag, strconv.Itoa(q.QSVGQ),
		"-vf", "format=nv12,hwupload",
	}
	return buildCommand(spec, []string{"-vaapi_device", v.device}, video)
}
