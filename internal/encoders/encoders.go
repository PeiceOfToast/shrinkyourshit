// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// Encoder is one encoder backend. Implementations are nvenc, qsv, vaapi,
// videotoolbox and cpu (see README.md for how to add more).
type Encoder interface {
	// Name is the stable backend identifier used in capabilities and job
	// records, for example "nvenc", "qsv" or "cpu".
	Name() string
	// Codecs lists the target codec names (hevc, av1, h264) the backend
	// can encode before detection narrows them down.
	Codecs() []string
	// Detect checks whether this backend is usable with the given ffmpeg
	// binary and returns its capability entry. An error means the backend
	// is not available on this machine. Detection parses
	// "ffmpeg -encoders" and "-hwaccels", checks hardware devices and runs
	// a one-second testsrc self-test per candidate codec.
	Detect(ctx context.Context, ffmpegPath string) (Capability, error)
	// SelfTest runs the one-second testsrc encode for the backend's
	// primary codec using "ffmpeg" from PATH. Detect runs the same test
	// against the binary it was given; SelfTest exists for standalone
	// checks (for example a "test hardware" button).
	SelfTest(ctx context.Context) error
	// BuildArgs returns the full ffmpeg argument list (without the binary
	// path) that applies the spec's Target to its Input and writes
	// spec.Output. The runner prepends the binary path.
	BuildArgs(spec Spec) []string
}

// Capability is the encoders part of the node capabilities shape from
// ARCHITECTURE.md: {encoders: [{name, codecs: []}], cpu_cores, ffmpeg_version}.
// Task 05 embeds it in the node record; this package owns the encoders part.
type Capability struct {
	Encoders []EncoderInfo `json:"encoders"`
}

// EncoderInfo is one detected backend: its name and the target codecs
// (hevc, av1, h264) that passed detection on this machine. The qsv backend
// additionally fills QSVFeatures with the optional features it probed
// (task 35); every other backend leaves it nil.
type EncoderInfo struct {
	Name   string   `json:"name"`
	Codecs []string `json:"codecs"`
	// QSVFeatures is the probed Intel QSV feature set of this machine;
	// omitted for non-qsv backends and when the backend detected nothing
	// probed. A probe that cannot run (no device, no ffmpeg) reports all
	// four features off rather than guessing.
	QSVFeatures *QSVFeatures `json:"qsv_features,omitempty"`
}

// QSVFeatures is what the qsv backend's capability probe found on this
// machine (task 35). The flags are optional in the driver, so detection
// tries each one with the standard one-second testsrc self-test and
// records the exit code; a feature is reported only when its self-test
// passed. Look-ahead is an H.264-only ffmpeg option, so it can only probe
// true for the h264 target; the node page and docs/intel-qsv.md say so in
// plain language.
type QSVFeatures struct {
	// LookAhead: -look_ahead 1 with extbrc — better quality at the same
	// size, slower.
	LookAhead bool `json:"look_ahead"`
	// ExtBRC: -extbrc 1 — external bitrate control, required by
	// look-ahead.
	ExtBRC bool `json:"extbrc"`
	// AsyncDepth: -async_depth N — more frames in flight, throughput
	// versus latency.
	AsyncDepth bool `json:"async_depth"`
	// LowPower: -low_power 1 — faster, slightly lower quality; works on
	// Ice Lake and newer.
	LowPower bool `json:"low_power"`
}

// find returns the detected entry for a backend name, or nil.
func (c Capability) find(name string) *EncoderInfo {
	for i := range c.Encoders {
		if c.Encoders[i].Name == name {
			return &c.Encoders[i]
		}
	}
	return nil
}

// Spec is one encode request: a probed file, the Target to apply and the
// stream selections (indexes into Info.Audio and Info.Subtitle, in stream
// order). Build the selections with flow.SelectAudio and
// flow.SelectSubtitles, or use NewSpec.
type Spec struct {
	Input  string
	Output string
	Info   probe.MediaInfo
	Target flow.Target
	// KeepAudio and KeepSubs are the stream indexes the output keeps.
	// Everything not kept counts as removed (subtitles.remove, or
	// keep_preferred tracks that do not match).
	KeepAudio []int
	KeepSubs  []int
}

// NewSpec fills a Spec from a probe result, using the flow selection
// helpers to compute KeepAudio and KeepSubs from the Target.
func NewSpec(input, output string, info probe.MediaInfo, target flow.Target) Spec {
	return Spec{
		Input:     input,
		Output:    output,
		Info:      info,
		Target:    target,
		KeepAudio: flow.SelectAudio(info, target),
		KeepSubs:  flow.SelectSubtitles(info, target),
	}
}

// The registered backends, in detection and fallback order: GPU first,
// cpu last. NVENC, QSV, VAAPI and CPU are singletons; qsv and vaapi keep
// detection state (their per-codec device/variant choices) on themselves.
// VideoToolbox is the macOS-only backend (task 30); off darwin it is the
// stub that fails detection, so it never reaches a capability list.
var (
	NVENC        Encoder = &nvencEncoder{}
	QSV          Encoder = &qsvEncoder{}
	VAAPI        Encoder = &vaapiEncoder{}
	VideoToolbox Encoder = &videotoolboxEncoder{}
	CPU          Encoder = &cpuEncoder{}
)

// allEncoders is the registration order for detection and registry
// building — the fallback precedence: nvenc, qsv, vaapi, videotoolbox,
// then cpu (README.md). Adding a backend means adding it here before cpu.
var allEncoders = []Encoder{NVENC, QSV, VAAPI, VideoToolbox, CPU}

// Registry returns the encoders to try for a Target, in order: detected GPU
// encoders whose codecs include the target video codec, then cpu. The cpu
// entry is always last and is returned even when caps lists nothing, so a
// caller can always fall back. For video.codec=copy no video encoding
// happens, so only cpu is needed.
//
// caps is the detected capability (from DetectAll). Filtering by the
// detected codec list means an encoder that failed its self-test, or that
// only supports some codecs in this ffmpeg build, is not offered here.
func Registry(target flow.Target, caps Capability) []Encoder {
	codec := target.Video.Codec
	var list []Encoder
	if codec != flow.CodecCopy {
		for _, enc := range allEncoders {
			if enc.Name() == CPU.Name() {
				continue // added below, always last
			}
			info := caps.find(enc.Name())
			if info == nil || !contains(info.Codecs, codec) {
				continue
			}
			list = append(list, enc)
		}
	}
	return append(list, CPU)
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
