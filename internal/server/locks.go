// SPDX-License-Identifier: AGPL-3.0-only

package server

import "sync"

// libraryLocks hands out one mutex per library so scans and target-driven
// re-evaluations of the same library serialize without blocking others.
type libraryLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newLibraryLocks() *libraryLocks {
	return &libraryLocks{locks: map[string]*sync.Mutex{}}
}

// lock acquires the mutex for a library and returns the unlock function.
func (l *libraryLocks) lock(libraryID string) func() {
	l.mu.Lock()
	m, ok := l.locks[libraryID]
	if !ok {
		m = &sync.Mutex{}
		l.locks[libraryID] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}
