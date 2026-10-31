// SPDX-License-Identifier: AGPL-3.0-only

//go:build !darwin

package encoders

import (
	"context"
	"errors"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// videotoolboxEncoder is the non-macOS stub (task 30): VideoToolbox exists
// only on macOS, but the encoder registry must compile and detect on every
// platform, and the capability list must stay honest — so the stub errors on
// detect and never reports a capability.
type videotoolboxEncoder struct{}

func (e *videotoolboxEncoder) Name() string { return "videotoolbox" }

func (e *videotoolboxEncoder) Codecs() []string {
	return []string{flow.CodecH264, flow.CodecHEVC}
}

var errVideotoolboxDarwinOnly = errors.New(
	"encoders: videotoolbox is only available on macOS")

// Detect always fails off macOS, so no non-macOS node ever reports
// videotoolbox, whatever ffmpeg build it runs.
func (e *videotoolboxEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
	return Capability{}, errVideotoolboxDarwinOnly
}

// detectWithEnv matches the batch-detection path (see detect.go).
func (e *videotoolboxEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	return nil, errVideotoolboxDarwinOnly
}

// SelfTest mirrors Detect: off macOS there is nothing to test.
func (e *videotoolboxEncoder) SelfTest(ctx context.Context) error {
	return errVideotoolboxDarwinOnly
}

// BuildArgs is unreachable in practice (the backend never passes detection
// off macOS, so the registry never contains it); it returns no arguments so
// a wiring bug surfaces as an ffmpeg failure and fallback reason instead of
// crashing the worker.
func (e *videotoolboxEncoder) BuildArgs(spec Spec) []string {
	return nil
}
