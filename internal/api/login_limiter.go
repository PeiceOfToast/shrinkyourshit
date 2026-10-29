// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"sync"
	"time"
)

// loginLimiter rate-limits login attempts per client IP: after maxFails
// failed attempts within the window, allow reports false and the API
// answers 429. Failed attempts only — a user logging in successfully five
// times a minute is not an attack (task 06).
type loginLimiter struct {
	mu     sync.Mutex
	fails  map[string][]time.Time
	window time.Duration
	max    int
	now    func() time.Time // injectable for tests
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		fails:  map[string][]time.Time{},
		window: time.Minute,
		max:    5,
		now:    time.Now,
	}
}

// allow reports whether a login attempt from ip may proceed. It prunes
// failures that fell out of the window.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)
	kept := l.fails[ip][:0]
	for _, t := range l.fails[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.fails[ip] = kept
	return len(kept) < l.max
}

// fail records a failed attempt from ip.
func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.fails[ip], l.now())
}
