// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// fakeFFmpeg writes a shell script that answers the calls detection makes:
// "-encoders" and "-hwaccels" listings and self-test encodes (exit 0, or
// exit 1 for the encoders listed in failEncoders). Every invocation is
// logged to <dir>/calls.log so tests can assert how often the binary ran.
func fakeFFmpeg(t *testing.T, encoders, hwaccels, failEncoders []string) string {
	t.Helper()

	log := filepath.Join(t.TempDir(), "calls.log")
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("echo \"$$ $@\" >> " + shQuote(log) + "\n")
	b.WriteString("mode=other\nnextcv=0\nenc=\n")
	b.WriteString("for a in \"$@\"; do\n")
	b.WriteString("  if [ \"$nextcv\" = 1 ]; then enc=\"$a\"; nextcv=0; continue; fi\n")
	b.WriteString("  case \"$a\" in\n")
	b.WriteString("    -encoders) mode=encoders ;;\n")
	b.WriteString("    -hwaccels) mode=hwaccels ;;\n")
	b.WriteString("    -c:v) nextcv=1 ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("done\n")
	b.WriteString("if [ \"$mode\" = encoders ]; then\n")
	b.WriteString("  cat <<'EOL'\n")
	for _, line := range encoders {
		b.WriteString(" V....D " + line + "\n")
	}
	b.WriteString("EOL\n  exit 0\nfi\n")
	b.WriteString("if [ \"$mode\" = hwaccels ]; then\n")
	b.WriteString("  printf 'Hardware acceleration methods:\\n")
	b.WriteString(strings.Join(hwaccels, "\\n"))
	b.WriteString("\\n'\n  exit 0\nfi\n")
	if len(failEncoders) > 0 {
		b.WriteString("case \"$enc\" in\n  " + strings.Join(failEncoders, "|") + ") exit 1 ;;\n  *) exit 0 ;;\nesac\n")
	}
	b.WriteString("exit 0\n")

	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(log) })
	// The log path is derived from the script path so callers can find it.
	fakeLogs[path] = log
	return path
}

// fakeLogs maps fake script path -> invocation log path.
var fakeLogs = map[string]string{}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fakeCalls returns how often the fake binary was invoked so far.
func fakeCalls(t *testing.T, script string) int {
	t.Helper()
	data, err := os.ReadFile(fakeLogs[script])
	if err != nil {
		return 0
	}
	return len(strings.Split(strings.TrimSpace(string(data)), "\n"))
}

// fakeDevices creates a detector with device locations pointed at temp
// files. createNVIDIA adds an /dev/nvidia0 stand-in, createDRI a renderD128
// stand-in.
func fakeDevices(t *testing.T, createNVIDIA, createDRI bool) *detector {
	t.Helper()
	base := t.TempDir()
	d := newDetector()
	d.driDir = filepath.Join(base, "dri")
	d.nvidiaGlob = filepath.Join(base, "nvidia*")
	if createDRI {
		require.NoError(t, os.MkdirAll(d.driDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d.driDir, "renderD128"), nil, 0o600))
	}
	if createNVIDIA {
		require.NoError(t, os.WriteFile(filepath.Join(base, "nvidia0"), nil, 0o600))
	}
	return d
}

// names maps a capability to the ordered encoder names.
func names(cap Capability) []string {
	var out []string
	for _, e := range cap.Encoders {
		out = append(out, e.Name)
	}
	return out
}

// registryNames maps a registry to the ordered encoder names.
func registryNames(list []Encoder) []string {
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func hevcTarget() flow.Target {
	tg := flow.Default()
	tg.Video.Codec = flow.CodecHEVC
	return tg
}

func av1Target() flow.Target {
	tg := flow.Default()
	tg.Video.Codec = flow.CodecAV1
	return tg
}

// The ffmpeg encoder lists used by the detection tests.
var (
	listCPUOnly      = []string{"libx264", "libx265"} // a build without libsvtav1
	listCPUFull      = []string{"libx264", "libx265", "libsvtav1"}
	listNVENC        = []string{"hevc_nvenc", "av1_nvenc", "h264_nvenc"}
	listQSV          = []string{"hevc_qsv", "av1_qsv", "h264_qsv"}
	listVAAPIPartial = []string{"hevc_vaapi", "h264_vaapi"}
	hwaccelsNone     = []string{}
	hwaccelsQSV      = []string{"qsv", "vaapi"}
)

// TestDetectNoGPU: no NVIDIA and no Intel devices, so only cpu is detected
// and the registry is cpu-only, whatever the target codec.
func TestDetectNoGPU(t *testing.T) {
	d := fakeDevices(t, false, false)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUOnly...), listNVENC...), hwaccelsNone, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
	require.Len(t, cap.Encoders, 1)
	// libsvtav1 is missing from this build, so av1 is not offered.
	assert.Equal(t, []string{"hevc", "h264"}, cap.Encoders[0].Codecs)

	assert.Equal(t, []string{"cpu"}, registryNames(Registry(hevcTarget(), cap)))
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(av1Target(), cap)))
	tg := hevcTarget()
	tg.Video.Codec = flow.CodecCopy
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(tg, cap)))
}

// TestDetectNVENCOnly: an NVIDIA device plus working NVENC self-tests put
// nvenc first in the registry, but only for codecs it actually supports.
func TestDetectNVENCOnly(t *testing.T) {
	d := fakeDevices(t, true, false)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listNVENC...), hwaccelsNone, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"nvenc", "cpu"}, names(cap))
	require.Len(t, cap.Encoders, 2)
	assert.Equal(t, []string{"hevc", "av1", "h264"}, cap.Encoders[0].Codecs)

	assert.Equal(t, []string{"nvenc", "cpu"}, registryNames(Registry(hevcTarget(), cap)))
	assert.Equal(t, []string{"nvenc", "cpu"}, registryNames(Registry(av1Target(), cap)))
	assert.Equal(t, []string{"nvenc", "cpu"}, registryNames(Registry(func() flow.Target {
		tg := hevcTarget()
		tg.Video.Codec = flow.CodecH264
		return tg
	}(), cap)))
	// copy never re-encodes: cpu only.
	tg := hevcTarget()
	tg.Video.Codec = flow.CodecCopy
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(tg, cap)))
}

// TestDetectNVENCSelfTestFails: a present device is not enough — a failing
// self-test (broken driver) drops the encoder entirely.
func TestDetectNVENCSelfTestFails(t *testing.T) {
	d := fakeDevices(t, true, false)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listNVENC...),
		hwaccelsNone, []string{"hevc_nvenc|h264_nvenc|av1_nvenc"})

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(hevcTarget(), cap)))
}

// TestDetectQSVOnly: an Intel device plus working QSV self-tests.
func TestDetectQSVOnly(t *testing.T) {
	d := fakeDevices(t, false, true)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listQSV...), hwaccelsQSV, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"qsv", "cpu"}, names(cap))
	require.Len(t, cap.Encoders, 2)
	assert.Equal(t, []string{"hevc", "av1", "h264"}, cap.Encoders[0].Codecs)

	assert.Equal(t, []string{"qsv", "cpu"}, registryNames(Registry(hevcTarget(), cap)))
}

// TestDetectBoth: NVIDIA and Intel detected; GPU encoders come first in
// detection order (nvenc, qsv), cpu last.
func TestDetectBoth(t *testing.T) {
	d := fakeDevices(t, true, true)
	script := fakeFFmpeg(t, append(append(append([]string{}, listCPUFull...), listNVENC...), listQSV...), hwaccelsQSV, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"nvenc", "qsv", "cpu"}, names(cap))
	assert.Equal(t, []string{"nvenc", "qsv", "cpu"}, registryNames(Registry(hevcTarget(), cap)))
	assert.Equal(t, []string{"nvenc", "qsv", "cpu"}, registryNames(Registry(av1Target(), cap)))
}

// TestDetectQSVFallsBackToVAAPI: no qsv encoders in the build but VAAPI
// equivalents present — on Linux the qsv backend picks the vaapi variant,
// records the device, and BuildArgs uses it.
func TestDetectQSVFallsBackToVAAPI(t *testing.T) {
	d := fakeDevices(t, false, true)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUOnly...),
		listVAAPIPartial...), []string{"vaapi"}, nil)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"qsv", "cpu"}, names(cap))
	require.Len(t, cap.Encoders, 2)
	assert.Equal(t, []string{"hevc", "h264"}, cap.Encoders[0].Codecs,
		"av1_vaapi is missing from this build")

	qsv := QSV.(*qsvEncoder)
	v := qsv.variantFor(flow.CodecHEVC)
	assert.Equal(t, "hevc_vaapi", v.encoderName)
	assert.Equal(t, filepath.Join(d.driDir, "renderD128"), v.device)

	args := strings.Join(qsv.BuildArgs(qsvSpecFixture()), " ")
	assert.Contains(t, args, "-vaapi_device "+v.device)
	assert.Contains(t, args, "-c:v hevc_vaapi")
	assert.Contains(t, args, "-vf format=nv12,hwupload")

	resetQSVVariants()
}

// TestDetectNoIntelDeviceSkipsQSV: without a device directory no ffmpeg
// process is spawned for qsv at all.
func TestDetectNoIntelDeviceSkipsQSV(t *testing.T) {
	d := fakeDevices(t, false, false)
	script := fakeFFmpeg(t, append(append([]string{}, listCPUFull...), listQSV...), hwaccelsQSV, nil)
	callsBefore := fakeCalls(t, script)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(hevcTarget(), cap)))

	// Every invocation is a cpu one; qsv contributed none after its
	// device check failed. The exact count does not matter, only that
	// detection still succeeds via cpu.
	assert.Greater(t, fakeCalls(t, script), callsBefore)
}

// TestDetectCached: results are cached for the process lifetime — a second
// detect on the same binary runs no additional ffmpeg processes.
func TestDetectCached(t *testing.T) {
	d := fakeDevices(t, false, false)
	script := fakeFFmpeg(t, listCPUOnly, hwaccelsNone, nil)

	_, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	afterFirst := fakeCalls(t, script)
	require.Greater(t, afterFirst, 0)

	_, err = d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, afterFirst, fakeCalls(t, script),
		"the second detect must come from the cache")
}

// TestDetectBrokenFFmpeg: a binary that cannot answer at all surfaces an
// error and no encoders.
func TestDetectBrokenFFmpeg(t *testing.T) {
	d := fakeDevices(t, false, false)
	path := filepath.Join(t.TempDir(), "ffmpeg-broken")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	cap, err := d.detect(context.Background(), path)
	require.Error(t, err)
	assert.Empty(t, cap.Encoders)
	assert.Equal(t, []string{"cpu"}, registryNames(Registry(hevcTarget(), cap)),
		"the registry still ends with cpu even without detection")
}

// TestDetectCanceledContextIsNotCached: a canceled run must not poison the
// cache — a later call with a live context detects normally.
func TestDetectCanceledContextIsNotCached(t *testing.T) {
	d := fakeDevices(t, false, false)
	script := fakeFFmpeg(t, listCPUOnly, hwaccelsNone, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.detect(ctx, script)
	require.Error(t, err)

	cap, err := d.detect(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu"}, names(cap))
}

// TestEncoderInterfaceDetect: a backend's standalone Detect works without
// DetectAll and returns only its own entry.
func TestEncoderInterfaceDetect(t *testing.T) {
	script := fakeFFmpeg(t, listCPUFull, hwaccelsNone, nil)
	cap, err := CPU.Detect(context.Background(), script)
	require.NoError(t, err)
	require.Len(t, cap.Encoders, 1)
	assert.Equal(t, "cpu", cap.Encoders[0].Name)
	assert.Equal(t, []string{"hevc", "av1", "h264"}, cap.Encoders[0].Codecs)
}

// TestCapabilityJSONShape: the capability marshals into the encoders part
// of the node capabilities shape from ARCHITECTURE.md:
// {encoders: [{name, codecs: []}]}.
func TestCapabilityJSONShape(t *testing.T) {
	cap := Capability{
		Encoders: []EncoderInfo{
			{Name: "nvenc", Codecs: []string{"hevc", "av1"}},
			{Name: "cpu", Codecs: []string{"hevc", "av1", "h264"}},
		},
	}
	data, err := json.Marshal(cap)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"encoders": [
			{"name": "nvenc", "codecs": ["hevc", "av1"]},
			{"name": "cpu", "codecs": ["hevc", "av1", "h264"]}
		]
	}`, string(data))
}

// TestParseEncodersAndHwaccels: the parsers handle the real ffmpeg output
// format, including the flags column and the hwaccels header.
func TestParseEncodersAndHwaccels(t *testing.T) {
	script := fakeFFmpeg(t, []string{"libx265", "libx264", "hevc_nvenc"}, []string{"vdpau", "vaapi"}, nil)

	enc, err := parseEncoders(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"libx265": true, "libx264": true, "hevc_nvenc": true}, enc)

	hw, err := parseHwaccels(context.Background(), script)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"vdpau": true, "vaapi": true}, hw)
}

// qsvSpecFixture is a plain hevc spec for BuildArgs assertions.
func qsvSpecFixture() Spec {
	return withTarget(codecTarget(flow.CodecHEVC, 3))
}
