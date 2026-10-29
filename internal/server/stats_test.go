// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// TestStatsTotalsMatchSumOfLibraries is the acceptance shape for GET
// /stats: with two libraries of mixed files, every total equals the sum of
// the per-library breakdown entries. No ffmpeg needed: files are written
// straight into the store.
func TestStatsTotalsMatchSumOfLibraries(t *testing.T) {
	h := newHarness(t)

	mkLib := func(name string) *store.Library {
		target, err := json.Marshal(flow.Default())
		require.NoError(t, err)
		lib, err := h.svc.Libraries().Create(context.Background(), LibraryParams{
			Name: name, Path: h.root, TargetJSON: target,
			ResultPolicy: store.ResultPolicyReplaceIfSmaller,
		})
		require.NoError(t, err)
		return lib
	}
	a := mkLib("Movies")
	b := mkLib("Series")

	// Library A: 2 satisfied, 1 queued, 1 done (500 bytes saved),
	// 1 failed (50 bytes saved).
	wantA := map[string]int64{
		store.FileStatusSatisfied: 2, store.FileStatusQueued: 1,
		store.FileStatusDone: 1, store.FileStatusFailed: 1,
	}
	// Library B: 1 new, 1 missing.
	wantB := map[string]int64{
		store.FileStatusNew: 1, store.FileStatusMissing: 1,
	}
	seed := func(lib *store.Library, wants map[string]int64, saved map[string]int64) {
		i := 0
		for status, n := range wants {
			for j := 0; j < int(n); j++ {
				f := &store.File{
					LibraryID:  lib.ID,
					Path:       lib.Path + "/file" + itoa2(i) + ".mkv",
					Status:     status,
					BytesSaved: saved[status],
				}
				require.NoError(t, h.st.UpsertFile(context.Background(), f))
				i++
			}
		}
	}
	seed(a, wantA, map[string]int64{
		store.FileStatusDone: 500, store.FileStatusFailed: 50,
	})
	seed(b, wantB, nil)

	stats, err := h.svc.Stats().Get(context.Background())
	require.NoError(t, err)

	require.Len(t, stats.Libraries, 2)
	byID := map[string]LibraryStats{}
	for _, ls := range stats.Libraries {
		byID[ls.LibraryID] = ls
	}

	aStats := byID[a.ID]
	require.Equal(t, "Movies", aStats.Name)
	require.EqualValues(t, 5, aStats.Files)
	require.EqualValues(t, 2, aStats.Satisfied)
	require.EqualValues(t, 1, aStats.Queued)
	require.EqualValues(t, 1, aStats.Done)
	require.EqualValues(t, 1, aStats.Failed)
	require.EqualValues(t, 550, aStats.BytesSaved)

	bStats := byID[b.ID]
	require.Equal(t, "Series", bStats.Name)
	require.EqualValues(t, 2, bStats.Files)
	require.EqualValues(t, 1, bStats.New)
	require.EqualValues(t, 1, bStats.Missing)
	require.EqualValues(t, 0, bStats.BytesSaved)

	// Totals are the sum of the breakdown, every field.
	require.EqualValues(t, aStats.Files+bStats.Files, stats.Files)
	require.EqualValues(t, aStats.Satisfied+bStats.Satisfied, stats.Satisfied)
	require.EqualValues(t, aStats.Queued+bStats.Queued, stats.Queued)
	require.EqualValues(t, aStats.Done+bStats.Done, stats.Done)
	require.EqualValues(t, aStats.Failed+bStats.Failed, stats.Failed)
	require.EqualValues(t, aStats.New+bStats.New, stats.New)
	require.EqualValues(t, aStats.Missing+bStats.Missing, stats.Missing)
	require.EqualValues(t, aStats.BytesSaved+bStats.BytesSaved, stats.BytesSaved)
}

// TestStatsEmptyIsZeroShape: with no libraries at all the response is all
// zeros with an empty (non-null) libraries list.
func TestStatsEmptyIsZeroShape(t *testing.T) {
	h := newHarness(t)
	stats, err := h.svc.Stats().Get(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 0, stats.Files)
	require.Empty(t, stats.Libraries)
	raw, err := json.Marshal(stats)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"files": 0, "new": 0, "satisfied": 0, "queued": 0, "processing": 0,
		"done": 0, "failed": 0, "missing": 0, "bytes_saved": 0, "libraries": []
	}`, string(raw))
}

func itoa2(n int) string {
	digits := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	return digits[n%10]
}
