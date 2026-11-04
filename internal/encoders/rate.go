// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"math"
	"strconv"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// Rate and preset helpers shared by the encoder builders (task 55). Quality
// mode is the existing scalar path and every builder keeps its vector; the
// helpers below only add arguments for crf and bitrate modes, the optional
// bitrate cap and the speed preset.

// rateMode is the rate mode a Target asks for. A nil Rate is quality.
func rateMode(r *flow.RateTarget) string {
	if r == nil || r.Mode == "" {
		return flow.RateModeQuality
	}
	return r.Mode
}

// rateModeSupported reports whether a backend can honour a rate mode. The
// VideoToolbox encoder has no CRF concept (its constant quality is -q:v on
// its own scale), so the registry does not offer it for crf; the chain falls
// back to a backend that can, and ultimately to cpu.
func rateModeSupported(encoderName, mode string) bool {
	return encoderName != "videotoolbox" || mode != flow.RateModeCRF
}

// scaleFilter is the scale filter for the spec, or "" when the resize does
// not change this file (no scale, already at the target height, unknown
// height, or upscale not allowed). The width is -2: aspect kept, even.
func scaleFilter(spec Spec) string {
	if len(spec.Info.Video) == 0 || !flow.ScaleApplies(spec.Info.Video[0].Height, spec.Scale) {
		return ""
	}
	return "scale=-2:" + itoa(spec.Scale.Height)
}

// withScale adds the scale filter to a video argument list. buildCommand calls
// it for every encoder, so the filter is applied once, in front of any chain
// the encoder already needs (for example "format=nv12,hwupload" for QSV and
// VAAPI): the result is one -vf value, scale first, in software.
func withScale(spec Spec, video []string) []string {
	f := scaleFilter(spec)
	if f == "" {
		return video
	}
	out := append([]string(nil), video...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == "-vf" {
			out[i+1] = f + "," + out[i+1]
			return out
		}
	}
	return append(out, "-vf", f)
}

// bitrateArgs is "-b:v <n>k" in bitrate mode and nothing otherwise.
func bitrateArgs(spec Spec) []string {
	if rateMode(spec.Rate) != flow.RateModeBitrate {
		return nil
	}
	return []string{"-b:v", itoa(spec.Rate.BitrateKBPS) + "k"}
}

// capArgs is the bitrate cap, "-maxrate <m>k -bufsize <2m>k", in crf and
// bitrate modes when MaxKBPS is set. Quality mode has no cap.
func capArgs(spec Spec) []string {
	r := spec.Rate
	if rateMode(r) == flow.RateModeQuality || r == nil || r.MaxKBPS <= 0 {
		return nil
	}
	return []string{"-maxrate", itoa(r.MaxKBPS) + "k", "-bufsize", itoa(2*r.MaxKBPS) + "k"}
}

// crfString is the CRF value for libx264, libx265 and libsvtav1. The float is
// written without trailing zeros: 22 stays "22", 22.5 becomes "22.5".
func crfString(spec Spec) string {
	return strconv.FormatFloat(spec.Rate.CRF, 'f', -1, 64)
}

// constantQuality rounds the CRF to the integer constant-quality scale of the
// hardware encoders: NVENC cq (0..51) and Intel/VAAPI global_quality (1..51).
// lo is the encoder's lowest accepted value.
func constantQuality(spec Spec, lo int) int {
	v := int(math.Round(spec.Rate.CRF))
	if v < lo {
		v = lo
	}
	if v > int(flow.CRFMax) {
		v = int(flow.CRFMax)
	}
	return v
}

// presetFor returns the mapped preset for a Target preset, falling back to
// the balanced default ("medium") when the Target leaves it empty. A value the
// map does not know also takes the default: Validate rejects such values, so
// this only guards a hand-built Spec.
func presetFor(spec Spec, table map[string]string) string {
	if v, ok := table[spec.Preset]; ok {
		return v
	}
	return table["medium"]
}

// svtAV1Presets maps the named Target presets onto libsvtav1's numeric scale
// (0 slowest and best, 13 fastest). medium is 6, the value quality mode uses.
var svtAV1Presets = map[string]string{
	"veryfast": "10", "faster": "9", "fast": "7", "medium": "6",
	"slow": "5", "slower": "4", "veryslow": "2",
}

// nvencPresets maps the named Target presets onto NVENC's p1..p7 scale (p1
// fastest). medium is p5, the value quality mode uses.
var nvencPresets = map[string]string{
	"veryfast": "p2", "faster": "p3", "fast": "p4", "medium": "p5",
	"slow": "p6", "slower": "p7", "veryslow": "p7",
}

// hwRateArgs is the video option list for the Intel QSV and VAAPI encoders
// that take a global quality value: "-c:v <name>" plus the rate options. In
// quality and crf modes the constant quality goes in qualityFlag
// (-global_quality, or -qp for h264 VAAPI); crf rounds the CRF onto the
// 1..51 scale. In bitrate mode the constant quality is left out, so the
// encoder runs on -b:v. The cap is appended last.
func hwRateArgs(spec Spec, encoderName, qualityFlag string, quality int) []string {
	args := []string{"-c:v", encoderName}
	switch rateMode(spec.Rate) {
	case flow.RateModeBitrate:
		args = append(args, bitrateArgs(spec)...)
	case flow.RateModeCRF:
		args = append(args, qualityFlag, itoa(constantQuality(spec, 1)))
	default:
		args = append(args, qualityFlag, itoa(quality))
	}
	return append(args, capArgs(spec)...)
}
