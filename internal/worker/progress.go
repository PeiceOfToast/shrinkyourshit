// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"strconv"
	"strings"
	"sync"
)

// tailBuffer keeps the last max bytes written to it: the ffmpeg stderr tail
// stored on the job record (capped at 64KiB, ARCHITECTURE.md). Safe for the
// concurrent writes ffmpeg's pipe goroutines make.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

// Tail returns the buffered text.
func (t *tailBuffer) Tail() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// progressState is the latest snapshot of ffmpeg's -progress output. The
// runner reads it on its report ticker.
type progressState struct {
	mu      sync.Mutex
	outTime float64 // seconds encoded so far
	speed   float64 // encoding speed as a multiple of real time
	ended   bool    // "progress=end" seen
}

func newProgressState() *progressState {
	return &progressState{}
}

// snapshot returns the current out_time and speed.
func (p *progressState) snapshot() (outTime, speed float64, ended bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.outTime, p.speed, p.ended
}

// progressWriter consumes ffmpeg stdout ("-progress pipe:1") and updates the
// state on every complete "key=value" line (STACK.md).
type progressWriter struct {
	state   *progressState
	partial []byte
}

func newProgressWriter(state *progressState) *progressWriter {
	return &progressWriter{state: state}
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.partial = append(w.partial, p...)
	for {
		i := indexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := string(w.partial[:i])
		w.partial = w.partial[i+1:]
		w.state.applyLine(line)
	}
	return len(p), nil
}

// applyLine parses one "key=value" progress line.
func (p *progressState) applyLine(line string) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if value == "" || value == "N/A" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch key {
	case "out_time_us", "out_time_ms":
		// Both are microseconds: out_time_ms is a long-standing ffmpeg
		// misnomer kept for compatibility (it never meant milliseconds).
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			p.outTime = v / 1e6
		}
	case "out_time":
		// "HH:MM:SS.ffffff" fallback for builds without the numeric keys.
		if d, ok := parseClock(value); ok {
			p.outTime = d
		}
	case "speed":
		// "2.31x" — negative values (rare, e.g. signed output) are ignored.
		p.speed = parseSpeed(value)
	case "progress":
		if value == "end" {
			p.ended = true
		}
	}
}

// parseClock parses "[-]HH:MM:SS.microseconds".
func parseClock(s string) (float64, bool) {
	s = strings.TrimPrefix(s, "-")
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	var secs float64
	for _, part := range parts {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, false
		}
		secs = secs*60 + v
	}
	return secs, true
}

// parseSpeed parses "2.31x"; anything unparsed is 0 (unknown).
func parseSpeed(s string) float64 {
	s = strings.TrimSuffix(s, "x")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
