// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerLoopFiresEveryMinute(t *testing.T) {
	h := newHarness(t)
	h.prober.mu.Lock()
	h.prober.byName["movie.mkv"] = h264Needs
	h.prober.mu.Unlock()
	writeFixture(t, h.root, "movie.mkv", 2048)

	// A schedule that fires every minute.
	lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
		Name: "Movies", Path: h.root, Schedule: "* * * * *",
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)
	_ = lib

	s := NewScheduler(h.st, h.svc.Scanner(), nil, 10*time.Millisecond)
	now := time.Now().Unix() / 60

	// First tick fires the schedule; repeats within the same minute must
	// not schedule anything new. Scan collapse plus the active-job guard
	// keep this at exactly one job.
	s.tick(context.Background(), now-1)
	s.tick(context.Background(), now)
	s.tick(context.Background(), now)

	waitFor(t, 15*time.Second, "scheduled scan to queue the file", func() bool {
		for _, f := range h.files(lib.ID) {
			if f.Status == store.FileStatusQueued {
				return true
			}
		}
		return false
	})
	require.Len(t, h.files(lib.ID), 1)
	assert.Len(t, h.jobsOf(h.files(lib.ID)["movie.mkv"].ID), 1, "never more than one job per file")
}

func TestSchedulerSkipsEmptySchedules(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(false)
	s := NewScheduler(h.st, h.svc.Scanner(), nil, 10*time.Millisecond)
	require.NotPanics(t, func() {
		s.tick(context.Background(), time.Now().Unix()/60-1)
	})
	assert.Empty(t, h.files(lib.ID))
}

func TestSchedulerBadStoredScheduleIsSkipped(t *testing.T) {
	h := newHarness(t)
	lib := h.createLibrary(false)
	// Corrupt the schedule directly in the store (bypassing validation).
	lib.Schedule = "not a schedule"
	require.NoError(t, h.st.UpdateLibrary(context.Background(), lib))

	s := NewScheduler(h.st, h.svc.Scanner(), nil, 10*time.Millisecond)
	require.NotPanics(t, func() {
		s.tick(context.Background(), time.Now().Unix()/60-1)
	})
}
