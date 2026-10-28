// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

// Package testutil generates ffmpeg test media at test time. Nothing is
// committed: fixtures are built with lavfi sources per AGENTS.md rule 12.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// RequireFFmpeg skips the test with a clear message when ffmpeg or ffprobe
// is not installed. CI installs both before running integration tests.
func RequireFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping integration test: ffmpeg not found on PATH")
	}
	ffprobe, err = exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("skipping integration test: ffprobe not found on PATH")
	}
	return ffmpeg, ffprobe
}

// Fixture paths returned by GenerateFixtures.
type Fixtures struct {
	H264TwoAudioSubtitleMKV string // h264 + aac(eng) + eac3(ger) + srt(eng) in mkv
	HEVCAudioSubtitleMKV    string // hevc + aac(eng) + srt(eng) in mkv
	H264AudioMP4            string // h264 + aac(eng) in mp4
	HDRH264MKV              string // h264 with HDR10 signaling (smpte2084)
}

// GenerateFixtures builds the shared test media into dir. Videos are tiny
// (2 s, 160x120, ultrafast) so the whole set encodes in seconds.
func GenerateFixtures(t *testing.T, ffmpeg, dir string) Fixtures {
	t.Helper()

	// A minimal srt subtitle file (text, not media).
	srtPath := filepath.Join(dir, "subs.srt")
	srt := "1\n00:00:00,000 --> 00:00:01,000\nHello\n\n2\n00:00:01,000 --> 00:00:02,000\nWorld\n"
	if err := os.WriteFile(srtPath, []byte(srt), 0o600); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	f := Fixtures{
		H264TwoAudioSubtitleMKV: filepath.Join(dir, "h264-two-audio-subtitle.mkv"),
		HEVCAudioSubtitleMKV:    filepath.Join(dir, "hevc-audio-subtitle.mkv"),
		H264AudioMP4:            filepath.Join(dir, "h264-audio.mp4"),
		HDRH264MKV:              filepath.Join(dir, "h264-hdr.mkv"),
	}

	video := []string{"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10"}
	audio := []string{"-f", "lavfi", "-i", "sine=duration=2"}
	common := []string{"-hide_banner", "-loglevel", "error", "-y"}

	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg %v failed: %v\n%s", args, err, out)
		}
	}

	// A: h264 + two audio languages + one text subtitle, in mkv.
	run(slices.Concat(common, video, audio, audio,
		[]string{"-i", srtPath,
			"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "35", "-pix_fmt", "yuv420p",
			"-ac", "2", "-c:a", "aac", "-b:a", "64k", "-ar", "44100",
			"-c:s", "srt",
			"-metadata:s:a:0", "language=eng", "-metadata:s:a:0", "title=English",
			"-metadata:s:a:1", "language=ger", "-metadata:s:a:1", "title=Deutsch",
			"-metadata:s:s:0", "language=eng",
			f.H264TwoAudioSubtitleMKV},
	)...)

	// B: hevc + one audio track + one text subtitle, in mkv.
	run(slices.Concat(common, video, audio,
		[]string{"-i", srtPath,
			"-map", "0:v", "-map", "1:a", "-map", "2:s",
			"-c:v", "libx265", "-preset", "ultrafast", "-crf", "40", "-pix_fmt", "yuv420p",
			"-ac", "2", "-c:a", "aac", "-b:a", "64k", "-ar", "44100",
			"-c:s", "srt",
			"-metadata:s:a:0", "language=eng", "-metadata:s:s:0", "language=eng",
			f.HEVCAudioSubtitleMKV},
	)...)

	// C: h264 + aac in mp4.
	run(slices.Concat(common, video, audio,
		[]string{"-map", "0:v", "-map", "1:a",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "35", "-pix_fmt", "yuv420p",
			"-ac", "2", "-c:a", "aac", "-b:a", "64k", "-ar", "44100",
			f.H264AudioMP4},
	)...)

	// D: h264 with HDR10 color transfer signaling.
	run(slices.Concat(common, video,
		[]string{"-map", "0:v",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "40", "-pix_fmt", "yuv420p",
			"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
			f.HDRH264MKV},
	)...)

	return f
}
