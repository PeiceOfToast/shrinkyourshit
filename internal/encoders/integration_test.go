// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package encoders

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// TestDetectRealFFmpeg runs detection against the installed ffmpeg. The
// machine the repo's CI runs on is CPU-only, so only cpu is expected; a GPU
// machine may list more, which must not fail the test.
func TestDetectRealFFmpeg(t *testing.T) {
	ffmpeg, _ := requireFFmpeg(t)

	cap, err := DetectAll(context.Background(), ffmpeg)
	require.NoError(t, err)

	cpuEntry := cap.find("cpu")
	require.NotNil(t, cpuEntry, "cpu encoder must always be detected")
	assert.NotEmpty(t, cpuEntry.Codecs)

	// Every reported backend must be GPU or cpu, and gpu encoders must
	// come before cpu (registry order follows detection order).
	for i, e := range cap.Encoders {
		assert.Contains(t, []string{"nvenc", "qsv", "cpu"}, e.Name)
		assert.NotEmpty(t, e.Codecs)
		if e.Name == "cpu" {
			assert.Equal(t, len(cap.Encoders)-1, i, "cpu is last")
		}
	}

	// The registry ends with cpu and only lists detected backends.
	list := Registry(flow.Default(), cap)
	got := registryNames(list)
	require.NotEmpty(t, got)
	assert.Equal(t, "cpu", got[len(got)-1])
	for _, name := range got {
		found := false
		for _, e := range cap.Encoders {
			if e.Name == name {
				found = true
			}
		}
		assert.True(t, found, "registry entry %q must be a detected encoder", name)
	}
}

// TestEncodeCPUOnly is the acceptance integration test: on a CPU-only
// machine, encode a 3-second testsrc fixture from h264 to hevc with quality
// step 3 and confirm with ffprobe: hevc video, correct track count.
func TestEncodeCPUOnly(t *testing.T) {
	ffmpeg, ffprobe := requireFFmpeg(t)
	dir := t.TempDir()

	input := generateH264Fixture(t, ffmpeg, dir)
	info, err := probe.Probe(context.Background(), ffprobe, input)
	require.NoError(t, err)
	require.Len(t, info.Video, 1)
	require.Len(t, info.Audio, 1)
	require.Len(t, info.Subtitle, 1)

	target := flow.Default() // hevc, quality 3, keep all, mkv
	target.Subtitles.Mode = flow.SubsRemove
	target.Subtitles.ExtractRemoved = true

	output := filepath.Join(dir, "output.mkv")
	spec := NewSpec(input, output, info, target)

	cap, err := DetectAll(context.Background(), ffmpeg)
	require.NoError(t, err)
	list := Registry(target, cap)

	run := func(ctx context.Context, args []string) error {
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		out, err := cmd.CombinedOutput() // stdout holds -progress lines, stderr the log
		if err != nil {
			t.Logf("ffmpeg failed: %s", out)
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := RunWithFallback(ctx, list, spec, run)
	require.NoError(t, err)
	assert.Equal(t, "cpu", res.EncoderUsed, "a CPU-only machine must end on cpu")
	assert.Empty(t, res.Fallbacks, "nothing should fail before cpu on this machine")

	// The extract commands are the runner's to execute (task 05); the
	// test plays that role to prove the emitted commands work.
	for _, ec := range ExtractCommands(spec) {
		cmd := exec.Command(ffmpeg, ec.Args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("extract command failed: %v\n%s", err, out)
		}
	}

	// ffprobe confirms the result: hevc video, both audio tracks kept,
	// the subtitle removed, and the extracted srt beside the output.
	outInfo, err := probe.Probe(context.Background(), ffprobe, output)
	require.NoError(t, err)
	require.Len(t, outInfo.Video, 1)
	assert.Equal(t, "hevc", outInfo.Video[0].Codec)
	assert.Len(t, outInfo.Audio, 1)
	assert.Equal(t, "aac", outInfo.Audio[0].Codec)
	assert.Empty(t, outInfo.Subtitle)

	srt, err := os.ReadFile(filepath.Join(dir, "output.eng.srt"))
	require.NoError(t, err, "extract_removed must write output.eng.srt")
	assert.Contains(t, string(srt), "Hello")

	// The original was never touched (rule 10: only the runner may
	// replace files, after verification).
	origInfo, err := probe.Probe(context.Background(), ffprobe, input)
	require.NoError(t, err)
	assert.Equal(t, "h264", origInfo.Video[0].Codec)
	assert.Len(t, origInfo.Subtitle, 1)
}

// generateH264Fixture builds a 3-second testsrc fixture: h264 video, one
// aac audio track and one srt subtitle, in mkv (AGENTS.md rule 12: test
// media is generated, never committed).
func generateH264Fixture(t *testing.T, ffmpeg, dir string) string {
	t.Helper()

	srtPath := filepath.Join(dir, "subs.srt")
	srt := "1\n00:00:00,000 --> 00:00:01,000\nHello\n\n2\n00:00:01,000 --> 00:00:02,000\nWorld\n"
	if err := os.WriteFile(srtPath, []byte(srt), 0o600); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	out := filepath.Join(dir, "input.mkv")
	args := slices.Concat(
		[]string{"-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc=duration=3:size=160x120:rate=10",
			"-f", "lavfi", "-i", "sine=duration=3",
			"-i", srtPath,
			"-map", "0:v", "-map", "1:a", "-map", "2:s",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "35", "-pix_fmt", "yuv420p",
			"-ac", "2", "-c:a", "aac", "-b:a", "64k", "-ar", "44100",
			"-c:s", "srt",
			"-metadata:s:a:0", "language=eng",
			"-metadata:s:s:0", "language=eng"},
		[]string{out},
	)
	cmd := exec.Command(ffmpeg, args...)
	if out2, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture generation failed: %v\n%s", err, out2)
	}
	return out
}

// requireFFmpeg is the integration-test gate (mirrors testutil, which is
// behind the same build tag but lives with the probe package).
func requireFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	var err error
	ffmpeg, err = exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping integration test: ffmpeg not found on PATH")
	}
	ffprobe, err = exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("skipping integration test: ffprobe not found on PATH")
	}
	return ffmpeg, ffprobe
}
