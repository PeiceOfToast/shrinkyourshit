// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// easyTargetJSON is the exact compiled Target of easy
// {quality:3, remove_subtitles:true, audio:2} under the default settings
// (codec hevc, languages eng). The stored target of an easy-mode library
// must match this byte for byte (semantically).
const easyTargetJSON = `{
	"video": {"codec": "hevc", "quality": 3, "reencode_if_same_codec": false},
	"audio": {"mode": "keep_preferred", "preferred_languages": ["eng"], "compact_codec": "aac"},
	"subtitles": {"mode": "remove", "preferred_languages": ["eng"], "extract_removed": false},
	"container": "mkv"
}`

// TestCreateLibraryWithEasy: the easy screen posts {quality, remove_subtitles,
// audio}; the server compiles it to the exact Target, stores both, and
// defaults the name to the folder's base name.
func TestCreateLibraryWithEasy(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"path": h.root, "watch": true,
		"easy": map[string]any{"quality": 3, "remove_subtitles": true, "audio": 2},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.NotEmpty(t, dto.ID)
	require.Equal(t, "library", dto.Name, "name defaults to the folder base name")
	require.JSONEq(t, `{"quality":3,"remove_subtitles":true,"audio":2}`, string(dto.Easy))
	targetRaw, err := json.Marshal(dto.Target)
	require.NoError(t, err)
	require.JSONEq(t, easyTargetJSON, string(targetRaw))

	// The stored row carries both, so an easy library round-trips.
	row, err := h.st.GetLibrary(context.Background(), dto.ID)
	require.NoError(t, err)
	require.JSONEq(t, easyTargetJSON, row.TargetJSON)
	require.JSONEq(t, `{"quality":3,"remove_subtitles":true,"audio":2}`, row.EasyJSON)
}

// TestCreateLibraryEasyValidation: bad easy values and target+easy are
// rejected with 400.
func TestCreateLibraryEasyValidation(t *testing.T) {
	h := newAPIHarness(t)

	// target and easy together are a client bug.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"path": h.root, "target": flow.Default(),
		"easy": map[string]any{"quality": 3, "remove_subtitles": false, "audio": 1},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Every out-of-range slider value.
	for _, easy := range []map[string]any{
		{"quality": 0, "remove_subtitles": false, "audio": 1},
		{"quality": 6, "remove_subtitles": false, "audio": 1},
		{"quality": 3, "remove_subtitles": false, "audio": 0},
		{"quality": 3, "remove_subtitles": false, "audio": 5},
	} {
		rec = h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
			"path": h.root, "easy": easy,
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "easy: %v", easy)
	}

	// A create without target or easy still gets the default Target.
	rec = h.do(http.MethodPost, "/api/v1/libraries", map[string]any{"path": h.root})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.Equal(t, flow.Default(), dto.Target)
}

// TestLibraryEasyRoundTripViaAPI: a target-only easy-shaped library still
// opens in easy mode (the DTO describes it), while a hand-edited target has
// no easy representation and an easy save updates the stored state.
func TestLibraryEasyRoundTripViaAPI(t *testing.T) {
	h := newAPIHarness(t)

	// Created with a full target that happens to be easy-shaped.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root, "target": flow.Default(),
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))
	require.JSONEq(t, `{"quality":3,"remove_subtitles":false,"audio":1}`, string(lib.Easy))

	// Re-saving through the easy screen persists the new easy state.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{
		"name": "Movies", "path": h.root, "watch": true,
		"easy": map[string]any{"quality": 5, "remove_subtitles": true, "audio": 4},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))
	require.JSONEq(t, `{"quality":5,"remove_subtitles":true,"audio":4}`, string(lib.Easy))
	require.Equal(t, 5, lib.Target.Video.Quality)
	require.Equal(t, flow.AudioCompact, lib.Target.Audio.Mode)
	require.Equal(t, flow.SubsRemove, lib.Target.Subtitles.Mode)
	row, err := h.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"quality":5,"remove_subtitles":true,"audio":4}`, row.EasyJSON)

	// A target-only save clears the stored easy input; a hand-edited
	// (copy codec) target then has no easy representation at all.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{
		"name": "Movies", "path": h.root, "watch": true,
		"target": flow.Target{
			Video:     flow.VideoTarget{Codec: flow.CodecCopy, Quality: 3},
			Audio:     flow.AudioTarget{Mode: flow.AudioKeepAll},
			Subtitles: flow.SubtitleTarget{Mode: flow.SubsKeepAll},
			Container: flow.ContainerMKV,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))
	require.JSONEq(t, "null", string(lib.Easy))
	row, err = h.st.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	require.Empty(t, row.EasyJSON)

	// An update with neither target nor easy is still a 400 (PUT
	// semantics, API.md task 03).
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{"name": "Movies"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestStatsEndpoint is the HTTP face of the stats acceptance: totals match
// the sum of the per-library breakdown with two libraries.
func TestStatsEndpoint(t *testing.T) {
	h := newAPIHarness(t)
	libA := h.createLibrary(false)

	// Second library through the API so both appear in the response.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Series", "path": h.root + "-series",
		"easy": map[string]any{"quality": 2, "remove_subtitles": false, "audio": 3},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var libB server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &libB))

	// Seed files with statuses straight into the store (no ffmpeg).
	seed := func(libID string, status string, n int, saved int64) {
		for i := 0; i < n; i++ {
			f := &store.File{
				LibraryID:  libID,
				Path:       libID + "/" + status + "-" + string(rune('a'+i)) + ".mkv",
				Status:     status,
				BytesSaved: saved,
			}
			require.NoError(t, h.st.UpsertFile(context.Background(), f))
		}
	}
	seed(libA.ID, store.FileStatusSatisfied, 2, 0)
	seed(libA.ID, store.FileStatusQueued, 1, 0)
	seed(libA.ID, store.FileStatusDone, 1, 500)
	seed(libB.ID, store.FileStatusNew, 1, 0)
	seed(libB.ID, store.FileStatusFailed, 1, 50)

	rec = h.do(http.MethodGet, "/api/v1/stats", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var stats server.Stats
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))

	require.Len(t, stats.Libraries, 2)
	byID := map[string]server.LibraryStats{}
	for _, ls := range stats.Libraries {
		byID[ls.LibraryID] = ls
	}
	require.EqualValues(t, 4, byID[libA.ID].Files)
	require.EqualValues(t, 2, byID[libA.ID].Satisfied)
	require.EqualValues(t, 1, byID[libA.ID].Queued)
	require.EqualValues(t, 1, byID[libA.ID].Done)
	require.EqualValues(t, 500, byID[libA.ID].BytesSaved)
	require.EqualValues(t, 2, byID[libB.ID].Files)
	require.EqualValues(t, 1, byID[libB.ID].New)
	require.EqualValues(t, 1, byID[libB.ID].Failed)
	require.EqualValues(t, 50, byID[libB.ID].BytesSaved)

	require.EqualValues(t, 6, stats.Files)
	require.EqualValues(t, 2, stats.Satisfied)
	require.EqualValues(t, 1, stats.Queued)
	require.EqualValues(t, 1, stats.Done)
	require.EqualValues(t, 1, stats.New)
	require.EqualValues(t, 1, stats.Failed)
	require.EqualValues(t, 550, stats.BytesSaved)
	// Totals equal the sum of the breakdown, byte for byte.
	require.EqualValues(t, byID[libA.ID].BytesSaved+byID[libB.ID].BytesSaved, stats.BytesSaved)
	require.EqualValues(t, byID[libA.ID].Files+byID[libB.ID].Files, stats.Files)
}

// TestJobsOrderDescEndpoint: ?order=desc returns the newest job first;
// anything else is a 400.
func TestJobsOrderDescEndpoint(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)

	fa := &store.File{LibraryID: lib.ID, Path: lib.ID + "/a.mkv", Status: store.FileStatusFailed}
	fb := &store.File{LibraryID: lib.ID, Path: lib.ID + "/b.mkv", Status: store.FileStatusFailed}
	require.NoError(t, h.st.UpsertFile(context.Background(), fa))
	require.NoError(t, h.st.UpsertFile(context.Background(), fb))
	require.NoError(t, h.st.CreateJob(context.Background(), &store.Job{FileID: fa.ID, LibraryID: lib.ID, Status: store.JobStatusFailed}))
	require.NoError(t, h.st.CreateJob(context.Background(), &store.Job{FileID: fb.ID, LibraryID: lib.ID, Status: store.JobStatusFailed}))

	rec := h.do(http.MethodGet, "/api/v1/jobs?order=desc", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page struct {
		Items []server.JobDTO `json:"items"`
		Total int             `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Items, 2)
	require.Equal(t, fb.ID, page.Items[0].FileID, "newest job first")

	rec = h.do(http.MethodGet, "/api/v1/jobs?order=sideways", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
