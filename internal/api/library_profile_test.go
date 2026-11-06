// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// libraryWithProfile is the body of a library save that follows a profile and
// carries no rule of its own (task 73).
func libraryWithProfile(path, profileID string) map[string]any {
	return map[string]any{
		"name": "Movies", "path": path, "watch": false,
		"profile_id": profileID, "result_policy": store.ResultPolicyReplaceIfSmaller,
	}
}

// flowHasType reports whether the library's effective flow holds a step of
// the given type.
func flowHasType(dto server.LibraryDTO, typ string) bool {
	if dto.Flow == nil {
		return false
	}
	for _, n := range dto.Flow.Nodes {
		if n.Type == typ {
			return true
		}
	}
	return false
}

// TestLibraryRunsItsProfileGraph covers spec 5.1 and 5.5: a library follows the
// graph of its profile. Switching the profile changes what the library runs,
// and editing a profile changes every library that uses it.
func TestLibraryRunsItsProfileGraph(t *testing.T) {
	h := newAPIHarness(t)
	basic := h.createProfileOK("Basic one", "basic", profileDefaultGraph(t))
	moderate := h.createProfileOK("Moderate one", "moderate", profileModerateGraph())

	rec := h.do(http.MethodPost, "/api/v1/libraries", libraryWithProfile(h.root, basic.ID))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))
	assert.Equal(t, basic.ID, lib.ProfileID)
	assert.False(t, flowHasType(lib, flow.StepScale), "the basic profile has no scale step")

	// Switch the library to the moderate profile: the scale step now runs.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, libraryWithProfile(h.root, moderate.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	lib = h.libraryDTO(lib.ID)
	assert.Equal(t, moderate.ID, lib.ProfileID)
	assert.True(t, flowHasType(lib, flow.StepScale), "the moderate profile scales")

	// Editing the moderate profile in place changes the library that uses it.
	rec = h.do(http.MethodPut, "/api/v1/profiles/"+moderate.ID,
		profileReq("Moderate one", "basic", store.ResultPolicyReplace, profileDefaultGraph(t)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	lib = h.libraryDTO(lib.ID)
	assert.False(t, flowHasType(lib, flow.StepScale), "the edited profile no longer scales")
}

// TestProfileOnlyUpdateKeepsTheStoredRule: a save that names a profile and
// nothing else is valid, and it leaves the library's compat target alone.
func TestProfileOnlyUpdateKeepsTheStoredRule(t *testing.T) {
	h := newAPIHarness(t)
	profile := h.createProfileOK("Default one", "basic", profileDefaultGraph(t))
	lib := h.createLibrary(false)
	before := h.libraryDTO(lib.ID).Target

	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{
		"name": lib.Name, "path": lib.Path, "watch": false, "profile_id": profile.ID,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	after := h.libraryDTO(lib.ID)
	assert.Equal(t, profile.ID, after.ProfileID)
	assert.Equal(t, before, after.Target, "a profile-only save keeps the stored target")

	// Without a profile a save still needs a rule.
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, map[string]any{
		"name": lib.Name, "path": lib.Path, "watch": false,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestLibraryProfileMustExist: an unknown profile id is refused on create and
// on update, and the library is left as it was.
func TestLibraryProfileMustExist(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/libraries", libraryWithProfile(h.root, "no-such-profile"))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	lib := h.createLibrary(false)
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID, libraryWithProfile(lib.Path, "no-such-profile"))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, h.libraryDTO(lib.ID).ProfileID)
}

// TestLibraryFlowSaveRefusedWhileFollowingProfile: a flow saved to a library
// that follows a profile would never run, so the API refuses it with 409.
func TestLibraryFlowSaveRefusedWhileFollowingProfile(t *testing.T) {
	h := newAPIHarness(t)
	profile := h.createProfileOK("Moderate one", "moderate", profileModerateGraph())
	rec := h.do(http.MethodPost, "/api/v1/libraries", libraryWithProfile(h.root, profile.ID))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))

	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "steps", "graph": profileDefaultGraph(t),
	})
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "follows_profile")
}

// TestProfileMoveIntoLibraryRefusedOnAttach: a profile is not bound to one
// folder, so its move destinations are checked when a library attaches it. A
// destination inside the library folder would be rescanned, so it is refused.
func TestProfileMoveIntoLibraryRefusedOnAttach(t *testing.T) {
	h := newAPIHarness(t)
	inside := filepath.Join(h.root, "done")
	g := flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "remux_container", Type: flow.StepRemuxContainer, Params: rawParams(`{"container":"mkv"}`)},
			{ID: "mv", Type: flow.StepMove, Params: rawParams(`{"destination":"` + inside + `","keep_tree":true}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "remux_container"},
			{From: "remux_container", To: "mv"},
		},
	}
	profile := h.createProfileOK("Moving one", "advanced", g)

	rec := h.do(http.MethodPost, "/api/v1/libraries", libraryWithProfile(h.root, profile.ID))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "invalid_flow")
}
