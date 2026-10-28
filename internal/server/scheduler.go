// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Scheduler runs per-library cron scans. It ticks twice a minute and fires
// every matching minute since the last tick (bounded catch-up after a stall
// or laptop sleep), never twice within the same minute.
type Scheduler struct {
	st       store.Store
	scanner  *Scanner
	log      *slog.Logger
	interval time.Duration
}

// NewScheduler builds a scheduler; interval 0 defaults to 30s.
func NewScheduler(st store.Store, scanner *Scanner, log *slog.Logger, interval time.Duration) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{st: st, scanner: scanner, log: log, interval: interval}
}

// Run blocks until ctx is done. Start it in its own goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// lastMinute is the last whole minute (unix/60) already processed.
	lastMinute := time.Now().Unix() / 60
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastMinute = s.tick(ctx, lastMinute)
		}
	}
}

// tick fires schedules for every whole minute since lastMinute.
func (s *Scheduler) tick(ctx context.Context, lastMinute int64) int64 {
	now := time.Now().Unix() / 60
	// Bound catch-up to a day of missed minutes so a clock jump cannot
	// spin through years of cron evaluations.
	if now-lastMinute > 1440 {
		lastMinute = now - 1440
	}
	if now == lastMinute {
		return now // same minute; wait for the next
	}

	libs, err := s.st.ListLibraries(ctx)
	if err != nil {
		s.log.Error("scheduler: list libraries", "err", err)
		return now
	}
	type scheduled struct {
		lib *store.Library
		sch *Schedule
	}
	var active []scheduled
	for _, lib := range libs {
		if lib.Schedule == "" {
			continue
		}
		sch, err := ParseSchedule(lib.Schedule)
		if err != nil {
			s.log.Error("scheduler: bad schedule", "library", lib.ID, "schedule", lib.Schedule, "err", err)
			continue
		}
		active = append(active, scheduled{lib: lib, sch: sch})
	}

	for m := lastMinute + 1; m <= now; m++ {
		minute := time.Unix(m*60, 0).In(time.Local)
		for _, a := range active {
			if a.sch.Match(minute) {
				s.log.Info("schedule fired", "library", a.lib.ID, "name", a.lib.Name, "at", minute.Format(time.RFC3339))
				s.scanner.ScanAsync(a.lib.ID)
			}
		}
	}
	return now
}
