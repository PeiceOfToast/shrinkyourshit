// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyLibrary is one library row written the way v1.11 leaves it: the six
// legacy rule columns filled, the profile not yet assigned.
type legacyLibrary struct {
	id, name, flow, source, tier, policy string
}

// buildV111DB creates a database with the current schema (every migration,
// 0018 included, which is what v1.11 shipped) and inserts the given rows with
// their legacy rule columns filled. It returns the path. The rows have no
// profile_id, the state a failed v1.11 boot can leave behind.
func buildV111DB(t *testing.T, rows []legacyLibrary) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	st, err := New(path)
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	const insert = `INSERT INTO libraries (id, name, path, target, easy, flow_json, flow_source, rule_tier, watch, schedule, result_policy, created_at, updated_at, file_kinds)
		VALUES (?, ?, ?, '{"video":{"codec":"hevc","quality":3},"container":"mkv"}', '{"quality":3}', ?, ?, ?, 0, '', ?, ?, ?, '["video"]')`
	for i, r := range rows {
		created := "2026-11-01T00:00:0" + string(rune('0'+i)) + "Z"
		_, err := st.db.ExecContext(context.Background(), insert, r.id, r.name, "/media/"+r.id, r.flow, r.source, r.tier, r.policy, created, created)
		require.NoError(t, err, r.id)
	}
	return path
}

// graphA is a valid stored graph for the legacy rows; the backfill never reads it.
const graphA = `{"schema":1,"nodes":[{"id":"probe","type":"probe"}],"edges":[]}`

// v111Libraries: a healthy row, a row whose legacy graph is not JSON, a graphless
// row, and a row with a non default tier and policy. None of that matters to the
// backfill: each one gets the default profile and nothing else.
var v111Libraries = []legacyLibrary{
	{id: "films", name: "Films", flow: graphA, source: "advanced", tier: RuleTierAdvanced, policy: "keep_both"},
	{id: "broken", name: "Broken", flow: `{not json`, source: "advanced", tier: RuleTierBasic, policy: "replace"},
	{id: "bare", name: "Bare", flow: "", source: "", tier: RuleTierBasic, policy: "replace_if_smaller"},
	{id: "shows", name: "Shows", flow: graphA, source: "steps", tier: RuleTierModerate, policy: "replace"},
}

// openV111 builds the v1.11 database and opens it the way the new release does.
func openV111(t *testing.T, rows []legacyLibrary) *SQLite {
	t.Helper()
	st, err := New(buildV111DB(t, rows))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// defaultProfileFor creates the profile the backfill assigns and returns it.
func defaultProfileFor(t *testing.T, st *SQLite) *Profile {
	t.Helper()
	p := &Profile{Name: "Default (basic)", Tier: RuleTierBasic, Graph: graphA, ResultPolicy: ResultPolicyReplaceIfSmaller}
	require.NoError(t, st.CreateProfile(context.Background(), p))
	return p
}

// TestBackfillAssignsOnlyTheDefaultProfile: every unassigned row gets the given
// profile, whatever its legacy columns say, and no profile is created or
// changed on the way.
func TestBackfillAssignsOnlyTheDefaultProfile(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries)
	def := defaultProfileFor(t, st)

	report, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)
	require.Len(t, report, len(v111Libraries))
	for _, e := range report {
		assert.Equal(t, def.ID, e.ProfileID, e.LibraryID)
		assert.Equal(t, def.Name, e.ProfileName, e.LibraryID)
	}

	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, len(v111Libraries))
	for _, l := range libs {
		assert.Equal(t, def.ID, l.ProfileID, "library %s", l.ID)
		assert.Equal(t, def.Graph, l.ProfileGraph, "library %s runs the default graph", l.ID)
		assert.Equal(t, RuleTierBasic, l.ProfileTier, "library %s", l.ID)
		assert.Equal(t, ResultPolicyReplaceIfSmaller, l.ProfileResultPolicy, "library %s", l.ID)
	}

	profiles, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 1, "no profile is created by the backfill")
}

// TestBackfillIsIdempotent: a second run, and a restart of the whole store,
// assign nothing and change nothing.
func TestBackfillIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := buildV111DB(t, v111Libraries)
	st, err := New(path)
	require.NoError(t, err)
	def := defaultProfileFor(t, st)
	_, err = st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)
	before, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	require.NoError(t, st.Close())

	st, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	report, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)
	assert.Empty(t, report, "nothing is left to assign")

	after, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, after, len(before))
	for i := range before {
		assert.Equal(t, before[i].ProfileID, after[i].ProfileID, before[i].ID)
	}
}

// TestBackfillNeedsAnExistingProfile: an unknown or empty profile id is refused
// and no library is touched.
func TestBackfillNeedsAnExistingProfile(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries)

	_, err := st.BackfillProfiles(ctx, "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = st.BackfillProfiles(ctx, "")
	require.Error(t, err)

	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	for _, l := range libs {
		assert.Empty(t, l.ProfileID, "library %s stays unassigned", l.ID)
	}
}

// TestBackfillLeavesAssignedLibrariesAlone: a library that already has a profile
// keeps it, even when the backfill is given another one.
func TestBackfillLeavesAssignedLibrariesAlone(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries[:2])
	def := defaultProfileFor(t, st)
	other := &Profile{Name: "Other", Tier: RuleTierModerate, Graph: graphA, ResultPolicy: ResultPolicyKeepBoth}
	require.NoError(t, st.CreateProfile(ctx, other))
	require.NoError(t, st.SetLibraryProfile(ctx, "films", other.ID))

	report, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)
	require.Len(t, report, 1, "only the unassigned library")
	assert.Equal(t, "broken", report[0].LibraryID)

	films, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	assert.Equal(t, other.ID, films.ProfileID)
}

// TestBackfillDoesNotWriteLegacyColumns: the backfill sets profile_id and nothing
// else; the legacy columns keep their v1.11 values until task 78 drops them.
func TestBackfillDoesNotWriteLegacyColumns(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries)
	def := defaultProfileFor(t, st)

	_, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)

	var flow, tier, policy string
	require.NoError(t, st.db.QueryRowContext(ctx, `SELECT flow_json, rule_tier, result_policy FROM libraries WHERE id = ?`, "films").Scan(&flow, &tier, &policy))
	assert.Equal(t, graphA, flow)
	assert.Equal(t, RuleTierAdvanced, tier)
	assert.Equal(t, "keep_both", policy)
}

// TestProfileIDSurvivesLibraryWrites: UpdateLibrary from a fresh read keeps the
// profile, and SetLibraryProfile changes only the reference.
func TestProfileIDSurvivesLibraryWrites(t *testing.T) {
	ctx := context.Background()
	st := openV111(t, v111Libraries[:1])
	def := defaultProfileFor(t, st)
	_, err := st.BackfillProfiles(ctx, def.ID)
	require.NoError(t, err)

	lib, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	lib.Name = "Renamed"
	require.NoError(t, st.UpdateLibrary(ctx, lib))
	again, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	assert.Equal(t, def.ID, again.ProfileID)
	assert.Equal(t, "Renamed", again.Name)
}

// TestProfileCRUD covers the profile methods the backfill does not reach.
func TestProfileCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	p := &Profile{Name: "Standard", Description: "d", Tier: RuleTierModerate, Graph: graphA, ResultPolicy: "replace_if_smaller"}
	require.NoError(t, st.CreateProfile(ctx, p))
	require.NotEmpty(t, p.ID)

	got, err := st.GetProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Standard", got.Name)
	assert.Equal(t, RuleTierModerate, got.Tier)
	assert.Equal(t, graphA, got.Graph)

	_, err = st.GetProfile(ctx, "missing")
	require.ErrorIs(t, err, ErrNotFound)

	require.Error(t, st.CreateProfile(ctx, &Profile{Name: "Bad tier", Tier: "expert", Graph: "{}", ResultPolicy: "replace"}))
	require.Error(t, st.CreateProfile(ctx, &Profile{Name: "Bad policy", Tier: RuleTierBasic, Graph: "{}", ResultPolicy: "nope"}))
	require.Error(t, st.CreateProfile(ctx, &Profile{Name: "Standard", Tier: RuleTierBasic, Graph: "{}", ResultPolicy: "replace"}), "names are unique")

	list, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	lib := validLibrary()
	require.NoError(t, st.CreateLibrary(ctx, lib))
	require.NoError(t, st.SetLibraryProfile(ctx, lib.ID, p.ID))
	fresh, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, fresh.ProfileID)

	require.NoError(t, st.SetLibraryProfile(ctx, lib.ID, ""))
	cleared, err := st.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Empty(t, cleared.ProfileID)

	require.ErrorIs(t, st.SetLibraryProfile(ctx, "missing", p.ID), ErrNotFound)
}
