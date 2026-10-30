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

// TestLibraryDTOEasy covers the three easy states of the DTO (task 07):
// stored easy wins, an easy-shaped target describes back, and a
// hand-edited target yields null.
func TestLibraryDTOEasy(t *testing.T) {
	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	easyInput := `{"quality":3,"remove_subtitles":true,"audio":2}`

	// Stored easy column wins, verbatim after canonical re-marshal.
	lib := &store.Library{Name: "A", Path: "/media/a", TargetJSON: string(target), EasyJSON: easyInput}
	dto := NewLibraryDTO(lib)
	require.JSONEq(t, easyInput, string(dto.Easy))

	// No stored easy but an easy-shaped target: describe fills it in.
	lib = &store.Library{Name: "B", Path: "/media/b", TargetJSON: string(target)}
	dto = NewLibraryDTO(lib)
	require.JSONEq(t, `{"quality":3,"remove_subtitles":false,"audio":1}`, string(dto.Easy))

	// A hand-edited target (copy codec) has no easy representation.
	handEdited, err := json.Marshal(flow.Target{
		Video:     flow.VideoTarget{Codec: flow.CodecCopy, Quality: 3},
		Audio:     flow.AudioTarget{Mode: flow.AudioKeepAll},
		Subtitles: flow.SubtitleTarget{Mode: flow.SubsKeepAll},
		Container: flow.ContainerMKV,
	})
	require.NoError(t, err)
	lib = &store.Library{Name: "C", Path: "/media/c", TargetJSON: string(handEdited)}
	dto = NewLibraryDTO(lib)
	require.JSONEq(t, "null", string(dto.Easy))
}

// TestJobFilterOrderDesc checks the ?order=desc support the dashboard uses
// for its recent-history list.
func TestJobFilterOrderDesc(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	lib := h.createLibrary(false)
	files := []*store.File{
		{LibraryID: lib.ID, Path: lib.Path + "/a.mkv", Status: store.FileStatusFailed},
		{LibraryID: lib.ID, Path: lib.Path + "/b.mkv", Status: store.FileStatusFailed},
	}
	for _, f := range files {
		require.NoError(t, h.st.UpsertFile(ctx, f))
	}
	for _, f := range files {
		require.NoError(t, h.st.CreateJob(ctx, &store.Job{FileID: f.ID, LibraryID: lib.ID, Status: store.JobStatusFailed}))
	}

	asc, err := h.st.ListJobs(ctx, store.JobFilter{LibraryID: lib.ID})
	require.NoError(t, err)
	require.Len(t, asc, 2)
	require.Equal(t, files[0].ID, asc[0].FileID)

	desc, err := h.st.ListJobs(ctx, store.JobFilter{LibraryID: lib.ID, Order: "desc"})
	require.NoError(t, err)
	require.Len(t, desc, 2)
	require.Equal(t, files[1].ID, desc[0].FileID, "desc returns the newest job first")

	_, err = h.st.ListJobs(ctx, store.JobFilter{Order: "sideways"})
	require.ErrorContains(t, err, "invalid job order")
}

// TestJobDTOCarriesFileName covers the task-18 file identity: the DTO
// carries the file's basename and full path resolved from file_id, and a
// missing file row leaves both fields empty instead of failing.
func TestJobDTOCarriesFileName(t *testing.T) {
	job := &store.Job{
		ID: "job-1", FileID: "file-1", LibraryID: "lib-1",
		Status: store.JobStatusRunning, BytesBefore: 32505856, BytesSoFar: 12582912,
	}
	file := &store.File{ID: "file-1", LibraryID: "lib-1", Path: "/media/movies/movie.mkv"}

	dto := NewJobDTO(job, file)
	require.Equal(t, "file-1", dto.FileID, "file_id stays")
	require.Equal(t, "movie.mkv", dto.FileName)
	require.Equal(t, "/media/movies/movie.mkv", dto.FilePath)
	require.Equal(t, int64(12582912), dto.BytesSoFar)

	// The file row vanished (or the caller had none): empty names, no error.
	dto = NewJobDTO(job, nil)
	require.Empty(t, dto.FileName)
	require.Empty(t, dto.FilePath)
	require.Equal(t, "file-1", dto.FileID)
}
