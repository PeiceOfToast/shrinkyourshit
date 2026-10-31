// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// vaapiOnly skips the test on platforms where VAAPI detection is gated off.
func vaapiOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("vaapi detection tests run on Linux, not %s", runtime.GOOS)
	}
}

// resetVAAPIVariants clears the device map the vaapi singleton accumulates,
// so golden expectations stay independent of test order.
func resetVAAPIVariants() {
	vaapi := VAAPI.(*vaapiEncoder)
	vaapi.mu.Lock()
	defer vaapi.mu.Unlock()
	vaapi.variants = nil
}

// fakeFFmpegFailingDevice writes a fake ffmpeg that fails every invocation
// whose arguments mention one of the devices (a driver that refuses that
// render node) and answers everything else.
func fakeFFmpegFailingDevice(t *testing.T, encoders, hwaccels, failDevices []string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("for a in \"$@\"; do\n  case \"$a\" in\n")
	for _, dev := range failDevices {
		b.WriteString("    " + shQuote(dev) + ") exit 1 ;;\n")
	}
	b.WriteString("  esac\ndone\n")
	// Reuse the standard listing/self-test behaviour for the rest.
	b.WriteString("mode=other\nnextcv=0\nenc=\n")
	b.WriteString("for a in \"$@\"; do\n")
	b.WriteString("  if [ \"$nextcv\" = 1 ]; then enc=\"$a\"; nextcv=0; continue; fi\n")
	b.WriteString("  case \"$a\" in\n")
	b.WriteString("    -encoders) mode=encoders ;;\n")
	b.WriteString("    -hwaccels) mode=hwaccels ;;\n")
	b.WriteString("    -c:v) nextcv=1 ;;\n")
	b.WriteString("  esac\ndone\n")
	b.WriteString("if [ \"$mode\" = encoders ]; then\n  cat <<'EOL'\n")
	for _, line := range encoders {
		b.WriteString(" V....D " + line + "\n")
	}
	b.WriteString("EOL\n  exit 0\nfi\n")
	b.WriteString("if [ \"$mode\" = hwaccels ]; then\n  printf 'Hardware acceleration methods:\\n")
	b.WriteString(strings.Join(hwaccels, "\\n"))
	b.WriteString("\\n'\n  exit 0\nfi\n")
	b.WriteString("exit 0\n")

	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

// fakeDRI creates a detector whose dri directory holds the named render
// nodes as plain files.
func fakeDRI(t *testing.T, nodes ...string) *detector {
	t.Helper()
	base := t.TempDir()
	d := newDetector()
	d.driDir = filepath.Join(base, "dri")
	d.nvidiaGlob = filepath.Join(base, "nvidia*")
	require.NoError(t, os.MkdirAll(d.driDir, 0o755))
	for _, n := range nodes {
		require.NoError(t, os.WriteFile(filepath.Join(d.driDir, n), nil, 0o600))
	}
	return d
}

// listVAAPI is the ffmpeg encoder list of a build with the vaapi encoders.
var listVAAPIFull = []string{"hevc_vaapi", "av1_vaapi", "h264_vaapi"}

// TestDetectVAAPIOnly: the only usable device is the second render node —
// the first one refuses every encode (broken driver), which also stops the
// qsv backend's vaapi fallback. vaapi is detected with exactly the codecs
// that passed their self-test, and the working device is recorded for
// BuildArgs.
func TestDetectVAAPIOnly(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128", "renderD129")
	script := fakeFFmpegFailingDevice(t,
		append(append([]string{}, listCPUOnly...), listVAAPIFull...),
		[]string{"vaapi"}, []string{filepath.Join(d.driDir, "renderD128")})

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"vaapi", "cpu"}, names(cap))
	require.Len(t, cap.Encoders, 2)
	assert.Equal(t, []string{"hevc", "av1", "h264"}, cap.Encoders[0].Codecs)

	assert.Equal(t, []string{"vaapi", "cpu"}, registryNames(Registry(hevcTarget(), cap)))

	vaapi := VAAPI.(*vaapiEncoder)
	v := vaapi.variantFor(flow.CodecHEVC)
	assert.Equal(t, "hevc_vaapi", v.encoderName)
	assert.Equal(t, filepath.Join(d.driDir, "renderD129"), v.device)
	resetVAAPIVariants()
}

// TestDetectVAAPI NeedsHwaccel: without vaapi in -hwaccels, no ffmpeg
// process is spawned for a self-test and the backend is not reported.
func TestDetectVAAPINeedsHwaccel(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128")
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listVAAPIFull...),
		hwaccelsNone, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
	assert.NotContains(t, names(cap), "vaapi")
	resetVAAPIVariants()
}

// TestDetectVAAPINeedsRenderDevice: without a renderD node the backend
// fails without running any self-test.
func TestDetectVAAPINeedsRenderDevice(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t) // /dev/dri exists but holds no render node
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listVAAPIFull...),
		[]string{"vaapi"}, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))

	// No invocation carried a device: the device check failed before any
	// self-test could run.
	data, err := os.ReadFile(fakeLogs[script])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "-vaapi_device")
	resetVAAPIVariants()
}

// TestDetectVAAPISkipsQSVCclaimedDevice: the qsv backend fell back to the
// vaapi variant on the only render node — the vaapi backend must not report
// the same device again (no double count).
func TestDetectVAAPISkipsQSVClaimedDevice(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128")
	script := fakeFFmpeg(t, append(append([]string{}, listCPUOnly...),
		listVAAPIFull...), []string{"qsv", "vaapi"}, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"qsv", "cpu"}, names(cap),
		"qsv claimed the render node via its vaapi variant; a second report would double count")

	qsv := QSV.(*qsvEncoder)
	require.NotEmpty(t, qsv.claimedDevices())
	resetQSVVariants()
	resetVAAPIVariants()
}

// TestDetectVAAPISecondDeviceAfterQSVCclaim: a machine with two render nodes
// where qsv claimed the first — vaapi still reports the second one.
func TestDetectVAAPISecondDeviceAfterQSVCclaim(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128", "renderD129")
	script := fakeFFmpeg(t, append(append([]string{}, listCPUOnly...),
		listVAAPIFull...), []string{"qsv", "vaapi"}, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"qsv", "vaapi", "cpu"}, names(cap))

	vaapi := VAAPI.(*vaapiEncoder)
	v := vaapi.variantFor(flow.CodecHEVC)
	assert.Equal(t, filepath.Join(d.driDir, "renderD129"), v.device,
		"vaapi must take the device qsv did not claim")
	resetQSVVariants()
	resetVAAPIVariants()
}

// TestDetectVAAPILegitimatelyWithQSV: qsv working through its own qsv
// encoders claims no device node — vaapi reporting the same machine is
// legitimate (the README states this).
func TestDetectVAAPILegitimatelyWithQSV(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128")
	list := append(append([]string{}, listCPUOnly...), listQSV...)
	list = append(list, listVAAPIFull...)
	script := fakeFFmpeg(t, list, []string{"qsv", "vaapi"}, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"qsv", "vaapi", "cpu"}, names(cap))
	resetQSVVariants()
	resetVAAPIVariants()
}

// TestDetectVAAPITriesNextDevice: renderD128 refuses every encode (broken
// driver), renderD129 works — each codec is recorded on the first device
// that passed, in the documented 128, 129, 130 order. The first failure
// also stops the qsv backend's vaapi fallback, which only ever tries the
// first render node, so this is a pure vaapi machine.
func TestDetectVAAPITriesNextDevice(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128", "renderD129")
	script := fakeFFmpegFailingDevice(t, listVAAPIFull, []string{"vaapi"},
		[]string{filepath.Join(d.driDir, "renderD128")})

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	require.Contains(t, names(cap), "vaapi")
	require.NotContains(t, names(cap), "qsv")

	vaapi := VAAPI.(*vaapiEncoder)
	v := vaapi.variantFor(flow.CodecH264)
	assert.Equal(t, filepath.Join(d.driDir, "renderD129"), v.device)
	resetVAAPIVariants()
}

// TestDetectVAAPISelfTestFails: a device that fails every self-test is not
// reported at all.
func TestDetectVAAPISelfTestFails(t *testing.T) {
	vaapiOnly(t)
	d := fakeDRI(t, "renderD128")
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listVAAPIFull...),
		[]string{"vaapi"}, listVAAPIFull)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
	resetVAAPIVariants()
}

// TestBuildArgsVAAPIQualitySteps: the QSV global_quality column drives both
// VAAPI flags — -global_quality for hevc and av1, -qp for h264 (h264_vaapi
// has no global_quality mode), one golden per step for hevc.
func TestBuildArgsVAAPIQualitySteps(t *testing.T) {
	resetVAAPIVariants()
	defer resetVAAPIVariants()

	gqs := []struct{ step, gq string }{
		{"1", "30"}, {"2", "28"}, {"3", "26"}, {"4", "24"}, {"5", "22"},
	}
	for _, c := range gqs {
		t.Run("hevc step "+c.step, func(t *testing.T) {
			tg := flow.Default()
			tg.Video.Quality = mustStep(t, c.step)
			assertArgs(t, VAAPI, withTarget(tg), join(
				"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
				"-vaapi_device", "/dev/dri/renderD128",
				"-i", "in.mkv",
				keepAllMaps,
				"-c:v", "hevc_vaapi", "-global_quality", c.gq, "-vf", "format=nv12,hwupload",
				keepAllAudioCodecs, keepAllSubCodecs,
				"out.mkv"))
		})
	}

	assertArgs(t, VAAPI, withTarget(codecTarget(flow.CodecH264, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD128",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "h264_vaapi", "-qp", "26", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
	assertArgs(t, VAAPI, withTarget(codecTarget(flow.CodecAV1, 3)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD128",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "av1_vaapi", "-global_quality", "26", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsVAAPIChosenDevice: the device detection recorded per codec is
// the one the args use.
func TestBuildArgsVAAPIChosenDevice(t *testing.T) {
	resetVAAPIVariants()
	defer resetVAAPIVariants()

	VAAPI.(*vaapiEncoder).setVariant(flow.CodecHEVC, vaapiVariant{
		encoderName: "hevc_vaapi", device: "/dev/dri/renderD129"})
	assertArgs(t, VAAPI, withTarget(codecTarget(flow.CodecHEVC, 2)), join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-vaapi_device", "/dev/dri/renderD129",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "hevc_vaapi", "-global_quality", "28", "-vf", "format=nv12,hwupload",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv"))
}

// TestBuildArgsVAAPICopyVideo: video.codec=copy is device free, like every
// other backend.
func TestBuildArgsVAAPICopyVideo(t *testing.T) {
	resetVAAPIVariants()
	defer resetVAAPIVariants()

	want := join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "copy",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv")
	assertArgs(t, VAAPI, withTarget(copyTarget()), want)
}

// TestBuildArgsSatisfiedRuleSkipsDevice: a file whose video already is the
// target codec is remuxed (DECISIONS.md B2), and a remux needs no VAAPI
// device — the args must match the plain remux shape of every backend, also
// for the qsv backend's vaapi variant. This is what keeps the parity gate
// byte-identical between the direct Target path and the compiled flow.
func TestBuildArgsSatisfiedRuleSkipsDevice(t *testing.T) {
	resetVAAPIVariants()
	resetQSVVariants()
	defer resetVAAPIVariants()
	defer resetQSVVariants()

	alreadyHEVC := testInfo()
	alreadyHEVC.Video[0].Codec = "hevc"

	device := "/dev/dri/renderD128"
	VAAPI.(*vaapiEncoder).setVariant(flow.CodecHEVC, vaapiVariant{
		encoderName: "hevc_vaapi", device: device})
	QSV.(*qsvEncoder).setVariant(flow.CodecHEVC, qsvVariant{
		encoderName: "hevc_vaapi", device: device})

	want := join(
		"-hide_banner", "-nostdin", "-progress", "pipe:1", "-nostats", "-y",
		"-i", "in.mkv",
		keepAllMaps,
		"-c:v", "copy",
		keepAllAudioCodecs, keepAllSubCodecs,
		"out.mkv")
	spec := NewSpec("in.mkv", "out.mkv", alreadyHEVC, flow.Default())
	assertArgs(t, VAAPI, spec, want)
	assertArgs(t, QSV, spec, want)
}

// TestEncoderPrecedence: the registry follows the documented precedence
// nvenc, qsv, vaapi, then cpu (README.md) — matched by a test, per the task
// acceptance.
func TestEncoderPrecedence(t *testing.T) {
	caps := Capability{Encoders: []EncoderInfo{
		{Name: "nvenc", Codecs: []string{"hevc", "av1", "h264"}},
		{Name: "qsv", Codecs: []string{"hevc", "av1", "h264"}},
		{Name: "vaapi", Codecs: []string{"hevc", "av1", "h264"}},
		{Name: "cpu", Codecs: []string{"hevc", "av1", "h264"}},
	}}
	for _, codec := range []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264} {
		tg := hevcTarget()
		tg.Video.Codec = codec
		assert.Equal(t, []string{"nvenc", "qsv", "vaapi", "cpu"},
			registryNames(Registry(tg, caps)), "codec %s", codec)
	}

	// A failing GPU encoder falls through in the same order: every failure
	// is recorded as a fallback reason until cpu's final failure ends it.
	list := Registry(hevcTarget(), caps)
	res, err := RunWithFallback(context.Background(), list, qsvSpecFixture(),
		func(_ context.Context, _ []string) error { return assert.AnError })
	require.Error(t, err)
	var tried []string
	for _, fr := range res.Fallbacks {
		tried = append(tried, fr.Encoder)
	}
	assert.Equal(t, []string{"nvenc", "qsv", "vaapi"}, tried,
		"the GPU encoders were tried in precedence order")
}
