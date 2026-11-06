// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Graphs used by the migration tests. graphAReformatted is graphA with other
// key order, whitespace and nothing else, so it must canonicalise to the same
// text and share its profile.
const (
	graphA            = `{"schema":1,"nodes":[{"id":"probe","type":"probe"}],"edges":[]}`
	graphAReformatted = "{\n  \"edges\": [],\n  \"nodes\": [ {\"type\":\"probe\",\"id\":\"probe\"} ],\n  \"schema\": 1\n}"
	graphB            = `{"schema":1,"nodes":[{"id":"probe","type":"probe"},{"id":"scale","type":"scale"}],"edges":[]}`
	graphC            = `{"schema":1,"nodes":[{"id":"probe","type":"probe"},{"id":"subs","type":"subs"}],"edges":[]}`
	// graphACanonical is graphA after canonicalJSON: sorted keys, no spaces.
	graphACanonical = `{"edges":[],"nodes":[{"id":"probe","type":"probe"}],"schema":1}`
	// fallbackGraph stands in for the server's default seed.
	fallbackGraph = `{"schema":1,"nodes":[{"id":"seed","type":"seed"}],"edges":[]}`
)

// buildV110DB creates a database as the v1.10 release leaves it: every
// migration before 0018 applied by hand, then the given library rows inserted.
// It returns the path; the caller opens it with New.
func buildV110DB(t *testing.T, rows []legacyLibrary) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`)
	require.NoError(t, err)
	migrations, err := loadMigrations()
	require.NoError(t, err)
	for _, m := range migrations {
		if m.name >= "0018_profiles.sql" {
			continue
		}
		_, err := db.ExecContext(ctx, m.sql)
		require.NoError(t, err, m.name)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.name, "2026-11-01T00:00:00Z")
		require.NoError(t, err, m.name)
	}

	const insert = `INSERT INTO libraries (id, name, path, target, easy, flow_json, flow_source, rule_tier, watch, schedule, result_policy, created_at, updated_at, file_kinds)
		VALUES (?, ?, ?, '{}', '', ?, ?, ?, 0, '', ?, ?, ?, '["video"]')`
	for i, r := range rows {
		created := "2026-11-01T00:00:0" + string(rune('0'+i)) + "Z"
		_, err := db.ExecContext(ctx, insert, r.id, r.name, "/media/"+r.id, r.flow, r.source, r.tier, r.policy, created, created)
		require.NoError(t, err, r.id)
	}
	return path
}

// legacyLibrary is one row of a v1.10 database.
type legacyLibrary struct {
	id, name, flow, source, tier, policy string
}

// v110Libraries covers the dedupe rules: identical graphs (once reformatted),
// a different graph, the same graph at another tier, the same graph under
// another result policy, a graphless row, and a second library named "shows"
// with a new graph (the name must be suffixed).
var v110Libraries = []legacyLibrary{
	{id: "films", name: "Films", flow: graphA, source: "advanced", tier: RuleTierBasic, policy: "replace"},
	{id: "films-copy", name: "Films copy", flow: graphAReformatted, source: "advanced", tier: RuleTierBasic, policy: "replace"},
	{id: "shows", name: "shows", flow: graphB, source: "steps", tier: RuleTierModerate, policy: "replace"},
	{id: "films-moderate", name: "Films moderate", flow: graphA, source: "steps", tier: RuleTierModerate, policy: "replace"},
	{id: "films-keep", name: "Films keep", flow: graphA, source: "advanced", tier: RuleTierBasic, policy: "keep_both"},
	{id: "bare", name: "Bare", flow: "", source: "", tier: RuleTierBasic, policy: "replace_if_smaller"},
	{id: "shows-two", name: "shows", flow: graphC, source: "advanced", tier: RuleTierAdvanced, policy: "replace"},
}

// openV110 builds the v1.10 database and opens it the way the new release does,
// which applies 0018 and nothing else. The boot backfill is run by the caller.
func openV110(t *testing.T, rows []legacyLibrary) *SQLite {
	t.Helper()
	st, err := New(buildV110DB(t, rows))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestProfilesMigrationBackfillsV110Database is the migration test from task
// 70: the v1.10 shape, the migration, then the backfill. It asserts the profile
// count, the hashes, and every library's profile_id.
func TestProfilesMigrationBackfillsV110Database(t *testing.T) {
	ctx := context.Background()
	st := openV110(t, v110Libraries)

	report, err := st.BackfillProfiles(ctx, fallbackGraph)
	require.NoError(t, err)
	require.Len(t, report, len(v110Libraries))
	seeded := map[string]bool{}
	for _, e := range report {
		require.NoError(t, e.Err, e.LibraryID)
		seeded[e.LibraryID] = e.Seeded
	}
	assert.True(t, seeded["bare"], "a graphless library is seeded")
	assert.False(t, seeded["films"])

	profiles, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	// films, films-copy and films-moderate differ in tier or policy; films-keep
	// differs in policy; shows, bare and shows-two are one each.
	require.Len(t, profiles, 6, "profile count")
	byName := map[string]*Profile{}
	for _, p := range profiles {
		byName[p.Name] = p
	}
	require.NotNil(t, byName["Films"])
	assert.Equal(t, graphACanonical, byName["Films"].Graph)
	assert.Equal(t, sha256Hex(graphACanonical), sha256Hex(byName["Films"].Graph), "hash of the stored graph")
	assert.Equal(t, RuleTierBasic, byName["Films"].Tier)
	assert.Equal(t, "replace", byName["Films"].ResultPolicy)
	assert.Equal(t, "shows (2)", byName["shows (2)"].Name, "a taken name gets a suffix")
	assert.Equal(t, RuleTierAdvanced, byName["shows (2)"].Tier)
	assert.Equal(t, RuleTierModerate, byName["shows"].Tier)
	canon, err := canonicalJSON(fallbackGraph)
	require.NoError(t, err)
	assert.Equal(t, canon, byName["Bare"].Graph, "a graphless library gets the seed graph")

	libs, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, len(v110Libraries))
	byLib := map[string]*Library{}
	for _, l := range libs {
		require.NotEmpty(t, l.ProfileID, "library %s has no profile", l.ID)
		byLib[l.ID] = l
	}
	// Two libraries with identical graphs share one profile.
	assert.Equal(t, byLib["films"].ProfileID, byLib["films-copy"].ProfileID)
	// A different graph gets its own profile.
	assert.NotEqual(t, byLib["films"].ProfileID, byLib["shows"].ProfileID)
	// Same graph, different tier or policy: its own profile.
	assert.NotEqual(t, byLib["films"].ProfileID, byLib["films-moderate"].ProfileID)
	assert.NotEqual(t, byLib["films"].ProfileID, byLib["films-keep"].ProfileID)
	assert.Equal(t, byName["Films"].ID, byLib["films"].ProfileID)
}

// TestProfilesBackfillIsIdempotent: a second run, and a restart of the whole
// store, change nothing. Old columns are not touched either.
func TestProfilesBackfillIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := buildV110DB(t, v110Libraries)
	st, err := New(path)
	require.NoError(t, err)
	_, err = st.BackfillProfiles(ctx, fallbackGraph)
	require.NoError(t, err)

	before, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	beforeProfiles, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	require.NoError(t, st.Close())

	// A restart runs the (already applied) migrations again and the backfill.
	st, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	report, err := st.BackfillProfiles(ctx, fallbackGraph)
	require.NoError(t, err)
	assert.Empty(t, report, "nothing is left to assign")

	after, err := st.ListLibraries(ctx)
	require.NoError(t, err)
	afterProfiles, err := st.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Equal(t, beforeProfiles, afterProfiles)
	require.Len(t, after, len(before))
	for i := range before {
		assert.Equal(t, before[i].ProfileID, after[i].ProfileID, before[i].ID)
		assert.Equal(t, before[i].FlowJSON, after[i].FlowJSON, "flow_json kept")
		assert.Equal(t, before[i].RuleTier, after[i].RuleTier, "rule_tier kept")
		assert.Equal(t, before[i].ResultPolicy, after[i].ResultPolicy, "result_policy kept")
	}
}

// TestProfilesBackfillSkipsBrokenGraph: a library whose stored graph is not
// JSON is reported with an error and left unassigned; the others still get
// their profile, so one bad row never blocks the boot.
func TestProfilesBackfillSkipsBrokenGraph(t *testing.T) {
	ctx := context.Background()
	st := openV110(t, []legacyLibrary{
		{id: "good", name: "Good", flow: graphA, source: "advanced", tier: RuleTierBasic, policy: "replace"},
		{id: "broken", name: "Broken", flow: `{not json`, source: "advanced", tier: RuleTierBasic, policy: "replace"},
	})
	report, err := st.BackfillProfiles(ctx, fallbackGraph)
	require.NoError(t, err)
	require.Len(t, report, 2)

	good, err := st.GetLibrary(ctx, "good")
	require.NoError(t, err)
	assert.NotEmpty(t, good.ProfileID)
	broken, err := st.GetLibrary(ctx, "broken")
	require.NoError(t, err)
	assert.Empty(t, broken.ProfileID)
	for _, e := range report {
		if e.LibraryID == "broken" {
			assert.Error(t, e.Err)
		} else {
			assert.NoError(t, e.Err)
		}
	}
}

// TestProfileIDSurvivesLibraryWrites: UpdateLibrary from a fresh read keeps the
// profile, and SetLibraryProfile changes only the reference.
func TestProfileIDSurvivesLibraryWrites(t *testing.T) {
	ctx := context.Background()
	st := openV110(t, v110Libraries[:1])
	_, err := st.BackfillProfiles(ctx, fallbackGraph)
	require.NoError(t, err)

	lib, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	lib.Name = "Renamed"
	require.NoError(t, st.UpdateLibrary(ctx, lib))
	again, err := st.GetLibrary(ctx, "films")
	require.NoError(t, err)
	assert.Equal(t, lib.ProfileID, again.ProfileID)
	assert.Equal(t, "Renamed", again.Name)
}

// TestProfileCRUD covers the profile methods the backfill does not reach.
func TestProfileCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	p := &Profile{Name: "Standard", Description: "d", Tier: RuleTierModerate, Graph: graphACanonical, ResultPolicy: "replace_if_smaller"}
	require.NoError(t, st.CreateProfile(ctx, p))
	require.NotEmpty(t, p.ID)

	got, err := st.GetProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Standard", got.Name)
	assert.Equal(t, RuleTierModerate, got.Tier)
	assert.Equal(t, graphACanonical, got.Graph)

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

// TestCanonicalJSONIgnoresFormatting: the same graph in two layouts canonicalises
// to one string, and malformed or trailing input is rejected.
func TestCanonicalJSONIgnoresFormatting(t *testing.T) {
	a, err := canonicalJSON(graphA)
	require.NoError(t, err)
	b, err := canonicalJSON(graphAReformatted)
	require.NoError(t, err)
	assert.Equal(t, graphACanonical, a)
	assert.Equal(t, a, b)

	_, err = canonicalJSON(`{"a":1} {"b":2}`)
	require.Error(t, err)
	_, err = canonicalJSON(`{not json`)
	require.Error(t, err)
}
