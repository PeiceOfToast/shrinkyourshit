// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"time"
)

// Job log retention (task 27): delete rows older than
// JobLogRetentionAge, then oldest first beyond JobLogMaxTotal rows. The
// store's PruneJobLog never touches rows belonging to a queued or running
// job and reclaims the freed space with PRAGMA incremental_vacuum.
const (
	JobLogRetentionAge = 14 * 24 * time.Hour
	JobLogMaxTotal     = 500_000
)

// pruneJobLog runs one retention pass.
func (s *Server) pruneJobLog(ctx context.Context) {
	removed, err := s.cfg.Store.PruneJobLog(ctx,
		time.Now().UTC().Add(-JobLogRetentionAge), JobLogMaxTotal)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("server: prune job log", "err", err)
		}
		return
	}
	if removed > 0 {
		s.log.Info("job log pruned", "rows", removed)
	}
}

// pruneJobLogLoop runs the retention at start and then every hour
// (configurable for tests through Config.JobLogPruneInterval).
func (s *Server) pruneJobLogLoop(ctx context.Context) {
	defer s.wg.Done()
	s.pruneJobLog(ctx)
	interval := s.cfg.JobLogPruneInterval
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pruneJobLog(ctx)
		}
	}
}
