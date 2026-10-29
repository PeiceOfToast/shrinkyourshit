// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Node lifecycle (task 11). desired_state is the operator intent
// (stopped | running | draining); enabled is the hard off switch that
// gates node auth. A node never encodes unless a human started it: fresh
// installs and newly paired nodes default to stopped, and the built-in
// worker learns its instruction from the heartbeat it already sends.

// EffectiveState is the state the UI shows: error while a transition
// failed to apply, otherwise the desired state itself.
func EffectiveState(n *store.Node) string {
	if strings.TrimSpace(n.StateError) != "" {
		return "error"
	}
	switch n.DesiredState {
	case store.NodeStateRunning, store.NodeStateDraining:
		return n.DesiredState
	default:
		return store.NodeStateStopped
	}
}

// setState persists a desired_state transition and clears any recorded
// error. A failed write is recorded on the node as state_error (best
// effort — the write itself just failed), so the UI sees why the node is
// not in the state the admin asked for.
func (r *NodeRegistry) setState(ctx context.Context, node *store.Node, desired string) error {
	node.DesiredState = desired
	node.StateError = ""
	if err := r.st.UpdateNode(ctx, node); err != nil {
		r.recordStateError(ctx, node, err)
		return fmt.Errorf("node: set desired_state %s: %w", desired, err)
	}
	r.publish(EventNodeUpdated, NewNodeDTO(node))
	return nil
}

// recordStateError stores a transition failure on the node for the UI.
// The persisting write is itself best effort; if it fails too the original
// error stands (already returned to the caller).
func (r *NodeRegistry) recordStateError(ctx context.Context, node *store.Node, cause error) {
	node.StateError = cause.Error()
	if err := r.st.UpdateNode(ctx, node); err != nil {
		r.log.Error("node: record state_error", "node", node.ID, "err", err)
		return
	}
	r.publish(EventNodeUpdated, NewNodeDTO(node))
}

// Start sets desired_state to running (idempotent). Claims resume on the
// node's next heartbeat — the process never needs a restart.
func (r *NodeRegistry) Start(ctx context.Context, id string) (*store.Node, error) {
	node, err := r.st.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.DesiredState == store.NodeStateRunning && strings.TrimSpace(node.StateError) == "" {
		return node, nil
	}
	if err := r.setState(ctx, node, store.NodeStateRunning); err != nil {
		return nil, err
	}
	r.log.Info("node started", "node", node.ID, "name", node.Name)
	return node, nil
}

// Stop sets the node's desired state: drain keeps the in-flight jobs
// (the worker finishes them and reports stopped once idle), otherwise the
// node stops claiming immediately and its in-flight job is abandoned to
// the heartbeat timeout. Idempotent per target state.
func (r *NodeRegistry) Stop(ctx context.Context, id string, drain bool) (*store.Node, error) {
	node, err := r.st.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	desired := store.NodeStateStopped
	if drain {
		desired = store.NodeStateDraining
	}
	if node.DesiredState == desired && strings.TrimSpace(node.StateError) == "" {
		return node, nil
	}
	if err := r.setState(ctx, node, desired); err != nil {
		return nil, err
	}
	r.log.Info("node stopped", "node", node.ID, "name", node.Name, "drain", drain)
	return node, nil
}

// pauseSnapshot is the pause-all memory: node id → prior desired_state.
type pauseSnapshot map[string]string

// readPauseSnapshot loads the stored pause-all snapshot, if any.
func (r *NodeRegistry) readPauseSnapshot(ctx context.Context) (pauseSnapshot, bool, error) {
	raw, ok, err := r.st.GetSetting(ctx, store.SettingPausedNodes)
	if err != nil || !ok {
		return nil, false, err
	}
	var snap pauseSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, true, fmt.Errorf("node: unreadable pause snapshot: %w", err)
	}
	return snap, true, nil
}

// PauseAll stops every node now and remembers the prior intent once: a
// repeat pause-all keeps the first snapshot, so resume-all always restores
// what the operator actually had running. A node whose write fails keeps
// its error recorded (state_error) and the batch continues. Returns the
// number of nodes now stopped.
func (r *NodeRegistry) PauseAll(ctx context.Context) (int, error) {
	nodes, err := r.st.ListNodes(ctx)
	if err != nil {
		return 0, err
	}
	if _, exists, err := r.readPauseSnapshot(ctx); err != nil {
		return 0, err
	} else if !exists {
		snap := pauseSnapshot{}
		for _, n := range nodes {
			state := n.DesiredState
			if state == "" {
				state = store.NodeStateStopped
			}
			snap[n.ID] = state
		}
		raw, err := json.Marshal(snap)
		if err != nil {
			return 0, err
		}
		if err := r.st.SetSetting(ctx, store.SettingPausedNodes, string(raw)); err != nil {
			return 0, fmt.Errorf("node: store pause snapshot: %w", err)
		}
	}

	stopped := 0
	for _, n := range nodes {
		if n.DesiredState == store.NodeStateStopped {
			continue
		}
		if err := r.setState(ctx, n, store.NodeStateStopped); err != nil {
			r.log.Error("node: pause-all", "node", n.ID, "err", err)
			continue
		}
		stopped++
	}
	r.log.Info("paused all nodes", "stopped", stopped)
	return stopped, nil
}

// ResumeAll restores the per-node intent recorded by PauseAll and drops
// the snapshot. With no snapshot it is a no-op (0 resumed).
func (r *NodeRegistry) ResumeAll(ctx context.Context) (int, error) {
	snap, exists, err := r.readPauseSnapshot(ctx)
	if err != nil || !exists {
		return 0, err
	}
	resumed := 0
	for id, desired := range snap {
		node, err := r.st.GetNode(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue // deleted while paused: nothing to restore
			}
			return resumed, err
		}
		if node.DesiredState == desired && strings.TrimSpace(node.StateError) == "" {
			continue
		}
		if err := r.setState(ctx, node, desired); err != nil {
			r.log.Error("node: resume-all", "node", id, "err", err)
			continue
		}
		resumed++
	}
	if err := r.st.DeleteSetting(ctx, store.SettingPausedNodes); err != nil {
		return resumed, fmt.Errorf("node: clear pause snapshot: %w", err)
	}
	r.log.Info("resumed nodes", "resumed", resumed)
	return resumed, nil
}

// CurrentJobIDs maps every node id to the id of its oldest claimed/running
// job — what the Nodes card shows as the current job. Nodes with no
// in-flight work are absent.
func (r *NodeRegistry) CurrentJobIDs(ctx context.Context) map[string]string {
	oldest := map[string]time.Time{}
	out := map[string]string{}
	for _, status := range []string{store.JobStatusClaimed, store.JobStatusRunning} {
		jobs, err := r.st.ListJobs(ctx, store.JobFilter{Status: status})
		if err != nil {
			r.log.Error("node: current job lookup", "err", err)
			continue
		}
		for _, j := range jobs {
			if j.NodeID == "" {
				continue
			}
			if cur, ok := oldest[j.NodeID]; ok && !j.CreatedAt.Before(cur) {
				continue
			}
			oldest[j.NodeID] = j.CreatedAt
			out[j.NodeID] = j.ID
		}
	}
	return out
}
