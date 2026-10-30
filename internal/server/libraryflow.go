// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Library flow helpers (task 14). Every place that applies a library's
// rules — the scanner, re-evaluation, the job spec builder, the transfer
// verify-and-replace — goes through these, so the flow graph is the single
// source of truth: the stored Target is compat only, and the flow wins
// when the two disagree (ARCHITECTURE.md).

// LibraryFlow returns the library's rule graph and its source. A stored
// flow is returned as-is; a legacy row without one gets the flow compiled
// from its Target, with the source inferred from the stored easy input
// (easy input present → easy, a hand-set target → advanced). Compile errors
// of a stored flow surface; a legacy target that cannot compile is an error.
func LibraryFlow(lib *store.Library) (flow.Graph, string, error) {
	if s := trimJSON(lib.FlowJSON); s != "" {
		g, err := flow.ParseGraph([]byte(s))
		if err != nil {
			return flow.Graph{}, "", fmt.Errorf("library flow: %w", err)
		}
		source := lib.FlowSource
		if source == "" {
			source = flow.FlowSourceAdvanced
		}
		return g, source, nil
	}
	target, err := flow.ParseTarget([]byte(lib.TargetJSON))
	if err != nil {
		return flow.Graph{}, "", fmt.Errorf("library flow: %w", err)
	}
	g, err := flow.TargetToFlow(target)
	if err != nil {
		return flow.Graph{}, "", fmt.Errorf("library flow: %w", err)
	}
	source := flow.FlowSourceAdvanced
	if trimJSON(lib.EasyJSON) != "" {
		source = flow.FlowSourceEasy
	}
	return g, source, nil
}

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

// EffectiveTarget resolves the library's flow for one probed file and
// returns the Target the satisfied rule and the command builder must use:
// a guard that skips the encode leaves the video at copy (satisfying the
// video clause), a running transcode becomes the effective video, and the
// container, audio and subtitle steps define the rest.
func EffectiveTarget(lib *store.Library, info probe.MediaInfo) (flow.Target, error) {
	plan, _, err := LibraryPlan(lib)
	if err != nil {
		return flow.Target{}, err
	}
	eff, err := flow.ResolvePlan(plan, info)
	if err != nil {
		return flow.Target{}, err
	}
	return eff.Target, nil
}

// EffectivePolicy returns the result policy and rename template the
// library's flow assigns to its jobs: a skip_if_smaller step wins over the
// library's result_policy column with replace_if_smaller, and a rename
// step carries its template (empty when none).
func EffectivePolicy(lib *store.Library) (policy, rename string, err error) {
	policy = lib.ResultPolicy
	if policy == "" {
		policy = store.ResultPolicyReplaceIfSmaller
	}
	plan, _, err := LibraryPlan(lib)
	if err != nil {
		return policy, "", fmt.Errorf("library flow: %w", err)
	}
	if plan.HasSkipIfSmaller() {
		policy = store.ResultPolicyReplaceIfSmaller
	}
	return policy, plan.RenameTemplate(), nil
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

// logFlowTargetMismatch warns when a stored flow disagrees with the stored
// Target: the flow wins, and the warning makes the disagreement visible
// instead of silently ignoring the Target column.
func logFlowTargetMismatch(log *slog.Logger, lib *store.Library, plan flow.Plan) {
	if trimJSON(lib.FlowJSON) == "" {
		return // generated flow; no disagreement possible
	}
	target, err := flow.ParseTarget([]byte(lib.TargetJSON))
	if err != nil {
		return // unparseable targets are reported elsewhere
	}
	stored, err := json.Marshal(target)
	if err != nil {
		return
	}
	compiled, err := json.Marshal(plan.Target())
	if err != nil {
		return
	}
	if string(stored) != string(compiled) {
		log.Warn("library flow and target disagree; the flow wins",
			"library", lib.ID,
			"target", string(stored),
			"flow_target", string(compiled))
	}
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
	msgs := make([]string, 0, len(e.Errors))
	for _, fe := range e.Errors {
		msgs = append(msgs, fe.Error())
	}
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

// IsInvalidFlow reports whether err is a flow validation failure (the API
// maps it to 400 invalid_flow with the field errors).
func IsInvalidFlow(err error) bool {
	var e *ErrInvalidFlow
	return errors.As(err, &e)
}

// backfillFlows compiles and persists the flow of every library that
// predates the flow columns, so the flow API and the step stack work with
// no manual migration (TestLegacyTargetStillRuns). Failures are logged per
// library; the lazy compile in LibraryFlow keeps them working either way.
func (m *LibraryManager) backfillFlows(ctx context.Context) {
	libs, err := m.st.ListLibraries(ctx)
	if err != nil {
		m.log.Error("library: flow backfill", "err", err)
		return
	}
	for _, lib := range libs {
		if trimJSON(lib.FlowJSON) != "" {
			continue
		}
		g, source, err := LibraryFlow(lib)
		if err != nil {
			m.log.Error("library: flow backfill", "library", lib.ID, "err", err)
			continue
		}
		raw, err := json.Marshal(g)
		if err != nil {
			m.log.Error("library: flow backfill", "library", lib.ID, "err", err)
			continue
		}
		updated := *lib
		updated.FlowJSON = string(raw)
		updated.FlowSource = source
		if err := m.st.UpdateLibrary(ctx, &updated); err != nil {
			m.log.Error("library: flow backfill", "library", lib.ID, "err", err)
			continue
		}
		m.log.Debug("library flow backfilled", "library", lib.ID, "source", source)
	}
}

// UpdateFlow validates and stores a flow graph from the API (PUT
// /libraries/{id}/flow): the graph becomes the library's rule, the source
// records the tier that produced it, and every file is re-evaluated against
// the new rule. The stored Target is re-synced from the compiled flow so
// the compat column never disagrees with the rule a flow author just saved.
func (m *LibraryManager) UpdateFlow(ctx context.Context, id, source string, g flow.Graph) (*store.Library, error) {
	if err := validateFlowSource(source); err != nil {
		return nil, err
	}
	if errs := flow.Validate(g); len(errs) > 0 {
		return nil, &ErrInvalidFlow{Errors: errs}
	}
	lib, err := m.st.GetLibrary(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("library: marshal flow: %w", err)
	}
	lib.FlowJSON = string(raw)
	lib.FlowSource = source
	if plan, _, perr := LibraryPlan(lib); perr == nil {
		if rawTarget, merr := json.Marshal(plan.Target()); merr == nil {
			lib.TargetJSON = string(rawTarget)
		}
	}
	if err := m.st.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("library: update flow: %w", err)
	}
	m.log.Info("library flow updated", "library", lib.ID, "source", source)
	m.publish(EventLibraryUpdated, NewLibraryDTO(lib))

	m.reevaluate(ctx, lib)
	return lib, nil
}

func validateFlowSource(source string) error {
	for _, ok := range flow.FlowSources() {
		if source == ok {
			return nil
		}
	}
	return fmt.Errorf("invalid flow source %q: want one of %v", source, flow.FlowSources())
}
