// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// LibraryStats is the per-library part of the stats response (API.md,
// task 07): file counts per status plus the space saved so far.
type LibraryStats struct {
	LibraryID  string `json:"library_id"`
	Name       string `json:"name"`
	Files      int64  `json:"files"`
	New        int64  `json:"new"`
	Satisfied  int64  `json:"satisfied"`
	Queued     int64  `json:"queued"`
	Processing int64  `json:"processing"`
	Done       int64  `json:"done"`
	Failed     int64  `json:"failed"`
	Missing    int64  `json:"missing"`
	BytesSaved int64  `json:"bytes_saved"`
}

// Stats is the GET /api/v1/stats response: totals plus the per-library
// breakdown. Every total equals the sum of the breakdown entries.
type Stats struct {
	Files      int64          `json:"files"`
	New        int64          `json:"new"`
	Satisfied  int64          `json:"satisfied"`
	Queued     int64          `json:"queued"`
	Processing int64          `json:"processing"`
	Done       int64          `json:"done"`
	Failed     int64          `json:"failed"`
	Missing    int64          `json:"missing"`
	BytesSaved int64          `json:"bytes_saved"`
	Libraries  []LibraryStats `json:"libraries"`
}

// StatsService computes file and space totals from the store. It is a
// read surface for the dashboard and the libraries list.
type StatsService struct {
	st store.Store
}

// NewStats builds the stats service.
func NewStats(st store.Store) *StatsService { return &StatsService{st: st} }

// Get tallies every file of every library by status and bytes saved.
// Home-lab scale: files are listed and counted in memory instead of adding
// SQL aggregation to the store interface.
func (s *StatsService) Get(ctx context.Context) (Stats, error) {
	out := Stats{Libraries: []LibraryStats{}}
	libs, err := s.st.ListLibraries(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("stats: list libraries: %w", err)
	}
	for _, lib := range libs {
		ls := LibraryStats{LibraryID: lib.ID, Name: lib.Name}
		files, err := s.st.ListFiles(ctx, lib.ID)
		if err != nil {
			return Stats{}, fmt.Errorf("stats: list files: %w", err)
		}
		for _, f := range files {
			ls.Files++
			switch f.Status {
			case store.FileStatusNew:
				ls.New++
			case store.FileStatusSatisfied:
				ls.Satisfied++
			case store.FileStatusQueued:
				ls.Queued++
			case store.FileStatusProcessing:
				ls.Processing++
			case store.FileStatusDone:
				ls.Done++
			case store.FileStatusFailed:
				ls.Failed++
			case store.FileStatusMissing:
				ls.Missing++
			}
			ls.BytesSaved += f.BytesSaved
		}
		out.Files += ls.Files
		out.New += ls.New
		out.Satisfied += ls.Satisfied
		out.Queued += ls.Queued
		out.Processing += ls.Processing
		out.Done += ls.Done
		out.Failed += ls.Failed
		out.Missing += ls.Missing
		out.BytesSaved += ls.BytesSaved
		out.Libraries = append(out.Libraries, ls)
	}
	return out, nil
}
