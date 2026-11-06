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

// TestCreateLibraryIgnoresLegacyEasy (task 74): the easy input is a pre-profile
// field. A create that sends it still succeeds, the library follows the default
// profile, the easy input is not compiled, and the reply names the field.
func TestCreateLibraryIgnoresLegacyEasy(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"path": h.root, "watch": true,
		"easy": map[string]any{"quality": 3, "remove_subtitles": true, "audio": 2},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var reply struct {
		server.LibraryDTO
		Deprecated []struct {
			Field string `json:"field"`
		} `json:"deprecated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
	require.Equal(t, "library", reply.Name, "name defaults to the folder base name")
	require.NotEmpty(t, reply.ProfileID, "the library follows a profile")
	require.Equal(t, flow.Default(), reply.Target, "the easy input was not compiled")
	require.Contains(t, deprecatedFields(reply.Deprecated), "easy")
}

// TestCreateLibraryIgnoresInvalidLegacyFields (task 74): values that used to be
// 400s are ignored now, because nothing reads them. The reply still names each
// field, so an old client can see what did not apply.
func TestCreateLibraryIgnoresInvalidLegacyFields(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"path": h.root, "target": flow.Default(),
		"easy":          map[string]any{"quality": 9, "remove_subtitles": false, "audio": 1},
		"result_policy": "delete_everything",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var reply struct {
		Deprecated []struct {
			Field string `json:"field"`
		} `json:"deprecated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
	require.ElementsMatch(t, []string{"target", "easy", "result_policy", "profile_id"}, deprecatedFields(reply.Deprecated))
}

// TestLegacyEasyOnUpdateIsIgnored (task 74): an old client's easy save on an
// existing library changes nothing that runs. The library keeps its profile,
// its target is unchanged, and the reply names the field. A save with neither
// rule field is a plain update and keeps the profile (it used to be a 400).
func TestLegacyEasyOnUpdateIsIgnored(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Movies", "path": h.root,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = h.do(http.MethodPut, "/api/v1/libraries/"+created.ID, map[string]any{
		"name": "Movies", "path": h.root, "watch": true,
		"easy": map[string]any{"quality": 5, "remove_subtitles": true, "audio": 4},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var reply struct {
		server.LibraryDTO
		Deprecated []struct {
			Field string `json:"field"`
		} `json:"deprecated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
	require.Equal(t, created.ProfileID, reply.ProfileID, "the profile is kept")
	require.Equal(t, flow.Default(), reply.Target, "the easy input did not change the rule")
	require.Contains(t, deprecatedFields(reply.Deprecated), "easy")

	// Name and watch only: a plain update, the profile stays.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+created.ID, map[string]any{"name": "Movies", "path": h.root})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
	require.Equal(t, created.ProfileID, reply.ProfileID)
	require.Equal(t, []string{"profile_id"}, deprecatedFields(reply.Deprecated),
		"no profile_id sent: the reply says the library kept its profile")
}

// deprecatedFields lists the field names of a deprecated list.
func deprecatedFields(list []struct {
	Field string `json:"field"`
}) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.Field)
	}
	return out
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
