// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// argIndex returns the position of the first argument equal to want, or -1.
func argIndex(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// TestBuildArgsRawArgsStagePositions: raw_args tokens (task 58) sit in the real
// argv for every backend that builds through buildCommand. An input stage
// token comes before "-i" and an output stage token comes before the output
// path, which stays the last argument.
func TestBuildArgsRawArgsStagePositions(t *testing.T) {
	resetQSVVariants()
	defer resetQSVVariants()

	for _, enc := range []Encoder{CPU, NVENC, QSV, VAAPI} {
		t.Run(enc.Name(), func(t *testing.T) {
			spec := testSpec()
			spec.InputArgs = []string{"-ss", "1"}
			spec.OutputArgs = []string{"-metadata", "comment=raw"}

			args := enc.BuildArgs(spec)
			require.NotEmpty(t, args)

			inIdx := argIndex(args, "-i")
			require.GreaterOrEqual(t, inIdx, 0, "argv has -i")
			require.Equal(t, "in.mkv", args[inIdx+1], "-i names the input")
			ss := argIndex(args, "-ss")
			require.GreaterOrEqual(t, ss, 0, "input stage token is in argv")
			require.Less(t, ss, inIdx, "input stage token sits before -i")
			require.Equal(t, "1", args[ss+1])

			require.Equal(t, "out.mkv", args[len(args)-1], "output path stays last")
			meta := argIndex(args, "-metadata")
			require.GreaterOrEqual(t, meta, 0, "output stage token is in argv")
			require.Less(t, meta, len(args)-1, "output stage token sits before the output path")
			require.Equal(t, "comment=raw", args[meta+1])
			require.Equal(t, 1, countArg(args, "-i"), "raw args add no second input")
		})
	}
}

// TestBuildArgsRawArgsCopyVideoAndNoRawArgs: the stage placement also holds on
// the remux (video copy) path, and an empty spec builds the same argv as
// before task 58 (no raw tokens).
func TestBuildArgsRawArgsCopyVideoAndNoRawArgs(t *testing.T) {
	spec := withTarget(copyTarget())
	spec.InputArgs = []string{"-threads", "2"}
	spec.OutputArgs = []string{"-map_metadata", "-1"}

	args := CPU.BuildArgs(spec)
	require.Less(t, argIndex(args, "-threads"), argIndex(args, "-i"))
	require.Less(t, argIndex(args, "-map_metadata"), len(args)-1)
	require.Equal(t, "out.mkv", args[len(args)-1])

	plain := CPU.BuildArgs(withTarget(copyTarget()))
	require.NotContains(t, plain, "-threads")
	require.NotContains(t, plain, "-map_metadata")
	require.Equal(t, "-c:v copy", joinAround(plain, "-c:v", 2))
}

func countArg(args []string, want string) int {
	n := 0
	for _, a := range args {
		if a == want {
			n++
		}
	}
	return n
}

// joinAround returns the token at i and the next n-1 tokens as one string.
func joinAround(args []string, token string, n int) string {
	i := argIndex(args, token)
	if i < 0 || i+n > len(args) {
		return ""
	}
	out := args[i]
	for _, a := range args[i+1 : i+n] {
		out += " " + a
	}
	return out
}
