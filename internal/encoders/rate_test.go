// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// Task 55 tests. Every case asserts the exact argument vector, not just that
// the build succeeds. testInfo is a 1080p vp9 source, so a 720 scale applies
// and a 1080 or 2160 scale (without upscale) does not.

// rateTarget is the default hevc target at quality 3 with a rate, scale or
// preset set by the case.
func rateTarget(codec string, mutate func(*flow.Target)) flow.Target {
	tg := codecTarget(codec, 3)
	if mutate != nil {
		mutate(&tg)
	}
	return tg
}

func withScaleTo(h int) func(*flow.Target) {
	return func(tg *flow.Target) {
		tg.Video.Scale = &flow.ScaleTarget{Height: h, OnlyIfLarger: true}
	}
}

func withRate(r flow.RateTarget) func(*flow.Target) {
	return func(tg *flow.Target) { tg.Video.Rate = &r }
}

func withPreset(p string) func(*flow.Target) {
	return func(tg *flow.Target) { tg.Video.Preset = p }
}

// mkvHead is the fixed head of every mkv golden: keep-all maps are added by
// the caller, audio and subtitle codecs and the output file follow the video.
func mkvHead(videoArgs ...string) string {
	return join(append([]string{
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
	}, append(videoArgs, keepAllAudioCodecs, keepAllSubCodecs, "out.mkv")...)...)
}

// TestRateArgsCPUScale: the scale filter on the software encoders, and the
// golden that a source already at or under the target is not touched.
func TestRateArgsCPUScale(t *testing.T) {
	t.Run("hevc scaled down to 720", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecHEVC, withScaleTo(720))), mkvHead(
			"-c:v", "libx265", "-crf", "24", "-preset", "medium", "-vf", "scale=-2:720"))
	})
	t.Run("av1 scaled down to 720", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecAV1, withScaleTo(720))), mkvHead(
			"-c:v", "libsvtav1", "-crf", "32", "-preset", "6", "-vf", "scale=-2:720"))
	})
	t.Run("h264 scaled down to 720", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecH264, withScaleTo(720))), mkvHead(
			"-c:v", "libx264", "-crf", "22", "-preset", "medium", "-vf", "scale=-2:720"))
	})
	t.Run("golden: source that fits the scale keeps the pre-task-55 vector", func(t *testing.T) {
		want := argsString(CPU, withTarget(flow.Default()))
		for _, h := range []int{1080, 2160} { // equal height; target above source, no upscale
			got := argsString(CPU, withTarget(rateTarget(flow.CodecHEVC, withScaleTo(h))))
			assert.Equal(t, want, got, "scale %dp over a 1080p source", h)
		}
	})
	t.Run("upscale allowed scales a 1080 source up to 2160", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, func(tg *flow.Target) {
			tg.Video.Scale = &flow.ScaleTarget{Height: 2160, OnlyIfLarger: true, AllowUpscale: true}
		})
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx265", "-crf", "24", "-preset", "medium", "-vf", "scale=-2:2160"))
	})
	t.Run("explicit copy ignores the scale", func(t *testing.T) {
		tg := copyTarget()
		tg.Video.Scale = &flow.ScaleTarget{Height: 720, OnlyIfLarger: true}
		assert.Equal(t, argsString(CPU, withTarget(copyTarget())), argsString(CPU, withTarget(tg)))
	})
	t.Run("satisfied hevc source is re-encoded, not remuxed, when the scale applies", func(t *testing.T) {
		info := testInfo()
		info.Video[0].Codec = flow.CodecHEVC
		tg := rateTarget(flow.CodecHEVC, withScaleTo(720))
		sp := NewSpec("in.mkv", "out.mkv", info, tg)
		got := join(CPU.BuildArgs(sp)...)
		assert.Contains(t, got, "-c:v libx265 -crf 24 -preset medium -vf scale=-2:720")
		assert.NotContains(t, got, "-c:v copy")
	})
}

// argsString is BuildArgs joined into one string, for golden compares.
func argsString(enc Encoder, spec Spec) string {
	return join(enc.BuildArgs(spec)...)
}

// TestRateArgsCPUCRF: crf mode passes the Target CRF on every software codec,
// quality ignored; the cap is added after it.
func TestRateArgsCPUCRF(t *testing.T) {
	t.Run("hevc crf 22.5", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 22.5}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx265", "-crf", "22.5", "-preset", "medium"))
	})
	t.Run("h264 crf 20", func(t *testing.T) {
		tg := rateTarget(flow.CodecH264, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 20}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx264", "-crf", "20", "-preset", "medium"))
	})
	t.Run("av1 crf 30", func(t *testing.T) {
		tg := rateTarget(flow.CodecAV1, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 30}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libsvtav1", "-crf", "30", "-preset", "6"))
	})
	t.Run("crf with a 6000k cap", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 22, MaxKBPS: 6000}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx265", "-crf", "22", "-maxrate", "6000k", "-bufsize", "12000k", "-preset", "medium"))
	})
}

// TestRateArgsCPUBitrate: bitrate mode passes -b:v and no CRF; a cap adds
// -maxrate and a buffer of twice the cap.
func TestRateArgsCPUBitrate(t *testing.T) {
	t.Run("bitrate 4000k with a 6000k cap", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000, MaxKBPS: 6000}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx265", "-b:v", "4000k", "-maxrate", "6000k", "-bufsize", "12000k", "-preset", "medium"))
	})
	t.Run("bitrate 4000k uncapped", func(t *testing.T) {
		tg := rateTarget(flow.CodecH264, withRate(flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000}))
		assertArgs(t, CPU, withTarget(tg), mkvHead(
			"-c:v", "libx264", "-b:v", "4000k", "-preset", "medium"))
	})
}

// TestRateArgsCPUPreset: the named preset passes through to x264/x265, and
// maps onto SVT-AV1's numeric scale. The balanced default is unchanged.
func TestRateArgsCPUPreset(t *testing.T) {
	t.Run("slow on hevc", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecHEVC, withPreset("slow"))), mkvHead(
			"-c:v", "libx265", "-crf", "24", "-preset", "slow"))
	})
	t.Run("veryslow on h264", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecH264, withPreset("veryslow"))), mkvHead(
			"-c:v", "libx264", "-crf", "22", "-preset", "veryslow"))
	})
	t.Run("veryfast on av1 maps to svt preset 10", func(t *testing.T) {
		assertArgs(t, CPU, withTarget(rateTarget(flow.CodecAV1, withPreset("veryfast"))), mkvHead(
			"-c:v", "libsvtav1", "-crf", "32", "-preset", "10"))
	})
}

// TestRateArgsQSV: the qsv backend's hevc_qsv path with a crf, a bitrate cap,
// and a named preset; one case per concern, all on the plain QSV variant.
func TestRateArgsQSV(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()
	pin := func() { setQSVVariant(flow.CodecHEVC, qsvVariant{encoderName: "hevc_qsv"}) }

	t.Run("crf 22 maps to global_quality 22", func(t *testing.T) {
		pin()
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 22}))
		assertArgs(t, QSV, withTarget(tg), mkvHead(
			"-c:v", "hevc_qsv", "-global_quality", "22"))
	})
	t.Run("bitrate 4000k with a 6000k cap drops global_quality", func(t *testing.T) {
		pin()
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000, MaxKBPS: 6000}))
		assertArgs(t, QSV, withTarget(tg), mkvHead(
			"-c:v", "hevc_qsv", "-b:v", "4000k", "-maxrate", "6000k", "-bufsize", "12000k"))
	})
	t.Run("slow preset passes through as -preset", func(t *testing.T) {
		pin()
		assertArgs(t, QSV, withTarget(rateTarget(flow.CodecHEVC, withPreset("slow"))), mkvHead(
			"-c:v", "hevc_qsv", "-global_quality", "26", "-preset", "slow"))
	})
	t.Run("scale on the plain path is a software filter before the encoder", func(t *testing.T) {
		pin()
		assertArgs(t, QSV, withTarget(rateTarget(flow.CodecHEVC, withScaleTo(720))), mkvHead(
			"-c:v", "hevc_qsv", "-global_quality", "26", "-vf", "scale=-2:720"))
	})
}

// TestRateArgsVAAPI: the VAAPI backend takes -global_quality for hevc and
// -qp for h264, and never a preset. The scale joins its hwupload chain.
func TestRateArgsVAAPI(t *testing.T) {
	resetVAAPIVariants()
	defer resetVAAPIVariants()

	t.Run("crf 22 maps to global_quality", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 22}))
		got := VAAPI.BuildArgs(withTarget(tg))
		assert.Contains(t, join(got...), "-c:v hevc_vaapi -global_quality 22 -vf format=nv12,hwupload")
	})
	t.Run("h264 crf 20 maps to qp", func(t *testing.T) {
		tg := rateTarget(flow.CodecH264, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 20}))
		got := VAAPI.BuildArgs(withTarget(tg))
		assert.Contains(t, join(got...), "-c:v h264_vaapi -qp 20 -vf format=nv12,hwupload")
	})
	t.Run("bitrate with a cap", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000, MaxKBPS: 6000}))
		got := VAAPI.BuildArgs(withTarget(tg))
		assert.Contains(t, join(got...), "-c:v hevc_vaapi -b:v 4000k -maxrate 6000k -bufsize 12000k -vf format=nv12,hwupload")
	})
	t.Run("preset is skipped, scale merges into the hwupload chain", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, func(tg *flow.Target) {
			withPreset("slow")(tg)
			withScaleTo(720)(tg)
		})
		got := join(VAAPI.BuildArgs(withTarget(tg))...)
		assert.Contains(t, got, "-c:v hevc_vaapi -global_quality 26 -vf scale=-2:720,format=nv12,hwupload")
		assert.NotContains(t, got, "-preset")
	})
}

// TestRateArgsNVENC: NVENC's constant quality is -cq (the CRF value in crf
// mode), its preset is p-number, and bitrate mode is -b:v.
func TestRateArgsNVENC(t *testing.T) {
	t.Run("crf 20 maps to cq 20", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 20}))
		assertArgs(t, NVENC, withTarget(tg), mkvHead(
			"-c:v", "hevc_nvenc", "-cq", "20", "-preset", "p5"))
	})
	t.Run("bitrate 4000k with a 6000k cap", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, withRate(flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000, MaxKBPS: 6000}))
		assertArgs(t, NVENC, withTarget(tg), mkvHead(
			"-c:v", "hevc_nvenc", "-b:v", "4000k", "-maxrate", "6000k", "-bufsize", "12000k", "-preset", "p5"))
	})
	t.Run("veryslow maps to p7, scale appended", func(t *testing.T) {
		tg := rateTarget(flow.CodecHEVC, func(tg *flow.Target) {
			withPreset("veryslow")(tg)
			withScaleTo(720)(tg)
		})
		assertArgs(t, NVENC, withTarget(tg), mkvHead(
			"-c:v", "hevc_nvenc", "-cq", "28", "-preset", "p7", "-vf", "scale=-2:720"))
	})
}

// TestRateArgsRegistryRateMode: videotoolbox has no CRF, so the registry
// skips it for crf and falls back; bitrate and quality keep it.
func TestRateArgsRegistryRateMode(t *testing.T) {
	caps := Capability{Encoders: []EncoderInfo{
		{Name: "nvenc", Codecs: []string{flow.CodecHEVC}},
		{Name: "videotoolbox", Codecs: []string{flow.CodecHEVC}},
	}}
	names := func(tg flow.Target) []string {
		var out []string
		for _, e := range Registry(tg, caps) {
			out = append(out, e.Name())
		}
		return out
	}

	crf := flow.Default()
	crf.Video.Rate = &flow.RateTarget{Mode: flow.RateModeCRF, CRF: 22}
	assert.Equal(t, []string{"nvenc", "cpu"}, names(crf))

	bitrate := flow.Default()
	bitrate.Video.Rate = &flow.RateTarget{Mode: flow.RateModeBitrate, BitrateKBPS: 4000}
	assert.Equal(t, []string{"nvenc", "videotoolbox", "cpu"}, names(bitrate))

	assert.Equal(t, []string{"nvenc", "videotoolbox", "cpu"}, names(flow.Default()))
	assert.False(t, rateModeSupported("videotoolbox", flow.RateModeCRF))
	assert.True(t, rateModeSupported("nvenc", flow.RateModeCRF))
}

// TestRateArgsPresetAndScaleFieldsCopied: NewSpec copies the rate, scale and
// preset from the resolved Target, the same Target the runner encodes with.
func TestRateArgsPresetAndScaleFieldsCopied(t *testing.T) {
	tg := rateTarget(flow.CodecHEVC, func(tg *flow.Target) {
		withScaleTo(720)(tg)
		withPreset("fast")(tg)
		withRate(flow.RateTarget{Mode: flow.RateModeCRF, CRF: 21})(tg)
	})
	sp := NewSpec("in.mkv", "out.mkv", testInfo(), tg)
	require.NotNil(t, sp.Scale)
	assert.Equal(t, 720, sp.Scale.Height)
	require.NotNil(t, sp.Rate)
	assert.Equal(t, flow.RateModeCRF, sp.Rate.Mode)
	assert.Equal(t, "fast", sp.Preset)
}

// TestRateArgsFitsScaleGoldenPerEncoder: a source already at or under the
// scale target keeps the exact pre-task-55 vector on every backend.
func TestRateArgsFitsScaleGoldenPerEncoder(t *testing.T) {
	resetQSVVariants()
	resetVAAPIVariants()
	defer resetQSVVariants()
	defer resetVAAPIVariants()
	setQSVVariant(flow.CodecHEVC, qsvVariant{encoderName: "hevc_qsv"})

	for _, enc := range []Encoder{CPU, NVENC, QSV, VAAPI} {
		plain := argsString(enc, withTarget(flow.Default()))
		fits := argsString(enc, withTarget(rateTarget(flow.CodecHEVC, withScaleTo(1080))))
		assert.Equal(t, plain, fits, "%s: scale to the source height changed the vector", enc.Name())
	}
}
