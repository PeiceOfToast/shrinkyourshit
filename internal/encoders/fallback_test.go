// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failFor returns a RunFn that fails (with a plausible exec error) for
// ffmpeg commands containing any of the markers and succeeds otherwise.
func failFor(markers []string, errText string) RunFn {
	return func(_ context.Context, args []string) error {
		joined := strings.Join(args, " ")
		for _, m := range markers {
			if strings.Contains(joined, m) {
				return errors.New(errText)
			}
		}
		return nil
	}
}

// TestRunWithFallbackGPUToCPU: the fake nvenc fails, cpu succeeds, and the
// result names cpu plus the nvenc fallback reason with the exit error.
func TestRunWithFallbackGPUToCPU(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	list := Registry(hevcTarget(), Capability{
		Encoders: []EncoderInfo{
			{Name: "nvenc", Codecs: []string{"hevc"}},
		},
	})
	require.Equal(t, []string{"nvenc", "cpu"}, registryNames(list))

	var tried []string
	run := func(ctx context.Context, args []string) error {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "hevc_nvenc"):
			tried = append(tried, "nvenc")
			return errors.New("exit status 1")
		case strings.Contains(joined, "libx265"):
			tried = append(tried, "cpu")
			return nil
		}
		return errors.New("unexpected command: " + joined)
	}

	res, err := RunWithFallback(context.Background(), list, testSpec(), run)
	require.NoError(t, err)
	assert.Equal(t, []string{"nvenc", "cpu"}, tried, "both encoders were tried in order")
	assert.Equal(t, "cpu", res.EncoderUsed)
	require.Len(t, res.Fallbacks, 1)
	assert.Equal(t, "nvenc", res.Fallbacks[0].Encoder)
	assert.Equal(t, "exit status 1", res.Fallbacks[0].Error)
}

// TestRunWithFallbackTwoGPUs: both GPU encoders fail; the result records
// both reasons in order.
func TestRunWithFallbackTwoGPUs(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	list := []Encoder{NVENC, QSV, CPU}
	run := failFor([]string{"_nvenc", "_qsv"}, "exit status 1")

	res, err := RunWithFallback(context.Background(), list, testSpec(), run)
	require.NoError(t, err)
	assert.Equal(t, "cpu", res.EncoderUsed)
	require.Len(t, res.Fallbacks, 2)
	assert.Equal(t, "nvenc", res.Fallbacks[0].Encoder)
	assert.Equal(t, "qsv", res.Fallbacks[1].Encoder)
}

// TestRunWithFallbackCPUFailureIsFinal: a cpu failure ends the run with an
// error; nothing retries after it.
func TestRunWithFallbackCPUFailureIsFinal(t *testing.T) {
	list := []Encoder{CPU}
	run := failFor([]string{"libx265"}, "exit status 137")

	res, err := RunWithFallback(context.Background(), list, testSpec(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpu encode failed")
	assert.Contains(t, err.Error(), "exit status 137")
	assert.Empty(t, res.Fallbacks)
	assert.Equal(t, "", res.EncoderUsed)
}

// TestRunWithFallbackEmptyRegistry: a missing registry is an error, not a
// nil-pointer surprise.
func TestRunWithFallbackEmptyRegistry(t *testing.T) {
	_, err := RunWithFallback(context.Background(), nil, testSpec(),
		func(context.Context, []string) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no encoder")
}

// TestRunWithFallbackRecordsProgressMarkers: the failing GPU encoder's args
// really went through runFn unchanged (the builder output reaches the runner).
func TestRunWithFallbackPassesBuiltArgs(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	var seen [][]string
	list := []Encoder{CPU}
	run := func(_ context.Context, args []string) error {
		seen = append(seen, args)
		return nil
	}
	_, err := RunWithFallback(context.Background(), list, testSpec(), run)
	require.NoError(t, err)
	require.Len(t, seen, 1)
	assert.Contains(t, strings.Join(seen[0], " "), "-c:v libx265 -crf 24 -preset medium")
	assert.Equal(t, "cpu", list[0].Name())
}
