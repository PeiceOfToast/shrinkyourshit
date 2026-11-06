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

// profiledLibrary is a library that follows a profile carrying graph g, the
// shape LibraryFlow reads (task 74). The legacy columns are left empty, as a
// library written by the new code keeps them only as a snapshot.
func profiledLibrary(t *testing.T, name string, g flow.Graph) *store.Library {
	t.Helper()
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	return &store.Library{
		Name: name, Path: "/media/" + name, ProfileID: "prof-" + name,
		ProfileGraph: string(raw), ProfileTier: store.RuleTierBasic,
		ProfileResultPolicy: store.ResultPolicyReplaceIfSmaller,
	}
}

// TestLibraryDTOEasy covers the easy state of the DTO (task 07, derived from
// the profile's graph since task 74): an easy-shaped graph describes back and a
// hand-edited graph yields null.
func TestLibraryDTOEasy(t *testing.T) {
	g, err := flow.TargetToFlow(flow.Default())
	require.NoError(t, err)
	lib := profiledLibrary(t, "B", g)
	dto := NewLibraryDTO(lib)
	require.JSONEq(t, `{"quality":3,"remove_subtitles":false,"audio":1}`, string(dto.Easy))

	// A hand-edited graph (copy codec) has no easy representation.
	handEdited, err := flow.TargetToFlow(flow.Target{
		Video:     flow.VideoTarget{Codec: flow.CodecCopy, Quality: 3},
		Audio:     flow.AudioTarget{Mode: flow.AudioKeepAll},
		Subtitles: flow.SubtitleTarget{Mode: flow.SubsKeepAll},
		Container: flow.ContainerMKV,
	})
	require.NoError(t, err)
	dto = NewLibraryDTO(profiledLibrary(t, "C", handEdited))
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
