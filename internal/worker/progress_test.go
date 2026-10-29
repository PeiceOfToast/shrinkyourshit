// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgressParserOutTimeUS(t *testing.T) {
	state := newProgressState()
	w := newProgressWriter(state)
	_, err := w.Write([]byte("frame=10\nout_time_us=1500000\nspeed=2.50x\nprogress=continue\n"))
	require.NoError(t, err)
	out, speed, ended := state.snapshot()
	require.Equal(t, 1.5, out)
	require.Equal(t, 2.5, speed)
	require.False(t, ended)
}

func TestProgressParserOutTimeMsIsMicroseconds(t *testing.T) {
	// ffmpeg's out_time_ms is a long-standing misnomer: it carries
	// microseconds, not milliseconds.
	state := newProgressState()
	w := newProgressWriter(state)
	_, err := w.Write([]byte("out_time_ms=3000000\n"))
	require.NoError(t, err)
	out, _, _ := state.snapshot()
	require.Equal(t, 3.0, out)
}

func TestProgressParserOutTimeFallback(t *testing.T) {
	state := newProgressState()
	w := newProgressWriter(state)
	_, err := w.Write([]byte("out_time=00:01:02.500000\n"))
	require.NoError(t, err)
	out, _, _ := state.snapshot()
	require.InDelta(t, 62.5, out, 0.001)
}

func TestProgressParserEndAndSplitWrites(t *testing.T) {
	state := newProgressState()
	w := newProgressWriter(state)
	// Lines split across writes must still parse.
	_, err := w.Write([]byte("out_time"))
	require.NoError(t, err)
	_, err = w.Write([]byte("_us=1000\nprogr"))
	require.NoError(t, err)
	_, err = w.Write([]byte("ess=end\n"))
	require.NoError(t, err)
	out, _, ended := state.snapshot()
	require.Equal(t, 0.001, out)
	require.True(t, ended)
}

func TestProgressParserIgnoresJunk(t *testing.T) {
	state := newProgressState()
	w := newProgressWriter(state)
	_, err := w.Write([]byte("N/A\nnoequals\nspeed=unknown\nout_time=N/A\nout_time_us=\n\n"))
	require.NoError(t, err)
	out, speed, ended := state.snapshot()
	require.Equal(t, 0.0, out)
	require.Equal(t, 0.0, speed)
	require.False(t, ended)
}

func TestTailBufferCapped(t *testing.T) {
	tail := newTailBuffer(16)
	_, err := tail.Write([]byte("1234567890"))
	require.NoError(t, err)
	_, err = tail.Write([]byte("abcdefghij"))
	require.NoError(t, err)
	require.Equal(t, "567890abcdefghij", tail.Tail()) // last 16 bytes
}

func TestTailBufferLargeWrite(t *testing.T) {
	tail := newTailBuffer(8)
	big := strings.Repeat("z", 100)
	_, err := tail.Write([]byte(big))
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("z", 8), tail.Tail())
}
