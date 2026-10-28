// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"
)

// RunFn executes one ffmpeg command: it receives the full argument list
// (without the binary path) and returns an error when the process fails.
// It is injectable so tests can fake failures and the runner can own
// process handling, progress parsing and cancellation.
type RunFn func(ctx context.Context, args []string) error

// FallbackReason records one GPU encoder that failed and why. The reasons
// land in the job record's fallback_reasons (ARCHITECTURE.md).
type FallbackReason struct {
	Encoder string `json:"encoder"`
	Error   string `json:"error"`
}

// Result reports which encoder produced the output and which fallbacks
// happened on the way.
type Result struct {
	// EncoderUsed is the Name() of the encoder that succeeded.
	EncoderUsed string `json:"encoder_used"`
	// Fallbacks lists the GPU encoders that failed before the success,
	// in try order.
	Fallbacks []FallbackReason `json:"fallback_reasons"`
}

// RunWithFallback tries the encoders in order, building args per encoder
// and running them through runFn. A GPU encoder failure records a fallback
// reason and moves to the next encoder; the encoder named "cpu" is the
// final fallback, and its failure ends the run with an error. On success it
// returns the result with the used encoder and the reasons collected so far.
func RunWithFallback(ctx context.Context, list []Encoder, spec Spec, run RunFn) (Result, error) {
	var res Result
	for _, enc := range list {
		err := run(ctx, enc.BuildArgs(spec))
		if err == nil {
			res.EncoderUsed = enc.Name()
			return res, nil
		}
		if enc.Name() == CPU.Name() {
			// CPU failure is final: nothing left to fall back to.
			return res, fmt.Errorf("encoders: cpu encode failed: %w", err)
		}
		res.Fallbacks = append(res.Fallbacks, FallbackReason{
			Encoder: enc.Name(),
			Error:   err.Error(),
		})
	}
	if len(list) == 0 {
		return res, fmt.Errorf("encoders: no encoder in the registry")
	}
	// Unreachable with a well-formed registry (it always ends with cpu
	// exactly once), but a GPU-only list must not silently lose the error.
	if len(res.Fallbacks) == 0 {
		return res, fmt.Errorf("encoders: all %d encoder(s) failed", len(list))
	}
	last := res.Fallbacks[len(res.Fallbacks)-1]
	return res, fmt.Errorf("encoders: all %d encoder(s) failed, last error (%s): %s",
		len(list), last.Encoder, last.Error)
}
