// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"io"
	"time"
)

// pacedReader caps a stream at bytesPerSec (task 09: the node's
// throughput_limit_mbps, 0 = unlimited). It paces after the fact: for every
// read it compares the bytes moved against the time elapsed and sleeps off
// the difference. The sleep is context-aware, so a cancelled transfer
// unblocks immediately. No external rate-limit dependency (STACK.md rule 3).
type pacedReader struct {
	r           io.Reader
	bytesPerSec int64
	ctx         context.Context
	start       time.Time
	moved       int64
}

// newPacedReader wraps r with the limit; a non-positive limit returns r
// unchanged.
func newPacedReader(r io.Reader, bytesPerSec int64, ctx context.Context) io.Reader {
	if bytesPerSec <= 0 || r == nil {
		return r
	}
	return &pacedReader{r: r, bytesPerSec: bytesPerSec, ctx: ctx, start: time.Now()}
}

func (p *pacedReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.moved += int64(n)
		if wait := p.ahead(); wait > 0 {
			if !sleepCtx(p.ctx, wait) {
				return n, p.ctx.Err()
			}
		}
	}
	return n, err
}

// ahead returns how far the stream is ahead of its allowance.
func (p *pacedReader) ahead() time.Duration {
	allowed := time.Duration(float64(p.moved) / float64(p.bytesPerSec) * float64(time.Second))
	spent := time.Since(p.start)
	if allowed <= spent {
		return 0
	}
	return allowed - spent
}

// sleepCtx waits d or until ctx is done; false means the context ended.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
