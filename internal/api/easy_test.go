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

// removedRuleFields is a library save body that still carries the three rule
// fields removed in v1.12 (task 77), with values that used to be refused.
func removedRuleFields(path string) map[string]any {
	return map[string]any{
		"name": "Movies", "path": path, "watch": true,
		"target":        flow.Default(),
		"easy":          map[string]any{"quality": 9, "remove_subtitles": false, "audio": 1},
		"result_policy": "delete_everything",
	}
}

// TestLibrarySaveIgnoresRemovedRuleFields (task 77): a create and an update
// that send target, easy or result_policy succeed. The fields change nothing:
// the library keeps the profile it follows, the rule and policy are the
// profile's, and the reply has no deprecated key (the list is removed).
func TestLibrarySaveIgnoresRemovedRuleFields(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/libraries", removedRuleFields(h.root))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), `"deprecated"`)
	var created server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEmpty(t, created.ProfileID, "the library follows the default profile")
	require.Equal(t, flow.Default(), created.Target, "the easy input was not compiled")
	profile := h.profileFor(created.ProfileID)
	require.Equal(t, profile.ResultPolicy, created.ResultPolicy, "the policy is the profile's, not the sent value")
	require.NotEqual(t, "delete_everything", created.ResultPolicy)

	// Update with the same removed fields and a different policy: nothing moves.
	body := removedRuleFields(h.root)
	body["result_policy"] = "replace"
	body["profile_id"] = created.ProfileID
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+created.ID, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), `"deprecated"`)
	var updated server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, created.ProfileID, updated.ProfileID)
	require.Equal(t, profile.ResultPolicy, updated.ResultPolicy)
	require.Equal(t, flow.Default(), updated.Target)
}

// TestLibrarySaveWithoutProfileOmitsDeprecated (task 77): a plain save with
// no profile_id keeps the library's profile and carries no deprecated list,
// where the v1.11 reply named the profile.
func TestLibrarySaveWithoutProfileOmitsDeprecated(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{
		"name": "Movies", "path": h.root, "watch": true,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), `"deprecated"`)
	var reply server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
	require.Equal(t, lib.ProfileID, reply.ProfileID, "the library keeps its profile")
}

// TestLibraryTierRouteIsGone (task 77): PUT /api/v1/libraries/{id}/tier is no
// longer routed. It used to answer 409 follows_profile. The API's catch-all
// answers 404 not_found, and the library's tier is unchanged.
func TestLibraryTierRouteIsGone(t *testing.T) {
	h := newAPIHarness(t)
	lib := h.createLibrary(false)
	before := h.libraryDTO(lib.ID).RuleTier

	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/tier", map[string]any{"tier": "basic"})
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "not_found")
	require.Equal(t, before, h.libraryDTO(lib.ID).RuleTier)
}

// TestStatsEndpoint is the HTTP face of the stats acceptance: totals match
// the sum of the per-library breakdown with two libraries.
func TestStatsEndpoint(t *testing.T) {
	h := newAPIHarness(t)
	libA := h.createLibrary(false)

	// Second library through the API so both appear in the response.
	rec := h.do(http.MethodPost, "/api/v1/libraries", map[string]any{
		"name": "Series", "path": h.root + "-series",
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
