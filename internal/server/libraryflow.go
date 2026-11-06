// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Library flow helpers (task 14). Every place that applies a library's
// rules — the scanner, re-evaluation, the job spec builder, the transfer
// verify-and-replace — goes through these, so the flow graph is the single
// source of truth: the stored Target is compat only, and the flow wins
// when the two disagree (ARCHITECTURE.md).

// ErrNoProfile is returned when a library has no encode profile attached.
// Since task 74 the profile is the only rule a library runs: a library
// without one runs nothing and the API says so.
var ErrNoProfile = errors.New("the library has no encode profile; attach one")

// LibraryFlow returns the library's rule graph and its source: the graph of
// the encode profile it follows (spec 5.1, 5.4). Source is derived from the
// profile's tier. The legacy flow_json, target and easy columns are not read.
func LibraryFlow(lib *store.Library) (flow.Graph, string, error) {
	if s := trimJSON(lib.ProfileGraph); s != "" {
		g, err := flow.ParseGraph([]byte(s))
		if err != nil {
			return flow.Graph{}, "", fmt.Errorf("library profile flow: %w", err)
		}
		return g, sourceForTier(lib.ProfileTier), nil
	}
	return flow.Graph{}, "", ErrNoProfile
}

// sourceForTier names the flow source a profile's editor tier implies: the
// easy screen, the step stack or the advanced canvas.
func sourceForTier(tier string) string {
	switch tier {
	case store.RuleTierBasic:
		return flow.FlowSourceEasy
	case store.RuleTierModerate:
		return flow.FlowSourceSteps
	default:
		return flow.FlowSourceAdvanced
	}
}

// LibraryTier returns the rule tier the library's profile is edited at (task
// 59 moved to the profile in task 74). Empty when no profile is attached.
func LibraryTier(lib *store.Library) string { return lib.ProfileTier }

// LibraryPlan compiles the library's flow into the executable plan. It is
// the entry point for the scanner, re-evaluation and the job spec builder.
func LibraryPlan(lib *store.Library) (flow.Plan, string, error) {
	g, source, err := LibraryFlow(lib)
	if err != nil {
		return flow.Plan{}, "", err
	}
	plan, err := flow.Compile(g)
	if err != nil {
		return flow.Plan{}, "", fmt.Errorf("library flow: %w", err)
	}
	return plan, source, nil
}

// EffectiveResultPolicy is the result policy the library's profile sets
// (task 74). replace_if_smaller when no profile is attached, which only a
// library that failed the boot backfill can be in.
func EffectiveResultPolicy(lib *store.Library) string {
	if lib.ProfileResultPolicy == "" {
		return store.ResultPolicyReplaceIfSmaller
	}
	return lib.ProfileResultPolicy
}

// EffectivePolicy returns the result policy and rename template the
// library's flow assigns to its jobs: a skip_if_smaller step wins over the
// profile's result policy with replace_if_smaller, and a rename step carries
// its template (empty when none).
func EffectivePolicy(lib *store.Library) (policy, rename string, err error) {
	policy = EffectiveResultPolicy(lib)
	plan, _, err := LibraryPlan(lib)
	if err != nil {
		return policy, "", fmt.Errorf("library flow: %w", err)
	}
	if plan.HasSkipIfSmaller() {
		policy = store.ResultPolicyReplaceIfSmaller
	}
	return policy, plan.RenameTemplate(), nil
}

// EffectiveMove returns the library flow's move directive (task 24): the
// trimmed destination in library-side terms and whether the folder layout
// is kept. Empty destination when the flow has no move step.
func EffectiveMove(lib *store.Library) (destination string, keepTree bool, err error) {
	plan, _, err := LibraryPlan(lib)
	if err != nil {
		return "", false, fmt.Errorf("library flow: %w", err)
	}
	return plan.MoveTo, plan.MoveKeepTree, nil
}

// EffectiveContainer returns the output container of the library's flow —
// the extension transfer-mode uploads are stored under.
func EffectiveContainer(lib *store.Library) (string, error) {
	plan, _, err := LibraryPlan(lib)
	if err != nil {
		return "", fmt.Errorf("library flow: %w", err)
	}
	container := plan.Target().Container
	if container == "" {
		return "", fmt.Errorf("library flow: no container step")
	}
	return container, nil
}

func trimJSON(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	return s
}

// ErrInvalidFlow marks a flow the API maps to 400 invalid_flow with its
// field errors (PUT /libraries/{id}/flow).
type ErrInvalidFlow struct {
	Errors []flow.FieldError
}

func (e *ErrInvalidFlow) Error() string {
	return "invalid flow: " + joinFlowErrors(e.Errors)
}

func joinFlowErrors(errs []flow.FieldError) string {
	out := ""
	for i, fe := range errs {
		if i > 0 {
			out += "; "
		}
		out += fe.Error()
	}
	return out
}

// ErrFollowsProfile is returned when a flow is saved to a library that follows
// an encode profile (task 73). The API answers 409 follows_profile: the flow
// belongs to the profile now, so the edit goes to the profile.
var ErrFollowsProfile = errors.New("the library follows an encode profile; edit the profile instead")

// IsInvalidFlow reports whether err is a flow validation failure (the API
// maps it to 400 invalid_flow with the field errors).
func IsInvalidFlow(err error) bool {
	var e *ErrInvalidFlow
	return errors.As(err, &e)
}

// IsPluginStepError reports whether err is a plugin-step availability
// failure (task 32: the API maps it to 422 plugin_unavailable).
func IsPluginStepError(err error) bool {
	var e *ErrPluginStep
	return errors.As(err, &e)
}

// backfillProfiles gives every library with no encode profile the default
// profile (encode profiles wave, spec 5.2). It runs once per boot and reads no
// legacy rule column: since v1.11 every library is assigned, so a row is
// unassigned only after a failed boot. The default profile is created first when
// the table is empty (defaultProfile). One log line names each library it gave a
// profile. The step is idempotent: a second boot finds every library assigned
// and logs nothing.
func (m *LibraryManager) backfillProfiles(ctx context.Context) {
	libs, err := m.st.ListLibraries(ctx)
	if err != nil {
		m.log.Error("library: profile backfill", "err", err)
		return
	}
	unassigned := false
	for _, lib := range libs {
		if lib.ProfileID == "" {
			unassigned = true
			break
		}
	}
	if !unassigned {
		return
	}
	def, err := m.defaultProfile(ctx)
	if err != nil {
		m.log.Error("library: profile backfill: default profile", "err", err)
		return
	}
	report, err := m.st.BackfillProfiles(ctx, def.ID)
	if err != nil {
		m.log.Error("library: profile backfill", "err", err)
		return
	}
	for _, e := range report {
		m.log.Info("library profile assigned", "library", e.LibraryName, "profile", e.ProfileName)
	}
}

// defaultProfile returns the profile a library follows when its save names
// none (task 74): the first profile in list order, the one the library form
// pre-selects. A fresh install has none, so the default seed is created first.
func (m *LibraryManager) defaultProfile(ctx context.Context) (*store.Profile, error) {
	list, err := m.st.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) > 0 {
		return list[0], nil
	}
	return m.seedDefaultProfile(ctx)
}

// seedDefaultProfile creates the default encode profile of a fresh install
// (task 71, spec 5.2), named for the default rule tier and carrying the
// default Target's graph. Boot calls defaultProfile, which reaches here only
// when no profile exists.
func (m *LibraryManager) seedDefaultProfile(ctx context.Context) (*store.Profile, error) {
	tier, err := m.defaultRuleTier(ctx)
	if err != nil {
		return nil, err
	}
	graph, err := defaultProfileGraph()
	if err != nil {
		return nil, fmt.Errorf("default graph: %w", err)
	}
	p := &store.Profile{
		Name:         fmt.Sprintf("Default (%s)", tier),
		Tier:         tier,
		Graph:        graph,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	}
	if err := m.st.CreateProfile(ctx, p); err != nil {
		return nil, err
	}
	m.log.Info("default encode profile created", "profile", p.Name)
	return p, nil
}

// defaultProfileGraph is the canonical graph of the default Target, the seed
// for a library that has no stored graph.
func defaultProfileGraph() (string, error) {
	g, err := flow.TargetToFlow(flow.Default())
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// UpdateFlow answers PUT /libraries/{id}/flow. The graph is validated first,
// so a bad graph is still 400 invalid_flow, and a valid one is refused with
// ErrFollowsProfile (409): the rule of a library is its profile's (task 74).
func (m *LibraryManager) UpdateFlow(ctx context.Context, id, source string, g flow.Graph) (*store.Library, error) {
	if err := validateFlowSource(source); err != nil {
		return nil, err
	}
	if errs := flow.Validate(g); len(errs) > 0 {
		return nil, &ErrInvalidFlow{Errors: errs}
	}
	// The raw arguments switch (task 58): refused, not dropped, while off.
	if m.settings != nil {
		refused, err := m.settings.RefusedRawArgs(ctx, g)
		if err != nil {
			return nil, err
		}
		if len(refused) > 0 {
			return nil, &ErrInvalidFlow{Errors: refused}
		}
	}
	if _, err := m.st.GetLibrary(ctx, id); err != nil {
		return nil, err
	}
	// Every library follows an encode profile (task 74), and a flow saved to
	// the library would be stored and never run. The edit goes to the profile.
	return nil, ErrFollowsProfile
}

func validateFlowSource(source string) error {
	for _, ok := range flow.FlowSources() {
		if source == ok {
			return nil
		}
	}
	return fmt.Errorf("invalid flow source %q: want one of %v", source, flow.FlowSources())
}

// validateMoveDestinations checks every move step's destination against the
// library it will run on (task 24): it must not be the library root or
// inside it. All sources live under the root; a file moved inside would be
// picked up again by the next scan and re-encoded forever. Field errors
// name the step so the UI can attach them to the destination row.
func validateMoveDestinations(lib *store.Library, g flow.Graph) []flow.FieldError {
	var errs []flow.FieldError
	root := filepath.Clean(lib.Path)
	for i, n := range g.Nodes {
		if n.Type != flow.StepMove {
			continue
		}
		dest := flow.MoveDestination(n)
		if dest == "" || sameOrInside(root, dest) {
			errs = append(errs, flow.FieldError{
				Path:    fmt.Sprintf("nodes[%d].params.destination", i),
				Message: fmt.Sprintf("the destination %q must be a folder outside the library %q; files inside it would be picked up again by the next scan", dest, lib.Path),
			})
		}
	}
	return errs
}
